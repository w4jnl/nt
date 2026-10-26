// Package importer turns an Obsidian vault into an nt repository, once (docs/design.md, phase 1):
// wikilinks and embeds become markdown links, names become dated slugs with the original title
// kept in frontmatter, attachments move to assets folders, and Obsidian's own files stay
// behind. It reads the vault and never changes it.
package importer

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/vault"
)

// Report says what an import did, or would do with DryRun.
type Report struct {
	Source     string    `json:"source"`
	Notes      []Moved   `json:"notes"`
	Journals   []Moved   `json:"journals"`
	Assets     []Moved   `json:"assets"`
	Skipped    []Skipped `json:"skipped"`
	Unresolved []Link    `json:"unresolved"` // links left as written: their target was not found
	Converted  int       `json:"converted"`  // wikilinks and embeds turned into markdown links
}

// Moved is a file and where it went.
type Moved struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Title string `json:"title,omitempty"`
}

// Skipped is a file left behind, and why.
type Skipped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Link is a link the import could not resolve.
type Link struct {
	Note string `json:"note"`
	Text string `json:"text"`
}

// Options control an import.
type Options struct {
	DryRun bool
}

type file struct {
	src, dst string // vault-relative and repository-relative, slash-separated
	stem     string // the original file name without .md
	title    string
	journal  bool
	asset    bool
	day      time.Time
}

// Obsidian imports the vault at src into v.
func Obsidian(src string, v *vault.Vault, o Options) (Report, error) {
	src, err := filepath.Abs(src)
	if err != nil {
		return Report{}, err
	}
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return Report{}, fmt.Errorf("%s is not a folder", src)
	}
	rep := Report{Source: src}
	var files []*file
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(mustRel(src, p))
		if rel == "." {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				rep.Skipped = append(rep.Skipped, Skipped{rel + "/", "hidden folder (Obsidian settings, trash or git)"})
				return filepath.SkipDir
			}
			rep.Skipped = append(rep.Skipped, Skipped{rel, "hidden file"})
			return nil
		}
		if d.IsDir() {
			if rel == "templates" {
				rep.Skipped = append(rep.Skipped, Skipped{rel + "/", "Obsidian templates, not notes"})
				return filepath.SkipDir
			}
			return nil
		}
		switch ext := strings.ToLower(path.Ext(name)); {
		case ext == ".base" || ext == ".canvas":
			rep.Skipped = append(rep.Skipped, Skipped{rel, "Obsidian " + strings.TrimPrefix(ext, ".") + " file"})
		case rel == "go.mod" || rel == "go.sum":
			rep.Skipped = append(rep.Skipped, Skipped{rel, "not a note"})
		case ext == ".md":
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if _, body := render.SplitFrontmatter(b); strings.TrimSpace(string(body)) == "" {
				rep.Skipped = append(rep.Skipped, Skipped{rel, "empty: no text besides frontmatter"})
				return nil
			}
			files = append(files, &file{src: rel})
		default:
			files = append(files, &file{src: rel, asset: true})
		}
		return nil
	})
	if err != nil {
		return rep, err
	}

	im := &importer{src: src, v: v, byPath: map[string]*file{}, byBase: map[string][]*file{}}
	for _, f := range files {
		im.place(f)
	}
	taken := map[string]bool{}
	sort.Slice(files, func(i, j int) bool { return files[i].src < files[j].src })
	for _, f := range files {
		if f.journal && (taken[strings.ToLower(f.dst)] || v.Unique(f.dst) != f.dst) {
			// the day's journal is taken: keep this one as a note beside its folder's notes
			folder := slugPath(strings.TrimSuffix(path.Dir(f.src), "."))
			f.journal, f.dst = false, firstNonEmpty(folder, v.Cfg.DefaultFolder)+"/"+f.day.Format("2006-01-02")+"-daily.md"
		}
		f.dst = uniqueIn(f.dst, taken, v)
		taken[strings.ToLower(f.dst)] = true
		im.byPath[strings.ToLower(f.src)] = f
		im.byPath[strings.ToLower(strings.TrimSuffix(f.src, ".md"))] = f
		base := strings.ToLower(path.Base(f.src))
		im.byBase[base] = append(im.byBase[base], f)
		if !f.asset {
			im.byBase[strings.TrimSuffix(base, ".md")] = append(im.byBase[strings.TrimSuffix(base, ".md")], f)
		}
	}

	var unlock func()
	if !o.DryRun {
		if unlock, err = v.Lock(); err != nil {
			return rep, err
		}
		defer unlock()
	}
	for _, f := range files {
		m := Moved{From: f.src, To: f.dst, Title: f.title}
		switch {
		case f.asset:
			rep.Assets = append(rep.Assets, m)
			if !o.DryRun {
				b, err := os.ReadFile(filepath.Join(src, f.src))
				if err != nil {
					return rep, err
				}
				if err := vault.WriteFile(v.Abs(f.dst), b); err != nil {
					return rep, err
				}
			}
			continue
		case f.journal:
			rep.Journals = append(rep.Journals, m)
		default:
			rep.Notes = append(rep.Notes, m)
		}
		b, err := os.ReadFile(filepath.Join(src, f.src))
		if err != nil {
			return rep, err
		}
		out, links, unresolved := im.convert(f, b)
		rep.Converted += links
		for _, u := range unresolved {
			rep.Unresolved = append(rep.Unresolved, Link{Note: f.dst, Text: u})
		}
		if !o.DryRun {
			if err := vault.WriteFile(v.Abs(f.dst), out); err != nil {
				return rep, err
			}
		}
	}
	return rep, nil
}

type importer struct {
	src    string
	v      *vault.Vault
	byPath map[string]*file   // lowercased vault path, with and without .md
	byBase map[string][]*file // lowercased base name, with and without .md
}

var (
	dateOnly  = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})$`)
	datedName = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})[ _-]+(.+)$`)
	compact   = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})(\d{4,6})?[ _-]+(.+)$`)
)

// place decides where a file goes: notes keep their folders (as slugs) and get dated slug names,
// date-only notes become journals, img/ becomes assets/, other attachments move into an assets
// folder beside their notes.
func (im *importer) place(f *file) {
	dir, base := path.Split(f.src)
	dir = strings.TrimSuffix(dir, "/")
	folder := slugPath(dir)
	if f.asset {
		ext := strings.ToLower(path.Ext(base))
		stem := vault.Slug(splitCamel(strings.TrimSuffix(base, path.Ext(base))))
		if stem == "" {
			stem = "image"
		}
		switch {
		case dir == "img" || dir == "attachments" || dir == "assets":
			f.dst = "assets/" + stem + ext
		case folder == "":
			f.dst = "assets/" + stem + ext
		default:
			f.dst = folder + "/assets/" + stem + ext
		}
		return
	}
	stem := strings.TrimSuffix(base, path.Ext(base))
	f.stem = stem
	front, _ := render.SplitFrontmatter(mustRead(filepath.Join(im.src, f.src)))
	f.title = frontValue(front, "title")
	if m := dateOnly.FindStringSubmatch(stem); m != nil {
		day, _ := time.ParseInLocation("2006-01-02", m[1], time.Local)
		f.journal, f.day = true, day
		f.dst = im.v.JournalRel(day)
		return
	}
	var day time.Time
	name := stem
	if m := datedName.FindStringSubmatch(stem); m != nil {
		day, _ = time.ParseInLocation("2006-01-02", m[1], time.Local)
		name = m[2]
	} else if m := compact.FindStringSubmatch(stem); m != nil {
		day, _ = time.ParseInLocation("20060102", m[1]+m[2]+m[3], time.Local)
		name = m[5]
	}
	if day.IsZero() {
		day = im.created(f.src, front)
	}
	if f.title == "" || f.title == stem {
		f.title = strings.TrimSpace(name)
		if dateOnly.MatchString(stem) || isClock(name) {
			f.title = stem
		}
	}
	slug := vault.Slug(splitCamel(name))
	if slug == "" {
		slug = "note"
	}
	if folder == "" {
		folder = im.v.Cfg.DefaultFolder
	}
	f.day = day
	f.dst = folder + "/" + day.Format("2006-01-02") + "-" + slug + ".md"
}

// created is the day an undated note was written: its frontmatter date, else the day git first
// saw it in the vault, else its modification time.
func (im *importer) created(rel string, front []byte) time.Time {
	if d := frontValue(front, "date"); d != "" {
		if t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(d)[:min(10, len(strings.TrimSpace(d)))], time.Local); err == nil {
			return t
		}
	}
	if out, err := exec.Command("git", "-C", im.src, "log", "--diff-filter=A", "--follow", "--format=%as", "--", rel).Output(); err == nil {
		lines := strings.Fields(string(out))
		if len(lines) > 0 {
			if t, err := time.ParseInLocation("2006-01-02", lines[len(lines)-1], time.Local); err == nil {
				return t
			}
		}
	}
	if st, err := os.Stat(filepath.Join(im.src, rel)); err == nil {
		t := st.ModTime()
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	}
	return time.Now()
}

var (
	wikiRe   = regexp.MustCompile(`(!?)\[\[([^\[\]|#]*)(#[^\[\]|]*)?(\|[^\[\]]*)?\]\]`)
	mdLinkRe = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)\s]+)(\s+"[^"]*")?\)`)
)

// convert rewrites one note: frontmatter completed, wikilinks and embeds turned into markdown
// links, local markdown links pointed at the new places. Code is left alone.
func (im *importer) convert(f *file, note []byte) (out []byte, converted int, unresolved []string) {
	front, body := render.SplitFrontmatter(note)
	lines := strings.Split(string(body), "\n")
	fence := ""
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = t[:3]
			continue
		}
		lines[i] = outsideCode(l, func(s string) string {
			// markdown links first, so the links made from wikilinks are not resolved twice
			s = mdLinkRe.ReplaceAllStringFunc(s, func(m string) string {
				r, ok := im.mdlink(f, mdLinkRe.FindStringSubmatch(m))
				if !ok {
					unresolved = append(unresolved, m)
					return m
				}
				return r
			})
			return wikiRe.ReplaceAllStringFunc(s, func(m string) string {
				r, ok := im.wikilink(f, wikiRe.FindStringSubmatch(m))
				if !ok {
					unresolved = append(unresolved, m)
					return m
				}
				converted++
				return r
			})
		})
	}
	return append(im.frontmatter(f, front), []byte(strings.Join(lines, "\n"))...), converted, unresolved
}

// wikilink turns [[target#heading|label]] or ![[file|size]] into a markdown link or image.
func (im *importer) wikilink(f *file, m []string) (string, bool) {
	embed, target, heading, label := m[1] == "!", strings.TrimSpace(m[2]), strings.TrimPrefix(m[3], "#"), strings.TrimPrefix(m[4], "|")
	if target == "" && heading != "" { // [[#heading]]: a heading in this note
		return "[" + firstNonEmpty(label, heading) + "](#" + anchor(heading) + ")", true
	}
	to := im.resolve(f, target)
	if to == nil {
		return "", false
	}
	link := vault.RelLink(f.dst, to.dst)
	if embed && to.asset {
		alt := strings.TrimSuffix(path.Base(to.src), path.Ext(to.src))
		if label != "" && !isSize(label) {
			alt = label
		}
		return "![" + alt + "](" + link + ")", true
	}
	if heading != "" {
		link += "#" + anchor(heading)
	}
	text := label
	if text == "" {
		text = firstNonEmpty(to.title, strings.TrimSuffix(path.Base(target), ".md"))
	}
	return "[" + text + "](" + link + ")", true
}

// mdlink points a relative markdown link at its target's new place. URLs, anchors and links to
// files the import does not carry stay as written; a missing local target is reported.
func (im *importer) mdlink(f *file, m []string) (string, bool) {
	dest := m[3]
	if strings.Contains(dest, ":") || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "/") {
		return m[0], true // a URL, an anchor or an absolute path: not the vault's to rewrite
	}
	p, frag, _ := strings.Cut(dest, "#")
	if d, err := url.PathUnescape(p); err == nil {
		p = d
	}
	to := im.byPath[strings.ToLower(path.Clean(path.Join(path.Dir(f.src), p)))]
	if to == nil { // Obsidian also reads a path from the vault's root
		to = im.byPath[strings.ToLower(path.Clean(p))]
	}
	if to == nil {
		return "", false
	}
	link := vault.RelLink(f.dst, to.dst)
	if frag != "" {
		link += "#" + frag
	}
	return m[1] + "[" + m[2] + "](" + link + m[4] + ")", true
}

// resolve finds a wikilink's target: a vault path (with or without .md), then a base name,
// preferring the candidate nearest to the linking note.
func (im *importer) resolve(from *file, target string) *file {
	t := strings.ToLower(strings.TrimPrefix(target, "/"))
	if f := im.byPath[t]; f != nil {
		return f
	}
	if f := im.byPath[strings.ToLower(path.Clean(path.Join(path.Dir(from.src), target)))]; f != nil {
		return f
	}
	cands := im.byBase[path.Base(t)]
	if len(cands) == 0 {
		return nil
	}
	best, bestScore := cands[0], -1
	for _, c := range cands {
		if s := commonPrefix(path.Dir(from.src), path.Dir(c.src)); s > bestScore {
			best, bestScore = c, s
		}
	}
	return best
}

// frontmatter is the note's frontmatter with title and date set when missing, type: daily on
// journals, and Hugo's draft: false dropped. Other keys stay, in their order.
func (im *importer) frontmatter(f *file, front []byte) []byte {
	var keep []string
	have := map[string]bool{}
	for _, l := range strings.Split(strings.TrimRight(string(front), "\n"), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		k, v, _ := strings.Cut(l, ":")
		k = strings.TrimSpace(k)
		if k == "draft" && strings.TrimSpace(v) == "false" {
			continue
		}
		if k == "date" {
			l = "date: " + strings.TrimSpace(v)
		}
		if k == "title" && strings.Trim(strings.TrimSpace(v), `"'`) == f.stem && f.title != f.stem && f.title != "" {
			l = "title: " + yamlQuote(f.title) // the template's file name, not a title
		}
		have[k] = true
		keep = append(keep, l)
	}
	var head []string
	if !have["title"] && f.title != "" {
		head = append(head, "title: "+yamlQuote(f.title))
	}
	if !have["date"] && !f.day.IsZero() {
		head = append(head, "date: "+f.day.Format("2006-01-02"))
	}
	if f.journal && !have["type"] {
		keep = append(keep, "type: daily")
	}
	all := append(head, keep...)
	return []byte("---\n" + strings.Join(all, "\n") + "\n---\n")
}

// outsideCode applies fn to the parts of a line that are not inline code.
func outsideCode(line string, fn func(string) string) string {
	if !strings.Contains(line, "`") {
		return fn(line)
	}
	parts := strings.Split(line, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = fn(parts[i])
	}
	return strings.Join(parts, "`")
}

// anchor is GitHub's anchor for a heading: lowercase, spaces to "-", other punctuation dropped.
func anchor(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// splitCamel puts spaces into CamelCase so slugs keep the words: CleanupStorscanDB → Cleanup Storscan DB.
func splitCamel(s string) string {
	r := []rune(s)
	var b strings.Builder
	for i, c := range r {
		if i > 0 && unicode.IsUpper(c) && (unicode.IsLower(r[i-1]) || i+1 < len(r) && unicode.IsLower(r[i+1]) && unicode.IsUpper(r[i-1])) {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func slugPath(dir string) string {
	if dir == "" {
		return ""
	}
	var parts []string
	for _, p := range strings.Split(dir, "/") {
		if s := vault.Slug(p); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "/")
}

func uniqueIn(rel string, taken map[string]bool, v *vault.Vault) string {
	ext := path.Ext(rel)
	stem := strings.TrimSuffix(rel, ext)
	for i := 1; ; i++ {
		c := rel
		if i > 1 {
			c = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}
		if !taken[strings.ToLower(c)] && v.Unique(c) == c {
			return c
		}
	}
}

func frontValue(front []byte, key string) string {
	for _, l := range strings.Split(string(front), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), key+":"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

func yamlQuote(s string) string {
	if strings.ContainsAny(s, ":#\"'{}[],&*!|>%@`") || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "?") {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
	}
	return s
}

func isSize(s string) bool  { return regexp.MustCompile(`^\d+(x\d+)?$`).MatchString(s) }
func isClock(s string) bool { return regexp.MustCompile(`^\d{6}$`).MatchString(s) }

func commonPrefix(a, b string) int {
	pa, pb := strings.Split(a, "/"), strings.Split(b, "/")
	n := 0
	for n < len(pa) && n < len(pb) && pa[n] == pb[n] {
		n++
	}
	return n
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func mustRel(base, p string) string {
	r, err := filepath.Rel(base, p)
	if err != nil {
		return p
	}
	return r
}

func mustRead(p string) []byte {
	b, _ := os.ReadFile(p)
	return b
}
