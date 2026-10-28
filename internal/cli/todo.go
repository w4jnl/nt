package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/ops"
	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/todo"
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
		folder = pos[0]
	}
	show, err := ops.Todos(v, folder, done, all)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		list := ops.TodoList{Todos: []ops.Todo{}}
		for _, it := range show {
			list.Todos = append(list.Todos, ops.TodoOf(v, it))
		}
		return printJSON(list)
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
	it, err := ops.AddTodo(v, folder, text, due)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return printJSON(ops.TodoOf(v, it))
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
	it, err := ops.ChangeTodo(v, kind, strings.Join(words, " "), when)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		out := ops.TodoOf(v, it)
		deleted := kind == "rm"
		out.Deleted = &deleted
		return printJSON(out)
	}
	verb := map[string]string{"done": "done", "undo": "open again", "due": "due", "rm": "deleted"}[kind]
	st := cliStyles(m)
	lipgloss.Println(st.Muted.Render(verb+" ") + oneRow(m, it, v.Day(v.Now())))
	return 0
}

// oneRow is a todo on its own line, as add, done, undo, due and rm print it.
func oneRow(m config.Machine, it todo.Item, today time.Time) string {
	word, _ := it.Word(today)
	return todoRow(m, it, today, lipgloss.Width(it.Text), lipgloss.Width(word))
}
