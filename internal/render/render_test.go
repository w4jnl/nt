package render

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/glamour/v2/ansi"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestSplitFrontmatter(t *testing.T) {
	for _, tc := range []struct{ in, front, body string }{
		{"---\ntitle: A\n---\n# A\n", "title: A\n", "# A\n"},
		{"---\r\ntitle: A\r\n---\r\nbody", "title: A\r\n", "body"},
		{"---\ntitle: A\n...\nbody", "title: A\n", "body"},
		{"# no frontmatter\n", "", "# no frontmatter\n"},
		{"---\nonly a rule, never closed\n", "", "---\nonly a rule, never closed\n"},
		{"text\n---\nnot: front\n---\n", "", "text\n---\nnot: front\n---\n"},
	} {
		f, b := SplitFrontmatter([]byte(tc.in))
		if string(f) != tc.front || string(b) != tc.body {
			t.Errorf("%q: front %q body %q, want %q %q", tc.in, f, b, tc.front, tc.body)
		}
	}
}

func TestRenderHidesFrontmatter(t *testing.T) {
	r, err := New(60, true)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render([]byte("---\ntitle: Secret key\ntags: [x]\n---\n# Hello\n\nworld\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Secret key") || !strings.Contains(out, "Hello") || !strings.Contains(out, "world") {
		t.Fatalf("frontmatter leaked or body missing:\n%s", out)
	}
}

func TestStyleJSONRoundTrips(t *testing.T) {
	for _, dark := range []bool{true, false} {
		b, err := JSON(dark)
		if err != nil {
			t.Fatal(err)
		}
		var s ansi.StyleConfig
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatal(err)
		}
		if s.Link.Color == nil || *s.Link.Color != map[bool]string{true: "#3FD0D4", false: "#0B7A7E"}[dark] {
			t.Errorf("dark=%v: link colour is not the accent token", dark)
		}
	}
}

func TestImageBlocks(t *testing.T) {
	body := []byte("# T\n\n![Chart](assets/a%20b.png)\n\nText with ![inline](x.png) image.\n\n- ![in a list](y.png)\n\n```\n![in code](z.png)\n```\n\n![](<assets/c d.png> \"title\")\nafter\n")
	got := imageBlocks(body)
	if len(got) != 1 {
		t.Fatalf("got %d standalone images, want 1: %+v", len(got), got)
	}
	if b := got[0]; b.dest != "assets/a%20b.png" || b.alt != "Chart" || string(body[b.start:b.end]) != "![Chart](assets/a%20b.png)" {
		t.Errorf("block %+v covers %q", b, body[b.start:b.end])
	}
}

func TestRenderImagesPlacesRows(t *testing.T) {
	r, err := New(60, true)
	if err != nil {
		t.Fatal(err)
	}
	note := []byte("---\ntitle: T\n---\nbefore\n\n![Chart](a.png)\n\n![Remote](https://example.com/b.png)\n\nafter ntimage\n")
	var asked []string
	out, err := r.RenderImages(note, func(dest string, width int) ([]string, bool) {
		asked = append(asked, dest)
		if width != 56 {
			t.Errorf("width %d, want 60 less the 2-cell margins", width)
		}
		if dest != "a.png" {
			return nil, false
		}
		return []string{"ROW0", "ROW1"}, true
	})
	if err != nil {
		t.Fatal(err)
	}
	plain := xansi.Strip(out)
	for _, want := range []string{"  ROW0\n  ROW1\n", "before", "after ntimage", "Remote"} {
		if !strings.Contains(plain, want) {
			t.Errorf("output lacks %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Chart") || strings.Contains(plain, "ntimagex") {
		t.Errorf("the drawn image left its alt text or marker:\n%s", plain)
	}
	if len(asked) != 2 {
		t.Errorf("placer asked for %v, want both images", asked)
	}
}
