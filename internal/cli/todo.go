package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/todo"
	"github.com/w4jnl/nt/internal/vault"
)

// nt todo [folder] [--done|--all] [--json]; nt todo add|done|undo|due|rm.
func runTodo(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "add":
			return runTodoAdd(args[1:])
		case "done", "undo", "due", "rm":
			return runTodoChange(args[0], args[1:])
		}
	}
	return runTodoList(args)
}

// todoJSON is a todo as the commands print it with --json: when is its date in words as the
// browser shows it (2d late, fri, 3 nov; yesterday for a done one), late whether it is overdue.
func todoJSON(v *vault.Vault, it todo.Item) map[string]any {
	when, late := it.Word(v.Day(v.Now()))
	return map[string]any{"path": v.Abs(it.Path), "rel": it.Path, "line": it.Line, "done": it.Done,
		"text": it.Text, "due": it.Due, "done_on": it.DoneOn, "section": it.Section, "when": when, "late": late}
}

// nt todo [folder] [--done|--all]: the open todos in every note, or in the notes under folder,
// overdue first; the done ones, latest first; or all of them.
func runTodoList(args []string) int {
	args, asJSON := takeBool(args, "--json")
	args, done := takeBool(args, "--done")
	args, all := takeBool(args, "--all")
	pos, err := positional("todo", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) > 1 {
		return fail(errors.New("todo: name one folder, or use add, done, undo, due or rm"))
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	folder := ""
	if len(pos) == 1 {
		folder = strings.Trim(path.Clean("/"+filepath.ToSlash(pos[0])), "/")
		if !isDir(v.Abs(folder)) {
			return fail(fmt.Errorf("todo: %q is no folder in %s, nor add, done, undo, due or rm", pos[0], tilde(v.Root)))
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
	if asJSON {
		out := []map[string]any{}
		for _, it := range show {
			out = append(out, todoJSON(v, it))
		}
		return printJSON(map[string]any{"todos": out})
	}
	st := cliStyles(m)
	if len(show) == 0 {
		switch {
		case all:
			lipgloss.Println(st.Muted.Render("no todos: nt todo add \"text\" adds one to " + todo.File))
		case done:
			lipgloss.Println(st.Muted.Render("no done todos"))
		default:
			lipgloss.Println(st.Muted.Render("no open todos"))
		}
		return 0
	}
	today := v.Day(v.Now())
	textW, whenW := 0, 0
	for _, it := range show {
		word, _ := it.Word(today)
		textW, whenW = max(textW, min(56, lipgloss.Width(it.Text))), max(whenW, lipgloss.Width(word))
	}
	for _, it := range show {
		lipgloss.Println(todoRow(m, it, today, textW, whenW))
	}
	return 0
}

// todoRow is one todo for the terminal: its glyph, its text, when it is due (or was done), and
// where it is. The text and the when are padded to textW and whenW columns.
func todoRow(m config.Machine, it todo.Item, today time.Time, textW, whenW int) string {
	st, pal := cliStyles(m), cliPalette(m)
	text := it.Text
	if lipgloss.Width(text) > textW {
		text = string([]rune(text)[:max(0, textW-1)]) + "…"
	}
	mark := st.Muted.Render("○")
	if it.Done {
		mark = lipgloss.NewStyle().Foreground(pal.Color(theme.Done)).Render(theme.Glyph(theme.Done, 0))
	}
	word, late := it.Word(today)
	when := st.Muted
	if late {
		when = lipgloss.NewStyle().Foreground(pal.Warn)
	}
	row := mark + " " + st.Text.Render(text) + strings.Repeat(" ", max(0, textW-lipgloss.Width(text))) + "  "
	if whenW > 0 {
		row += when.Render(word) + strings.Repeat(" ", whenW-lipgloss.Width(word)) + "  "
	}
	return row + st.Faint.Render(it.Path+":"+strconv.Itoa(it.Line))
}

// nt todo add "text" [--due when] [--in folder]: a new todo at the end of Inbox in todo.md, the
// root's or folder's. The text can come on stdin.
func runTodoAdd(args []string) int {
	args, asJSON := takeBool(args, "--json")
	args, due, _, err := takeValue(args, "--due")
	if err != nil {
		return fail(fmt.Errorf("todo add: %w", err))
	}
	args, folder, _, err := takeValue(args, "--in")
	if err != nil {
		return fail(fmt.Errorf("todo add: %w", err))
	}
	words, err := positional("todo add", args)
	if err != nil {
		return fail(err)
	}
	text := strings.Join(words, " ")
	if text == "" && !stdinIsTerminal() {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 64<<10))
		if err != nil {
			return fail(err)
		}
		text = string(b)
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	it, err := todo.Add(v, folder, text, due, v.Now())
	if err != nil {
		return fail(fmt.Errorf("todo add: %w", err))
	}
	if asJSON {
		return printJSON(todoJSON(v, it))
	}
	st := cliStyles(m)
	lipgloss.Println(st.Muted.Render("added ") + oneRow(m, it, v.Day(v.Now())))
	return 0
}

// nt todo done|undo|rm <match>, nt todo due <match> <when>: check, uncheck, delete a todo or
// set its due date. match is path:line as nt todo prints it, or words from the todo's text.
func runTodoChange(kind string, args []string) int {
	args, asJSON := takeBool(args, "--json")
	words, err := positional("todo "+kind, args)
	if err != nil {
		return fail(err)
	}
	when := ""
	if kind == "due" {
		if len(words) < 2 {
			return fail(errors.New(`todo due: give the todo and when it is due: nt todo due "passport" fri (none clears it)`))
		}
		when, words = words[len(words)-1], words[:len(words)-1]
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	today := v.Day(v.Now())
	date := ""
	if kind == "due" {
		if date, err = todo.ParseWhen(when, today); err != nil {
			return fail(fmt.Errorf("todo due: %w", err))
		}
	}
	want := map[string]func(todo.Item) bool{
		"done": func(it todo.Item) bool { return !it.Done },
		"due":  func(it todo.Item) bool { return !it.Done },
		"undo": func(it todo.Item) bool { return it.Done },
		"rm":   func(todo.Item) bool { return true },
	}[kind]
	it, err := matchTodo(todo.Collect(v), strings.Join(words, " "), want)
	if err != nil {
		return fail(fmt.Errorf("todo %s: %w", kind, err))
	}
	verb := map[string]string{"done": "done", "undo": "open again", "due": "due", "rm": "deleted"}[kind]
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
		return fail(fmt.Errorf("todo %s: %w", kind, err))
	}
	if asJSON {
		out := todoJSON(v, it)
		out["deleted"] = kind == "rm"
		return printJSON(out)
	}
	st := cliStyles(m)
	lipgloss.Println(st.Muted.Render(verb+" ") + oneRow(m, it, today))
	return 0
}

// oneRow is a todo on its own line, as add, done, undo, due and rm print it.
func oneRow(m config.Machine, it todo.Item, today time.Time) string {
	word, _ := it.Word(today)
	return todoRow(m, it, today, lipgloss.Width(it.Text), lipgloss.Width(word))
}

var lineRef = regexp.MustCompile(`^(.+\.md):(\d+)$`)

// matchTodo picks the todo match names among those want accepts: path:line, else the one whose
// text holds match (case does not matter), else the one whose text is match.
func matchTodo(items []todo.Item, match string, want func(todo.Item) bool) (todo.Item, error) {
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
