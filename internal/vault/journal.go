package vault

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// The built-in templates (docs/format.md §5 and §8). The daily one uses only the date, so two
// devices that create the same day write the same bytes and git merges them cleanly.
const (
	dailyTemplate = `---
title: {{.Long}}
date: {{.Date}}
type: daily
---

# {{.Long}}

[← {{.Yesterday}}]({{.Rel .YesterdayJournal}}) · [{{.Tomorrow}} →]({{.Rel .TomorrowJournal}}) · [agents]({{.Rel .AgentsDigest}})

## Log

## Notes
`
	noteTemplate = `---
title: {{yaml .Title}}
date: {{.Date}}
---

# {{.Title}}

`
)

// TemplateData is what a template sees (docs/format.md §8).
type TemplateData struct {
	Title, Slug, Date, Time, Weekday, Long, Yesterday, Tomorrow, Folder string
	// repository paths of the neighbouring journals and the day's agent digest
	YesterdayJournal, TomorrowJournal, AgentsDigest string
	self                                            string
}

// Rel is the relative link from the note being created to a repository path.
func (d TemplateData) Rel(to string) string { return RelLink(d.self, to) }

// RelLink is the link text from the file at repository path from to repository path to.
func RelLink(from, to string) string {
	fromDir := strings.Split(path.Dir(from), "/")
	if path.Dir(from) == "." {
		fromDir = nil
	}
	toParts := strings.Split(to, "/")
	i := 0
	for i < len(fromDir) && i < len(toParts)-1 && fromDir[i] == toParts[i] {
		i++
	}
	ups := strings.Repeat("../", len(fromDir)-i)
	return linkEscape(ups + strings.Join(toParts[i:], "/"))
}

// linkEscape percent-encodes what a markdown link destination cannot hold as is.
func linkEscape(p string) string {
	r := strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E")
	return r.Replace(p)
}

func (v *Vault) data(self, title, folder string, t time.Time) TemplateData {
	day := v.Day(t)
	y, tm := day.AddDate(0, 0, -1), day.AddDate(0, 0, 1)
	return TemplateData{
		Title: title, Slug: Slug(title), Folder: folder,
		Date: day.Format("2006-01-02"), Time: t.Format("15:04"), Weekday: day.Format("Monday"),
		Long:      day.Format("Monday 2 January 2006"),
		Yesterday: y.Format("2006-01-02"), Tomorrow: tm.Format("2006-01-02"),
		YesterdayJournal: v.JournalRel(y), TomorrowJournal: v.JournalRel(tm),
		AgentsDigest: v.AgentsRel(day) + "/README.md",
		self:         self,
	}
}

// render runs the repository's template name (.nt/templates/<name>) or the built-in one.
func (v *Vault) render(name, builtin string, d TemplateData) ([]byte, error) {
	src := builtin
	if b, err := os.ReadFile(v.Abs(".nt/templates/" + name)); err == nil {
		src = string(b)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	t, err := template.New(name).Funcs(template.FuncMap{"yaml": yamlString}).Parse(src)
	if err != nil {
		return nil, fmt.Errorf(".nt/templates/%s: %w", name, err)
	}
	var b bytes.Buffer
	if err := t.Execute(&b, d); err != nil {
		return nil, fmt.Errorf(".nt/templates/%s: %w", name, err)
	}
	return b.Bytes(), nil
}

// YAMLString writes s as a YAML scalar for frontmatter: plain when that reads back as the same
// string, else double-quoted.
func YAMLString(s string) string { return yamlString(s) }

// yamlString writes s as a YAML scalar: plain when that reads back as the same string, else
// double-quoted.
func yamlString(s string) string {
	plain := s != "" && s == strings.TrimSpace(s) && !strings.ContainsAny(s, ":#\"'\\\n\t{}[],&*!|>%@`") &&
		!strings.ContainsAny(s[:1], "-?") && !isYAMLWord(s)
	if plain {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

func isYAMLWord(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off", "null", "~":
		return true
	}
	for _, c := range s {
		if (c < '0' || c > '9') && c != '.' && c != '-' && c != '+' && c != 'e' {
			return false
		}
	}
	return true // a number
}

// EnsureJournal creates the journal of the day t belongs to unless it exists, and returns its
// repository path.
func (v *Vault) EnsureJournal(t time.Time) (rel string, created bool, err error) {
	rel = v.JournalRel(v.Day(t))
	err = v.Update(rel, func(old []byte) ([]byte, error) {
		if old != nil {
			return old, nil
		}
		created = true
		return v.render("daily.md", dailyTemplate, v.data(rel, "", "", t))
	})
	return rel, created, err
}

// Append adds "- HH:MM text" to the end of the Log section of the journal of the day t belongs
// to (docs/format.md §5), creating the journal first. Lines after the first stay in the same
// list item, indented. It returns the journal and the line as written.
func (v *Vault) Append(text string, t time.Time) (rel, line string, err error) {
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if strings.TrimSpace(text) == "" {
		return "", "", errors.New("nothing to add: give the text as an argument or on stdin")
	}
	lines := strings.Split(text, "\n")
	line = "- " + t.Format("15:04") + " " + strings.TrimSpace(lines[0])
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			line += "\n"
		} else {
			line += "\n  " + l
		}
	}
	rel, _, err = v.EnsureJournal(t)
	if err != nil {
		return "", "", err
	}
	err = v.Update(rel, func(old []byte) ([]byte, error) { return AddToSection(old, "Log", line), nil })
	return rel, line, err
}

// LinkFromJournal adds "- [title](link)" for the note at repository path note to the end of the
// Notes section of the journal of the day t belongs to, unless the journal links it already.
func (v *Vault) LinkFromJournal(title, note string, t time.Time) (string, error) {
	rel, _, err := v.EnsureJournal(t)
	if err != nil {
		return "", err
	}
	link := RelLink(rel, note)
	item := "- [" + strings.NewReplacer("[", `\[`, "]", `\]`).Replace(title) + "](" + link + ")"
	err = v.Update(rel, func(old []byte) ([]byte, error) {
		if bytes.Contains(old, []byte("]("+link+")")) {
			return old, nil
		}
		return AddToSection(old, "Notes", item), nil
	})
	return rel, err
}

// AddToSection puts item at the end of the "## <name>" section: after its last non-blank line,
// with a blank line after the heading and before the next heading. A missing section is added
// at the end of the file.
func AddToSection(doc []byte, name, item string) []byte {
	lines := strings.Split(string(doc), "\n")
	head := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t\r") == "## "+name {
			head = i
			break
		}
	}
	if head < 0 {
		s := strings.TrimRight(string(doc), "\n")
		if s != "" {
			s += "\n\n"
		}
		return []byte(s + "## " + name + "\n\n" + item + "\n")
	}
	end := len(lines)
	for i := head + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") || strings.HasPrefix(lines[i], "# ") {
			end = i
			break
		}
	}
	last := head
	for i := head + 1; i < end; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			last = i
		}
	}
	var out []string
	out = append(out, lines[:last+1]...)
	if last == head {
		out = append(out, "")
	}
	out = append(out, item)
	if end < len(lines) {
		out = append(out, "")
		out = append(out, lines[end:]...)
	} else {
		out = append(out, "")
	}
	return []byte(strings.Join(out, "\n"))
}

// NewNote creates a dated topic note titled title in folder (the default folder when empty) and
// links it from the day's journal. It returns the note's repository path.
func (v *Vault) NewNote(title, folder string, t time.Time) (string, error) {
	title = strings.TrimSpace(title)
	folder = strings.Trim(path.Clean("/"+strings.TrimSpace(folder)), "/")
	if folder == "" || folder == "." {
		folder = v.Cfg.DefaultFolder
	}
	if strings.HasPrefix(folder, ".") || strings.Contains(folder, "/.") {
		return "", fmt.Errorf("%s: folders starting with . are not notes", folder)
	}
	day := v.Day(t)
	name := day.Format("2006-01-02") + "-" + Slug(title)
	if Slug(title) == "" {
		name = day.Format("2006-01-02") + "-" + t.Format("150405")
	}
	var rel string
	unlock, err := v.Lock()
	if err != nil {
		return "", err
	}
	rel = v.Unique(folder + "/" + name + ".md")
	if title == "" {
		title = strings.TrimSuffix(path.Base(rel), ".md")
	}
	body, err := v.render("note.md", noteTemplate, v.data(rel, title, folder, t))
	if err == nil {
		err = WriteFile(v.Abs(rel), body)
	}
	unlock()
	if err != nil {
		return "", err
	}
	if _, err := v.LinkFromJournal(title, rel, t); err != nil {
		return rel, err
	}
	return rel, nil
}

var logTime = regexp.MustCompile(`^- (\d{2}):(\d{2})\b`)

// SortLog puts the items of a journal's ## Log in the order of their times and drops an item
// that is there twice, word for word, as a line-by-line merge can leave it. An item is a line
// "- HH:MM …" with the lines under it that have no time of their own; times before dayStart
// belong to the end of the day. The rest of the journal stays byte for byte.
func SortLog(doc []byte, dayStart time.Duration) []byte {
	lines := strings.Split(string(doc), "\n")
	head := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t\r") == "## Log" {
			head = i
			break
		}
	}
	if head < 0 {
		return doc
	}
	end := len(lines)
	for i := head + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") || strings.HasPrefix(lines[i], "# ") {
			end = i
			break
		}
	}
	body := lines[head+1 : end]
	tail := len(body)
	for tail > 0 && strings.TrimSpace(body[tail-1]) == "" {
		tail--
	}
	type item struct {
		at    int
		lines []string
	}
	var lead []string
	var items []item
	for _, l := range body[:tail] {
		if m := logTime.FindStringSubmatch(l); m != nil {
			h, _ := strconv.Atoi(m[1])
			mi, _ := strconv.Atoi(m[2])
			at := h*60 + mi
			if time.Duration(at)*time.Minute < dayStart {
				at += 24 * 60
			}
			items = append(items, item{at, []string{l}})
			continue
		}
		if len(items) == 0 {
			lead = append(lead, l)
			continue
		}
		items[len(items)-1].lines = append(items[len(items)-1].lines, l)
	}
	seen := map[string]bool{}
	kept := items[:0]
	for _, it := range items {
		if k := strings.Join(it.lines, "\n"); !seen[k] {
			seen[k] = true
			kept = append(kept, it)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].at < kept[j].at })
	out := append([]string{}, lines[:head+1]...)
	out = append(out, lead...)
	for _, it := range kept {
		out = append(out, it.lines...)
	}
	out = append(out, body[tail:]...)
	out = append(out, lines[end:]...)
	if s := strings.Join(out, "\n"); s != string(doc) {
		return []byte(s)
	}
	return doc
}

// SortPastLogs puts the ## Log of the journals of the week before today in time order, where
// merges of lines added on two devices at once left them out of it (docs/format.md §5). Today's
// journal is left as it is: lines moved in a file another device is still adding to would come
// back twice from the line-by-line merge. It returns the journals it wrote.
func (v *Vault) SortPastLogs(now time.Time) ([]string, error) {
	start, _ := v.Cfg.DayStart()
	today := v.Day(now)
	var wrote []string
	for i := 1; i <= 7; i++ {
		rel := v.JournalRel(today.AddDate(0, 0, -i))
		if !isFile(v.Abs(rel)) {
			continue
		}
		changed := false
		err := v.Update(rel, func(old []byte) ([]byte, error) {
			if old == nil {
				return old, nil
			}
			out := SortLog(old, start)
			changed = !bytes.Equal(out, old)
			return out, nil
		})
		if err != nil {
			return wrote, err
		}
		if changed {
			wrote = append(wrote, rel)
		}
	}
	return wrote, nil
}
