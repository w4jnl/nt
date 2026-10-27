// Package todo reads and writes the todos in a notes repository (docs/format.md §12): GFM task
// list lines in any note, with due: and done: dates. nt adds new ones to todo.md. The files are
// the truth: todos are read from them each time, and a change rewrites one line under the
// repository lock.
package todo

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/w4jnl/nt/internal/vault"
)

// File is the todo list's name: nt adds new todos to the end of the Inbox section of the one at
// the root, or of the one in a folder the person names.
const File = "todo.md"

// Item is one todo: a task list line in a note.
type Item struct {
	Path    string `json:"rel"`  // repository path of the note
	Line    int    `json:"line"` // 1-based
	Done    bool   `json:"done"`
	Text    string `json:"text"` // without the checkbox, due: and done:
	Due     string `json:"due,omitempty"`
	DoneOn  string `json:"done_on,omitempty"`
	Section string `json:"section,omitempty"` // the heading above it
	Raw     string `json:"-"`                 // the line as it is in the file
}

var (
	// a list item whose text starts with a checkbox: "- [ ] text", "* [x] text", "1. [ ] text"
	taskRe    = regexp.MustCompile(`^(\s*(?:[-*+]|\d{1,9}[.)])[ \t]+)\[([ xX])\]([ \t].*)?$`)
	headingRe = regexp.MustCompile(`^#{1,6}[ \t]+(.+)$`)
	isoRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	inlineDue = regexp.MustCompile(`(?:^|\s)due:(\S+)`)
	stamps    = map[string]*regexp.Regexp{}
)

func init() {
	for _, k := range []string{"due", "done"} {
		stamps[k] = regexp.MustCompile(`(?:^|\s)` + k + `:(\d{4}-\d{2}-\d{2})\b`)
	}
}

// stamp is the date of key: in text, or "".
func stamp(text, key string) string {
	if m := stamps[key].FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// unstamp takes every key:date out of text.
func unstamp(text, key string) string {
	return stamps[key].ReplaceAllString(text, "")
}

// Parse reads the todos in the note at rel. Lines in frontmatter and fenced code blocks are
// not todos, nor is a checkbox with no text.
func Parse(rel string, data []byte) []Item {
	var items []Item
	lines := strings.Split(string(data), "\n")
	start := 0
	if len(lines) > 0 && strings.TrimRight(lines[0], "\r") == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimRight(lines[i], "\r") == "---" {
				start = i + 1
				break
			}
		}
	}
	fence, section := "", ""
	for i := start; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], "\r")
		t := strings.TrimLeft(l, " \t")
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
		if m := headingRe.FindStringSubmatch(l); m != nil {
			section = strings.TrimSpace(strings.TrimRight(m[1], "# "))
			continue
		}
		m := taskRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		it := Item{Path: rel, Line: i + 1, Done: m[2] != " ", Raw: lines[i], Section: section,
			Due: stamp(m[3], "due"), DoneOn: stamp(m[3], "done")}
		it.Text = strings.TrimSpace(unstamp(unstamp(m[3], "due"), "done"))
		if it.Text != "" {
			items = append(items, it)
		}
	}
	return items
}

// Collect reads the todos in every note of the repository. Hidden files and folders, the
// agents folder and conflict copies are left out.
func Collect(v *vault.Vault) []Item {
	agents := strings.Trim(v.Cfg.Folders.Agents, "/")
	var items []Item
	_ = filepath.WalkDir(v.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == v.Root {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || v.RelPath(p) == agents {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".md") || strings.Contains(name, ".conflict-") {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil {
			items = append(items, Parse(v.RelPath(p), b)...)
		}
		return nil
	})
	return items
}

// Sort puts open todos first, by due date with the overdue ones first and the undated ones
// after; among undated ones the todo lists come first in file order (the root's, then the
// folders' by path), then other notes newest first. Done todos follow, the latest done first.
func Sort(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch {
		case a.Done != b.Done:
			return !a.Done
		case a.Done && a.DoneOn != b.DoneOn:
			return a.DoneOn > b.DoneOn
		case !a.Done && (a.Due == "") != (b.Due == ""):
			return a.Due != ""
		case !a.Done && a.Due != b.Due:
			return a.Due < b.Due
		case a.Path != b.Path:
			if a.Path == File || b.Path == File {
				return a.Path == File
			}
			if la, lb := path.Base(a.Path) == File, path.Base(b.Path) == File; la || lb {
				if la && lb {
					return a.Path < b.Path
				}
				return la
			}
			return a.Path > b.Path
		}
		return a.Line < b.Line
	})
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday, "mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday, "wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday, "fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

var relRe = regexp.MustCompile(`^\+(\d{1,3})([dwm])$`)

// ParseWhen turns what a person types for a due date into YYYY-MM-DD, counting from today (a
// day as vault.Day gives it): a date, today, tomorrow, a weekday (the next one after today),
// +3d, +2w or +1m. "", none and - clear the date, which is "".
func ParseWhen(s string, today time.Time) (string, error) {
	w := strings.ToLower(strings.TrimSpace(s))
	day := func(t time.Time) (string, error) { return t.Format("2006-01-02"), nil }
	switch w {
	case "", "none", "-":
		return "", nil
	case "today":
		return day(today)
	case "tomorrow":
		return day(today.AddDate(0, 0, 1))
	}
	if wd, ok := weekdays[w]; ok {
		n := (int(wd) - int(today.Weekday()) + 7) % 7
		if n == 0 {
			n = 7
		}
		return day(today.AddDate(0, 0, n))
	}
	if m := relRe.FindStringSubmatch(w); m != nil {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "d":
			return day(today.AddDate(0, 0, n))
		case "w":
			return day(today.AddDate(0, 0, 7*n))
		}
		return day(today.AddDate(0, n, 0))
	}
	if isoRe.MatchString(w) {
		if t, err := time.ParseInLocation("2006-01-02", w, today.Location()); err == nil {
			return day(t)
		}
	}
	return "", fmt.Errorf("%q is not a date: use YYYY-MM-DD, today, tomorrow, mon…sun, +3d, +2w, +1m or none", s)
}

// days is how many days date is after today: negative when it is before.
func days(date string, today time.Time) (int, bool) {
	t, err := time.ParseInLocation("2006-01-02", date, today.Location())
	if err != nil {
		return 0, false
	}
	return int(math.Round(t.Sub(today).Hours() / 24)), true
}

// When says when a due date is, in words: "2d late", today, tomorrow, a weekday within the
// week, else the date. late reports a date before today.
func When(due string, today time.Time) (word string, late bool) {
	n, ok := days(due, today)
	switch {
	case !ok:
		return due, false
	case n < 0:
		return strconv.Itoa(-n) + "d late", true
	case n == 0:
		return "today", false
	case n == 1:
		return "tomorrow", false
	case n < 7:
		t, _ := time.ParseInLocation("2006-01-02", due, today.Location())
		return strings.ToLower(t.Format("Mon")), false
	}
	return shortDate(due, today), false
}

// Word says when the todo is due, or for a done one when it was done, in words; late reports
// an open todo whose due date has passed.
func (it Item) Word(today time.Time) (word string, late bool) {
	switch {
	case it.Done && it.DoneOn != "":
		return Ago(it.DoneOn, today), false
	case !it.Done && it.Due != "":
		return When(it.Due, today)
	}
	return "", false
}

// Ago says when a todo was done, in words: today, yesterday, else the date.
func Ago(date string, today time.Time) string {
	n, ok := days(date, today)
	switch {
	case !ok:
		return date
	case n == 0:
		return "today"
	case n == -1:
		return "yesterday"
	}
	return shortDate(date, today)
}

func shortDate(date string, today time.Time) string {
	t, err := time.ParseInLocation("2006-01-02", date, today.Location())
	if err != nil {
		return date
	}
	if t.Year() == today.Year() {
		return strings.ToLower(t.Format("2 Jan"))
	}
	return strings.ToLower(t.Format("2 Jan 2006"))
}

// ListIn is the repository path of folder's todo list, folder/todo.md; "" is the root's. A
// hidden folder or the agents folder has none.
func ListIn(v *vault.Vault, folder string) (string, error) {
	folder = strings.Trim(path.Clean("/"+filepath.ToSlash(folder)), "/")
	if folder == "" {
		return File, nil
	}
	agents := strings.Trim(v.Cfg.Folders.Agents, "/")
	if folder == agents || strings.HasPrefix(folder, agents+"/") {
		return "", fmt.Errorf("%s: the agents folder holds reports, not todos", folder)
	}
	for _, part := range strings.Split(folder, "/") {
		if strings.HasPrefix(part, ".") {
			return "", fmt.Errorf("%s: a hidden folder holds no todos", folder)
		}
	}
	return folder + "/" + File, nil
}

// Add puts "- [ ] text", with " due:YYYY-MM-DD" when due is set, at the end of the Inbox
// section of folder's todo.md ("" is the root's), making the file or the section when it is
// missing. A due:<word> in the text counts as due when due is empty. It returns the todo as
// written.
func Add(v *vault.Vault, folder, text, due string, now time.Time) (Item, error) {
	list, err := ListIn(v, folder)
	if err != nil {
		return Item{}, err
	}
	text = strings.Join(strings.Fields(text), " ")
	if m := inlineDue.FindStringSubmatch(text); m != nil && due == "" {
		due = m[1]
	}
	text = strings.TrimSpace(inlineDue.ReplaceAllString(text, ""))
	if text == "" {
		return Item{}, errors.New("nothing to add: give the todo's text")
	}
	date, err := ParseWhen(due, v.Day(now))
	if err != nil {
		return Item{}, err
	}
	line := "- [ ] " + text
	if date != "" {
		line += " due:" + date
	}
	at := 0
	err = v.Update(list, func(old []byte) ([]byte, error) {
		if old == nil {
			old = []byte("# Todo\n")
		}
		doc := vault.AddToSection(old, "Inbox", line)
		lines := strings.Split(string(doc), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if lines[i] == line {
				at = i + 1
				break
			}
		}
		return doc, nil
	})
	return Item{Path: list, Line: at, Text: text, Due: date, Section: "Inbox", Raw: line}, err
}

// Check marks the todo done on day, adding done:YYYY-MM-DD.
func Check(v *vault.Vault, it Item, day time.Time) (Item, error) {
	return change(v, it, func(l string) string {
		return restamp(box(l, "x"), "done", day.Format("2006-01-02"))
	})
}

// Uncheck opens the todo again, taking its done: date away.
func Uncheck(v *vault.Vault, it Item) (Item, error) {
	return change(v, it, func(l string) string { return restamp(box(l, " "), "done", "") })
}

// SetDue sets the todo's due date, YYYY-MM-DD, or with "" takes it away. done: stays last.
func SetDue(v *vault.Vault, it Item, date string) (Item, error) {
	return change(v, it, func(l string) string {
		done := stamp(l, "done")
		l = restamp(restamp(l, "done", ""), "due", date)
		if done != "" {
			l = restamp(l, "done", done)
		}
		return l
	})
}

// Remove deletes the todo's line.
func Remove(v *vault.Vault, it Item) error {
	_, err := rewrite(v, it, func(lines []string, i int) []string {
		return append(lines[:i:i], lines[i+1:]...)
	})
	return err
}

// box puts mark (" " or "x") in the line's checkbox.
func box(line, mark string) string {
	m := taskRe.FindStringSubmatchIndex(strings.TrimRight(line, "\r"))
	if m == nil {
		return line
	}
	return line[:m[4]] + mark + line[m[5]:]
}

// restamp takes key:date out of the line and, when date is set, puts key:date at its end.
func restamp(line, key, date string) string {
	cr := strings.HasSuffix(line, "\r")
	l := strings.TrimRight(unstamp(strings.TrimSuffix(line, "\r"), key), " \t")
	if date != "" {
		l += " " + key + ":" + date
	}
	if cr {
		l += "\r"
	}
	return l
}

func change(v *vault.Vault, it Item, edit func(string) string) (Item, error) {
	var out Item
	at, err := rewrite(v, it, func(lines []string, i int) []string {
		lines[i] = edit(lines[i])
		return lines
	})
	if err != nil {
		return Item{}, err
	}
	if b, err := os.ReadFile(v.Abs(it.Path)); err == nil {
		for _, n := range Parse(it.Path, b) {
			if n.Line == at {
				out = n
			}
		}
	}
	return out, nil
}

// rewrite finds the todo's line in its file under the lock, where it was or, when the file
// changed around it, the one line equal to it, and replaces the file's lines with fn's. It
// returns the line's 1-based number.
func rewrite(v *vault.Vault, it Item, fn func(lines []string, i int) []string) (int, error) {
	at := 0
	err := v.Update(it.Path, func(old []byte) ([]byte, error) {
		lines := strings.Split(string(old), "\n")
		i := it.Line - 1
		if i < 0 || i >= len(lines) || lines[i] != it.Raw {
			i = -1
			for j, l := range lines {
				if l == it.Raw {
					if i >= 0 {
						i = -1
						break
					}
					i = j
				}
			}
		}
		if old == nil || i < 0 {
			return nil, fmt.Errorf("%s: the todo %q changed since it was read; read the todos again", it.Path, it.Text)
		}
		at = i + 1
		return []byte(strings.Join(fn(lines, i), "\n")), nil
	})
	return at, err
}
