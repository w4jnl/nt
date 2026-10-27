package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/todo"
)

// The todo list (docs/design.md §4, Todos): every todo in the notes, open or done, in the list
// pane where the pickers go, with the note it is in on the right. It is a picker whose rows are
// todos, so moving, enter and esc work as in the others; x, d, a, e, X, D and r are its own.

// openTodos shows the todo list: the open todos, or with done the done ones. A todo added there
// goes to the todo.md of the folder the browser is in.
func (b *browser) openTodos(done bool) tea.Cmd {
	b.pick = &picker{title: "todo · reading the notes", todos: true, done: done, folder: b.todoFolder()}
	return b.loadTodos("")
}

// todoFolder is the folder the browser is in, or the root from the journals and agent reports,
// which keep no todo list of their own.
func (b *browser) todoFolder() string {
	c := b.v.Cfg.Folders
	for _, f := range []string{c.Daily, c.Agents} {
		if b.dir == f || strings.HasPrefix(b.dir, f+"/") {
			return ""
		}
	}
	return b.dir
}

// loadTodos reads the todos from the notes in the background. keep is the todo to select when
// they arrive, as path:line; when it is gone the selection stays where it was.
func (b *browser) loadTodos(keep string) tea.Cmd {
	b.todoSeq++
	seq, v := b.todoSeq, b.v
	return func() tea.Msg { return todosMsg{seq: seq, keep: keep, items: todo.Collect(v)} }
}

func todoRef(it *todo.Item) string { return it.Path + ":" + strconv.Itoa(it.Line) }

// setTodos fills the todo list with the todos read, open or done, in nt todo's order.
func (b *browser) setTodos(msg todosMsg) tea.Cmd {
	p := b.pick
	if msg.seq != b.todoSeq || p == nil || !p.todos {
		return nil
	}
	todo.Sort(msg.items)
	today := b.v.Day(b.v.Now())
	var items []pickItem
	late := 0
	for _, it := range msg.items {
		if it.Done != p.done {
			continue
		}
		if _, l := it.Word(today); l {
			late++
		}
		items = append(items, pickItem{label: it.Text, hint: it.Path, kind: "note", target: it.Path, line: it.Line, todo: &it})
	}
	sel := min(p.sel, max(0, len(items)-1))
	for i, it := range items {
		if todoRef(it.todo) == msg.keep {
			sel = i
		}
	}
	p.items, p.sel, p.ready = items, sel, true
	switch {
	case p.done:
		p.title = "done · " + strconv.Itoa(len(items))
	case late > 0:
		p.title = "todo · " + strconv.Itoa(len(items)) + " open · " + strconv.Itoa(late) + " late"
	default:
		p.title = "todo · " + strconv.Itoa(len(items)) + " open"
	}
	return b.pickMove(0)
}

// todoSelected is the todo on the selected row of the todo list, or nil.
func (b *browser) todoSelected() *todo.Item {
	p := b.pick
	if p == nil || !p.todos || p.sel >= len(p.items) {
		return nil
	}
	return p.items[p.sel].todo
}

// todoKeys handles the todo list's own keys and reports whether k was one of them.
func (b *browser) todoKeys(k tea.KeyPressMsg) (tea.Cmd, bool) {
	it := b.todoSelected()
	switch s := k.String(); s {
	case "a":
		list, err := todo.ListIn(b.v, b.pick.folder)
		if err != nil {
			b.say(err.Error(), true)
			return nil, true
		}
		return b.prompt("todo", "todo in "+list+": "), true
	case "X":
		b.pick.done, b.pick.sel, b.pick.top, b.pick.ready = !b.pick.done, 0, 0, false
		return b.loadTodos(""), true
	case "r":
		keep := ""
		if it != nil {
			keep = todoRef(it)
		}
		return b.loadTodos(keep), true
	case "x", "space", "d", "e", "D":
		if it == nil {
			return nil, true
		}
		c := *it
		switch s {
		case "d":
			b.todoAt = &c
			return b.prompt("due", "due (fri, +3d, YYYY-MM-DD, empty for none): "), true
		case "e":
			return b.editAt(c.Path, c.Line), true
		case "D":
			b.ask = &question{"delete the todo " + strconv.Quote(c.Text) + "?", func() tea.Cmd {
				if err := todo.Remove(b.v, c); err != nil {
					b.say(err.Error(), true)
				} else {
					b.say("deleted the todo", false)
				}
				return b.loadTodos("")
			}}
			return nil, true
		}
		return b.toggleTodo(c), true
	}
	return nil, false
}

// toggleTodo checks an open todo or opens a done one again.
func (b *browser) toggleTodo(it todo.Item) tea.Cmd {
	var err error
	if it.Done {
		_, err = todo.Uncheck(b.v, it)
	} else {
		_, err = todo.Check(b.v, it, b.v.Day(b.v.Now()))
	}
	switch {
	case err != nil:
		b.say(err.Error(), true)
	case it.Done:
		b.say("open again: "+it.Text, false)
	default:
		b.say("done: "+it.Text, false)
	}
	return b.loadTodos("")
}

// todoInput acts on what was typed at the due and todo prompts.
func (b *browser) todoInput(kind, text string) tea.Cmd {
	today := b.v.Day(b.v.Now())
	switch kind {
	case "due":
		it := b.todoAt
		b.todoAt = nil
		if it == nil {
			return nil
		}
		date, err := todo.ParseWhen(text, today)
		if err == nil {
			_, err = todo.SetDue(b.v, *it, date)
		}
		switch {
		case err != nil:
			b.say(err.Error(), true)
			return nil
		case date == "":
			b.say("no due date: "+it.Text, false)
		default:
			word, _ := todo.When(date, today)
			b.say("due "+word+": "+it.Text, false)
		}
		return b.loadTodos(todoRef(it))
	case "todo":
		if text == "" {
			return nil
		}
		folder := ""
		if b.pick != nil {
			folder = b.pick.folder
		}
		it, err := todo.Add(b.v, folder, text, "", b.v.Now())
		if err != nil {
			b.say(err.Error(), true)
			return nil
		}
		b.say("added to "+it.Path, false)
		if b.pick != nil && b.pick.todos {
			b.pick.done = false
		}
		return b.loadTodos(todoRef(&it))
	}
	return nil
}

// todoRow draws a todo: ○ open or ✓ done, its text, and at the right edge when it is due (late
// in warn, with the word) or when it was done.
func (b *browser) todoRow(it todo.Item, selected bool, w int) string {
	text, muted := b.st.Text, b.st.Muted
	warn, done := lipgloss.NewStyle().Foreground(b.pal.Warn), lipgloss.NewStyle().Foreground(b.pal.Color(theme.Done))
	lead := " "
	if selected {
		bg := b.pal.Surface
		text, muted, warn, done = text.Background(bg), muted.Background(bg), warn.Background(bg), done.Background(bg)
		lead = b.st.Bar.Render("▎")
	}
	mark := muted.Render(" ○ ")
	if it.Done {
		mark = done.Render(" " + theme.Glyph(theme.Done, 0) + " ")
	}
	right := ""
	if word, late := it.Word(b.v.Day(b.v.Now())); word != "" {
		right = muted.Render(word)
		if late {
			right = warn.Render(word)
		}
		right = text.Render("  ") + right + text.Render(" ")
	}
	left := lead + mark + text.Render(it.Text)
	if room := w - ansi.StringWidth(right); ansi.StringWidth(left) > room {
		left = ansi.Truncate(left, max(0, room), "…")
	}
	gap := max(0, w-ansi.StringWidth(left)-ansi.StringWidth(right))
	if !selected && right == "" {
		return left
	}
	return left + text.Render(strings.Repeat(" ", gap)) + right
}
