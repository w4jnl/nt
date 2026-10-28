package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestLocalImage(t *testing.T) {
	for _, tc := range []struct {
		dest, want string
		ok         bool
	}{
		{"assets/a.png", "/n/topic/assets/a.png", true},
		{"assets/a%20b.png", "/n/topic/assets/a b.png", true},
		{"../img/x.jpg", "/n/img/x.jpg", true},
		{"/abs/x.png", "/abs/x.png", true},
		{"https://example.com/x.png", "", false},
		{"data:image/png;base64,AAAA", "", false},
		{"", "", false},
	} {
		got, ok := localImage("/n/topic/note.md", tc.dest)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%q: got %q %v, want %q %v", tc.dest, got, ok, tc.want, tc.ok)
		}
	}
}

// testViewer is the viewer on a.md in a folder with b.md, notes.txt and a picture, on a screen
// of 100 by 30.
func testViewer(t *testing.T) (*viewer, string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.md":      "# A\n\nSee [b](b.md#later), [the list](notes.txt), [x](x.png) and [gone](gone.md).\n\n## Section\n\nText.\n",
		"b.md":      "# B\n\nback to [a](a.md)\n\n## Later\n\nHere.\n",
		"notes.txt": "first line\nsecond line\n# not a heading\n\tindented\n",
		"x.png":     "\x89PNG",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := newViewer(ViewOptions{Path: filepath.Join(dir, "a.md"), Root: dir, Mode: "dark", Images: "off", ImageViewer: "true"})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, dir
}

func TestViewerFollowsLinks(t *testing.T) {
	m, dir := testViewer(t)
	key := func(s string) { m.Update(tea.KeyPressMsg{Code: rune(s[0]), Text: s}) }
	key("f")
	if len(m.p.hints) != 4 {
		t.Fatalf("f labels %d links, want 4", len(m.p.hints))
	}
	key("a") // the first label: b.md#later
	if m.p.hints != nil || m.p.path != filepath.Join(dir, "b.md") {
		t.Fatalf("the label a goes to %s", m.p.path)
	}
	m.follow("notes.txt")
	if m.p.path != filepath.Join(dir, "notes.txt") || len(m.history) != 2 {
		t.Fatalf("a text file is not shown here: %s, history %d", m.p.path, len(m.history))
	}
	key("b")
	key("b")
	if m.p.path != filepath.Join(dir, "a.md") || len(m.history) != 0 {
		t.Fatalf("b twice comes back to %s", m.p.path)
	}
	m.follow("gone.md")
	if !m.alert || !strings.Contains(m.message, "points to nothing") || m.p.path != filepath.Join(dir, "a.md") {
		t.Fatalf("a link to nothing: %q, now on %s", m.message, m.p.path)
	}
	m.follow("x.png") // an image opens with the image viewer (true here) and the note stays
	if m.p.path != filepath.Join(dir, "a.md") || len(m.history) != 0 {
		t.Fatalf("an image is shown in the viewer: %s", m.p.path)
	}
	key("o")
	if m.pick == nil || len(m.pick.items) != 2 {
		t.Fatalf("o: %+v", m.pick)
	}
	m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.pick != nil || m.p.vp.YOffset() == 0 && m.p.vp.TotalLineCount() > m.p.height {
		t.Fatal("picking Section does not close the outline")
	}
	if f := ansi.Strip(m.footer()); !strings.Contains(f, "f follow") || !strings.Contains(f, "b back") {
		t.Fatalf("the footer: %s", f)
	}
}

func TestViewerShowsTextAsWritten(t *testing.T) {
	m, dir := testViewer(t)
	m.follow("notes.txt")
	body := ansi.Strip(m.p.body)
	for _, want := range []string{"\n  first line\n  second line\n  # not a heading\n      indented\n"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the text file is not as written:\n%s", body)
		}
	}
	if m.p.title != "notes.txt" {
		t.Errorf("a text file's title is %q", m.p.title)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("\x1b]0;owned\x07plain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.p.load()
	m.p.render()
	if strings.Contains(m.p.body, "\x1b]0;") || !strings.Contains(ansi.Strip(m.p.body), "plain") {
		t.Fatalf("an escape sequence in a text file reaches the terminal: %q", m.p.body)
	}
}
