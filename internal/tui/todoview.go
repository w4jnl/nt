package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/todo"
)

// The todos in the note's pane (docs/design.md §4, Todos), as the Mac app's Todos view shows
// them: the open ones, overdue first, then the done ones under Done. The home list's Todos row
// shows them there; enter, or x from anywhere, gives them the keys, the folder list staying as
// it was. j and k move, x checks, d sets the due date, a adds one, e edits the line, D deletes
// it, enter opens its note at it, X folds Done, esc goes back. A tick holds as in the todo list.

// todoView is the todos in the note's pane while they have the keys.
type todoView struct {
	items   []todo.Item     // as last read, in nt todo's order
	ready   bool            // the todos are read
	sel     int             // the selected row, an index of rows()
	top     int             // the first line drawn
	fold    bool            // Done is folded
	held    map[string]bool // a todo ticked since the list was read → the list it stays in, done or not
	folder  string          // whose todo.md a todo added here goes to; "" is the root's
	clicked bool            // the last click selected a row; a second one opens its note
	full    bool            // the pane was given the whole screen for it (narrow screens)
}

// done says which list a todo is in: the one it was read in while a tick holds it there.
func (tv *todoView) done(it todo.Item) bool {
	if d, ok := tv.held[todoRef(&it)]; ok {
		return d
	}
	return it.Done
}

// lists are the open todos and the done ones.
func (tv *todoView) lists() (open, done []todo.Item) {
	for _, it := range tv.items {
		if tv.done(it) {
			done = append(done, it)
		} else {
			open = append(open, it)
		}
	}
	return open, done
}

// rows are the todos that can be selected: the open ones, then the done ones unless folded.
func (tv *todoView) rows() []todo.Item {
	open, done := tv.lists()
	if tv.fold {
		return open
	}
	return append(open, done...)
}

func (tv *todoView) selected() *todo.Item {
	rows := tv.rows()
	if tv.sel < 0 || tv.sel >= len(rows) {
		return nil
	}
	it := rows[tv.sel]
	return &it
}

// openTodoView gives the todos the keys in the note's pane.
func (b *browser) openTodoView() tea.Cmd {
	if b.tv == nil {
		b.tv = &todoView{folder: b.plainFolder()}
		if b.todoKnown {
			b.tv.items, b.tv.ready = append([]todo.Item(nil), b.todoItems...), true
		}
		if b.narrow() && !b.full {
			b.full, b.tv.full = true, true
			b.layout()
		}
	}
	b.focus = onPreview
	b.say("", false)
	return b.loadTodos("")
}

func (b *browser) closeTodoView() tea.Cmd {
	if b.tv.full {
		b.full = false
		b.layout()
	}
	b.tv = nil
	b.focus = onList
	return b.loadNow()
}

// setTodoView puts the todos read in the view, keep selected (path:line) when it is there, else
// the row in the selected one's place.
func (b *browser) setTodoView(items []todo.Item, keep string) {
	tv := b.tv
	tv.items, tv.ready, tv.held = append([]todo.Item(nil), items...), true, nil
	todo.Sort(tv.items)
	rows := tv.rows()
	tv.sel = max(0, min(tv.sel, len(rows)-1))
	for i := range rows {
		if todoRef(&rows[i]) == keep {
			tv.sel = i
		}
	}
}

func (b *browser) todoViewMove(d int) {
	tv := b.tv
	tv.sel = max(0, min(len(tv.rows())-1, tv.sel+d))
	tv.clicked = false
}

// todoViewKey handles the keys while the todos have them and reports whether k was one of them:
// all are but help, sync and quit, so no key acts on the folder list behind them.
func (b *browser) todoViewKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	tv := b.tv
	it := tv.selected()
	switch s := k.String(); s {
	case "?", "s", "ctrl+c":
		return nil, false
	case "j", "down":
		b.todoViewMove(1)
	case "k", "up":
		b.todoViewMove(-1)
	case "g", "home":
		b.todoViewMove(-len(tv.items))
	case "G", "end":
		b.todoViewMove(len(tv.items))
	case "X":
		keep := ""
		if it != nil {
			keep = todoRef(it)
		}
		tv.fold = !tv.fold
		tv.sel = 0
		for i, r := range tv.rows() {
			if todoRef(&r) == keep {
				tv.sel = i
			}
		}
	case "a":
		list, err := todo.ListIn(b.v, tv.folder)
		if err != nil {
			b.say(err.Error(), true)
			return nil, true
		}
		return b.prompt("todo", "todo in "+list+": "), true
	case "r":
		keep := ""
		if it != nil {
			keep = todoRef(it)
		}
		return b.loadTodos(keep), true
	case "x", "space", "d", "e", "D", "enter", "right", "l":
		if it == nil {
			return nil, true
		}
		c := *it
		switch s {
		case "x", "space":
			return b.toggleTodo(c), true
		case "d":
			if tv.done(c) {
				return nil, true
			}
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
		return b.todoViewOpen(c), true
	case "esc", "q", "h", "left", "tab":
		return b.closeTodoView(), true
	}
	return nil, true
}

// todoViewOpen opens the note a todo is in, at the todo.
func (b *browser) todoViewOpen(it todo.Item) tea.Cmd {
	b.closeTodoView()
	cmd := b.reveal(it.Path)
	b.scrollTo(pickItem{line: it.Line, todo: &it})
	b.focus = onPreview
	return cmd
}

// todoViewLine is one line of the pane: a todo (an index of rows()), Done's heading, or text.
type todoViewLine struct {
	row  int // -1 when the line is no todo
	done bool
	text string
}

func (b *browser) todoViewLines(tv *todoView, active bool) []todoViewLine {
	open, done := tv.lists()
	lines := []todoViewLine{{row: -1}, {row: -1, text: "summary"}, {row: -1}}
	if !tv.ready {
		return lines
	}
	for i := range open {
		lines = append(lines, todoViewLine{row: i})
	}
	if len(open) == 0 {
		lines = append(lines, todoViewLine{row: -1, text: "nothing open · a adds one"})
	}
	if len(done) > 0 {
		lines = append(lines, todoViewLine{row: -1}, todoViewLine{row: -1, done: true})
		if !tv.fold {
			for i := range done {
				lines = append(lines, todoViewLine{row: len(open) + i})
			}
		}
	}
	if !active {
		lines = append(lines, todoViewLine{row: -1}, todoViewLine{row: -1, text: "enter, then: x checks, d due date, a adds, enter opens the note"})
	}
	return lines
}

// todoViewPane draws the todos in the note's pane; active when they have the keys, else as the
// home list's Todos row shows them.
func (b *browser) todoViewPane(tv *todoView, w, h int, active bool) string {
	lines := b.todoViewLines(tv, active)
	if active {
		at := 0
		for i, l := range lines {
			if l.row == tv.sel {
				at = i
			}
		}
		if at < tv.top+1 {
			tv.top = max(0, at-3) // the summary stays in view at the top
		}
		if h > 0 && at >= tv.top+h {
			tv.top = at - h + 1
		}
	} else {
		tv.top = 0
	}
	rows := tv.rows()
	_, done := tv.lists()
	var out []string
	for i := tv.top; i < len(lines) && len(out) < h; i++ {
		l := lines[i]
		switch {
		case l.row >= 0 && l.row < len(rows):
			it := rows[l.row]
			out = append(out, " "+b.todoRow(it, active && l.row == tv.sel, w-1, b.todoFrom(it)))
		case l.done:
			mark := "▾"
			if tv.fold {
				mark = "▸"
			}
			out = append(out, "  "+b.st.Muted.Render(mark+" Done ")+b.st.Faint.Render(strconv.Itoa(len(done))))
		case l.text == "summary":
			out = append(out, "  "+b.todoSummary(tv))
		case l.text != "":
			out = append(out, "  "+b.st.Faint.Render(l.text))
		default:
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}

// todoSummary is the line above the todos: how many are open, the late ones in warn.
func (b *browser) todoSummary(tv *todoView) string {
	if !tv.ready {
		return b.st.Muted.Render("reading the todos")
	}
	open, _ := tv.lists()
	late := 0
	today := b.v.Day(b.v.Now())
	for _, it := range open {
		if _, l := it.Word(today); l {
			late++
		}
	}
	s := b.st.Muted.Render(plural(len(open), "open todo"))
	if late > 0 {
		s += b.st.Muted.Render(" · ") + lipgloss.NewStyle().Foreground(b.pal.Warn).Render(strconv.Itoa(late)+" late")
	}
	return s
}

// homeTodos is the view the home list's Todos row shows, before it has the keys.
func (b *browser) homeTodos() *todoView {
	tv := &todoView{items: b.todoItems, ready: b.todoKnown, sel: -1}
	return tv
}

// todoViewClick selects the todo on the pane's row; a click on its ○ or ✓ checks it, a second
// click on it opens its note.
func (b *browser) todoViewClick(row, x int) tea.Cmd {
	tv := b.tv
	lines := b.todoViewLines(tv, true)
	i := tv.top + row
	if i < 0 || i >= len(lines) {
		return nil
	}
	l := lines[i]
	if l.done {
		tv.fold = !tv.fold
		return nil
	}
	if l.row < 0 {
		return nil
	}
	if l.row == tv.sel && tv.clicked && x > 4 {
		if it := tv.selected(); it != nil {
			return b.todoViewOpen(*it)
		}
	}
	tv.sel, tv.clicked = l.row, true
	if x <= 4 { // the glyph
		if it := tv.selected(); it != nil {
			tv.clicked = false
			return b.toggleTodo(*it)
		}
	}
	return nil
}
