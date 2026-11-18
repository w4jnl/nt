package tui

import (
	"path"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/scan"
	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/todo"
	"github.com/w4jnl/nt/internal/vault"
)

// The todos in the note's pane (docs/design.md §4, Todos), as the Mac app's Todos view shows
// them: the open ones, overdue first, then the done ones under Done. The home list's Todos row
// shows them there; enter, or x from anywhere, gives them the keys, the folder list staying as
// it was. j and k move, x checks, d sets the due date, a adds one, e edits the line, D deletes
// it, enter opens its note at it, X folds Done, f keeps to one folder's todos, / keeps
// to the todos with the words typed, esc goes back. A tick holds as in the todo list.

// todoView is the todos in the note's pane while they have the keys.
type todoView struct {
	items   []todo.Item     // as last read, in nt todo's order
	ready   bool            // the todos are read
	sel     int             // the selected row, an index of rows()
	top     int             // the first line drawn
	fold    bool            // Done is folded
	held    map[string]bool // a todo ticked since the list was read → the list it stays in, done or not
	folder  string          // whose todo.md a todo added here goes to; "" is the root's
	scope   string          // the top-level folder whose todos show; "" is every note
	filter  string          // the words typed after /: only the todos with them, in their text or their note's name, show
	clicked bool            // the last click selected a row; a second one opens its note
	full    bool            // the pane was given the whole screen for it (narrow screens)
	pin     map[string]bool // whether a todo was due or in progress when the list was read: it is above the lists
	place   todo.Place      // where the todo being typed goes: after the selected one, or the place chosen with tab
	draft   string          // the todo's text while the places are open
}

// read takes the todos as nt todo orders them and notes which are above the lists: the open
// ones due some day or in progress. A row keeps its place until the list is read again.
func (tv *todoView) read(items []todo.Item) {
	tv.items, tv.ready, tv.held = append([]todo.Item(nil), items...), true, nil
	todo.Sort(tv.items)
	tv.pin = map[string]bool{}
	for i := range tv.items {
		tv.pin[todoRef(&tv.items[i])] = tv.items[i].Pinned()
	}
}

// pinned says a todo is listed above the lists, with where it is at its side.
func (tv *todoView) pinned(it todo.Item) bool {
	if p, ok := tv.pin[todoRef(&it)]; ok {
		return p
	}
	return it.Pinned()
}

// cursorPlace is where a todo added here goes before tab chooses: with the selection on a todo
// under a list's heading, the end of the paragraph it is in; else (on one above the lists or a
// done one, or with none) Inbox in the todo.md of the folder kept to, or the browser's.
func (tv *todoView) cursorPlace() todo.Place {
	if it := tv.selected(); it != nil && !tv.done(*it) && !tv.pinned(*it) {
		return todo.Place{After: it}
	}
	return todo.Place{Folder: tv.addTo()}
}

// todoNote is what to call the note a todo is in when it is no todo.md: its title.
func (b *browser) todoNote(it todo.Item) string {
	if it.IsList() || b.ix == nil {
		return ""
	}
	return b.ix.Title(it.Path)
}

// placeName names a place as the lists' headings do: "work/shell · NA reimport".
func (b *browser) placeName(at todo.Place) string {
	if at.After != nil {
		return at.After.Group(b.todoNote(*at.After))
	}
	list, err := todo.ListIn(b.v, at.Folder)
	if err != nil {
		list = todo.File
	}
	return todo.Item{Path: list, Section: firstNonEmpty(at.Section, todo.Inbox)}.Group("")
}

// addPrompt opens the prompt for a new todo's text, naming where it goes; tab chooses another
// list or section.
func (b *browser) addPrompt(text string) tea.Cmd {
	b.prompt("todo", "todo in "+b.placeName(b.tv.place)+" (tab: elsewhere): ")
	b.input.insert(text)
	return nil
}

// places are the lists and sections a todo can go to: Inbox of the root's todo.md and of the
// folder's the browser is in, then every section of a todo.md that holds a todo, a list's in
// the file's order; each with its open todos.
func (tv *todoView) places() []pickItem {
	type key struct{ folder, section string }
	open, seen := map[key]int{}, map[key]bool{}
	var keys []key
	add := func(k key) {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	add(key{"", todo.Inbox})
	add(key{tv.addTo(), todo.Inbox})
	lists := append([]todo.Item(nil), tv.items...)
	sort.SliceStable(lists, func(i, j int) bool {
		if lists[i].Path != lists[j].Path {
			return pathBefore(lists[i].Path, lists[j].Path)
		}
		return lists[i].Line < lists[j].Line
	})
	for _, it := range lists {
		if !it.IsList() || it.Section == "" {
			continue
		}
		k := key{strings.TrimSuffix(strings.TrimSuffix(it.Path, todo.File), "/"), it.Section}
		add(k)
		if !it.Done {
			open[k]++
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return keys[i].folder != keys[j].folder && pathBefore(keys[i].folder, keys[j].folder)
	})
	var items []pickItem
	for _, k := range keys {
		name := todo.Item{Path: path.Join(k.folder, todo.File), Section: k.section}.Group("")
		items = append(items, pickItem{label: name, hint: strconv.Itoa(open[k]) + " open", kind: "place", target: k.folder, text: k.section})
	}
	return items
}

// placeTargets fills the place picker with the places that match what is typed, and, for a name
// no section of the list has, a row that makes the section there.
func (b *browser) placeTargets() tea.Cmd {
	p, tv := b.pick, b.tv
	if p == nil || tv == nil {
		return nil
	}
	q := strings.Join(strings.Fields(b.input.Value()), " ")
	folder := tv.addTo()
	if it := tv.place.After; it != nil && it.IsList() {
		folder = strings.TrimSuffix(strings.TrimSuffix(it.Path, todo.File), "/")
	} else if it == nil {
		folder = tv.place.Folder
	}
	p.items, p.sel, p.top = nil, 0, 0
	has := false
	for _, it := range tv.places() {
		if _, ok := scan.Match(q, it.label); ok {
			p.items = append(p.items, it)
		}
		has = has || it.target == folder && strings.EqualFold(it.text, q)
	}
	if q != "" && !has {
		name := todo.Item{Path: path.Join(folder, todo.File)}.Group("")
		p.items = append(p.items, pickItem{label: "new section " + strconv.Quote(q) + " in " + name, kind: "place", target: folder, text: q})
	}
	return nil
}

// choosePlace takes the place chosen (keep is false after esc) and goes back to the todo's text.
func (b *browser) choosePlace(keep bool) tea.Cmd {
	tv := b.tv
	if tv == nil {
		b.pick = nil
		return nil
	}
	if p := b.pick; keep && p != nil && p.sel < len(p.items) {
		tv.place = todo.Place{Folder: p.items[p.sel].target, Section: p.items[p.sel].text}
	}
	b.pick = nil
	return b.addPrompt(tv.draft)
}

// done says which list a todo is in: the one it was read in while a tick holds it there.
func (tv *todoView) done(it todo.Item) bool {
	if d, ok := tv.held[todoRef(&it)]; ok {
		return d
	}
	return it.Done
}

// lists are the open todos and the done ones, in the scope.
func (tv *todoView) lists() (open, done []todo.Item) {
	for _, it := range tv.items {
		if tv.scope != "" && !strings.HasPrefix(it.Path, tv.scope+"/") {
			continue
		}
		if !tv.matches(it) {
			continue
		}
		if tv.done(it) {
			done = append(done, it)
		} else {
			open = append(open, it)
		}
	}
	return open, done
}

// matches says whether a todo has the filter's words, in its text or in its note's name, any
// case; every todo does with no filter.
func (tv *todoView) matches(it todo.Item) bool {
	q := strings.ToLower(strings.TrimSpace(tv.filter))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(it.Text), q) || strings.Contains(strings.ToLower(it.Path), q)
}

// setFilter keeps to the todos with the words typed, the selection staying in the rows.
func (tv *todoView) setFilter(q string) {
	tv.filter = q
	if n := len(tv.rows()); tv.sel >= n {
		tv.sel = max(0, n-1)
	}
}

// rows are the todos that can be selected: the open ones, then the done ones unless folded.
func (tv *todoView) rows() []todo.Item {
	open, done := tv.lists()
	if tv.fold {
		return open
	}
	return append(open, done...)
}

// addTo is the folder whose todo.md a todo added here goes to: the scope's, when there is one.
func (tv *todoView) addTo() string {
	if tv.scope != "" {
		return tv.scope
	}
	return tv.folder
}

// topFolder is the top-level folder of a repository path, "" for a file at the root.
func topFolder(rel string) string {
	if i := strings.IndexByte(rel, '/'); i > 0 {
		return rel[:i]
	}
	return ""
}

// scopes are what the todos can keep to: every note, then the folders with open todos, each
// counting everything under it: a top-level folder with any, a deeper one when its own notes
// have some, in nt's folder order; the scope in use stays even with none left.
func (tv *todoView) scopes() []pickItem {
	all, per := 0, map[string]int{}
	own := map[string]bool{}
	for _, it := range tv.items {
		if !tv.done(it) {
			all++
			dir := path.Dir(it.Path)
			if dir != "." {
				own[dir] = true
			}
			for f := dir; f != "." && f != "/"; f = path.Dir(f) {
				per[f]++
			}
		}
	}
	if _, ok := per[tv.scope]; !ok && tv.scope != "" {
		per[tv.scope] = 0
		own[tv.scope] = true
	}
	folders := make([]string, 0, len(per))
	for f := range per {
		if own[f] || !strings.Contains(f, "/") {
			folders = append(folders, f)
		}
	}
	sort.Slice(folders, func(i, j int) bool { return pathBefore(folders[i], folders[j]) })
	items := []pickItem{{label: "every note", hint: strconv.Itoa(all) + " open", kind: "scope"}}
	for _, f := range folders {
		items = append(items, pickItem{label: f + "/", hint: strconv.Itoa(per[f]) + " open", kind: "scope", target: f})
	}
	return items
}

// pathBefore orders folder paths as nt lists folders, part by part, a folder before what is
// under it.
func pathBefore(a, b string) bool {
	pa, pb := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			return vault.FolderBefore(pa[i], pb[i])
		}
	}
	return len(pa) < len(pb)
}

// scopeTargets fills the scope picker with the scopes whose names match what is typed.
func (b *browser) scopeTargets() tea.Cmd {
	p, tv := b.pick, b.tv
	if p == nil || tv == nil {
		return nil
	}
	q := strings.TrimSpace(b.input.Value())
	p.items, p.sel, p.top = nil, 0, 0
	for _, it := range tv.scopes() {
		if _, ok := scan.Match(q, it.label); ok {
			if it.target == tv.scope {
				p.sel = len(p.items)
			}
			p.items = append(p.items, it)
		}
	}
	if q != "" {
		p.sel = 0
	}
	return nil
}

// setScope keeps the todos to a folder's, at any depth, or "" every note's.
func (b *browser) setScope(folder string) {
	tv := b.tv
	tv.scope, tv.sel, tv.top = folder, 0, 0
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
			b.tv.read(b.todoItems)
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
	tv.read(items)
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
	case "f":
		b.pick = &picker{title: "todos from"}
		b.prompt("scope", "todos from: ")
		return b.scopeTargets(), true
	case "/":
		return b.prompt("todos", "todos: "), true
	case "a":
		tv.place = tv.cursorPlace()
		if _, err := todo.ListIn(b.v, tv.place.Folder); err != nil {
			b.say(err.Error(), true)
			return nil, true
		}
		return b.addPrompt(""), true
	case "r":
		keep := ""
		if it != nil {
			keep = todoRef(it)
		}
		return b.loadTodos(keep), true
	case "x", "space", "i", "d", "e", "D", "enter", "right", "l":
		if it == nil {
			return nil, true
		}
		c := *it
		switch s {
		case "x", "space":
			return b.toggleTodo(c), true
		case "i":
			return b.startTodo(c), true
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
	head string // a list and section, above its todos
}

func (b *browser) todoViewLines(tv *todoView, active bool) []todoViewLine {
	open, done := tv.lists()
	lines := []todoViewLine{{row: -1}, {row: -1, text: "summary"}, {row: -1}}
	if !tv.ready {
		return lines
	}
	// the ones due or in progress, then the rest under their list and section as the file has
	// them, with a gap where it has a blank line between two
	var prev *todo.Item
	for i, it := range open {
		if !tv.pinned(it) {
			switch {
			case prev == nil || !prev.SameGroup(it):
				if i > 0 {
					lines = append(lines, todoViewLine{row: -1})
				}
				lines = append(lines, todoViewLine{row: -1, head: it.Group(b.todoNote(it))})
			case prev.Para != it.Para:
				lines = append(lines, todoViewLine{row: -1})
			}
			prev = &open[i]
		}
		lines = append(lines, todoViewLine{row: i})
	}
	if len(open) == 0 && tv.filter != "" {
		lines = append(lines, todoViewLine{row: -1, text: "no open todo matches"})
	} else if len(open) == 0 {
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
		lines = append(lines, todoViewLine{row: -1}, todoViewLine{row: -1, text: "enter, then: x checks, i starts, d due date, a adds, enter opens the note"})
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
			from := ""
			if tv.done(it) || tv.pinned(it) {
				from = it.From(b.todoNote(it))
			}
			out = append(out, " "+b.todoRow(it, active && l.row == tv.sel, w-1, from))
		case l.head != "":
			out = append(out, "  "+b.st.Muted.Render(ansi.Truncate(l.head, max(0, w-3), "…")))
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

// todoSummary is the line above the todos: how many are open, how many of them in progress,
// the late ones in warn.
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
	doing := 0
	for _, it := range open {
		if it.InProgress() {
			doing++
		}
	}
	s := b.st.Muted.Render(plural(len(open), "open todo"))
	if doing > 0 {
		s += b.st.Muted.Render(" · ") + lipgloss.NewStyle().Foreground(b.pal.Color(theme.InProgress)).Render(strconv.Itoa(doing)+" in progress")
	}
	if late > 0 {
		s += b.st.Muted.Render(" · ") + lipgloss.NewStyle().Foreground(b.pal.Warn).Render(strconv.Itoa(late)+" late")
	}
	if tv.scope != "" {
		s += b.st.Muted.Render(" · from " + tv.scope + "/")
	}
	if tv.filter != "" {
		s += b.st.Muted.Render(" · matching " + strconv.Quote(tv.filter))
	}
	return s
}

// homeTodos is the view the home list's Todos row shows, before it has the keys.
func (b *browser) homeTodos() *todoView {
	tv := &todoView{sel: -1}
	if b.todoKnown {
		tv.read(b.todoItems)
	}
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
