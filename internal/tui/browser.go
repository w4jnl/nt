package tui

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/clip"
	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/editor"
	"github.com/w4jnl/nt/internal/gitsync"
	"github.com/w4jnl/nt/internal/mdparse"
	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/scan"
	"github.com/w4jnl/nt/internal/search"
	"github.com/w4jnl/nt/internal/termimg"
	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/todo"
	"github.com/w4jnl/nt/internal/vault"
	"github.com/w4jnl/nt/internal/weekly"
)

// Layout from tokens.json terminal/*: the list is list-width columns until the person drags the
// divider or presses < >, at least list-min; under list-min + preview-min columns the preview
// hides and Enter shows the note on its own. list-step is how far < > move the divider.
const (
	listDefault = 44
	listMin     = 28
	listStep    = 4
	previewMin  = 40
)

// BrowserOptions configures the browser.
type BrowserOptions struct {
	Vault    *vault.Vault
	Machine  config.Machine
	Folder   string // where to start, a repository path; "" is the root
	Bin      string // the nt binary, for the image viewer over ssh
	Kick     func() // starts a background sync after the editor closes
	StateDir string
	Version  string // nt's version, at the right end of the footer
}

// RunBrowser shows the browser until the person quits.
func RunBrowser(o BrowserOptions) error {
	m := newBrowser(o)
	_, err := tea.NewProgram(m).Run()
	_, _ = os.Stdout.WriteString(m.p.clear())
	if m.listW != m.savedW {
		saveListWidth(o.StateDir, m.listW)
	}
	return err
}

// browserState is what the browser remembers between runs, in the machine's state folder: UI
// state never goes into the notes repository.
type browserState struct {
	ListWidth int `json:"list_width"`
}

func browserStatePath(stateDir string) string { return filepath.Join(stateDir, "browser.json") }

func loadListWidth(stateDir string) int {
	var s browserState
	if b, err := os.ReadFile(browserStatePath(stateDir)); err == nil && json.Unmarshal(b, &s) == nil && s.ListWidth >= listMin {
		return s.ListWidth
	}
	return listDefault
}

func saveListWidth(stateDir string, w int) {
	if stateDir == "" {
		return
	}
	b, _ := json.MarshalIndent(browserState{ListWidth: w}, "", "  ")
	_ = vault.WriteFile(browserStatePath(stateDir), append(b, '\n'))
}

type focus int

const (
	onList focus = iota
	onPreview
)

// entry is one row of the list: a folder or a note; at the root also a view, a pinned folder or
// the rule between them (home).
type entry struct {
	name, rel string
	dir       bool
	date      string // YYYY-MM-DD from the file name
	host      string // the machine an agent report is from: <host>--… in its name
	title     string
	journal   bool
	started   time.Time // when an agent report's session began, its created

	view string // today, todos, calendar, weekly or agents: a row of the home list
	pin  bool   // a pinned folder in the home list
	key  string // the key that does what the row does, at its right
	rule bool   // the line between the home list's groups, never selected
}

// id tells rows apart: a view or a pinned folder by what it is, a folder or a note by its path.
func (e entry) id() string {
	switch {
	case e.view != "":
		return "view:" + e.view
	case e.pin:
		return "pin:" + e.rel
	}
	return e.rel
}

// pickItem is one row of a picker; what choosing it does depends on kind.
type pickItem struct {
	label, hint string
	kind        string // note, url, image, heading
	target      string // repository path, URL or file
	line        int
	text        string     // a heading or matched text to scroll to
	todo        *todo.Item // a row of the todo list
}

// question is asked on the footer line: y does it, any other key leaves things as they are.
type question struct {
	text string
	yes  func() tea.Cmd
}

type picker struct {
	title  string
	items  []pickItem
	sel    int
	top    int
	todos  bool   // the todo list (todos.go)
	done   bool   // its done todos, not the open ones
	ready  bool   // its todos are read
	folder string // whose todo.md a todo added there goes to; "" is the root's

	folders []string // the move picker's folders, before what is typed filters them
	day     string   // the todo list from the calendar: only the todos due or done that day
}

type browser struct {
	o     BrowserOptions
	v     *vault.Vault
	known bool
	dark  bool
	pal   theme.Palette
	st    theme.Styles

	width, height int
	listW, savedW int  // the list width asked for, and as read at start
	dragging      bool // the divider is held with the mouse
	selecting     bool // the mouse is down on the note, selecting text

	dir     string
	entries []entry
	dirMod  time.Time
	sel     int
	top     int
	focus   focus
	full    bool // narrow screen: the preview shown on its own

	p       *preview
	showing string // repository path in the preview
	loadSeq int

	ix       *scan.Index
	indexSeq int

	pick       *picker
	input      *lineInput
	inputKind  string // add, new, search, find, due, todo, move, rename, mkdir
	searchSeq  int
	searchStop context.CancelFunc

	help bool
	ask  *question // a yes-or-no question on the footer line

	todoSeq int
	todoAt  *todo.Item // the todo the due prompt is for

	todoOpen, todoLate int         // the open todos and the late ones, for the home list
	todoKnown          bool        // they have been counted
	todoItems          []todo.Item // every todo, overdue first, as last read
	homeCal            *calendar   // today's month, while the home list's Calendar is selected

	moving string // the note or folder m or R was pressed on

	cal *calendar     // the calendar, in the list pane's place (calendar.go)
	set *settingsView // the settings, in the note pane's place (settings.go)

	history []visit // the notes links were followed from, for b (follow.go)
	hinted  string  // the letters typed after f so far

	status    gitsync.Status
	hasStatus bool
	statusMod time.Time
	syncing   bool
	frame     int

	editing func() // releases the edit hold while the editor runs
	msg     string
	alert   bool
}

type (
	tickMsg        struct{}
	spinMsg        struct{}
	previewMsg     struct{ seq int }
	pickPreviewMsg struct{ seq int }
	indexMsg       struct {
		seq   int
		ix    *scan.Index
		todos []todo.Item
	}
	searchMsg struct {
		seq     int
		matches []search.Match
		err     error
	}
	syncedMsg struct{ st gitsync.Status }
	todosMsg  struct {
		seq   int
		keep  string
		items []todo.Item
	}
	notedMsg struct {
		rel string
		err error
	}
)

func newBrowser(o BrowserOptions) *browser {
	mode := o.Machine.Theme.Mode
	img, why := termimg.Detect(o.Machine.InlineImages)
	b := &browser{o: o, v: o.Vault, known: mode == "dark" || mode == "light", msg: why, alert: why != ""}
	b.listW = loadListWidth(o.StateDir)
	b.savedW = b.listW
	b.p = newPreview(img, mode != "light")
	b.p.expected = func(file string) bool { return b.v.Dated(b.v.RelPath(file)) }
	b.setTheme(mode != "light")
	b.dir = strings.Trim(o.Folder, "/")
	if st, err := os.Stat(b.v.Abs(b.dir)); err != nil || !st.IsDir() {
		b.dir = ""
	}
	b.list()
	b.status, b.hasStatus = gitsync.ReadStatus(o.StateDir)
	b.statusMod = modTime(gitsync.StatusPath(o.StateDir))
	return b
}

func (b *browser) setTheme(dark bool) {
	b.dark = dark
	b.pal = theme.New(dark)
	b.st = b.pal.Styles()
	b.p.setTheme(dark)
}

func (b *browser) Init() tea.Cmd {
	cmds := []tea.Cmd{b.tick(), b.reindex(), b.loadNow()}
	if !b.known {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	if b.o.Machine.Sync.Auto {
		cmds = append(cmds, b.sync()) // the browser starting is one of sync's triggers
	}
	return tea.Batch(cmds...)
}

func (b *browser) tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

// list reads the current folder: folders first, then notes, newest first (in the agent reports,
// a day's digest above them). Hidden names, assets folders and files other than notes are left
// out. The root starts with the home list.
func (b *browser) list() {
	keep := ""
	if e, ok := b.selected(); ok {
		keep = e.id()
	}
	b.entries = listFolder(b.v, b.dir)
	if b.dir == "" {
		b.entries = append(b.home(), b.entries...)
	}
	b.dirMod = modTime(b.v.Abs(b.dir))
	b.sel = 0
	for i, e := range b.entries {
		if keep != "" && e.id() == keep {
			b.sel = i
		}
	}
	b.clampList()
}

// home is the top of the root's list (docs/design.md §4): the views, then the pinned folders,
// each with the key that does the same, and a rule after each group.
func (b *browser) home() []entry {
	es := []entry{
		{name: "Today", view: "today", key: "t"},
		{name: "Todos", view: "todos", key: "x"},
		{name: "Calendar", view: "calendar", key: "c"},
		{name: "Weekly", view: "weekly", key: "w"},
		{name: "Agents", view: "agents", key: "A"},
		{rule: true},
	}
	pinned := false
	for i, p := range b.v.Cfg.Pins {
		rel := strings.Trim(p, "/")
		if i >= 9 || !isDir(b.v.Abs(rel)) {
			continue
		}
		es = append(es, entry{name: rel, rel: rel, dir: true, pin: true, key: strconv.Itoa(i + 1)})
		pinned = true
	}
	if pinned {
		es = append(es, entry{rule: true})
	}
	return es
}

// viewNote is the note a view's row shows while it is selected: today's journal, the newest
// weekly digest, today's agent digest; "" when there is none, or for the todos and the calendar.
func (b *browser) viewNote(view string) string {
	today := b.v.Day(b.v.Now())
	rel := ""
	switch view {
	case "today":
		rel = b.v.JournalRel(today)
	case "weekly":
		rel, _ = weekly.Latest(b.v)
	case "agents":
		rel = b.v.AgentsRel(today) + "/README.md"
	}
	if rel == "" || !isFile(b.v.Abs(rel)) {
		return ""
	}
	return rel
}

// openView does what a view's row stands for, as its key does.
func (b *browser) openView(view string) tea.Cmd {
	switch view {
	case "today":
		return b.today()
	case "todos":
		return b.openTodos(false)
	case "calendar":
		return b.openCalendar()
	case "weekly":
		return b.openWeekly()
	case "agents":
		return b.agents()
	}
	return nil
}

// count keeps the todos read for the home list: how many are open, how many of those are late,
// and the todos themselves for its Todos and Calendar.
func (b *browser) count(items []todo.Item) {
	today := b.v.Day(b.v.Now())
	b.todoItems = append([]todo.Item(nil), items...)
	todo.Sort(b.todoItems)
	if b.homeCal != nil {
		b.homeCal.setTodos(b.todoItems)
	}
	b.todoOpen, b.todoLate, b.todoKnown = 0, 0, true
	for _, it := range items {
		if it.Done {
			continue
		}
		b.todoOpen++
		if _, late := it.Word(today); late {
			b.todoLate++
		}
	}
}

var datePrefix = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})`)

// reportHost is the host of an agent report's name, <host>--<agent>--<project>--<sid8>.md.
var reportHost = regexp.MustCompile(`^([a-z0-9]+(?:-[a-z0-9]+)*)--[a-z0-9-]+--[a-z0-9-]+--[a-z0-9-]+\.md$`)

func listFolder(v *vault.Vault, dir string) []entry {
	des, err := os.ReadDir(v.Abs(dir))
	if err != nil {
		return nil
	}
	var dirs, notes []entry
	inDaily := dir == v.Cfg.Folders.Daily || strings.HasPrefix(dir, v.Cfg.Folders.Daily+"/")
	inAgents := dir == v.Cfg.Folders.Agents || strings.HasPrefix(dir, v.Cfg.Folders.Agents+"/")
	for _, d := range des {
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		rel := path.Join(dir, name)
		if d.IsDir() {
			if name != "assets" {
				dirs = append(dirs, entry{name: name, rel: rel, dir: true})
			}
			continue
		}
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		head := readHead(v.Abs(rel))
		e := entry{name: name, rel: rel, title: render.Title(head, v.Abs(rel))}
		if m := datePrefix.FindStringSubmatch(name); m != nil {
			e.date = m[1]
			e.journal = inDaily && name == m[1]+".md"
		}
		if m := reportHost.FindStringSubmatch(name); m != nil && inAgents {
			e.host = m[1]
			e.started = created(head)
		}
		notes = append(notes, e)
	}
	sort.Slice(dirs, func(i, j int) bool { return vault.FolderBefore(dirs[i].name, dirs[j].name) })
	sort.Slice(notes, func(i, j int) bool {
		a, b := notes[i], notes[j]
		switch {
		case inAgents && (a.name == "README.md") != (b.name == "README.md"):
			return a.name == "README.md" // the day's digest above its reports
		case !a.started.Equal(b.started):
			return a.started.After(b.started) // the latest session first
		}
		return a.name > b.name
	})
	return append(dirs, notes...)
}

// readHead is the first 4 KB of a note: enough for its frontmatter and its title.
func readHead(p string) []byte {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := f.Read(head)
	return head[:n]
}

func titleOf(p string) string { return render.Title(readHead(p), p) }

// created is the time in a note's frontmatter created key; zero when there is none.
func created(head []byte) time.Time {
	t, _ := time.Parse(time.RFC3339, render.Field(head, "created"))
	return t
}

func (b *browser) selected() (entry, bool) {
	if b.sel < 0 || b.sel >= len(b.entries) {
		return entry{}, false
	}
	return b.entries[b.sel], true
}

// move changes the selection and loads the preview a moment later, so holding j does not render
// every note on the way.
func (b *browser) move(d int) tea.Cmd {
	if len(b.entries) == 0 {
		return nil
	}
	b.sel = max(0, min(len(b.entries)-1, b.sel+d))
	b.offRule(d)
	b.clampList()
	b.loadSeq++
	seq := b.loadSeq
	return tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg { return previewMsg{seq} })
}

// offRule moves the selection off a rule, on in the direction d it was going, else back.
func (b *browser) offRule(d int) {
	step := 1
	if d < 0 {
		step = -1
	}
	for n := 0; n < len(b.entries) && b.entries[b.sel].rule; n++ {
		if next := b.sel + step; next >= 0 && next < len(b.entries) {
			b.sel = next
		} else {
			step = -step
		}
	}
}

// loadNow shows the selected note in the preview, or for a view's row the note it stands for.
func (b *browser) loadNow() tea.Cmd {
	if b.cal != nil {
		return b.calShow()
	}
	e, ok := b.selected()
	b.homeCal = nil
	if ok && e.view == "calendar" {
		b.homeCal = b.homeMonth()
	}
	if ok && e.view != "" {
		if rel := b.viewNote(e.view); rel != "" {
			return b.show(rel)
		}
	}
	if !ok || e.dir || e.view != "" {
		b.showing = ""
		b.p.open("", "")
		b.p.vp.SetContent("")
		return b.p.sendImages()
	}
	return b.show(e.rel)
}

// show puts the note at rel in the preview, whatever the list has selected.
func (b *browser) show(rel string) tea.Cmd {
	b.showing = rel
	b.p.open(b.v.Abs(rel), path.Dir(rel))
	b.p.extra = b.linkedFrom(rel)
	b.p.render()
	return b.p.sendImages()
}

// followPick shows what the highlighted picker row points at: its note, at its line, a moment
// later; a heading of the note on screen at once.
func (b *browser) followPick() tea.Cmd {
	if b.pick == nil || b.pick.sel >= len(b.pick.items) {
		return nil
	}
	it := b.pick.items[b.pick.sel]
	switch it.kind {
	case "heading":
		if !b.p.showText(it.text) {
			b.p.showLine(it.line)
		}
	case "note":
		b.loadSeq++
		seq := b.loadSeq
		return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return pickPreviewMsg{seq} })
	}
	return nil
}

// closePick closes the picker and gives the preview back to the list's selection, or to the
// calendar's day.
func (b *browser) closePick() tea.Cmd {
	b.pick = nil
	if b.cal != nil {
		return tea.Batch(b.loadNow(), b.calTodos())
	}
	return b.loadNow()
}

// linkedFrom is the "Linked from" section shown after a note: the notes that link to it.
func (b *browser) linkedFrom(rel string) string {
	if b.ix == nil || len(b.ix.Back[rel]) == 0 {
		return ""
	}
	var s strings.Builder
	s.WriteString("\n\n  " + b.st.Rule.Render(strings.Repeat("─", 24)) + "\n")
	s.WriteString("  " + b.st.Muted.Render("Linked from") + "\n")
	for _, from := range b.ix.Back[rel] { // hyperlinks, like the note's own links, so they follow
		link := vault.RelLink(rel, from)
		s.WriteString("  " + ansi.SetHyperlink(link) + b.st.Text.Render("← "+firstNonEmpty(b.ix.Title(from), from)) + ansi.ResetHyperlink() + "  " +
			ansi.SetHyperlink(link) + b.st.Faint.Render(from) + ansi.ResetHyperlink() + "\n")
	}
	return s.String()
}

func (b *browser) clampList() {
	h := b.bodyHeight()
	if b.pick != nil {
		h-- // the picker's title row
	}
	if b.sel < b.top {
		b.top = b.sel
	}
	if h > 0 && b.sel >= b.top+h {
		b.top = b.sel - h + 1
	}
	b.top = max(0, min(b.top, max(0, len(b.entries)-h)))
}

// open goes into the folder at rel.
func (b *browser) openFolder(rel string) tea.Cmd {
	b.dir = strings.Trim(rel, "/")
	b.sel, b.top = 0, 0
	b.list()
	return b.loadNow()
}

// reveal goes to the folder of the note at rel, selects it and shows it.
func (b *browser) reveal(rel string) tea.Cmd {
	dir := path.Dir(rel)
	if dir == "." {
		dir = ""
	}
	if dir != b.dir || len(b.entries) == 0 {
		b.dir = dir
		b.sel, b.top = 0, 0
		b.list()
	} else {
		b.list()
	}
	for i, e := range b.entries {
		if e.rel == rel {
			b.sel = i
		}
	}
	b.clampList()
	return b.loadNow()
}

func (b *browser) up() tea.Cmd {
	if b.dir == "" {
		return nil
	}
	from := b.dir
	b.dir = path.Dir(b.dir)
	if b.dir == "." {
		b.dir = ""
	}
	b.sel, b.top = 0, 0
	b.list()
	for i, e := range b.entries {
		if e.rel == from {
			b.sel = i
		}
	}
	b.clampList()
	return b.loadNow()
}

func (b *browser) say(msg string, alert bool) { b.msg, b.alert = msg, alert }

func (b *browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		if !b.known {
			b.known = true
			b.setTheme(msg.IsDark())
			b.p.extra = b.linkedFrom(b.showing)
			b.p.render()
		}
		return b, b.p.sendImages()
	case tea.WindowSizeMsg:
		b.width, b.height = msg.Width, msg.Height
		b.layout()
		b.p.render()
		return b, b.p.sendImages()
	case tickMsg:
		return b, tea.Batch(b.tick(), b.poll())
	case spinMsg:
		if !b.syncing {
			return b, nil
		}
		b.frame++
		return b, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return spinMsg{} })
	case previewMsg:
		if msg.seq != b.loadSeq {
			return b, nil
		}
		return b, b.loadNow()
	case pickPreviewMsg:
		if msg.seq != b.loadSeq || b.pick == nil || b.pick.sel >= len(b.pick.items) {
			return b, nil
		}
		it := b.pick.items[b.pick.sel]
		if _, err := os.Stat(b.v.Abs(it.target)); err != nil {
			return b, nil
		}
		cmd := b.show(it.target)
		b.scrollTo(it)
		return b, cmd
	case indexMsg:
		if msg.seq != b.indexSeq {
			return b, nil
		}
		b.ix = msg.ix
		b.count(msg.todos)
		if b.showing != "" {
			b.p.extra = b.linkedFrom(b.showing)
			b.p.render()
		}
		if b.pick != nil && b.inputKind == "find" {
			return b, tea.Batch(b.findTitles(), b.p.sendImages())
		}
		return b, b.p.sendImages()
	case searchMsg:
		return b, b.searched(msg)
	case todosMsg:
		return b, b.setTodos(msg)
	case movedMsg:
		return b, b.moved(msg)
	case calTodosMsg:
		b.setCalTodos(msg)
		return b, nil
	case calPreviewMsg:
		if msg.seq != b.loadSeq || b.cal == nil {
			return b, nil
		}
		return b, b.calShow()
	case syncedMsg:
		return b, b.synced(msg.st)
	case editedMsg:
		if b.editing != nil {
			b.editing()
			b.editing = nil
		}
		if msg.err != nil {
			b.say("editor: "+msg.err.Error(), true)
		}
		if b.o.Kick != nil {
			b.o.Kick()
		}
		b.p.forget()
		b.list()
		if b.pick != nil && b.pick.todos {
			keep := ""
			if it := b.todoSelected(); it != nil {
				keep = todoRef(it)
			}
			return b, tea.Batch(b.loadTodos(keep), b.reindex())
		}
		if s, ok := b.settingSelected(); ok { // a config file edited: read it again
			return b, b.openSettings(s.Key)
		}
		if b.cal != nil {
			b.calRead()
			return b, tea.Batch(b.loadNow(), b.calTodos(), b.reindex())
		}
		return b, tea.Batch(b.loadNow(), b.reindex())
	case notedMsg:
		if msg.err != nil {
			b.say(msg.err.Error(), true)
			return b, nil
		}
		cmd := b.reveal(msg.rel)
		return b, tea.Batch(cmd, b.edit(msg.rel))
	case imagesLostMsg:
		return b, b.p.resend(msg.try)
	case tea.PasteMsg:
		if b.input != nil {
			b.input.paste(msg.Content)
			return b, b.inputChanged()
		}
		return b, nil
	case tea.KeyPressMsg:
		b.msg = ""
		if q := b.ask; q != nil {
			b.ask = nil
			if msg.String() == "y" {
				return b, q.yes()
			}
			return b, nil
		}
		if b.input != nil {
			return b, b.inputKey(msg)
		}
		if b.p.hints != nil {
			return b, b.hintKey(msg)
		}
		if b.set != nil {
			if cmd, ok := b.settingsKey(msg); ok {
				return b, cmd
			}
		}
		if b.pick != nil {
			return b, b.pickKey(msg)
		}
		if b.cal != nil && b.focus == onList {
			if cmd, ok := b.calKey(msg); ok {
				return b, cmd
			}
		}
		return b, b.key(msg)
	case tea.MouseMsg:
		return b, b.mouse(msg)
	}
	return b, nil
}

// mouse: dragging the divider resizes the list, dragging over the note selects its text (the
// terminal's own selection would run across both panes), the wheel scrolls the pane under the
// pointer, a click selects a row (a second click opens it) or gives the note the focus.
func (b *browser) mouse(msg tea.MouseMsg) tea.Cmd {
	m := msg.Mouse()
	lw, split := b.listWidth(), !b.full && !b.narrow()
	onNote := b.full || split && m.X > lw+1
	row := m.Y - 2 // under the brand line and the rule
	switch msg.(type) {
	case tea.MouseMotionMsg:
		switch {
		case b.dragging:
			b.resize(m.X)
		case b.selecting:
			b.p.mark.head = b.notePoint(m.X, m.Y)
		}
		return nil
	case tea.MouseReleaseMsg:
		switch {
		case b.dragging:
			b.dragging = false
			return b.p.sendImages()
		case b.selecting:
			b.selecting = false
			if b.p.mark.empty() { // a click, not a drag: on a link, it follows it
				at := b.p.mark.anchor
				b.p.mark = nil
				if s, ok := b.p.linkAt(at); ok {
					return b.follow(s.dest)
				}
			}
		}
		return nil
	case tea.MouseWheelMsg:
		d := map[tea.MouseButton]int{tea.MouseWheelDown: 1, tea.MouseWheelUp: -1}[m.Button]
		switch {
		case d == 0:
		case onNote && b.set != nil:
			b.settingMove(d)
		case onNote && d > 0:
			b.p.vp.ScrollDown(3)
		case onNote:
			b.p.vp.ScrollUp(3)
		case b.pick != nil:
			return b.pickMove(d)
		default:
			return b.move(d)
		}
		return nil
	}
	if _, ok := msg.(tea.MouseClickMsg); !ok || m.Button != tea.MouseLeft || row < 0 || row >= b.bodyHeight() {
		return nil
	}
	b.msg = ""
	switch {
	case split && m.X >= lw-1 && m.X <= lw+1:
		b.dragging = true
		return nil
	case onNote && b.set != nil:
		return b.settingsClick(row)
	case onNote:
		if b.showing != "" {
			b.focus = onPreview
			at := b.notePoint(m.X, m.Y)
			b.p.mark, b.selecting = &span{anchor: at, head: at}, true
		}
		return nil
	case b.pick == nil && b.cal != nil:
		return b.calClick(row, m.X)
	case b.pick != nil:
		i := b.pick.top + row - 1 // under the picker's title
		if row == 0 || i >= len(b.pick.items) {
			return nil
		}
		if i == b.pick.sel {
			b.closeInput()
			return b.choose()
		}
		return b.pickMove(i - b.pick.sel)
	case b.input != nil:
		return nil
	}
	i := b.top + row
	if i >= len(b.entries) || b.entries[i].rule {
		return nil
	}
	if i == b.sel {
		return b.enter()
	}
	b.sel, b.focus = i, onList
	b.clampList()
	return b.loadNow()
}

// notePoint is the place in the rendered note under the screen cell x, y. Above or below the
// pane the note scrolls a line, so a selection can reach past what is on screen.
func (b *browser) notePoint(x, y int) point {
	left := 0
	if !b.full {
		left = b.listWidth() + 1
	}
	row := y - 2
	switch {
	case row < 0:
		b.p.vp.ScrollUp(1)
		row = 0
	case row >= b.p.height:
		b.p.vp.ScrollDown(1)
		row = b.p.height - 1
	}
	return point{line: b.p.vp.YOffset() + row, col: max(0, x-left)}
}

// copy puts text on the clipboard and says so.
func (b *browser) copy(text, what string) tea.Cmd {
	b.say("copied "+what, false)
	return copyText(text)
}

// copyText puts text on the clipboard with the OS, else through the terminal (OSC 52).
func copyText(text string) tea.Cmd {
	if clip.Write(text) == nil {
		return nil
	}
	return tea.SetClipboard(text)
}

// resize makes the list w columns wide, between list-min and what leaves the note preview-min,
// and draws the note again at its new width; the images follow when the drag ends.
func (b *browser) resize(w int) {
	if b.full || b.narrow() {
		return
	}
	w = max(listMin, min(w, b.width-previewMin-1))
	if w == b.listWidth() {
		return
	}
	b.listW = w
	b.layout()
	b.p.render()
}

// poll notices changes made outside the browser: the note on screen, the folder, the status a
// background sync wrote.
func (b *browser) poll() tea.Cmd {
	var cmds []tea.Cmd
	if b.showing != "" && b.p.changed() {
		b.p.load()
		b.p.render()
		cmds = append(cmds, b.p.sendImages())
	}
	if m := modTime(b.v.Abs(b.dir)); !m.Equal(b.dirMod) {
		b.list()
	}
	if e, ok := b.selected(); ok && e.view != "" && b.pick == nil && b.cal == nil {
		switch {
		case b.viewNote(e.view) != b.showing:
			cmds = append(cmds, b.loadNow()) // a journal or digest made since, by nt a or a sync
		case b.homeCal != nil:
			b.readMonth(b.homeCal) // a journal written since
		}
	}
	if b.cal != nil {
		b.calRead()
	}
	if m := modTime(gitsync.StatusPath(b.o.StateDir)); !m.Equal(b.statusMod) {
		b.statusMod = m
		if st, ok := gitsync.ReadStatus(b.o.StateDir); ok {
			b.status, b.hasStatus = st, true
		}
	}
	return tea.Batch(cmds...)
}

func (b *browser) key(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if s == "esc" && b.p.mark != nil {
		b.p.mark = nil
		return nil
	}
	if b.focus == onPreview {
		switch s {
		case "esc", "left", "h", "tab":
			b.focus, b.full = onList, false
			b.layout()
			b.p.render()
			return b.p.sendImages()
		case "j", "down":
			b.p.vp.ScrollDown(1)
			return nil
		case "k", "up":
			b.p.vp.ScrollUp(1)
			return nil
		case "g", "home":
			b.p.vp.GotoTop()
			return nil
		case "G", "end":
			b.p.vp.GotoBottom()
			return nil
		case "space", "pgdown", "ctrl+d", "ctrl+f":
			b.p.vp.HalfPageDown()
			return nil
		case "pgup", "ctrl+u", "ctrl+b":
			b.p.vp.HalfPageUp()
			return nil
		}
	}
	switch s {
	case "q", "ctrl+c":
		return tea.Quit
	case "j", "down":
		return b.move(1)
	case "k", "up":
		return b.move(-1)
	case "g", "home":
		return b.move(-len(b.entries))
	case "G", "end":
		return b.move(len(b.entries))
	case "pgdown", "ctrl+d", "space":
		b.p.vp.HalfPageDown()
		return nil
	case "pgup", "ctrl+u":
		b.p.vp.HalfPageUp()
		return nil
	case "enter", "right":
		return b.enter()
	case "<", ">":
		b.resize(b.listWidth() + map[string]int{"<": -listStep, ">": listStep}[s])
		return b.p.sendImages()
	case "tab":
		if b.showing != "" && !b.narrow() {
			b.focus = onPreview
		}
		return nil
	case "left", "h", "backspace":
		return b.up()
	case "esc":
		b.help = false
		return nil
	case "t":
		return b.today()
	case "~":
		return b.goHome()
	case "[", "]":
		return b.day(map[string]int{"[": -1, "]": 1}[s])
	case "A":
		return b.agents()
	case "w":
		return b.openWeekly()
	case "x":
		return b.openTodos(false)
	case "c":
		return b.openCalendar()
	case ",":
		return b.openSettings("")
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return b.pin(int(s[0] - '1'))
	case "a":
		return b.prompt("add", "add to today: ")
	case "n":
		return b.prompt("new", "new note in "+firstNonEmpty(b.noteFolder(), "/")+": ")
	case "/":
		b.pick = &picker{title: "search"}
		return b.prompt("search", "search: ")
	case "ctrl+p":
		b.pick = &picker{title: "notes by title"}
		b.prompt("find", "title: ")
		return b.findTitles()
	case "e":
		if b.showing == "" {
			return nil
		}
		return b.edit(b.showing)
	case "l":
		return b.links()
	case "o":
		return b.outline()
	case "i":
		return b.imagesPicker()
	case "v":
		return b.glow()
	case "y":
		if text := b.p.selected(); text != "" {
			n := strings.Count(text, "\n") + 1
			b.p.mark = nil
			return b.copy(text, plural(n, "line"))
		}
		if b.showing == "" {
			return nil
		}
		return b.copy(b.showing, b.showing)
	case "D":
		return b.askDelete()
	case "f":
		return b.startHints()
	case "b":
		return b.back()
	case "m":
		return b.startMove()
	case "R":
		return b.startRename()
	case "N":
		return b.startMkdir()
	case "s":
		return b.sync()
	case "r":
		b.list()
		return tea.Batch(b.loadNow(), b.reindex())
	case "?":
		b.help = !b.help
		return nil
	}
	return nil
}

// enter opens the selected row: a view does what its key does, a folder is entered, a note gets
// the focus, and on a narrow screen the whole screen.
func (b *browser) enter() tea.Cmd {
	e, ok := b.selected()
	if !ok {
		return nil
	}
	if e.view != "" {
		return b.openView(e.view)
	}
	if e.dir {
		return b.openFolder(e.rel)
	}
	b.focus = onPreview
	if b.narrow() {
		b.full = true
		b.layout()
		b.p.render()
		return b.p.sendImages()
	}
	return nil
}

// askDelete asks before deleting the selected row: a note with the images no other note uses,
// or an empty folder. With the note in focus, it is the note on screen.
func (b *browser) askDelete() tea.Cmd {
	rel := b.showing
	if b.focus == onList {
		e, ok := b.selected()
		if !ok || e.view != "" || e.pin {
			return nil
		}
		if e.dir {
			if !b.v.FolderEmpty(e.rel) {
				b.say(e.rel+"/ is not empty: delete what is in it first", false)
				return nil
			}
			b.ask = &question{"delete the empty folder " + e.rel + "/?", func() tea.Cmd {
				if err := b.v.DeleteFolder(e.rel); err != nil {
					b.say(err.Error(), true)
					return nil
				}
				b.say("deleted "+e.rel+"/", false)
				return b.afterDelete()
			}}
			return nil
		}
		rel = e.rel
	}
	if rel == "" {
		return nil
	}
	raw, _ := os.ReadFile(b.v.Abs(rel))
	var images []string
	for _, l := range mdparse.Parse(raw).Links {
		if !l.Image {
			continue
		}
		if f, ok := localImage(b.v.Abs(rel), l.Dest); ok {
			if r := b.v.RelPath(f); !strings.HasPrefix(r, "../") && strings.Contains("/"+r, "/assets/") && isFile(f) {
				images = append(images, r)
			}
		}
	}
	images = b.v.Unshared(rel, images)
	what := strconv.Quote(firstNonEmpty(render.Title(raw, rel), rel))
	if len(images) > 0 {
		what += " and " + plural(len(images), "image")
	}
	text := "delete " + what + "?"
	if n := b.linkers(rel); n > 0 {
		text += " " + plural(n, "note") + " link to it"
	}
	b.ask = &question{text, func() tea.Cmd {
		if err := b.v.Delete(rel, images); err != nil {
			b.say(err.Error(), true)
			return nil
		}
		b.say("deleted "+rel, false)
		return b.afterDelete()
	}}
	return nil
}

// linkers is how many notes link to rel, leaving out the journal line delete takes away.
func (b *browser) linkers(rel string) int {
	if b.ix == nil {
		return 0
	}
	n := 0
	day := datePrefix.FindString(path.Base(rel))
	for _, from := range b.ix.Back[rel] {
		if day == "" || from != b.journalRel(day) {
			n++
		}
	}
	return n
}

// afterDelete shows the list again with the row that took the deleted one's place.
func (b *browser) afterDelete() tea.Cmd {
	at := b.sel
	b.list()
	b.sel = max(0, min(at, len(b.entries)-1))
	b.offRule(-1)
	b.focus, b.full = onList, false
	b.layout()
	b.clampList()
	return tea.Batch(b.loadNow(), b.reindex())
}

func isFile(p string) bool { st, err := os.Stat(p); return err == nil && st.Mode().IsRegular() }
func isDir(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }

// noteFolder is where n puts a note: the current folder, or the default folder when the browser
// is at the root or in the journals or agent reports.
func (b *browser) noteFolder() string {
	d := b.dir
	c := b.v.Cfg.Folders
	if d == "" || d == c.Daily || strings.HasPrefix(d, c.Daily+"/") || d == c.Agents || strings.HasPrefix(d, c.Agents+"/") {
		return b.v.Cfg.DefaultFolder
	}
	return d
}

// day goes to the previous (-1) or next (+1) journal that exists, from the one selected or today.
func (b *browser) day(step int) tea.Cmd {
	var days []string
	root := b.v.Abs(b.v.Cfg.Folders.Daily)
	years, _ := os.ReadDir(root)
	for _, y := range years {
		files, _ := os.ReadDir(path.Join(root, y.Name()))
		for _, f := range files {
			if m := datePrefix.FindStringSubmatch(f.Name()); m != nil && f.Name() == m[1]+".md" {
				days = append(days, m[1])
			}
		}
	}
	sort.Strings(days)
	from := b.v.Day(b.v.Now()).Format("2006-01-02")
	if e, ok := b.selected(); ok && e.journal {
		from = e.date
	}
	i := sort.SearchStrings(days, from)
	switch {
	case step < 0 && i > 0:
		return b.reveal(b.journalRel(days[i-1]))
	case step > 0 && i < len(days) && days[i] == from && i+1 < len(days):
		return b.reveal(b.journalRel(days[i+1]))
	case step > 0 && i < len(days) && days[i] != from:
		return b.reveal(b.journalRel(days[i]))
	}
	if step < 0 {
		b.say("no earlier journal", false)
	} else {
		b.say("no later journal", false)
	}
	return nil
}

func (b *browser) journalRel(day string) string {
	t, _ := time.ParseInLocation("2006-01-02", day, time.Local)
	return b.v.JournalRel(t)
}

// goHome goes to the root, with the top-level folder the browser was in selected; at the root,
// to Today.
func (b *browser) goHome() tea.Cmd {
	top, _, _ := strings.Cut(b.dir, "/")
	b.focus, b.full = onList, false
	b.layout()
	b.dir, b.sel, b.top = "", 0, 0
	b.list()
	b.sel = 0
	for i, e := range b.entries {
		if top != "" && e.dir && !e.pin && e.rel == top {
			b.sel = i
		}
	}
	b.clampList()
	return b.loadNow()
}

// today goes to today's journal, starting it when there is none.
func (b *browser) today() tea.Cmd {
	rel, _, err := b.v.EnsureJournal(b.v.Now())
	if err != nil {
		b.say(err.Error(), true)
		return nil
	}
	return b.reveal(rel)
}

// openWeekly goes to the newest weekly digest.
func (b *browser) openWeekly() tea.Cmd {
	rel, ok := weekly.Latest(b.v)
	if !ok {
		b.say("no weekly digest yet: nt config set weekly true on one machine", false)
		return nil
	}
	return b.reveal(rel)
}

func (b *browser) agents() tea.Cmd {
	rel := b.v.AgentsRel(b.v.Day(b.v.Now()))
	if _, err := os.Stat(b.v.Abs(rel)); err != nil {
		b.say("no agent reports today", false)
		return nil
	}
	if _, err := os.Stat(b.v.Abs(rel + "/README.md")); err == nil {
		return b.reveal(rel + "/README.md")
	}
	return b.openFolder(rel)
}

func (b *browser) pin(i int) tea.Cmd {
	pins := b.v.Cfg.Pins
	if i >= len(pins) {
		b.say("no folder on "+string(rune('1'+i))+": add it to pins in .nt/config.toml", false)
		return nil
	}
	rel := strings.Trim(pins[i], "/")
	if st, err := os.Stat(b.v.Abs(rel)); err != nil || !st.IsDir() {
		b.say(rel+": no such folder", true)
		return nil
	}
	return b.openFolder(rel)
}

// edit hands the terminal to the editor for the note at rel, holding back sync's merge until it
// closes.
func (b *browser) edit(rel string) tea.Cmd { return b.editAt(rel, 0) }

// editAt is edit at line; 0 is the line on screen in the preview, or a journal's last line.
func (b *browser) editAt(rel string, line int) tea.Cmd {
	release, err := b.v.LockEdit()
	if err != nil {
		b.say(err.Error(), true)
		return nil
	}
	b.editing = release
	if line == 0 && rel == b.showing {
		line = b.p.sourceLine()
	}
	if line == 0 {
		if e, ok := b.selected(); ok && (e.journal && e.rel == rel || e.view == "today") {
			line = editor.LastLine(b.v.Abs(rel))
		}
	}
	argv := editor.Argv(b.v.Abs(rel), line)
	return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg { return editedMsg{err} })
}

// prompt opens the one-line prompt at the bottom.
func (b *browser) prompt(kind, label string) tea.Cmd {
	b.input, b.inputKind = &lineInput{prompt: label}, kind
	return nil
}

func (b *browser) closeInput() {
	b.input, b.inputKind = nil, ""
	if b.searchStop != nil {
		b.searchStop()
		b.searchStop = nil
	}
}

func (b *browser) inputKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc", "ctrl+c":
		kind := b.inputKind
		b.closeInput()
		b.moving = ""
		if kind == "search" || kind == "find" || kind == "move" {
			return b.closePick()
		}
		return nil
	case "up", "ctrl+k":
		if b.pick != nil {
			return b.pickMove(-1)
		}
		return nil
	case "down", "ctrl+j", "tab":
		if b.pick != nil {
			return b.pickMove(1)
		}
		return nil
	case "enter":
		text := strings.TrimSpace(b.input.Value())
		kind := b.inputKind
		b.closeInput()
		switch kind {
		case "add":
			if text == "" {
				return nil
			}
			rel, _, err := b.v.Append(text, b.v.Now())
			if err != nil {
				b.say(err.Error(), true)
				return nil
			}
			b.say("added to "+rel, false)
			if b.showing == rel {
				b.p.load()
				b.p.render()
				b.p.vp.GotoBottom()
			}
			return b.p.sendImages()
		case "new":
			folder, now := b.noteFolder(), b.v.Now()
			return func() tea.Msg {
				rel, err := b.v.NewNote(text, folder, now)
				return notedMsg{rel, err}
			}
		case "search", "find":
			return b.choose()
		case "due", "todo":
			return b.todoInput(kind, text)
		case "move":
			if p := b.pick; p != nil && p.sel < len(p.items) {
				return b.moveInto(p.items[p.sel])
			}
			return b.moveNowhere(text)
		case "rename":
			return b.rename(text)
		case "setting":
			if s, ok := b.settingSelected(); ok {
				return b.setSetting(s, text)
			}
			return nil
		case "mkdir":
			return b.mkdir(text)
		}
		return nil
	}
	if b.input.key(k) {
		return b.inputChanged()
	}
	return nil
}

func (b *browser) inputChanged() tea.Cmd {
	switch b.inputKind {
	case "search":
		return b.runSearch(b.input.Value())
	case "find":
		return b.findTitles()
	case "move":
		return b.moveTargets()
	}
	return nil
}

// runSearch starts rg for query after a short pause, so fast typing runs one search, not one
// per key; a newer search cancels the one running.
func (b *browser) runSearch(query string) tea.Cmd {
	b.searchSeq++
	seq := b.searchSeq
	if b.searchStop != nil {
		b.searchStop()
	}
	ctx, stop := context.WithCancel(context.Background())
	b.searchStop = stop
	root := b.v.Root
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		if strings.TrimSpace(query) == "" || ctx.Err() != nil {
			return searchMsg{seq: seq}
		}
		m, err := search.Rg(ctx, root, query)
		return searchMsg{seq: seq, matches: m, err: err}
	})
}

func (b *browser) searched(msg searchMsg) tea.Cmd {
	if msg.seq != b.searchSeq || b.pick == nil || b.inputKind != "search" {
		return nil
	}
	if msg.err != nil && msg.err != context.Canceled {
		b.say(msg.err.Error(), true)
	}
	var items []pickItem
	for _, m := range msg.matches {
		items = append(items, pickItem{label: strings.TrimSpace(m.Text), hint: m.Path, kind: "note", target: m.Path, line: m.Line})
		if len(items) >= 500 {
			break
		}
	}
	b.pick.items, b.pick.sel, b.pick.top = items, 0, 0
	b.pick.title = "search · " + plural(len(items), "line")
	return b.followPick()
}

func (b *browser) findTitles() tea.Cmd {
	if b.pick == nil {
		return nil
	}
	if b.ix == nil {
		b.pick.title = "notes by title · reading the notes"
		return nil
	}
	q := ""
	if b.input != nil {
		q = b.input.Value()
	}
	var items []pickItem
	for _, n := range b.ix.Find(q, 300) {
		items = append(items, pickItem{label: n.Title, hint: n.Path, kind: "note", target: n.Path})
	}
	b.pick.items, b.pick.sel, b.pick.top = items, 0, 0
	b.pick.title = "notes by title · " + plural(len(items), "note")
	return b.followPick()
}

// links lists the note's links and the notes that link to it.
func (b *browser) links() tea.Cmd {
	if b.showing == "" {
		return nil
	}
	var items []pickItem
	for _, l := range mdparse.Parse(b.p.raw).Links {
		if l.Image {
			continue
		}
		it := pickItem{label: "→ " + firstNonEmpty(l.Text, l.Dest), hint: l.Dest}
		switch {
		case mdparse.External(l.Dest):
			it.kind, it.target = "url", l.Dest
		case l.Wiki && b.ix != nil:
			to, ok := b.ix.Target(b.showing, l)
			if !ok {
				it.kind = "missing"
			} else {
				it.kind, it.target, it.text = "note", to, anchorOf(l.Dest)
			}
		default:
			to, anchor, ok := mdparse.Resolve(b.showing, l.Dest)
			if !ok {
				it.kind = "missing"
			} else {
				it.kind, it.target, it.text = "note", to, anchor
			}
		}
		items = append(items, it)
	}
	if b.ix != nil {
		for _, from := range b.ix.Back[b.showing] {
			items = append(items, pickItem{label: "← " + firstNonEmpty(b.ix.Title(from), from), hint: from, kind: "note", target: from})
		}
	}
	if len(items) == 0 {
		b.say("no links in or to this note", false)
		return nil
	}
	b.pick = &picker{title: "links · " + plural(len(items), "link"), items: items}
	return nil
}

func anchorOf(dest string) string {
	_, a, _ := strings.Cut(dest, "#")
	return a
}

func (b *browser) outline() tea.Cmd {
	if b.showing == "" {
		return nil
	}
	var items []pickItem
	for _, h := range mdparse.Parse(b.p.raw).Headings {
		items = append(items, pickItem{label: strings.Repeat("  ", h.Level-1) + h.Text, kind: "heading", text: h.Text, line: h.Line})
	}
	if len(items) == 0 {
		b.say("no headings in this note", false)
		return nil
	}
	b.pick = &picker{title: "outline · " + plural(len(items), "heading"), items: items}
	return nil
}

func (b *browser) imagesPicker() tea.Cmd {
	if b.showing == "" {
		return nil
	}
	var items []pickItem
	for _, l := range mdparse.Parse(b.p.raw).Links {
		if !l.Image {
			continue
		}
		it := pickItem{label: firstNonEmpty(l.Text, path.Base(l.Dest)), hint: l.Dest}
		if mdparse.External(l.Dest) {
			it.kind, it.target = "url", l.Dest
		} else if f, ok := localImage(b.v.Abs(b.showing), l.Dest); ok {
			it.kind, it.target = "image", f
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		b.say("no images in this note", false)
		return nil
	}
	b.pick = &picker{title: "images · " + plural(len(items), "image"), items: items}
	return nil
}

func (b *browser) pickKey(k tea.KeyPressMsg) tea.Cmd {
	if b.pick.todos {
		if cmd, ok := b.todoKeys(k); ok {
			return cmd
		}
	}

	switch k.String() {
	case "esc", "q", "left", "h":
		return b.closePick()
	case "j", "down":
		return b.pickMove(1)
	case "k", "up":
		return b.pickMove(-1)
	case "g", "home":
		return b.pickMove(-len(b.pick.items))
	case "G", "end":
		return b.pickMove(len(b.pick.items))
	case "enter", "right":
		return b.choose()
	case "ctrl+c":
		return tea.Quit
	}
	return nil
}

func (b *browser) pickMove(d int) tea.Cmd {
	p := b.pick
	if len(p.items) == 0 {
		return nil
	}
	p.sel = max(0, min(len(p.items)-1, p.sel+d))
	h := b.bodyHeight() - 1
	if p.sel < p.top {
		p.top = p.sel
	}
	if h > 0 && p.sel >= p.top+h {
		p.top = p.sel - h + 1
	}
	return b.followPick()
}

// choose acts on the picked item and closes the picker.
func (b *browser) choose() tea.Cmd {
	p := b.pick
	b.pick = nil
	if p == nil || p.sel >= len(p.items) {
		return nil
	}
	it := p.items[p.sel]
	switch it.kind {
	case "note":
		if _, err := os.Stat(b.v.Abs(it.target)); err != nil {
			b.say(it.target+": no such note", true)
			return nil
		}
		b.cal = nil // a todo picked from a day of the calendar: its note, in the list
		cmd := b.reveal(it.target)
		b.scrollTo(it)
		return cmd
	case "heading":
		if !b.p.showText(it.text) {
			b.p.showLine(it.line)
		}
		return nil
	case "url":
		b.say("opened "+it.target, false)
		openDetached(osOpener(), it.target)
		return nil
	case "image":
		return openImage(b.o.Machine.ImageViewer, b.o.Bin, it.target)
	case "missing":
		b.say(it.hint+": the link points to nothing", true)
	case "folder":
		return b.moveInto(it)
	}
	return nil
}

// scrollTo scrolls the preview to what a picker row points at: a todo's text, a heading or a
// search hit, else its line.
func (b *browser) scrollTo(it pickItem) {
	switch {
	case it.todo != nil && b.p.showText(it.todo.Text):
	case it.todo == nil && it.text != "" && b.p.showText(strings.ReplaceAll(it.text, "-", " ")):
	case it.line > 0:
		b.p.showLine(it.line)
	}
}

// openImage opens a picture full size: viewer (image_viewer from the config), else Quick Look on
// macOS, xdg-open on a Linux desktop, and chafa through bin, nt itself, in the terminal
// everywhere else (docs/design.md §5).
func openImage(viewer, bin, f string) tea.Cmd {
	if v := strings.Fields(viewer); len(v) > 0 {
		openDetached(v, f)
		return nil
	}
	switch {
	case runtime.GOOS == "darwin":
		openDetached([]string{"qlmanage", "-p"}, f)
	case os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "":
		openDetached([]string{"xdg-open"}, f)
	default:
		return tea.ExecProcess(exec.Command(bin, "_image", f), func(err error) tea.Msg { return editedMsg{err} })
	}
	return nil
}

func osOpener() []string {
	if runtime.GOOS == "darwin" {
		return []string{"open"}
	}
	return []string{"xdg-open"}
}

// openDetached runs argv with arg, not waiting for it and not letting it write to the screen.
func openDetached(argv []string, arg string) {
	c := exec.Command(argv[0], append(argv[1:], arg)...)
	if c.Start() == nil {
		go func() { _ = c.Wait() }()
	}
}

// glow opens the note in glow's pager, in nt's style.
func (b *browser) glow() tea.Cmd {
	if b.showing == "" {
		return nil
	}
	if _, err := exec.LookPath("glow"); err != nil {
		b.say("glow is not installed: brew install glow", true)
		return nil
	}
	name := "glamour-dark.json"
	if !b.dark {
		name = "glamour-light.json"
	}
	style := path.Join(b.o.StateDir, name)
	if js, err := render.JSON(b.dark); err == nil {
		_ = vault.WriteFile(style, js)
	}
	c := exec.Command("glow", "-p", "-s", style, b.v.Abs(b.showing))
	return tea.ExecProcess(c, func(err error) tea.Msg { return editedMsg{err} })
}

// sync runs nt sync in the browser's process; a sync already running (the timer's) wins.
func (b *browser) sync() tea.Cmd {
	if b.syncing {
		return nil
	}
	b.syncing = true
	v, dir, m := b.v, b.o.StateDir, b.o.Machine // copied: the settings list can change them meanwhile
	run := func() tea.Msg {
		st := gitsync.Run(v, gitsync.Options{Notify: gitsync.Notify, Digest: m.Digest, Weekly: m.Weekly, StateDir: dir,
			Summary: weekly.Tool{Name: m.Summary.Tool, Model: m.Summary.Model}})
		_ = gitsync.WriteStatus(dir, st)
		return syncedMsg{st}
	}
	return tea.Batch(run, func() tea.Msg { return spinMsg{} })
}

func (b *browser) synced(st gitsync.Status) tea.Cmd {
	b.syncing = false
	if st.State == "busy" {
		return nil
	}
	b.status, b.hasStatus = st, true
	b.statusMod = modTime(gitsync.StatusPath(b.o.StateDir))
	if st.State == "error" {
		b.say("sync: "+st.Message, true)
	}
	if st.Merged || st.Committed > 0 {
		b.list()
		var cmds []tea.Cmd
		if b.showing != "" && b.p.changed() {
			b.p.load()
			b.p.render()
			cmds = append(cmds, b.p.sendImages())
		}
		return tea.Batch(append(cmds, b.reindex())...)
	}
	return nil
}

// reindex reads every note again for titles and backlinks, and the todos for their count, off
// the screen's thread.
func (b *browser) reindex() tea.Cmd {
	b.indexSeq++
	seq, root, v := b.indexSeq, b.v.Root, b.v
	return func() tea.Msg { return indexMsg{seq, scan.Build(root), todo.Collect(v)} }
}

func modTime(p string) time.Time {
	st, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return strconv.Itoa(n) + " " + what + "s"
}
