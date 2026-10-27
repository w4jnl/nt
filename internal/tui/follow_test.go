package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/scan"
)

func TestFindLinks(t *testing.T) {
	osc := func(uri, text string) string { return "\x1b]8;id=1;" + uri + "\a" + text + "\x1b]8;;\a" }
	st := func(uri, text string) string { return "\x1b]8;;" + uri + "\x1b\\" + text + "\x1b]8;;\x1b\\" } // ST, not BEL
	lines := []string{
		"  See \x1b[38;2;1;2;3;1m" + osc("tech/nas.md", "the NAS") + "\x1b[m " + osc("tech/nas.md", "tech/nas.md") + " end",
		"  日本 " + st("https://x.org", "web"),
		"  wrapped " + osc("a.md", "start"),
		osc("a.md", "end") + " of it",
		"  no links here",
	}
	got := findLinks(lines)
	want := []linkSpot{
		{0, 6, 13, "tech/nas.md"}, {0, 14, 25, "tech/nas.md"},
		{1, 7, 10, "https://x.org"}, // the wide characters take two cells each
		{2, 10, 15, "a.md"}, {3, 0, 3, "a.md"},
	}
	if len(got) != len(want) {
		t.Fatalf("findLinks = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestHintLabels(t *testing.T) {
	if got := strings.Join(hintLabels(3), " "); got != "a s d" {
		t.Errorf("hintLabels(3) = %s", got)
	}
	labels := hintLabels(30)
	seen := map[string]bool{}
	for _, l := range labels {
		if len(l) != 2 || seen[l] {
			t.Fatalf("hintLabels(30): %v; want 30 distinct two-letter labels", labels)
		}
		seen[l] = true
	}
}

func TestFollowLinks(t *testing.T) {
	b := testBrowser(t, 200, t.TempDir())
	note := "# A\n\nSee [B](2026-10-21-b.md), [the C part](2026-10-22-c.md#c), [web](https://example.org) and [gone](nope.md).\n"
	if err := os.WriteFile(b.v.Abs("tech/2026-10-20-a.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	b.reveal("tech/2026-10-20-a.md")
	dests := map[string]bool{}
	for _, s := range b.p.links {
		dests[s.dest] = true
	}
	for _, d := range []string{"2026-10-21-b.md", "2026-10-22-c.md#c", "https://example.org", "nope.md"} { // as written
		if !dests[d] {
			t.Errorf("the preview does not know the link to %s: %v", d, dests)
		}
	}

	press(b, "f")
	if len(b.p.hints) != 4 || !strings.Contains(ansi.Strip(b.footer()), "follow the link they label") {
		t.Fatalf("f labels %d links", len(b.p.hints))
	}
	label := ""
	for l, s := range b.p.hints {
		if s.dest == "2026-10-21-b.md" {
			label = l
		}
	}
	press(b, label)
	if b.showing != "tech/2026-10-21-b.md" || b.p.hints != nil || len(b.history) != 1 {
		t.Fatalf("typing %q shows %q, history %v", label, b.showing, b.history)
	}

	press(b, "b")
	if b.showing != "tech/2026-10-20-a.md" || len(b.history) != 0 {
		t.Fatalf("b goes back to %q", b.showing)
	}

	// the notes that link to this one, under it, follow too
	b.ix = scan.Build(b.v.Root)
	b.reveal("tech/2026-10-21-b.md")
	press(b, "f")
	for l, s := range b.p.hints {
		if s.dest == "2026-10-20-a.md" {
			press(b, l)
		}
	}
	if b.showing != "tech/2026-10-20-a.md" {
		t.Fatalf("following the Linked from line shows %q", b.showing)
	}

	press(b, "f")
	press(b, "z") // no link has it
	if b.p.hints != nil || b.msg != "no link has the letters z" {
		t.Errorf("a letter no label has: %q", b.msg)
	}

	// a click on a link follows it; the cell is the link's, on screen
	var gone linkSpot
	for _, s := range b.p.links {
		if s.dest == "nope.md" {
			gone = s
		}
	}
	x, y := b.listWidth()+1+gone.from, 2+gone.line-b.p.vp.YOffset()
	b.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	b.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	if b.msg != "tech/nope.md: the link points to nothing" {
		t.Errorf("a click on a link to nothing: %q", b.msg)
	}
	for _, s := range b.p.links {
		if s.dest == "2026-10-22-c.md#c" {
			x, y = b.listWidth()+1+s.from, 2+s.line-b.p.vp.YOffset()
		}
	}
	b.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	b.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	if b.showing != "tech/2026-10-22-c.md" {
		t.Errorf("a click on a link shows %q", b.showing)
	}
}
