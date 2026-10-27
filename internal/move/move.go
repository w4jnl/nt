// Package move moves and renames notes and folders in a notes repository (docs/format.md §13)
// and keeps every link working: the links to what moved and the relative links inside it are
// rewritten in the same pass, under the repository lock. Nothing here runs git; sync sees the
// files move like any other change.
package move

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/w4jnl/nt/internal/mdparse"
	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/scan"
	"github.com/w4jnl/nt/internal/vault"
)

// Plan is what a move or a rename does, worked out before anything is written.
type Plan struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Folder bool   `json:"folder"`
	Title  string `json:"title,omitempty"` // a renamed note's new title
	Files  []Step `json:"files"`           // the files that move: the note and its attachments, or all a folder holds
	Links  []Edit `json:"links"`           // the notes whose links change, at their paths after the move
	Config bool   `json:"config"`          // pins or default_folder in .nt/config.toml follow the folder
	DryRun bool   `json:"dry_run"`

	content map[string][]byte // the new text of the notes that change, by their path after the move
	config  []byte
}

// Step is one file moving.
type Step struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Edit is a note whose links change, and how many.
type Edit struct {
	Path  string `json:"path"`
	Links int    `json:"links"`
}

// Move moves the note or folder at from, a repository path, to to: into the folder to names
// when it ends in "/", is "" or "/" (the root), or is a folder already; else to that path. The
// folders it needs are made, and new names in to become slugs. With dry it only works out what
// would change.
func Move(v *vault.Vault, from, to string, dry bool) (*Plan, error) {
	if strings.TrimSpace(to) == "/" {
		to = "" // the root, not the file system's
	}
	return run(v, from, dry, func(j *job) error {
		into := to == "" || strings.HasSuffix(strings.TrimSpace(to), "/")
		dest, err := j.clean(to)
		if err != nil {
			return err
		}
		if !into {
			into = isDir(v.Abs(dest))
		}
		if into || j.p.Folder {
			dir, err := j.folder(dest)
			if err != nil {
				return err
			}
			if !into {
				j.p.To = dir
				return nil
			}
			j.p.To = path.Join(dir, path.Base(from))
			if !j.p.Folder && path.Dir(j.p.To) == path.Dir(from) {
				return fmt.Errorf("%s is in %s already", from, folderName(path.Dir(from)))
			}
			if !j.p.Folder {
				j.p.To = v.Unique(j.p.To)
			}
			return nil
		}
		dir, err := j.folder(path.Dir(dest))
		if err != nil {
			return err
		}
		slug := vault.Slug(strings.TrimSuffix(path.Base(dest), ".md"))
		if slug == "" {
			return fmt.Errorf("%q has no letters or digits for a file name", path.Base(dest))
		}
		j.p.To = path.Join(dir, slug+".md")
		return nil
	})
}

// Rename gives the note at rel a new title: the title in its frontmatter and its heading, the
// file name that goes with it (its date kept), and the text of the links that showed the old
// title. A folder gets the new name, as a slug, in the same parent folder.
func Rename(v *vault.Vault, rel, name string, dry bool) (*Plan, error) {
	name = strings.TrimSpace(name)
	return run(v, rel, dry, func(j *job) error {
		slug := vault.Slug(name)
		if slug == "" {
			return fmt.Errorf("%q has no letters or digits for a file name", name)
		}
		dir := path.Dir(rel)
		if j.p.Folder {
			j.p.To = path.Join(dir, slug)
			return nil
		}
		if d := datePrefix.FindString(path.Base(rel)); d != "" {
			slug = d + "-" + strings.TrimPrefix(slug, d+"-")
		}
		j.p.To = path.Join(dir, slug+".md")
		if j.p.To != rel && !strings.EqualFold(j.p.To, rel) {
			j.p.To = v.Unique(j.p.To)
		}
		j.p.Title, j.newTitle = name, name
		return nil
	})
}

// Mkdir makes the folder rel, new names in it as slugs, with an empty .gitkeep so git keeps it
// while it holds no note. It returns the folder's repository path.
func Mkdir(v *vault.Vault, rel string) (string, error) {
	j := &job{v: v, p: &Plan{}}
	c, err := j.clean(rel)
	if err != nil {
		return "", err
	}
	if c == "" {
		return "", errors.New("name the folder to make, as a path from the root of the notes")
	}
	unlock, err := v.Lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	dir, err := j.folder(c)
	if err != nil {
		return "", err
	}
	if isDir(v.Abs(dir)) {
		return "", fmt.Errorf("%s/ is there already", dir)
	}
	if err := os.MkdirAll(v.Abs(dir), 0o755); err != nil {
		return "", err
	}
	return dir, vault.WriteFile(v.Abs(path.Join(dir, ".gitkeep")), nil)
}

var datePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// job is one move or rename on its way.
type job struct {
	v *vault.Vault
	p *Plan

	moves              map[string]string // a note and its attachments: old path → new
	oldTitle, newTitle string            // a renamed note's titles
}

// Movable says why the note or folder at rel cannot move or be renamed, or nil when it can.
func Movable(v *vault.Vault, rel string) error {
	_, err := (&job{v: v, p: &Plan{}}).source(rel)
	return err
}

// Folder is the repository path a folder typed as dir names: the folders that exist keep their
// names as they are on disk, new ones become slugs.
func Folder(v *vault.Vault, dir string) (string, error) {
	j := &job{v: v, p: &Plan{}}
	c, err := j.clean(dir)
	if err != nil {
		return "", err
	}
	return j.folder(c)
}

// run checks from, lets target set p.To, and under the repository lock works out the plan
// and, unless dry, carries it out.
func run(v *vault.Vault, from string, dry bool, target func(*job) error) (*Plan, error) {
	j := &job{v: v, p: &Plan{DryRun: dry}, moves: map[string]string{}}
	from, err := j.source(from)
	if err != nil {
		return nil, err
	}
	if !dry && v.Editing() {
		return nil, errors.New("a note has unsaved changes in an editor: save it, then move")
	}
	unlock, err := v.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	j.p.From = from
	if err := target(j); err != nil {
		return nil, err
	}
	if err := j.checkTarget(); err != nil {
		return nil, err
	}
	if err := j.plan(); err != nil {
		return nil, err
	}
	if dry {
		return j.p, nil
	}
	return j.p, j.apply()
}

// source is the repository path of the note or folder that moves, when it can.
func (j *job) source(from string) (string, error) {
	from, err := j.clean(from)
	if err != nil {
		return "", err
	}
	if from == "" {
		return "", errors.New("the root of the notes does not move")
	}
	st, err := os.Stat(j.v.Abs(from))
	switch {
	case err != nil:
		return "", fmt.Errorf("%s: no such note or folder", from)
	case st.IsDir():
		j.p.Folder = true
	case !strings.HasSuffix(from, ".md"):
		return "", fmt.Errorf("%s is not a note: nt moves notes (.md) and folders; attachments go with their notes", from)
	}
	if err := j.check(from, "move"); err != nil {
		return "", err
	}
	if path.Base(from) == "assets" && j.p.Folder {
		return "", fmt.Errorf("%s/ holds attachments: they move with their notes", from)
	}
	return from, nil
}

// clean turns a path as typed into a repository path: "" is the root. An absolute path must be
// inside the repository.
func (j *job) clean(p string) (string, error) {
	p = filepath.ToSlash(strings.TrimSpace(p))
	if filepath.IsAbs(p) {
		root, _ := filepath.EvalSymlinks(j.v.Root)
		abs := filepath.FromSlash(p)
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			abs = r
		}
		r, err := filepath.Rel(root, abs)
		if err != nil {
			return "", err
		}
		p = filepath.ToSlash(r)
	}
	c := path.Clean(p)
	if c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("%s is outside the notes repository %s", p, j.v.Root)
	}
	if c == "." {
		return "", nil // the root
	}
	return strings.Trim(c, "/"), nil
}

// folder is the repository path of the folder dir names: the folders that exist keep their
// names as they are on disk, whatever case was typed; new ones are slugs.
func (j *job) folder(dir string) (string, error) {
	if dir == "" || dir == "." {
		return "", nil
	}
	var out []string
	for _, part := range strings.Split(dir, "/") {
		parent := path.Join(out...)
		if name, ok := existing(j.v.Abs(parent), part); ok {
			out = append(out, name)
			continue
		}
		s := vault.Slug(part)
		if s == "" {
			return "", fmt.Errorf("%q has no letters or digits for a folder name", part)
		}
		out = append(out, s)
	}
	f := path.Join(out...)
	return f, j.check(f, "go")
}

// existing finds the folder in parent whose name is name, ignoring case as macOS does.
func existing(parent, name string) (string, bool) {
	des, _ := os.ReadDir(parent)
	for _, d := range des {
		if d.IsDir() && d.Name() == name {
			return name, true
		}
	}
	for _, d := range des {
		if d.IsDir() && strings.EqualFold(d.Name(), name) {
			return d.Name(), true
		}
	}
	return "", false
}

// check refuses what nt keeps by date or does not show: the journals and the agent reports,
// hidden names, assets folders as a place for notes. how is "move" for what moves and "go" for
// where it goes.
func (j *job) check(rel, how string) error {
	c := j.v.Cfg.Folders
	for _, f := range []struct{ dir, what string }{{c.Daily, "the journals"}, {c.Agents, "the agent reports"}, {c.Weekly, "the weekly digests"}} {
		if f.dir == "" || rel != f.dir && !strings.HasPrefix(rel, f.dir+"/") {
			continue
		}
		if how == "move" {
			return fmt.Errorf("%s: %s/ holds %s, which stay where their date puts them", rel, f.dir, f.what)
		}
		return fmt.Errorf("%s: %s/ holds only %s: pick another folder", rel, f.dir, f.what)
	}
	for _, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") {
			return fmt.Errorf("%s: names starting with . are not notes", rel)
		}
		if how == "go" && part == "assets" {
			return fmt.Errorf("%s: assets folders hold attachments, not notes", rel)
		}
	}
	return nil
}

// checkTarget refuses a target that is taken, the same, or inside the folder that moves.
func (j *job) checkTarget() error {
	from, to := j.p.From, j.p.To
	if err := j.check(to, "go"); err != nil {
		return err
	}
	switch {
	case to == from && j.newTitle == "":
		return fmt.Errorf("%s is there already", from)
	case to != from && strings.EqualFold(to, from):
		return fmt.Errorf("%s and %s differ only in case, which git on macOS does not see: pick another name", from, to)
	case j.p.Folder && strings.HasPrefix(to, from+"/"):
		return fmt.Errorf("%s/ cannot move into itself", from)
	case to != from && taken(j.v, to):
		return fmt.Errorf("%s is there already: pick another name", to)
	}
	return nil
}

// taken reports a file or folder at rel, ignoring case as macOS does.
func taken(v *vault.Vault, rel string) bool {
	if _, err := os.Lstat(v.Abs(rel)); err == nil {
		return true
	}
	des, _ := os.ReadDir(v.Abs(path.Dir(rel)))
	for _, d := range des {
		if strings.EqualFold(d.Name(), path.Base(rel)) {
			return true
		}
	}
	return false
}

// plan works out every file that moves and every note whose links change.
func (j *job) plan() error {
	v, p := j.v, j.p
	if p.Folder {
		err := filepath.WalkDir(v.Abs(p.From), func(f string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
				rel := v.RelPath(f)
				to, _ := j.newPath(rel)
				p.Files = append(p.Files, Step{rel, to})
			}
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		note, err := os.ReadFile(v.Abs(p.From))
		if err != nil {
			return err
		}
		if j.newTitle != "" {
			j.oldTitle = render.Title(note, p.From)
		}
		if p.To != p.From {
			j.moves[p.From] = p.To
			p.Files = append(p.Files, Step{p.From, p.To})
		}
		if path.Dir(p.To) != path.Dir(p.From) {
			for _, a := range v.Unshared(p.From, j.attachments(note)) {
				to := v.Unique(path.Join(path.Dir(p.To), "assets", path.Base(a)))
				j.moves[a] = to
				p.Files = append(p.Files, Step{a, to})
			}
		}
	}
	if err := j.links(); err != nil {
		return err
	}
	j.p.config = j.configFollows()
	j.p.Config = j.p.config != nil
	return nil
}

// attachments is the files in the note's own assets folder that it links to.
func (j *job) attachments(note []byte) []string {
	dir := path.Join(path.Dir(j.p.From), "assets")
	seen := map[string]bool{}
	var out []string
	for _, l := range mdparse.Parse(note).Links {
		if l.Wiki || mdparse.External(l.Dest) {
			continue
		}
		to, _, ok := mdparse.Resolve(j.p.From, l.Dest)
		if !ok || path.Dir(to) != dir || seen[to] || !isFile(j.v.Abs(to)) {
			continue
		}
		seen[to] = true
		out = append(out, to)
	}
	return out
}

// newPath is where the file at rel is after the move, and whether it moves.
func (j *job) newPath(rel string) (string, bool) {
	if to, ok := j.moves[rel]; ok {
		return to, true
	}
	if j.p.Folder {
		if rel == j.p.From {
			return j.p.To, true
		}
		if rest, ok := strings.CutPrefix(rel, j.p.From+"/"); ok {
			return j.p.To + "/" + rest, true
		}
	}
	return rel, false
}

// links rewrites the links in every note that moves or links to something that moves, and the
// title of a renamed note.
func (j *job) links() error {
	v, p := j.v, j.p
	var ix *scan.Index // for legacy wikilinks, read once the first one turns up
	p.content = map[string][]byte{}
	err := filepath.WalkDir(v.Root, func(f string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && f != v.Root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		from := v.RelPath(f)
		note, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		to, _ := j.newPath(from)
		out, n := mdparse.Rewrite(note, func(l *mdparse.Link) bool {
			if l.Wiki && ix == nil {
				ix = scan.Build(v.Root)
			}
			return j.link(from, to, l, ix)
		})
		if from == p.From && j.newTitle != "" {
			out = retitle(out, j.oldTitle, j.newTitle)
		}
		if n > 0 {
			p.Links = append(p.Links, Edit{to, n})
		}
		if string(out) != string(note) {
			p.content[to] = out
		}
		return nil
	})
	sort.Slice(p.Links, func(a, b int) bool { return p.Links[a].Path < p.Links[b].Path })
	return err
}

// link points one link in the note at from, which will be at to, at where its target will be,
// and gives a link that showed a renamed note's old title the new one.
func (j *job) link(from, to string, l *mdparse.Link, ix *scan.Index) bool {
	var target, anchor string
	if l.Wiki {
		t, ok := ix.Target(from, *l)
		if !ok || t == from {
			return false
		}
		if _, moves := j.newPath(t); !moves {
			return false // still found by its name
		}
		target = t
		if _, a, ok := strings.Cut(l.Dest, "#"); ok {
			anchor = mdparse.Anchor(a)
		}
	} else {
		if l.Dest == "" || mdparse.External(l.Dest) || strings.HasPrefix(l.Dest, "#") || strings.HasPrefix(l.Dest, "/") {
			return false
		}
		t, a, ok := mdparse.Resolve(from, l.Dest)
		if !ok {
			return false
		}
		target, anchor = t, a
	}
	newTarget, moves := j.newPath(target)
	changed := false
	if !moves && to != from {
		if _, err := os.Stat(j.v.Abs(target)); err != nil {
			return false // pointed at nothing before the move, or was not a path at all: as it was
		}
	}
	if l.Wiki || moves || to != from {
		dest := vault.RelLink(to, newTarget)
		if p, _, _ := strings.Cut(l.Dest, "#"); strings.HasSuffix(p, "/") {
			dest += "/"
		}
		if anchor != "" {
			dest += "#" + anchor
		}
		if dest != l.Dest || l.Wiki {
			l.Dest, changed = dest, true
		}
	}
	if j.newTitle != "" && target == j.p.From && !l.Image && j.newTitle != j.oldTitle &&
		(l.Text == j.oldTitle || l.Wiki && strings.EqualFold(l.Text, strings.TrimSuffix(path.Base(target), ".md"))) {
		l.Text, changed = j.newTitle, true
	}
	return changed
}

// retitle sets a note's title: the title key in its frontmatter, and its first # heading when
// the title came from it or it showed the old title. A note with neither gets a heading.
func retitle(note []byte, old, title string) []byte {
	front, body := render.SplitFrontmatter(note)
	head := string(note[:len(note)-len(body)])
	raw, keyed := "", false // the title key as written, and whether there was one
	if front != nil {
		lines := strings.SplitAfter(head, "\n")
		for i, l := range lines[1:] {
			if v, ok := strings.CutPrefix(l, "title:"); ok {
				raw = strings.Trim(strings.TrimSpace(v), `"'`)
				lines[i+1] = "title: " + vault.YAMLString(title) + "\n"
				keyed = true
				break
			}
		}
		if !keyed {
			lines = append(lines[:1], append([]string{"title: " + vault.YAMLString(title) + "\n"}, lines[1:]...)...)
		}
		head = strings.Join(lines, "")
	}
	lines := strings.SplitAfter(string(body), "\n")
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
		h, ok := strings.CutPrefix(strings.TrimRight(l, "\r\n"), "# ")
		if !ok {
			continue
		}
		if h = strings.TrimSpace(h); !keyed || h == old || h == raw {
			lines[i] = "# " + title + l[len(strings.TrimRight(l, "\r\n")):]
		}
		return []byte(head + strings.Join(lines, ""))
	}
	if front == nil {
		return []byte("# " + title + "\n\n" + string(body))
	}
	return []byte(head + string(body))
}

// configFollows is .nt/config.toml with pins and default_folder pointing at the folder's new
// place, or nil when they do not name it.
func (j *job) configFollows() []byte {
	if !j.p.Folder {
		return nil
	}
	old, err := os.ReadFile(j.v.Abs(".nt/config.toml"))
	if err != nil {
		return nil
	}
	lines := strings.SplitAfter(string(old), "\n")
	inPins := false
	for i, l := range lines {
		key, _, _ := strings.Cut(l, "=")
		key = strings.TrimSpace(key)
		if !inPins && key != "pins" && key != "default_folder" {
			continue
		}
		value, comment := l, ""
		if k := commentAt(l); k >= 0 {
			value, comment = l[:k], l[k:]
		}
		value = quoted.ReplaceAllStringFunc(value, func(q string) string {
			if to, ok := j.newPath(strings.Trim(q, `"`)); ok {
				return `"` + to + `"`
			}
			return q
		})
		lines[i] = value + comment
		if key == "pins" {
			inPins = strings.Contains(value, "[") && !strings.Contains(value, "]")
		} else if inPins && strings.Contains(value, "]") {
			inPins = false
		}
	}
	if out := strings.Join(lines, ""); out != string(old) {
		return []byte(out)
	}
	return nil
}

var quoted = regexp.MustCompile(`"[^"]*"`)

// commentAt is where a TOML comment starts on the line, outside quotes, or -1.
func commentAt(l string) int {
	in := false
	for i, r := range l {
		switch {
		case r == '"':
			in = !in
		case r == '#' && !in:
			return i
		}
	}
	return -1
}

// apply carries the plan out: the files move, then the notes whose links changed are written at
// their new places, then the config.
func (j *job) apply() error {
	v, p := j.v, j.p
	if p.Folder {
		if err := os.MkdirAll(v.Abs(path.Dir(p.To)), 0o755); err != nil {
			return err
		}
		if err := os.Rename(v.Abs(p.From), v.Abs(p.To)); err != nil {
			return err
		}
	} else {
		for _, s := range p.Files {
			if err := os.MkdirAll(v.Abs(path.Dir(s.To)), 0o755); err != nil {
				return err
			}
			if err := os.Rename(v.Abs(s.From), v.Abs(s.To)); err != nil {
				return err
			}
		}
		if len(p.Files) > 1 {
			_ = os.Remove(v.Abs(path.Join(path.Dir(p.From), "assets"))) // only when empty
		}
	}
	paths := make([]string, 0, len(p.content))
	for rel := range p.content {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		if err := vault.WriteFile(v.Abs(rel), p.content[rel]); err != nil {
			return err
		}
	}
	if p.config != nil {
		if err := vault.WriteFile(v.Abs(".nt/config.toml"), p.config); err != nil {
			return err
		}
	}
	return nil
}

func folderName(dir string) string {
	if dir == "." || dir == "" {
		return "the root"
	}
	return dir + "/"
}

func isDir(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }
func isFile(p string) bool { st, err := os.Stat(p); return err == nil && st.Mode().IsRegular() }
