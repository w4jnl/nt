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

// run sends msg to the browser, then the todo list's reading of the notes or the move when msg
// started one.
func run(b *browser, msg tea.Msg) {
	if _, cmd := b.Update(msg); cmd != nil {
		switch m := cmd().(type) {
		case todosMsg, movedMsg:
			b.Update(m)
		}
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
