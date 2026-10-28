// Package ops is what the apps ask of nt (docs/design.md §9): the operations the command line
// runs with --json, as functions that return what it prints. The command line prints them; the
// iOS app's core answers with them through Runner.Run, which takes the same command lines the
// Mac app runs nt with, so both apps read one JSON.
package ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/gitsync"
	"github.com/w4jnl/nt/internal/scan"
	"github.com/w4jnl/nt/internal/search"
	"github.com/w4jnl/nt/internal/todo"
	"github.com/w4jnl/nt/internal/vault"
)

// Open finds and opens the notes repository with this machine's config.
func Open() (*vault.Vault, config.Machine, error) {
	m, err := config.LoadMachine()
	if err != nil {
		return nil, m, err
	}
	root := vault.Find(m)
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil, m, fmt.Errorf("no notes repository at %s: run nt init %s, or set repo in %s", Tilde(root), Tilde(root), Tilde(config.MachinePath()))
	}
	v, err := vault.Open(root, m.Host)
	return v, m, err
}

// JSON is v as nt prints it with --json: indented, one value, a newline after it.
func JSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	err := enc.Encode(v)
	return b.Bytes(), err
}

// Tilde shortens a path under the home folder to ~/….
func Tilde(p string) string {
	if h, err := os.UserHomeDir(); err == nil {
		if rest, ok := strings.CutPrefix(p, h+"/"); ok {
			return "~/" + rest
		}
		if p == h {
			return "~"
		}
	}
	return p
}

// MARK: Arguments

// Flag removes the flag name from args and reports whether it was there.
func Flag(args []string, name string) ([]string, bool) {
	out, found := args[:0:0], false
	for _, a := range args {
		if a == name {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

// Value removes "name value" or "name=value" from args.
func Value(args []string, name string) ([]string, string, bool, error) {
	var out []string
	val, found := "", false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == name:
			if i+1 >= len(args) {
				return nil, "", false, fmt.Errorf("%s needs a value", name)
			}
			i++
			val, found = args[i], true
		case strings.HasPrefix(a, name+"="):
			val, found = strings.TrimPrefix(a, name+"="), true
		default:
			out = append(out, a)
		}
	}
	return out, val, found, nil
}

// Positional checks no unknown flags are left; "--" ends the flags.
func Positional(cmd string, args []string) ([]string, error) {
	var out []string
	for i, a := range args {
		if a == "--" {
			return append(out, args[i+1:]...), nil
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			return nil, fmt.Errorf("%s: unknown flag %s", cmd, a)
		}
		out = append(out, a)
	}
	return out, nil
}

// MARK: Journals and notes

// Journal is today's journal, as nt t --json prints it.
type Journal struct {
	Path    string `json:"path"`
	Rel     string `json:"rel"`
	Created bool   `json:"created"`
}

// Today makes today's journal when there is none.
func Today(v *vault.Vault) (Journal, error) {
	rel, created, err := v.EnsureJournal(v.Now())
	return Journal{Path: v.Abs(rel), Rel: rel, Created: created}, err
}

// Line is a line added to today's journal, as nt a --json prints it.
type Line struct {
	Path string `json:"path"`
	Rel  string `json:"rel"`
	Line string `json:"line"`
}

// Append adds a timestamped line to today's journal.
func Append(v *vault.Vault, text string) (Line, error) {
	rel, line, err := v.Append(text, v.Now())
	return Line{Path: v.Abs(rel), Rel: rel, Line: line}, err
}

// Note is a note made, as nt n --json prints it.
type Note struct {
	Path string `json:"path"`
	Rel  string `json:"rel"`
}

// New makes a dated note titled title in folder, linked from today's journal.
func New(v *vault.Vault, title, folder string) (Note, error) {
	rel, err := v.NewNote(title, folder, v.Now())
	return Note{Path: v.Abs(rel), Rel: rel}, err
}

// Day is the day as nt counts it (day_starts), YYYY-MM-DD.
func Day(v *vault.Vault) string { return v.Day(v.Now()).Format("2006-01-02") }

// MARK: Todos

// Todo is a todo as the commands print it with --json: When is its date in words as the browser
// shows it (2d late, fri, 3 nov; yesterday for a done one), Late whether it is overdue, Deleted
// whether nt todo rm took it out.
type Todo struct {
	Path    string `json:"path"`
	Rel     string `json:"rel"`
	Line    int    `json:"line"`
	Done    bool   `json:"done"`
	Text    string `json:"text"`
	Due     string `json:"due"`
	DoneOn  string `json:"done_on"`
	Section string `json:"section"`
	When    string `json:"when"`
	Late    bool   `json:"late"`
	Deleted *bool  `json:"deleted,omitempty"`
}

// TodoOf is it as the commands print it.
func TodoOf(v *vault.Vault, it todo.Item) Todo {
	when, late := it.Word(v.Day(v.Now()))
	return Todo{Path: v.Abs(it.Path), Rel: it.Path, Line: it.Line, Done: it.Done, Text: it.Text, Due: it.Due,
		DoneOn: it.DoneOn, Section: it.Section, When: when, Late: late}
}

// TodoList is nt todo --json: {"todos": [...]}.
type TodoList struct {
	Todos []Todo `json:"todos"`
}

// Todos are the todos in every note, or under folder, overdue first: the open ones, the done
// ones, or all of them.
func Todos(v *vault.Vault, folder string, done, all bool) ([]todo.Item, error) {
	if named := folder; folder != "" {
		folder = strings.Trim(path.Clean("/"+filepath.ToSlash(folder)), "/")
		if !isDir(v.Abs(folder)) {
			return nil, fmt.Errorf("todo: %q is no folder in %s, nor add, done, undo, due or rm", named, Tilde(v.Root))
		}
	}
	items := todo.Collect(v)
	todo.Sort(items)
	var show []todo.Item
	for _, it := range items {
		if (all || it.Done == done) && (folder == "" || strings.HasPrefix(it.Path, folder+"/")) {
			show = append(show, it)
		}
	}
	return show, nil
}

// AddTodo adds a todo to the end of Inbox in the todo.md of folder ("" the root's).
func AddTodo(v *vault.Vault, folder, text, due string) (todo.Item, error) {
	it, err := todo.Add(v, folder, text, due, v.Now())
	if err != nil {
		return it, fmt.Errorf("todo add: %w", err)
	}
	return it, nil
}

// ChangeTodo checks (done), unchecks (undo), deletes (rm) or dates (due, to when) the todo match
// names: path:line, or words from its text.
func ChangeTodo(v *vault.Vault, kind, match, when string) (todo.Item, error) {
	today := v.Day(v.Now())
	date := ""
	if kind == "due" {
		var err error
		if date, err = todo.ParseWhen(when, today); err != nil {
			return todo.Item{}, fmt.Errorf("todo due: %w", err)
		}
	}
	want := map[string]func(todo.Item) bool{
		"done": func(it todo.Item) bool { return !it.Done },
		"due":  func(it todo.Item) bool { return !it.Done },
		"undo": func(it todo.Item) bool { return it.Done },
		"rm":   func(todo.Item) bool { return true },
	}[kind]
	if want == nil {
		return todo.Item{}, fmt.Errorf("todo: %q is not add, done, undo, due or rm", kind)
	}
	it, err := MatchTodo(todo.Collect(v), match, want)
	if err != nil {
		return it, fmt.Errorf("todo %s: %w", kind, err)
	}
	switch kind {
	case "done":
		it, err = todo.Check(v, it, today)
	case "undo":
		it, err = todo.Uncheck(v, it)
	case "due":
		it, err = todo.SetDue(v, it, date)
	case "rm":
		err = todo.Remove(v, it)
	}
	if err != nil {
		return it, fmt.Errorf("todo %s: %w", kind, err)
	}
	return it, nil
}

var lineRef = regexp.MustCompile(`^(.+\.md):(\d+)$`)

// MatchTodo picks the todo match names among those want accepts: path:line, else the one whose
// text holds match (case does not matter), else the one whose text is match.
func MatchTodo(items []todo.Item, match string, want func(todo.Item) bool) (todo.Item, error) {
	if m := lineRef.FindStringSubmatch(match); m != nil {
		n, _ := strconv.Atoi(m[2])
		for _, it := range items {
			if it.Path == m[1] && it.Line == n {
				return it, nil
			}
		}
		return todo.Item{}, fmt.Errorf("no todo at %s: nt todo --all lists them", match)
	}
	q := strings.ToLower(strings.TrimSpace(match))
	if q == "" {
		return todo.Item{}, errors.New("name the todo: words from its text, or path:line as nt todo prints it")
	}
	var found, exact []todo.Item
	for _, it := range items {
		if !want(it) || !strings.Contains(strings.ToLower(it.Text), q) {
			continue
		}
		found = append(found, it)
		if strings.ToLower(it.Text) == q {
			exact = append(exact, it)
		}
	}
	switch {
	case len(found) == 1:
		return found[0], nil
	case len(exact) == 1:
		return exact[0], nil
	case len(found) == 0:
		return todo.Item{}, fmt.Errorf("no todo matches %q: nt todo --all lists them", match)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d todos match %q; name one by path:line:", len(found), match)
	for _, it := range found {
		fmt.Fprintf(&b, "\n  %s:%d  %s", it.Path, it.Line, it.Text)
	}
	return todo.Item{}, errors.New(b.String())
}

// MARK: Editors

// Folders are the repository's own folders, as the index names them.
type Folders struct {
	Daily  string `json:"daily"`
	Agents string `json:"agents"`
	Weekly string `json:"weekly"`
}

// Index is the repository as an editor's sidebar and "Linked from" need it (nt _editor index).
type Index struct {
	Root    string              `json:"root"`
	State   string              `json:"state"`
	Day     string              `json:"day"`
	Pins    []string            `json:"pins"`
	Folders Folders             `json:"folders"`
	Dirs    []string            `json:"dirs"`
	Notes   []scan.Note         `json:"notes"`
	Back    map[string][]string `json:"back"`
}

// IndexOf reads every note of the repository for the index; state is where status.json is.
func IndexOf(v *vault.Vault, state string) Index {
	ix := scan.Build(v.Root)
	notes := ix.Notes
	if notes == nil {
		notes = []scan.Note{}
	}
	pins := v.Cfg.Pins
	if pins == nil {
		pins = []string{}
	}
	return Index{Root: v.Root, State: state, Day: Day(v), Pins: pins,
		Folders: Folders{Daily: v.Cfg.Folders.Daily, Agents: v.Cfg.Folders.Agents, Weekly: v.Cfg.Folders.Weekly},
		Dirs:    dirsOf(v.Root), Notes: notes, Back: ix.Back}
}

// dirsOf lists every folder under root as a repository path, sorted, without hidden ones.
func dirsOf(root string) []string {
	dirs := []string{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(root, p)
		dirs = append(dirs, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(dirs)
	return dirs
}

// Saved is what an editor's save did (nt _editor save): saved, the file's SHA-256 now, and when
// another writer changed it since the editor read it, its text as it is now (not saved) or the
// conflict copy its version was kept in (saved with keep).
type Saved struct {
	Saved bool    `json:"saved"`
	Sha   string  `json:"sha"`
	Text  *string `json:"text,omitempty"`
	Copy  string  `json:"copy,omitempty"`
}

// Save writes a note an editor changed, under the repository lock through a temp file
// (docs/format.md §10). With base it writes only when the file on disk is still the one the
// editor read, whose SHA-256 is base. When another writer changed it since, nothing is written
// and Saved carries the file as it is now, for the editor to merge its changes into; keep writes
// all the same, after saving that file beside the note as a conflict copy (§3). A note that is
// gone is written again.
func Save(v *vault.Vault, rel string, text []byte, base string, keep bool) (Saved, error) {
	if !strings.HasSuffix(rel, ".md") || strings.HasPrefix(rel, ".") || strings.Contains(rel, "/.") {
		return Saved{}, fmt.Errorf("_editor save: %s is not a note: notes are .md files outside folders starting with .", rel)
	}
	unlock, err := v.Lock()
	if err != nil {
		return Saved{}, err
	}
	defer unlock()
	p := v.Abs(rel)
	cur, err := os.ReadFile(p)
	gone := errors.Is(err, os.ErrNotExist)
	if err != nil && !gone {
		return Saved{}, fmt.Errorf("_editor save: %w", err)
	}
	reply := Saved{Saved: true, Sha: SHA256(text)}
	if !gone && base != "" && SHA256(cur) != base && !bytes.Equal(cur, text) {
		if !keep {
			now := string(cur)
			return Saved{Saved: false, Sha: SHA256(cur), Text: &now}, nil
		}
		cp := v.Unique(gitsync.ConflictName(rel, v.Host, v.Now()))
		if err := vault.WriteFile(v.Abs(cp), cur); err != nil {
			return Saved{}, fmt.Errorf("_editor save: %w", err)
		}
		reply.Copy = cp
	}
	if gone || !bytes.Equal(cur, text) {
		if err := vault.WriteFile(p, text); err != nil {
			return Saved{}, fmt.Errorf("_editor save: %w", err)
		}
	}
	return reply, nil
}

// SHA256 is the hex SHA-256 of b, as editors name the version of a note they read.
func SHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// InRepo is the repository path of the note at p: p from the working directory, else p as a
// repository path. The note need not exist yet, its folder must.
func InRepo(v *vault.Vault, p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if !isFile(abs) && (isFile(v.Abs(p)) || !isDir(filepath.Dir(abs)) && isDir(filepath.Dir(v.Abs(p)))) {
		abs = v.Abs(p)
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("%s: no such folder", filepath.Dir(p))
	}
	root, err := filepath.EvalSymlinks(v.Root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, filepath.Join(dir, filepath.Base(abs)))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is not in the notes repository %s", p, Tilde(v.Root))
	}
	return filepath.ToSlash(rel), nil
}

// MARK: Search, settings, images

// Search finds the lines that match query in the notes: with rg, else in Go.
func Search(v *vault.Vault, query string) ([]search.Match, error) {
	m, err := search.Search(context.Background(), v.Root, query)
	if err != nil {
		return nil, fmt.Errorf("s: %w", err)
	}
	if m == nil {
		m = []search.Match{}
	}
	return m, nil
}

// Setting is one setting as nt config --json prints it.
type Setting struct {
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	File    string   `json:"file"`
	Path    string   `json:"path"`
	Kind    string   `json:"kind"`
	Choices []string `json:"choices"`
	Help    string   `json:"help"`
}

// SettingList is nt config --json: {"settings": [...]}.
type SettingList struct {
	Settings []Setting `json:"settings"`
}

// LoadBoth reads this machine's config and the repository's, and where the repository's is.
func LoadBoth() (config.Machine, config.Repo, string, error) {
	m, err := config.LoadMachine()
	if err != nil {
		return m, config.Repo{}, "", err
	}
	root := vault.Find(m)
	r, err := config.LoadRepo(root)
	return m, r, filepath.Join(root, ".nt", "config.toml"), err
}

// Settings is every setting with its value, its file and what it does.
func Settings() ([]Setting, error) {
	m, r, repoPath, err := LoadBoth()
	if err != nil {
		return nil, err
	}
	var out []Setting
	for _, s := range config.Settings {
		p := config.MachinePath()
		if s.File == "repo" {
			p = repoPath
		}
		out = append(out, Setting{Key: s.Key, Value: s.Value(m, r), File: s.File, Path: p, Kind: s.Kind, Choices: s.Choices(), Help: s.Help})
	}
	return out, nil
}

// SetSetting writes one setting typed as raw: this machine's in its config file, the
// repository's in .nt/config.toml under the repository lock, for sync to share. It says where.
func SetSetting(s config.Setting, raw string) (string, error) {
	if s.File == "machine" {
		if _, err := config.SetMachine(s.Key, raw); err != nil {
			return "", err
		}
		return "this machine: " + Tilde(config.MachinePath()), nil
	}
	v, _, err := Open()
	if err != nil {
		return "", err
	}
	if err := v.SetSetting(s.Key, raw); err != nil {
		return "", err
	}
	return "the repository: " + Tilde(v.Abs(".nt/config.toml")) + ", shared by sync", nil
}

// ImageTypes are the image files nt paste takes as they are: what GitHub and browsers show.
var ImageTypes = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true}

// Pasted is an image saved beside a note, as nt paste --json prints it, with the line that shows it.
type Pasted struct {
	Path     string `json:"path"`
	Rel      string `json:"rel"`
	Markdown string `json:"markdown"`
}

// PasteFile saves the image file from beside the note as assets/YYYY-MM-DD-<name><ext>, name
// from the file's own when empty.
func PasteFile(v *vault.Vault, note, name, from string) (Pasted, error) {
	ext := filepath.Ext(from)
	if !ImageTypes[strings.ToLower(ext)] {
		return Pasted{}, fmt.Errorf("paste: %s is not an image nt shows (png, jpg, gif, webp, svg)", filepath.Base(from))
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(from), ext)
	}
	return Paste(v, note, name, from, ext)
}

// Paste saves the image at src, of type ext, beside the note.
func Paste(v *vault.Vault, note, name, src, ext string) (Pasted, error) {
	st, err := os.Stat(src)
	if err != nil {
		return Pasted{}, fmt.Errorf("paste: %w", err)
	}
	if st.Size() > gitsync.MaxFile {
		return Pasted{}, fmt.Errorf("paste: the image is %d MB, and sync leaves files over 20 MB out: make it smaller first", st.Size()>>20)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return Pasted{}, fmt.Errorf("paste: %w", err)
	}
	rel, err := v.SaveAsset(note, name, ext, data, v.Now())
	if err != nil {
		return Pasted{}, fmt.Errorf("paste: %w", err)
	}
	alt := strings.TrimSpace(name)
	if alt == "" {
		alt = "image"
	}
	line := "![" + strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(alt) + "](" + vault.RelLink(note, rel) + ")"
	return Pasted{Path: v.Abs(rel), Rel: rel, Markdown: line}, nil
}

func isFile(p string) bool { st, err := os.Stat(p); return err == nil && st.Mode().IsRegular() }
func isDir(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }
