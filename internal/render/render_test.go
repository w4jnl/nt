package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/glamour/v2/ansi"
	xansi "github.com/charmbracelet/x/ansi"
	astext "github.com/yuin/goldmark/extension/ast"
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

func TestRelativeLinksShowAsRepositoryPaths(t *testing.T) {
	r, err := NewAt(200, true, "daily/2026") // wide: the paths must not wrap
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render([]byte("[NAS](../../tech/2026-10-26-nas.md) and [yesterday](2026-10-25.md) and [web](https://example.com/x)\n\n" +
		"[← 2026-10-25](2026-10-25.md) · [2026-10-27 →](2026-10-27.md) · [agents](../../agents/2026/2026-10-26/README.md)\n"))
	if err != nil {
		t.Fatal(err)
	}
	plain := xansi.Strip(out)
	for _, want := range []string{"tech/2026-10-26-nas.md", "daily/2026/2026-10-25.md", "https://example.com/x", "agents/2026/2026-10-26/README.md"} {
		if !strings.Contains(plain, want) {
			t.Errorf("output lacks %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "ntrepo") || strings.Contains(plain, " /tech") {
		t.Errorf("the base leaked or the link lost its folder:\n%s", plain)
	}
}

func TestTitle(t *testing.T) {
	for _, tc := range []struct{ note, path, want string }{
		{"---\ntitle: NAS setup\n---\n# Other\n", "x.md", "NAS setup"},
		{"# Heading only\n", "x.md", "Heading only"},
		{"no title\n", "notes/2026-10-26-nas-setup.md", "nas-setup"},
		{"---\ntitle: 2023-09-08 105704\n---\n## HPC\n", "notes/2023-09-08-hpc-migration.md", "hpc-migration"},
		{"---\ntitle: 2023-09-08 105704\n---\n# HPC Migration\n", "notes/2023-09-08-hpc-migration.md", "HPC Migration"},
		{"---\ntitle: 2025-11-05 LDAP SPN\n---\n", "x.md", "LDAP SPN"},
		{"---\ntitle: Monday 26 October 2026\n---\n", "daily/2026/2026-10-26.md", "Monday 26 October 2026"},
		{"---\ndate: 2023-04-11\n---\n# Testing\n", "daily/2023/2023-04-11.md", "Testing"},
		{"plain\n", "daily/2026/2026-10-26.md", "2026-10-26"},
	} {
		if got := Title([]byte(tc.note), tc.path); got != tc.want {
			t.Errorf("Title(%q, %s) = %q, want %q", tc.note, tc.path, got, tc.want)
		}
	}
}

func TestImageAloneOnALineOfAParagraph(t *testing.T) {
	body := []byte("For example:\n![iptables](assets/iptables.png)\nand after it.\n\nText with ![inline](x.png) image.\n")
	got := imageBlocks(body)
	if len(got) != 1 || got[0].dest != "assets/iptables.png" || got[0].alt != "iptables" || !got[0].split {
		t.Fatalf("blocks %+v", got)
	}
	r, _ := New(60, true)
	out, err := r.RenderImages(body, func(dest string, width int) ([]string, bool) { return []string{"ROW-" + dest}, true })
	if err != nil {
		t.Fatal(err)
	}
	plain := xansi.Strip(out)
	for _, want := range []string{"For example:", "  ROW-assets/iptables.png\n", "and after it.", "Image: inline"} {
		if !strings.Contains(plain, want) {
			t.Errorf("output lacks %q:\n%s", want, plain)
		}
	}
}

func TestCodeBlockBreaksAtTheEdgeNotAtAWord(t *testing.T) {
	r, err := New(40, true)
	if err != nil {
		t.Fatal(err)
	}
	line1 := "CONTAINER ID  IMAGE                         COMMAND"
	line2 := "0f927365796e  localhost/custom_atime_web:1.0  /run-httpd.sh"
	out, err := r.Render([]byte("Before\n```sh\n" + line1 + "\n\n\t" + line2 + "\n```\nafter `x`\n"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, l := range strings.Split(out, "\n") {
		plain := strings.TrimRight(xansi.Strip(l), " ")
		if strings.Contains(plain, "after") {
			break
		}
		if strings.Contains(l, "48;2;") { // on the sunk background, 36 cells after the margin: 40 less the margins
			if w := xansi.StringWidth(l); w != 38 {
				t.Errorf("row %q is %d cells wide, want 38", plain, w)
			}
			rows = append(rows, strings.TrimPrefix(plain, "  "))
		}
	}
	// the language at the top right, then each line broken at the 34th cell, the tab as four cells
	want := []string{
		"                                 sh",
		" " + strings.TrimRight(line1[:34], " "),
		" " + line1[34:],
		"",
		" " + "    " + line2[:30],
		" " + line2[30:],
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	plain := xansi.Strip(out)
	if strings.Contains(plain, "```") || !strings.Contains(plain, "Before") || !strings.Contains(plain, "after") {
		t.Errorf("the fences showed or the text around the block is missing:\n%s", plain)
	}
}

func TestCodeBlocks(t *testing.T) {
	body := []byte("p\n```sh\na\n```\ntext\n\n~~~~\nb\n~~~~~\n\n- item\n\n  ```\n  nested\n  ```\n\n```\nopen to the end\n")
	got := codeBlocks(body)
	want := []codeBlock{
		{lang: "sh", code: "a\n", start: 2, end: 13},
		{lang: "", code: "b\n", start: 20, end: 32},
		{lang: "", code: "open to the end\n", start: 64, end: 83},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("block %d: got %+v (%q), want %+v", i, got[i], body[got[i].start:got[i].end], want[i])
		}
	}
}

func TestTableBlocks(t *testing.T) {
	body := []byte("p\n| a | b |\n|---|:-:|\n| 1 | 2 |\n\ntext\n\n- item\n\n  | n | m |\n  |---|---|\n  | 3 | 4 |\n\n| h |\n|---|\n")
	got := tableBlocks(body)
	if len(got) != 2 {
		t.Fatalf("got %d tables, want 2: %+v", len(got), got)
	}
	if s := string(body[got[0].start:got[0].end]); s != "| a | b |\n|---|:-:|\n| 1 | 2 |" {
		t.Errorf("the first table's bytes: %q", s)
	}
	if got[0].cells[0][1] != "b" || got[0].cells[1][0] != "1" || got[0].aligns[1] != astext.AlignCenter {
		t.Errorf("the first table's cells: %+v", got[0])
	}
	if s := string(body[got[1].start:got[1].end]); s != "| h |\n|---|" {
		t.Errorf("a header alone takes its delimiter row: %q", s)
	}
}

func TestTableWidths(t *testing.T) {
	for _, c := range []struct {
		name     string
		max, min []int
		avail    int
		want     []int
	}{
		{"all fit", []int{1, 4, 25}, []int{1, 4, 25}, 40, []int{1, 4, 25}},
		{"words fit, the slack shared by what wraps", []int{1, 4, 25, 97, 112}, []int{1, 4, 25, 25, 7}, 84, []int{1, 4, 25, 33, 21}},
		{"long words break, short ones stay whole", []int{1, 4, 25, 97, 112}, []int{1, 4, 25, 25, 7}, 54, []int{1, 4, 11, 19, 19}},
		{"not even the short words fit", []int{1, 4, 25}, []int{1, 4, 25}, 6, []int{1, 2, 3}},
	} {
		got := tableWidths(c.max, c.min, c.avail)
		sum := 0
		for _, w := range got {
			sum += w
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) || sum > c.avail {
			t.Errorf("%s: got %v (sum %d), want %v within %d", c.name, got, sum, c.want, c.avail)
		}
	}
}

func TestTableColumnsKeepTheirWords(t *testing.T) {
	md := "# Risks\n\n| # | Sev | Location | Issue | Effect |\n|---|-----|----------|------:|--------|\n" +
		"| 1 | High | `ou_script_synciq.py:772` | The code replaces the site list with `['riosed-tcs-w1-mgmt…']`. The file changed on 13 Oct 2026. | If the live copy also has this change, only Rio, AMS and HOU get new data. The old OU files of the other sites stay. |\n" +
		"| 2 | High | `ou_script_synciq.py:801` | The code sets `ou` one time, before the loop. | A scan file without a match gets the **OU of the** file before it. |\n\nafter\n"
	for _, c := range []struct {
		width   int
		wantSev string // the header's row, stripped
	}{{100, " # │ Sev  │ Location                  │"}, {70, " # │ Sev  │ Location    │"}} {
		r, err := New(c.width, true)
		if err != nil {
			t.Fatal(err)
		}
		out, err := r.Render([]byte(md))
		if err != nil {
			t.Fatal(err)
		}
		plain := xansi.Strip(out)
		var rows []string
		for _, l := range strings.Split(plain, "\n") {
			if strings.Contains(l, "│") || strings.Contains(l, "┼") {
				rows = append(rows, strings.TrimPrefix(l, "  "))
			}
		}
		if len(rows) < 4 || !strings.HasPrefix(rows[0], c.wantSev) {
			t.Fatalf("width %d: the header row %q, want it to start %q\n%s", c.width, rows[0], c.wantSev, plain)
		}
		if !strings.Contains(rows[2], "│ High │") {
			t.Errorf("width %d: \"High\" is not whole on the first row: %q", c.width, rows[2])
		}
		if c.width == 100 && !strings.Contains(rows[2], "ou_script_synciq.py:772") {
			t.Errorf("width 100: the path is cut: %q", rows[2])
		}
		for _, row := range rows {
			if w := xansi.StringWidth(row); w > c.width-2 {
				t.Errorf("width %d: a row of %d cells: %q", c.width, w, row)
			}
		}
		if !strings.Contains(plain, "after") || !strings.Contains(plain, "Risks") {
			t.Errorf("the text around the table: %s", plain)
		}
	}
	// a right-aligned column, and the markdown in a cell rendered
	r, _ := New(80, true)
	out, _ := r.Render([]byte("| n | word |\n|--:|------|\n| 1 | **bold** `x` |\n| 100 | b |\n"))
	plain := xansi.Strip(out)
	if !strings.Contains(plain, "   1 │ bold") || !strings.Contains(plain, " 100 │ b") {
		t.Errorf("right alignment: %s", plain)
	}
	if n := strings.Count(out, "\x1b[1m") + strings.Count(out, ";1m"); n == 0 {
		t.Errorf("bold in a cell: %q", out)
	}
}
