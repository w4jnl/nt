package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/vault"
)

// testBrowser is a browser on a repository with one folder of three notes, on a screen of w
// columns, its state in state.
func testBrowser(t *testing.T, w int, state string) *browser {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{".git/HEAD", "tech/2026-10-20-a.md", "tech/2026-10-21-b.md", "tech/2026-10-22-c.md"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f), []byte("# "+filepath.Base(f)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v, err := vault.Open(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	m := config.DefaultMachine()
	m.InlineImages, m.Theme.Mode = "off", "dark"
	b := newBrowser(BrowserOptions{Vault: v, Machine: m, Folder: "tech", StateDir: state})
	b.Update(tea.WindowSizeMsg{Width: w, Height: 30})
	b.loadNow()
	return b
}

func TestListWidth(t *testing.T) {
	for _, tc := range []struct {
		screen, asked, want int
	}{
		{242, listDefault, listDefault},
		{100, listDefault, listDefault},
		{80, listDefault, 80 - previewMin - 1}, // the note keeps preview-min
		{242, 10, listMin},
		{242, 300, 242 - previewMin - 1},
		{60, listDefault, 60}, // narrow: the list alone
	} {
		b := testBrowser(t, tc.screen, t.TempDir())
		b.listW = tc.asked
		if got := b.listWidth(); got != tc.want {
			t.Errorf("screen %d, asked %d: list %d, want %d", tc.screen, tc.asked, got, tc.want)
		}
	}
}

func TestDragTheDivider(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	lw := b.listWidth()
	b.Update(tea.MouseClickMsg{X: lw, Y: 5, Button: tea.MouseLeft})
	if !b.dragging {
		t.Fatal("a press on the divider does not hold it")
	}
	b.Update(tea.MouseMotionMsg{X: 70, Y: 5, Button: tea.MouseLeft})
	if b.listWidth() != 70 || b.p.width != 200-70-1 {
		t.Fatalf("after the drag the list is %d and the note %d, want 70 and %d", b.listWidth(), b.p.width, 200-70-1)
	}
	b.Update(tea.MouseReleaseMsg{X: 70, Y: 5, Button: tea.MouseLeft})
	b.Update(tea.MouseMotionMsg{X: 50, Y: 5})
	if b.dragging || b.listWidth() != 70 {
		t.Fatalf("the divider still moves after the release: list %d", b.listWidth())
	}
	b.Update(tea.KeyPressMsg{Code: '<', Text: "<"})
	if b.listWidth() != 70-listStep {
		t.Fatalf("< makes the list %d, want %d", b.listWidth(), 70-listStep)
	}
}

func TestClickSelectsThenOpens(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	if b.sel != 0 || b.showing != "tech/2026-10-22-c.md" {
		t.Fatalf("starts on %d showing %q", b.sel, b.showing)
	}
	row := func(i int) tea.MouseClickMsg { return tea.MouseClickMsg{X: 5, Y: 2 + i, Button: tea.MouseLeft} }
	b.Update(row(2))
	if b.sel != 2 || b.showing != "tech/2026-10-20-a.md" || b.focus != onList {
		t.Fatalf("a click on the third row: selected %d showing %q", b.sel, b.showing)
	}
	b.Update(row(2))
	if b.focus != onPreview {
		t.Fatal("a second click on the row does not open the note")
	}
	b.Update(row(1))
	if b.sel != 1 || b.focus != onList {
		t.Fatal("a click on the list does not give it the focus back")
	}
	b.Update(row(9))
	if b.sel != 1 {
		t.Fatal("a click under the last row moves the selection")
	}
	b.Update(tea.MouseWheelMsg{X: 5, Y: 4, Button: tea.MouseWheelDown})
	if b.sel != 2 {
		t.Fatal("the wheel over the list does not move the selection")
	}
}

func TestListWidthIsRemembered(t *testing.T) {
	state := t.TempDir()
	if got := loadListWidth(state); got != listDefault {
		t.Fatalf("without a saved width: %d, want %d", got, listDefault)
	}
	saveListWidth(state, 52)
	if got := testBrowser(t, 200, state).listWidth(); got != 52 {
		t.Fatalf("a new browser starts at %d, want the saved 52", got)
	}
}

// screenAt is the screen cell where text begins in the rendered note, false when it is not on
// screen.
func screenAt(b *browser, text string) (x, y int, ok bool) {
	for i, l := range b.p.lines {
		if c := strings.Index(ansi.Strip(l), text); c >= 0 && i >= b.p.vp.YOffset() && i < b.p.vp.YOffset()+b.p.height {
			return b.listWidth() + 1 + ansi.StringWidth(ansi.Strip(l)[:c]), 2 + i - b.p.vp.YOffset(), true
		}
	}
	return 0, 0, false
}

func TestSelectText(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	note := "tech/2026-10-22-c.md"
	if err := os.WriteFile(b.v.Abs(note), []byte("# Title\n\nA paragraph before.\n\n```\nadcli info example.com\n[domain]\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b.show(note)
	x1, y1, ok1 := screenAt(b, "adcli")
	x2, y2, ok2 := screenAt(b, "[domain]")
	if !ok1 || !ok2 {
		t.Fatalf("the code block is not on screen:\n%s", strings.Join(b.p.lines, "\n"))
	}
	b.Update(tea.MouseClickMsg{X: x1, Y: y1, Button: tea.MouseLeft})
	b.Update(tea.MouseMotionMsg{X: x2 + len("[domain]") - 1, Y: y2, Button: tea.MouseLeft})
	b.Update(tea.MouseReleaseMsg{X: x2 + len("[domain]") - 1, Y: y2, Button: tea.MouseLeft})
	if got, want := b.p.selected(), "adcli info example.com\n[domain]"; got != want {
		t.Fatalf("selected %q, want %q", got, want)
	}
	if !strings.Contains(ansi.Strip(b.View().Content), "y copy") {
		t.Error("the footer does not offer to copy")
	}
	b.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if b.p.mark != nil {
		t.Fatal("esc does not clear the selection")
	}
	b.Update(tea.MouseClickMsg{X: x1, Y: y1, Button: tea.MouseLeft})
	b.Update(tea.MouseReleaseMsg{X: x1, Y: y1, Button: tea.MouseLeft})
	if b.p.mark != nil || b.focus != onPreview {
		t.Fatal("a click on the note selects text, or does not give the note the focus")
	}
}

func TestDeleteAsks(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	first := b.entries[0].rel
	b.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	if b.ask == nil || !strings.Contains(b.ask.text, "delete") {
		t.Fatal("D does not ask")
	}
	b.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if _, err := os.Stat(b.v.Abs(first)); err != nil || b.ask != nil {
		t.Fatal("answering n deleted the note or kept asking")
	}
	b.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	b.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if _, err := os.Stat(b.v.Abs(first)); err == nil {
		t.Fatal("answering y kept the note")
	}
	if len(b.entries) != 2 || b.sel != 0 || b.showing != b.entries[0].rel {
		t.Fatalf("after the delete: %d rows, row %d selected, showing %q", len(b.entries), b.sel, b.showing)
	}
}

// run sends msg to the browser, then what it started that the tests wait for: the todo list's
// or the calendar's reading of the notes, a move, the calendar's preview.
func run(b *browser, msg tea.Msg) {
	_, cmd := b.Update(msg)
	feed(b, cmd)
}

func feed(b *browser, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch m := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range m {
			feed(b, c)
		}
	case todosMsg, movedMsg, calTodosMsg, calPreviewMsg, previewMsg:
		b.Update(m)
	}
}

func press(b *browser, keys string) {
	for _, r := range keys {
		run(b, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func TestTodoList(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	b.v.Now = func() time.Time { return time.Date(2026, 10, 27, 10, 0, 0, 0, time.Local) }
	if err := os.WriteFile(b.v.Abs("todo.md"), []byte("# Todo\n\n## Inbox\n\n- [ ] Renew passport due:2026-11-03\n- [ ] Call the plumber due:2026-10-25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.v.Abs("tech/2026-10-20-a.md"), []byte("# a\n\n- [ ] from a note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := func() string { s, _ := os.ReadFile(b.v.Abs("todo.md")); return string(s) }
	rows := func() (out []string) {
		for _, it := range b.pick.items {
			out = append(out, it.todo.Text)
		}
		return out
	}

	press(b, "x")
	if b.pick == nil || !b.pick.todos || b.pick.title != "todo · 3 open · 1 late" {
		t.Fatalf("x does not show the todo list: %+v", b.pick)
	}
	if got := strings.Join(rows(), ", "); got != "Call the plumber, Renew passport, from a note" {
		t.Fatalf("the open todos: %s", got)
	}
	if screen := ansi.Strip(b.View().Content); !strings.Contains(screen, "○ Call the plumber") || !strings.Contains(screen, "2d late") {
		t.Fatalf("the list on screen:\n%s", screen)
	}
	if f := ansi.Strip(b.footer()); !strings.Contains(f, "x done") || !strings.Contains(f, "d due") {
		t.Errorf("footer: %s", f)
	}

	press(b, "x") // checks the selected one
	if !strings.Contains(read(), "- [x] Call the plumber due:2026-10-25 done:2026-10-27\n") || len(b.pick.items) != 2 {
		t.Fatalf("x does not check the todo:\n%s", read())
	}

	press(b, "d") // the selected one, Renew passport, is due friday
	press(b, "fri")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(read(), "- [ ] Renew passport due:2026-10-30\n") || b.msg != "due fri: Renew passport" {
		t.Fatalf("d does not set the due date (%s):\n%s", b.msg, read())
	}

	press(b, "a") // the browser is in tech/, so tech/todo.md
	if f := ansi.Strip(b.footer()); !strings.Contains(f, "todo in tech/todo.md:") {
		t.Errorf("the add prompt does not say where: %s", f)
	}
	press(b, "Buy milk")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	list, _ := os.ReadFile(b.v.Abs("tech/todo.md"))
	if string(list) != "# Todo\n\n## Inbox\n\n- [ ] Buy milk\n" || b.todoSelected() == nil || b.todoSelected().Text != "Buy milk" {
		t.Fatalf("a does not add a todo to the folder's list:\n%s", list)
	}

	press(b, "X")
	if got := strings.Join(rows(), ", "); got != "Call the plumber" || !strings.Contains(ansi.Strip(b.footer()), "x open again") {
		t.Fatalf("X does not show the done ones: %s", got)
	}
	press(b, "D")
	press(b, "y")
	if strings.Contains(read(), "plumber") || len(b.pick.items) != 0 {
		t.Fatalf("D y does not delete the todo:\n%s", read())
	}
	run(b, tea.KeyPressMsg{Code: tea.KeyEscape})
	if b.pick != nil {
		t.Fatal("esc does not close the todo list")
	}
}

func TestFolders(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	for f, body := range map[string]string{
		"tech/2026-10-22-c.md":     "---\ntitle: Cables\n---\n\n# Cables\n",
		"daily/2026/2026-10-22.md": "# Day\n\n- [Cables](../../tech/2026-10-22-c.md)\n",
	} {
		if err := os.MkdirAll(filepath.Dir(b.v.Abs(f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(b.v.Abs(f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	journal := func() string { s, _ := os.ReadFile(b.v.Abs("daily/2026/2026-10-22.md")); return string(s) }
	enter := func() { run(b, tea.KeyPressMsg{Code: tea.KeyEnter}) }
	clear := func() { run(b, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}) }
	pick := func(rel string) {
		for i, e := range b.entries {
			if e.rel == rel {
				b.sel = i
				return
			}
		}
		t.Fatalf("%s is not in the list", rel)
	}

	press(b, "N")
	if f := ansi.Strip(b.footer()); !strings.Contains(f, "new folder in tech:") {
		t.Errorf("N does not say where: %s", f)
	}
	press(b, "Work Stuff")
	enter()
	if _, err := os.Stat(b.v.Abs("tech/work-stuff/.gitkeep")); err != nil || b.msg != "made tech/work-stuff/" {
		t.Fatalf("N does not make the folder, with .gitkeep: %v, %q", err, b.msg)
	}

	pick("tech/2026-10-22-c.md")
	press(b, "m")
	if b.pick == nil || b.pick.title != "move tech/2026-10-22-c.md to" {
		t.Fatalf("m does not ask where to: %+v", b.pick)
	}
	var rows []string
	for _, it := range b.pick.items {
		rows = append(rows, it.label)
	}
	if got := strings.Join(rows, " "); got != "/ tech/work-stuff/" {
		t.Errorf("the folders it can go to: %s; not its own", got)
	}
	press(b, "stuff")
	enter()
	if _, err := os.Stat(b.v.Abs("tech/work-stuff/2026-10-22-c.md")); err != nil || b.showing != "tech/work-stuff/2026-10-22-c.md" {
		t.Fatalf("m does not move the note there and show it: %v, showing %q", err, b.showing)
	}
	if !strings.Contains(journal(), "(../../tech/work-stuff/2026-10-22-c.md)") || !strings.Contains(b.msg, "links fixed in 1 note") {
		t.Fatalf("the journal's link (%s):\n%s", b.msg, journal())
	}

	press(b, "R")
	if f := ansi.Strip(b.footer()); !strings.Contains(f, "new title: Cables") {
		t.Errorf("R does not start from the title: %s", f)
	}
	clear()
	press(b, "Cat6 cables")
	enter()
	if _, err := os.Stat(b.v.Abs("tech/work-stuff/2026-10-22-cat6-cables.md")); err != nil || !strings.Contains(journal(), "- [Cat6 cables](../../tech/work-stuff/2026-10-22-cat6-cables.md)") {
		t.Fatalf("R does not rename the note and its link (%v):\n%s", err, journal())
	}

	press(b, "h")
	pick("tech/work-stuff")
	press(b, "R")
	clear()
	press(b, "Projects")
	enter()
	if !strings.Contains(journal(), "(../../tech/projects/2026-10-22-cat6-cables.md)") || b.entries[b.sel].rel != "tech/projects" {
		t.Fatalf("R on a folder (%s):\n%s", b.msg, journal())
	}

	b.dir = "daily/2026"
	b.list()
	press(b, "m")
	if b.pick != nil || !strings.Contains(b.msg, "stay where their date puts them") {
		t.Fatalf("m on a journal: %q", b.msg)
	}
}

func TestFooterVersion(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	b.o.Version = "v0.2.0"
	if f := ansi.Strip(b.footer()); !strings.HasSuffix(f, "nt 0.2.0 ") || !strings.Contains(f, "? help") {
		t.Errorf("a wide footer has the hints and the version at the right: %q", f)
	}
	b.say("synced", false)
	if f := ansi.Strip(b.footer()); strings.Contains(f, "0.2.0") || !strings.HasSuffix(f, "synced ") {
		t.Errorf("a message takes the version's place: %q", f)
	}
	b.say("", false)
	b.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if f := ansi.Strip(b.footer()); strings.Contains(f, "0.2.0") || !strings.Contains(f, "? help") {
		t.Errorf("a narrow footer keeps the hints, not the version: %q", f)
	}
}

func TestCalendar(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	b.v.Now = func() time.Time { return time.Date(2026, 10, 27, 10, 0, 0, 0, time.Local) }
	for f, body := range map[string]string{
		"daily/2026/2026-10-26.md": "# Monday\n\n## Log\n\n- 09:00 yesterday\n",
		"todo.md":                  "# Todo\n\n## Inbox\n\n- [ ] Call the plumber due:2026-10-20\n- [ ] Renew passport due:2026-11-03\n- [x] Pay due:2026-10-21 done:2026-10-21\n",
	} {
		if err := os.MkdirAll(filepath.Dir(b.v.Abs(f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(b.v.Abs(f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	screen := func() string { return ansi.Strip(b.View().Content) }
	day := func() string { return b.cal.day.Format("2006-01-02") }

	press(b, "c")
	if b.cal == nil || day() != "2026-10-27" {
		t.Fatal("c does not open the calendar on today")
	}
	for _, want := range []string{"October 2026", "  mo  tu  we  th  fr  sa  su", "              1   2   3   4", " 19  20• 21  22", "▎27 ", "Tuesday 27 October · today", "no journal · e starts one", "no journal on Tuesday 27 October 2026"} {
		if !strings.Contains(screen(), want) {
			t.Errorf("the calendar does not show %q:\n%s", want, screen())
		}
	}
	if strings.Contains(screen(), "21•") {
		t.Error("a day whose todos are all done is marked")
	}

	press(b, "h")
	if day() != "2026-10-26" || b.showing != "daily/2026/2026-10-26.md" || !strings.Contains(screen(), "journal · enter opens it") {
		t.Fatalf("h: %s, showing %q", day(), b.showing)
	}
	for _, c := range []struct{ keys, want string }{
		{"]", "2026-11-26"}, {"j", "2026-12-03"}, {"[[", "2026-10-03"}, {"k", "2026-09-26"}, {"l", "2026-09-27"}, {"t", "2026-10-27"},
	} {
		press(b, c.keys)
		if day() != c.want {
			t.Errorf("%s: %s, want %s", c.keys, day(), c.want)
		}
	}

	press(b, "k") // Tuesday 20th: the overdue plumber
	press(b, "x")
	if b.pick == nil || b.pick.title != "Tue 20 Oct · todo · 1 open · 1 late" || len(b.pick.items) != 1 {
		t.Fatalf("x does not list the day's todos: %+v", b.pick)
	}
	run(b, tea.KeyPressMsg{Code: tea.KeyEscape})
	if b.pick != nil || b.cal == nil {
		t.Fatal("esc from the day's todos does not go back to the calendar")
	}
	press(b, "?")
	run(b, tea.KeyPressMsg{Code: tea.KeyEscape})
	if b.help || b.cal == nil {
		t.Fatal("esc closes the calendar under the help, not the help")
	}

	_, cmd := b.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if _, err := os.Stat(b.v.Abs("daily/2026/2026-10-20.md")); err != nil || cmd == nil {
		t.Fatalf("e does not start the day's journal and edit it: %v", err)
	}
	b.editing()
	b.editing = nil

	// a click on the 13th selects it; a second one opens its journal, which it has none of
	b.Update(tea.MouseClickMsg{X: 5, Y: 6, Button: tea.MouseLeft})
	if day() != "2026-10-13" {
		t.Fatalf("a click selected %s, want 2026-10-13", day())
	}
	b.Update(tea.MouseClickMsg{X: 5, Y: 6, Button: tea.MouseLeft})
	if b.msg != "no journal on Tue 13 Oct: e starts one" {
		t.Errorf("a second click: %q", b.msg)
	}

	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if b.cal == nil {
		t.Fatal("enter on a day without a journal left the calendar")
	}
	press(b, "t")
	press(b, "h")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if b.cal != nil || b.dir != "daily/2026" || b.entries[b.sel].rel != "daily/2026/2026-10-26.md" {
		t.Fatalf("enter does not go to the day's journal: dir %q", b.dir)
	}

	press(b, "c")
	press(b, "k")
	press(b, "x")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if b.cal != nil || b.pick != nil || b.showing != "todo.md" {
		t.Fatalf("enter on a day's todo does not go to its note: showing %q", b.showing)
	}
}

func TestAddMonths(t *testing.T) {
	for _, c := range []struct {
		from string
		n    int
		want string
	}{{"2026-01-31", 1, "2026-02-28"}, {"2026-03-31", -1, "2026-02-28"}, {"2026-12-15", 1, "2027-01-15"}, {"2028-01-31", 1, "2028-02-29"}} {
		d, _ := time.ParseInLocation("2006-01-02", c.from, time.Local)
		if got := addMonths(d, c.n).Format("2006-01-02"); got != c.want {
			t.Errorf("addMonths(%s, %d) = %s, want %s", c.from, c.n, got, c.want)
		}
	}
}

func TestSettings(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	machine := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("NT_CONFIG", machine)
	if err := os.WriteFile(machine, []byte("repo = \"~/notes\"\nweekly = false   # one machine only\n\n[theme]\nmode = \"dark\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := func(p string) string { s, _ := os.ReadFile(p); return string(s) }
	pick := func(key string) {
		t.Helper()
		for i, s := range config.Settings {
			if s.Key == key {
				b.set.sel = i
				return
			}
		}
		t.Fatalf("%s is not a setting", key)
	}

	press(b, ",")
	if b.set == nil {
		t.Fatal(", does not show the settings")
	}
	screen := ansi.Strip(b.View().Content)
	lw := b.listWidth()
	var left, right []string
	for _, l := range strings.Split(screen, "\n") {
		if r := []rune(l); len(r) > lw {
			left, right = append(left, string(r[:lw])), append(right, string(r[lw:]))
		}
	}
	if l, r := strings.Join(left, "\n"), strings.Join(right, "\n"); !strings.Contains(l, "a.md") || !strings.Contains(r, "this machine") ||
		!strings.Contains(r, "the repository") || !strings.Contains(r, "summary.model") {
		t.Errorf("the settings are not in the right pane beside the folder list:\n%s", screen)
	}
	if f := ansi.Strip(b.footer()); !strings.Contains(f, "enter change") || !strings.Contains(f, "the notes repository this machine works on") {
		t.Errorf("the footer: %s", f)
	}

	pick("weekly")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(read(machine), "weekly = true    # one machine only") || !b.o.Machine.Weekly || b.msg != "weekly = true" {
		t.Fatalf("enter does not flip weekly (%q):\n%s", b.msg, read(machine))
	}

	pick("summary.model")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if b.input == nil || b.inputKind != "setting" {
		t.Fatal("enter on a value does not ask for it")
	}
	press(b, "haiku")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(read(machine), "[summary]\nmodel = \"haiku\"") || b.set == nil {
		t.Fatalf("the prompt does not set summary.model, or closes the settings:\n%s", read(machine))
	}

	pick("pins")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	press(b, "tech")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(read(b.v.Abs(".nt/config.toml")), `pins = ["tech"]`) || strings.Join(b.v.Cfg.Pins, ",") != "tech" {
		t.Fatalf("pins is not set in the repository's file, or not at once: %v", b.v.Cfg.Pins)
	}

	pick("day_starts")
	before := read(b.v.Abs(".nt/config.toml"))
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	run(b, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	press(b, "5")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !b.alert || !strings.Contains(b.msg, "a time like 04:00") || read(b.v.Abs(".nt/config.toml")) != before {
		t.Errorf("a bad value: %q; the file changed: %v", b.msg, read(b.v.Abs(".nt/config.toml")) != before)
	}

	press(b, "j") // j moves in the settings, not in the folder list
	if config.Settings[b.set.sel].Key != "pins" || b.sel != 0 {
		t.Errorf("j selected %s, folder row %d", config.Settings[b.set.sel].Key, b.sel)
	}
	run(b, tea.KeyPressMsg{Code: tea.KeyEscape})
	if b.set != nil {
		t.Error("esc does not close the settings")
	}
}

func TestAgentReportsShowTheirHost(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	day := "agents/2026/2026-10-28"
	for name, body := range map[string]string{
		"work--copilot-cli--vuln-remediation--43fbe292.md": "# vuln-remediation\n",
		"jaromac--claude-code--flok--f0ceca08.md":          "# flok\n",
		"work--claude-code--flok--1a2b3c4d.md":             "# flok\n",
		"README.md":                                        "# Agents\n",
	} {
		p := b.v.Abs(day + "/" + name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hosts := map[string]string{}
	for _, e := range listFolder(b.v, day) {
		hosts[e.name] = e.host
	}
	if hosts["work--copilot-cli--vuln-remediation--43fbe292.md"] != "work" || hosts["jaromac--claude-code--flok--f0ceca08.md"] != "jaromac" || hosts["README.md"] != "" {
		t.Fatalf("hosts %v", hosts)
	}
	// the same project from two machines reads apart
	var rows []string
	for _, e := range listFolder(b.v, day) {
		rows = append(rows, ansi.Strip(b.row(e, false, 60)))
	}
	all := strings.Join(rows, "\n")
	for _, want := range []string{" jaromac flok", " work flok", " work vuln-remediation"} {
		if !strings.Contains(all, want) {
			t.Errorf("no row %q in\n%s", want, all)
		}
	}
	// outside the agents folder a name like that is just a note
	if e := listFolder(b.v, "tech"); len(e) > 0 && e[0].host != "" {
		t.Errorf("a note outside the agents folder got a host: %+v", e[0])
	}
}

func TestHomeList(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	b.v.Now = func() time.Time { return time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local) }
	b.v.Cfg.Pins = []string{"tech", "gone"}
	for f, body := range map[string]string{
		"daily/2026/2026-10-28.md":  "# Wednesday 28 October 2026\n\n## Log\n\n- 08:58 Stand-up\n",
		"weekly/2026/2026-W43.md":   "# Week 43\n",
		"weekly/2026/2026-W44.md":   "# Week 44\n",
		"weekly/2026/2026-W44-a.md": "# Week 44 on a\n",
		"todo.md":                   "# Todo\n\n## Inbox\n\n- [ ] Call the plumber due:2026-10-25\n- [ ] Renew passport\n- [x] Done already\n",
	} {
		if err := os.MkdirAll(filepath.Dir(b.v.Abs(f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(b.v.Abs(f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b.openFolder("")
	b.Update(b.reindex()())
	var ids []string
	for _, e := range b.entries {
		ids = append(ids, firstNonEmpty(e.id(), "─"))
	}
	if got := strings.Join(ids, " "); got != "view:today view:todos view:calendar view:weekly view:agents ─ pin:tech ─ daily tech weekly todo.md" {
		t.Fatalf("the root's list: %s", got)
	}
	// it starts on Today, showing today's journal without making one
	if b.sel != 0 || b.showing != "daily/2026/2026-10-28.md" {
		t.Fatalf("the root starts on %d showing %q", b.sel, b.showing)
	}
	screen := ansi.Strip(b.listPane(44, 20))
	for _, want := range []string{"Today", "t \n", "Todos", "2 · 1 late  x", "Weekly", "w \n", "tech/", "1 \n", "  ────"} {
		if !strings.Contains(screen+"\n", want) {
			t.Errorf("the list lacks %q:\n%s", want, screen)
		}
	}
	// j and k step over the rules; a view shows its note, or says what enter does
	press(b, "jjj")
	if b.showing != "weekly/2026/2026-W44.md" {
		t.Fatalf("Weekly shows %q", b.showing)
	}
	press(b, "jj")
	if e, _ := b.selected(); e.id() != "pin:tech" {
		t.Fatalf("j from Agents lands on %q", e.id())
	}
	press(b, "k")
	if e, _ := b.selected(); e.id() != "view:agents" || b.showing != "" || !strings.Contains(ansi.Strip(b.previewPane(80, 10)), "no agent sessions today") {
		t.Fatalf("k from the pin lands on %q showing %q", e.id(), b.showing)
	}
	// a click on a rule does nothing
	b.Update(tea.MouseClickMsg{X: 5, Y: 2 + 5, Button: tea.MouseLeft})
	if e, _ := b.selected(); e.id() != "view:agents" {
		t.Fatalf("a click on the rule selected %q", e.id())
	}
	// enter on a view does what its key does; h comes back to the root
	press(b, "k")
	run(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if b.dir != "weekly/2026" || b.showing != "weekly/2026/2026-W44.md" {
		t.Fatalf("enter on Weekly: in %q showing %q", b.dir, b.showing)
	}
	press(b, "hh")
	if e, _ := b.selected(); b.dir != "" || e.id() != "weekly" {
		t.Fatalf("back at the root on %q in %q", e.id(), b.dir)
	}
	press(b, "w")
	if b.dir != "weekly/2026" {
		t.Fatalf("w goes to %q", b.dir)
	}
	// the views and pins are no notes: D, m and R leave them alone
	b.openFolder("")
	press(b, "D")
	if b.ask != nil {
		t.Fatalf("D on Today asks %q", b.ask.text)
	}
	if _, _, ok := b.target(); ok {
		t.Fatal("m and R act on a view's row")
	}
	// a folder below the root has no home list
	b.openFolder("tech")
	if e, _ := b.selected(); e.view != "" || e.rule {
		t.Fatalf("tech starts on %+v", e)
	}
}
