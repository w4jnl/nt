package cli

import (
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/editor"
	"github.com/w4jnl/nt/internal/ops"
	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/vault"
)

// runEditor is the editor hand-off, a variable so tests can replace it.
var runEditor = func(path string, line int) error { return editor.Command(path, line).Run() }

// nt t: open today's journal, creating it first. With --json, or when stdout is not a
// terminal, print where it is instead.
func runToday(args []string) int {
	args, asJSON := takeBool(args, "--json")
	if _, err := positional("t", args); err != nil {
		return fail(err)
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	j, err := ops.Today(v)
	if err != nil {
		return fail(err)
	}
	abs := j.Path
	if asJSON {
		return printJSON(j)
	}
	if !isTerminal(os.Stdout) {
		lipgloss.Println(abs)
		return 0
	}
	return edit(v, m, abs, sectionEnd(abs, "Log"))
}

// nt a "text": add a timestamped line to today's journal; the text can come on stdin.
func runAppend(args []string) int {
	args, asJSON := takeBool(args, "--json")
	words, err := positional("a", args)
	if err != nil {
		return fail(err)
	}
	text := strings.Join(words, " ")
	if text == "" && !stdinIsTerminal() {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		if err != nil {
			return fail(err)
		}
		text = string(b)
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	l, err := ops.Append(v, text)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return printJSON(l)
	}
	st := cliStyles(m)
	lipgloss.Println(st.Muted.Render(l.Rel+": ") + st.Text.Render(strings.SplitN(l.Line, "\n", 2)[0]))
	return 0
}

// nt n "title" [folder]: a dated note in folder (the default folder when none), linked from
// today's journal, opened in the editor when a person is at the terminal.
func runNew(args []string) int {
	args, asJSON := takeBool(args, "--json")
	pos, err := positional("n", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) > 2 {
		return fail(errors.New(`n: use nt n "title" [folder]; quote a title with spaces`))
	}
	title, folder := "", ""
	if len(pos) > 0 {
		title = pos[0]
	}
	if len(pos) > 1 {
		folder = pos[1]
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	n, err := ops.New(v, title, folder)
	if err != nil {
		return fail(err)
	}
	abs := n.Path
	if asJSON {
		return printJSON(n)
	}
	if !isTerminal(os.Stdout) {
		lipgloss.Println(abs)
		return 0
	}
	return edit(v, m, abs, editor.LastLine(abs)+1)
}

// nt ls [folder]: the folders and notes in a folder, newest first.
func runList(args []string) int {
	args, asJSON := takeBool(args, "--json")
	pos, err := positional("ls", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) > 1 {
		return fail(errors.New("ls: name one folder"))
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	folder := ""
	if len(pos) == 1 {
		folder = strings.Trim(path.Clean("/"+filepath.ToSlash(pos[0])), "/")
	}
	entries, err := os.ReadDir(v.Abs(folder))
	if err != nil {
		return fail(err)
	}
	type note struct {
		Path  string `json:"path"`
		Title string `json:"title"`
	}
	var folders []string
	var notes []note
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		rel := path.Join(folder, name)
		switch {
		case e.IsDir():
			folders = append(folders, rel+"/")
		case strings.HasSuffix(name, ".md"):
			notes = append(notes, note{Path: rel, Title: titleOfFile(v.Abs(rel))})
		}
	}
	sort.Slice(folders, func(i, j int) bool {
		return vault.FolderBefore(path.Base(folders[i]), path.Base(folders[j]))
	})
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path > notes[j].Path })
	if asJSON {
		if folders == nil {
			folders = []string{}
		}
		if notes == nil {
			notes = []note{}
		}
		return printJSON(map[string]any{"folder": folder, "folders": folders, "notes": notes})
	}
	st := cliStyles(m)
	for _, f := range folders {
		lipgloss.Println(st.Key.Render(f))
	}
	width := 0
	for _, n := range notes {
		width = max(width, len(path.Base(n.Path)))
	}
	for _, n := range notes {
		base := path.Base(n.Path)
		lipgloss.Println(st.Text.Render(base) + strings.Repeat(" ", width-len(base)+2) + st.Muted.Render(n.Title))
	}
	return 0
}

// titleOfFile reads a note's title: frontmatter title, first "# " heading, file stem.
func titleOfFile(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := io.ReadFull(f, head)
	return render.Title(head[:n], p)
}

// sectionEnd is the 1-based line of the last non-blank line of "## name" (the heading itself
// when the section is empty), or the last line when there is no such section.
func sectionEnd(p, name string) int {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(b), "\n")
	head := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t\r") == "## "+name {
			head = i
			break
		}
	}
	if head < 0 {
		return editor.LastLine(p)
	}
	last := head
	for i := head + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") || strings.HasPrefix(lines[i], "# ") {
			break
		}
		if strings.TrimSpace(lines[i]) != "" {
			last = i
		}
	}
	return last + 1
}
