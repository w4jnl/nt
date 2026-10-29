package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestGoFindsWhatRgFinds: without rg, as on a phone, the same lines are found: smart case,
// markdown files only, hidden folders and temp files left out.
func TestGoFindsWhatRgFinds(t *testing.T) {
	root := t.TempDir()
	for p, body := range map[string]string{
		"homelab/nas.md":       "# NAS setup\n\nThe NAS is on VLAN 10.\n- [ ] Replace the fan in bay 2\n",
		"work/net.md":          "# Network\n\nvlan 20 is for guests; the nas sits on vlan 10\n",
		"notes.txt":            "vlan in a text file\n",
		".hidden/x.md":         "vlan in a hidden folder\n",
		"homelab/.nt-tmp-1.md": "vlan in a temp file\n",
		"daily/2026/a.md":      "fan in bay 2 is loud again\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lines := func(ms []Match) string {
		var out []string
		for _, m := range ms {
			out = append(out, m.Path+":"+strconv.Itoa(m.Line)+":"+m.Text)
		}
		sort.Strings(out)
		return strings.Join(out, "\n")
	}
	want := map[string]string{
		"vlan":       "homelab/nas.md:3:The NAS is on VLAN 10.\nwork/net.md:3:vlan 20 is for guests; the nas sits on vlan 10",
		"NAS":        "homelab/nas.md:1:# NAS setup\nhomelab/nas.md:3:The NAS is on VLAN 10.",
		"fan.*bay 2": "daily/2026/a.md:1:fan in bay 2 is loud again\nhomelab/nas.md:4:- [ ] Replace the fan in bay 2",
	}
	_, rgErr := exec.LookPath("rg")
	for q, w := range want {
		got, err := Go(context.Background(), root, q)
		if err != nil || lines(got) != w {
			t.Errorf("Go(%q) = %v\n%s\nwant\n%s", q, err, lines(got), w)
		}
		if rgErr == nil {
			rg, err := Rg(context.Background(), root, q)
			if err != nil || lines(rg) != lines(got) {
				t.Errorf("%q: rg finds\n%s\nGo finds\n%s", q, lines(rg), lines(got))
			}
		}
	}
	if m, err := Go(context.Background(), root, "VLAN"); err != nil || len(m) != 1 || len(m[0].Spans) != 1 {
		t.Errorf("a capital makes the search case-sensitive: %v %+v", err, m)
	}
	if _, err := Go(context.Background(), root, "["); err == nil {
		t.Error("a query that is no regular expression is not refused")
	}
}

// TestTagsKeepASearchToTheirNotes: #tag words keep to the notes with every one of the tags, or
// one under them; alone, one line per note, newest first; with words, their lines in those
// notes, with rg and in Go alike.
func TestTagsKeepASearchToTheirNotes(t *testing.T) {
	root := t.TempDir()
	for p, body := range map[string]string{
		"homelab/2026-10-22-nas.md": "---\ntags: [homelab, docker]\n---\n# NAS\n\nrestic backs it up at 02:00\n",
		"homelab/2026-10-25-net.md": "# Network\n\nThe VLANs. #homelab/net\nrestic is not here\n",
		"daily/2026/2026-10-28.md":  "# Day\n\n- 09:00 moved the nas to #Docker compose\n",
		"work/q3.md":                "---\ntags:\n  - work/q3\n---\nrestic for work\n",
		"work/plain.md":             "restic, untagged\n",
		"homelab/.nt-tmp-1.md":      "#homelab in a temp file\n",
		".hidden/2026-10-29-x.md":   "#homelab in a hidden folder\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	in := func(ms []Match) string {
		var out []string
		for _, m := range ms {
			out = append(out, m.Path+":"+strconv.Itoa(m.Line))
		}
		return strings.Join(out, " ")
	}
	sorted := func(ms []Match) string {
		s := strings.Fields(in(ms))
		sort.Strings(s)
		return strings.Join(s, " ")
	}
	if tags, words := Split("#Docker  restic #work/  2026"); strings.Join(tags, ",") != "docker,work" || words != "restic 2026" {
		t.Errorf("Split = %q %q", tags, words)
	}
	if tags, words := Split("#12 C# #"); len(tags) != 0 || words != "#12 C# #" {
		t.Errorf("Split keeps what is no tag as words: %q %q", tags, words)
	}
	ctx := context.Background()
	for q, want := range map[string]string{
		"#homelab":         "homelab/2026-10-25-net.md:3 homelab/2026-10-22-nas.md:2",
		"#docker":          "daily/2026/2026-10-28.md:3 homelab/2026-10-22-nas.md:2",
		"#homelab #docker": "homelab/2026-10-22-nas.md:2",
		"#work":            "work/q3.md:3",
		"#nothing":         "",
	} {
		got, err := Search(ctx, root, q)
		if err != nil || in(got) != want {
			t.Errorf("Search(%q) = %v %q, want %q", q, err, in(got), want)
		}
	}
	got, _ := Search(ctx, root, "#docker")
	if len(got) == 0 || got[0].Text != "- 09:00 moved the nas to #Docker compose" || len(got[0].Spans) != 1 || got[0].Text[got[0].Spans[0][0]:got[0].Spans[0][1]] != "Docker" {
		t.Errorf("a tag's line is the one that tags the note, with the tag marked: %+v", got)
	}
	// with words: their lines in the tagged notes, in Go and with rg
	want := "homelab/2026-10-22-nas.md:6 homelab/2026-10-25-net.md:4"
	if got, err := goSearch(ctx, root, "restic", []string{"homelab/2026-10-22-nas.md", "homelab/2026-10-25-net.md"}); err != nil || sorted(got) != want {
		t.Errorf("Go in the tagged notes = %v %q, want %q", err, sorted(got), want)
	}
	if _, err := exec.LookPath("rg"); err == nil {
		if got, err := rg(ctx, root, "restic", []string{"homelab/2026-10-22-nas.md", "homelab/2026-10-25-net.md"}); err != nil || sorted(got) != want {
			t.Errorf("rg in the tagged notes = %v %q, want %q", err, sorted(got), want)
		}
	}
	if got, err := Search(ctx, root, "#homelab restic"); err != nil || sorted(got) != want {
		t.Errorf("Search(#homelab restic) = %v %q, want %q", err, sorted(got), want)
	}
}
