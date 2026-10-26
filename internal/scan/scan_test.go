package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/w4jnl/nt/internal/mdparse"
)

func TestParse(t *testing.T) {
	note := "---\ntitle: T\n---\n# Top\n\nSee [NAS](../tech/nas.md#setup) and [web](https://x.org).\n" +
		"![Chart](assets/c.png)\n[[Old Note|label]] and `[[code]]`\n\n```\n[not](a.md)\n## not a heading\n```\n" +
		"## Second ##\n<p><img src=\"hero.png\" width=\"800\"></p>\n"
	d := mdparse.Parse([]byte(note))
	if len(d.Headings) != 2 || d.Headings[0] != (mdparse.Heading{Level: 1, Text: "Top", Line: 4}) || d.Headings[1].Text != "Second" || d.Headings[1].Line != 14 {
		t.Errorf("headings %+v", d.Headings)
	}
	want := []mdparse.Link{
		{Text: "NAS", Dest: "../tech/nas.md#setup", Line: 6},
		{Text: "web", Dest: "https://x.org", Line: 6},
		{Text: "Chart", Dest: "assets/c.png", Line: 7, Image: true},
		{Text: "label", Dest: "Old Note", Line: 8, Wiki: true},
		{Dest: "hero.png", Line: 15, Image: true},
	}
	if len(d.Links) != len(want) {
		t.Fatalf("links %+v", d.Links)
	}
	for i := range want {
		if d.Links[i] != want[i] {
			t.Errorf("link %d = %+v, want %+v", i, d.Links[i], want[i])
		}
	}
}

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		from, dest, rel, anchor string
		ok                      bool
	}{
		{"daily/2026/x.md", "../../tech/a%20b.md#setup", "tech/a b.md", "setup", true},
		{"daily/2026/x.md", "2026-10-25.md", "daily/2026/2026-10-25.md", "", true},
		{"notes/x.md", "#top", "notes/x.md", "top", true},
		{"notes/x.md", "https://example.com", "", "", false},
		{"notes/x.md", "mailto:a@b.c", "", "", false},
		{"notes/x.md", "../../outside.md", "", "", false},
	} {
		rel, anchor, ok := mdparse.Resolve(tc.from, tc.dest)
		if rel != tc.rel || anchor != tc.anchor || ok != tc.ok {
			t.Errorf("Resolve(%s, %s) = %q %q %v", tc.from, tc.dest, rel, anchor, ok)
		}
	}
}

func TestBuildFindsBacklinks(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"tech/nas.md":              "# NAS setup\n",
		"tech/Old Note.md":         "# Old\n",
		"daily/2026/2026-10-26.md": "# Day\n- [NAS](../../tech/nas.md)\n- [NAS again](../../tech/nas.md#x)\n",
		"notes/plan.md":            "---\ntitle: The plan\n---\nsee [[nas]] and [[Old Note]] and [gone](missing.md)\n",
		".git/ignored.md":          "[x](../tech/nas.md)\n",
	}
	for p, s := range files {
		full := filepath.Join(root, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte(s), 0o644)
	}
	ix := Build(root)
	if len(ix.Notes) != 4 {
		t.Errorf("notes %+v", ix.Notes)
	}
	if got := ix.Back["tech/nas.md"]; len(got) != 2 || got[0] != "daily/2026/2026-10-26.md" || got[1] != "notes/plan.md" {
		t.Errorf("backlinks of nas: %v", got)
	}
	if got := ix.Back["tech/Old Note.md"]; len(got) != 1 {
		t.Errorf("a wikilink by file name is a backlink: %v", got)
	}
	if ix.Title("notes/plan.md") != "The plan" {
		t.Errorf("title %q", ix.Title("notes/plan.md"))
	}
}

func TestMatchAndFind(t *testing.T) {
	if _, ok := Match("nsm", "NAS Mount Setup"); !ok {
		t.Error("nsm should match NAS Mount Setup")
	}
	if _, ok := Match("xyz", "NAS Mount Setup"); ok {
		t.Error("xyz should not match")
	}
	a, _ := Match("nas", "NAS Mount Setup")
	b, _ := Match("nas", "Financial analysis")
	if a <= b {
		t.Errorf("a word start should beat letters scattered inside words: %d vs %d", a, b)
	}
	ix := &Index{Notes: []Note{
		{"notes/2023-10-16-docker-to-podman-migration.md", "Docker to Podman migration"},
		{"notes/2026-04-11-nas-mount-setup.md", "NAS Mount Setup"},
		{"technical-notes/2023-03-06-docker-service-override.md", "DockerServiceOverride"},
	}}
	got := ix.Find("docker", 10)
	if len(got) != 2 {
		t.Fatalf("find docker: %+v", got)
	}
	if got := ix.Find("nas mount", 10); len(got) != 1 || got[0].Title != "NAS Mount Setup" {
		t.Errorf("find nas mount: %+v", got)
	}
}
