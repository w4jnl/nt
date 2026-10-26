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

func TestHTMLImageBlocks(t *testing.T) {
	for _, tc := range []struct {
		name, body, dest, alt string
	}{
		{"centred banner", "<p align=\"center\"><img src=\"assets/brand/nt-hero.png\" alt=\"nt: notes\" width=\"800\"></p>\n", "assets/brand/nt-hero.png", "nt: notes"},
		{"alone", "<img src='a b.png' alt='A &amp; B'>\n", "a b.png", "A & B"},
		{"self-closing in a div", "<div>\n  <img alt=x src=pic.png />\n</div>\n", "pic.png", "x"},
		{"linked", "<a href=\"https://example.com\"><img src=\"logo.png\"></a>\n", "logo.png", ""},
	} {
		got := imageBlocks([]byte(tc.body))
		if len(got) != 1 || got[0].dest != tc.dest || got[0].alt != tc.alt {
			t.Errorf("%s: got %+v, want %s %q", tc.name, got, tc.dest, tc.alt)
		}
	}
	for name, body := range map[string]string{
		"text beside the image":  "<p>Look: <img src=\"a.png\"></p>\n",
		"two images":             "<p><img src=\"a.png\"><img src=\"b.png\"></p>\n",
		"no src":                 "<p><img alt=\"nothing\"></p>\n",
		"inline in a sentence":   "Look at <img src=\"a.png\"> here.\n",
		"in a code block":        "```html\n<img src=\"a.png\">\n```\n",
		"a different html block": "<details><summary>More</summary>text</details>\n",
	} {
		if got := imageBlocks([]byte(body)); len(got) != 0 {
			t.Errorf("%s: drew %+v", name, got)
		}
	}
}

func TestRenderImagesDrawsHTMLImage(t *testing.T) {
	r, err := New(60, true)
	if err != nil {
		t.Fatal(err)
	}
	note := []byte("<p align=\"center\"><img src=\"hero.png\" alt=\"Hero\" width=\"800\"></p>\n\nPlain markdown follows.\n\n![Mark](mark.png)\n")
	var asked []string
	out, err := r.RenderImages(note, func(dest string, width int) ([]string, bool) {
		asked = append(asked, dest)
		return []string{"ROW-" + dest}, true
	})
	if err != nil {
		t.Fatal(err)
	}
	plain := xansi.Strip(out)
	for _, want := range []string{"  ROW-hero.png\n", "Plain markdown follows.", "  ROW-mark.png"} {
		if !strings.Contains(plain, want) {
			t.Errorf("output lacks %q:\n%s", want, plain)
		}
	}
	if len(asked) != 2 {
		t.Errorf("placer asked for %v, want both images", asked)
	}
}
