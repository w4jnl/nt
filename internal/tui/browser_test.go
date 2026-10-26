package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

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
