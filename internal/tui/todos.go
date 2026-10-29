package tui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/todo"
)

// A day's todo list (docs/design.md §4, Calendar): the todos due or done on a day, in the list
// pane where the pickers go, with the note each is in on the right. It is a picker whose rows are
// todos, so moving, enter and esc work as in the others; x, d, a, e, X, D and r are its own. The
// todos of every day are in the note's pane (todoview.go).

// plainFolder is the folder the browser is in, or the root from the journals and agent reports,
// which keep no todo list and no folders of their own.
func (b *browser) plainFolder() string {
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
	if msg.seq != b.todoSeq {
		return nil
	}
	b.count(msg.items)
	if b.tv != nil {
		b.setTodoView(msg.items, msg.keep)
	}
	if p == nil || !p.todos {
		return nil
	}
	todo.Sort(msg.items)
	today := b.v.Day(b.v.Now())
	var items []pickItem
	late := 0
	for _, it := range msg.items {
		if it.Done != p.done || p.day != "" && it.Due != p.day && it.DoneOn != p.day {
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
	if p.day != "" {
		if d, err := time.ParseInLocation("2006-01-02", p.day, time.Local); err == nil {
			p.title = d.Format("Mon 2 Jan") + " · " + p.title
		}
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

// todoHold is how long a ticked row stays in the todo list after the last tick.
const todoHold = 700 * time.Millisecond

type todoSettleMsg struct{ seq int }

// toggleTodo checks an open todo or opens a done one again. The row stays in the list with its
// glyph turned until the ticks settle, todoHold after the last, so the person sees what they did
// and another x takes it back; then the list is read again (settleTodos).
func (b *browser) toggleTodo(it todo.Item) tea.Cmd {
	var n todo.Item
	var err error
	if it.Done {
		n, err = todo.Uncheck(b.v, it)
	} else {
		n, err = todo.Check(b.v, it, b.v.Day(b.v.Now()))
	}
	if err != nil {
		b.say(err.Error(), true)
		return b.loadTodos("")
	}
	if it.Done {
		b.say("open again: "+it.Text, false)
	} else {
		b.say("done: "+it.Text, false)
	}
	if p := b.pick; p != nil && p.todos && p.sel < len(p.items) {
		p.items[p.sel].todo = &n // the line as it is now, for the glyph and for another x
		if p.ticked == nil {
			p.ticked = map[string]bool{}
		}
		p.ticked[todoRef(&n)] = true
	}
	if tv := b.tv; tv != nil {
		ref := todoRef(&n)
		for i := range tv.items {
			if todoRef(&tv.items[i]) == ref {
				tv.items[i] = n
			}
		}
		if tv.held == nil {
			tv.held = map[string]bool{}
		}
		if _, ok := tv.held[ref]; !ok {
			tv.held[ref] = it.Done // it stays in the list it was in until the ticks settle
		}
	}
	var cmds []tea.Cmd
	if b.showing == n.Path && b.p.changed() { // the note beside the list shows the tick too
		b.p.load()
		b.p.render()
		cmds = append(cmds, b.p.sendImages())
	}
	b.todoSettleSeq++
	seq := b.todoSettleSeq
	return tea.Batch(append(cmds, tea.Tick(todoHold, func(time.Time) tea.Msg { return todoSettleMsg{seq} }))...)
}

// settleTodos reads the todo list again once the ticks settle: the ticked rows go to the other
// list. The selection stays on its row, or, on one that went, moves to the one that took its
// place.
func (b *browser) settleTodos(msg todoSettleMsg) tea.Cmd {
	if msg.seq != b.todoSettleSeq {
		return nil
	}
	if tv := b.tv; tv != nil {
		keep := ""
		if it := tv.selected(); it != nil {
			if _, held := tv.held[todoRef(it)]; !held {
				keep = todoRef(it)
			}
		}
		return b.loadTodos(keep)
	}
	p := b.pick
	if p == nil || !p.todos {
		return nil
	}
	keep := ""
	if it := b.todoSelected(); it != nil && !p.ticked[todoRef(it)] {
		keep = todoRef(it)
	}
	p.ticked = nil
	return b.loadTodos(keep)
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
		folder, due := "", ""
		if b.pick != nil {
			folder, due = b.pick.folder, b.pick.day
		} else if b.tv != nil {
			folder = b.tv.addTo()
		}
		it, err := todo.Add(b.v, folder, text, due, b.v.Now())
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

// todoRow draws a todo: ○ open or ✓ done, its text, the note it is in (from, faint) when given,
// and at the right edge when it is due (late in warn, with the word) or when it was done.
func (b *browser) todoRow(it todo.Item, selected bool, w int, from string) string {
	text, muted, faint := b.st.Text, b.st.Muted, b.st.Faint
	warn, done := lipgloss.NewStyle().Foreground(b.pal.Warn), lipgloss.NewStyle().Foreground(b.pal.Color(theme.Done))
	lead := " "
	if selected {
		bg := b.pal.Surface
		text, muted, faint, warn, done = text.Background(bg), muted.Background(bg), faint.Background(bg), warn.Background(bg), done.Background(bg)
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
	if from != "" {
		left += faint.Render("  " + from)
	}
	if room := w - ansi.StringWidth(right); ansi.StringWidth(left) > room {
		left = ansi.Truncate(left, max(0, room), "…")
	}
	gap := max(0, w-ansi.StringWidth(left)-ansi.StringWidth(right))
	if !selected && right == "" {
		return left
	}
	return left + text.Render(strings.Repeat(" ", gap)) + right
}
