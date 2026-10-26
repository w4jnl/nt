package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/w4jnl/nt/internal/config"
)

func newVault(t *testing.T) *Vault {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	v, err := Open(root, "MBP")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func read(t *testing.T, v *Vault, rel string) string {
	t.Helper()
	b, err := os.ReadFile(v.Abs(rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSlug(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Project Report", "project-report"},
		{"Team-wide manual QA reporting", "team-wide-manual-qa-reporting"},
		{"  Docker → Podman!  ", "docker-podman"},
		{"Café crème brûlée", "cafe-creme-brulee"},
		{"Straße & Ærø", "strasse-aero"},
		{"a--b__c", "a-b-c"},
		{"日本語", ""},
		{"", ""},
		{strings.Repeat("abcd-", 20), strings.TrimSuffix(strings.Repeat("abcd-", 12), "-")},
		{strings.Repeat("x", 70), strings.Repeat("x", 60)},
	} {
		if got := Slug(tc.in); got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if got := Slug(tc.in); len(got) > 60 || strings.Contains(got, "--") {
			t.Errorf("Slug(%q) = %q breaks the rules", tc.in, got)
		}
	}
}

func TestDayStarts(t *testing.T) {
	v := newVault(t)
	for _, tc := range []struct{ at, want string }{
		{"2026-10-26 09:00", "2026-10-26"},
		{"2026-10-26 04:00", "2026-10-26"},
		{"2026-10-26 03:59", "2026-10-25"},
		{"2026-01-01 01:00", "2025-12-31"},
	} {
		if got := v.Day(at(tc.at)).Format("2006-01-02"); got != tc.want {
			t.Errorf("%s belongs to %s, want %s", tc.at, got, tc.want)
		}
	}
	v.Cfg.DayStarts = "00:00"
	if got := v.Day(at("2026-10-26 00:30")).Format("2006-01-02"); got != "2026-10-26" {
		t.Errorf("day_starts 00:00: got %s", got)
	}
}

func TestRelLink(t *testing.T) {
	for _, tc := range []struct{ from, to, want string }{
		{"daily/2026/2026-10-26.md", "daily/2026/2026-10-25.md", "2026-10-25.md"},
		{"daily/2026/2026-01-01.md", "daily/2025/2025-12-31.md", "../2025/2025-12-31.md"},
		{"daily/2026/2026-10-26.md", "agents/2026/2026-10-26/README.md", "../../agents/2026/2026-10-26/README.md"},
		{"daily/2026/2026-10-26.md", "tech/infra/2026-10-26-nas.md", "../../tech/infra/2026-10-26-nas.md"},
		{"README.md", "notes/a b.md", "notes/a%20b.md"},
	} {
		if got := RelLink(tc.from, tc.to); got != tc.want {
			t.Errorf("RelLink(%s, %s) = %s, want %s", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestYAMLString(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Project Report", "Project Report"},
		{"Docker: to Podman", `"Docker: to Podman"`},
		{`say "hi"`, `"say \"hi\""`},
		{"#1 idea", `"#1 idea"`},
		{"- dash", `"- dash"`},
		{"yes", `"yes"`},
		{"2026", `"2026"`},
		{"Café", "Café"},
	} {
		if got := yamlString(tc.in); got != tc.want {
			t.Errorf("yamlString(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestJournalIsTheSameOnEveryDevice(t *testing.T) {
	a, b := newVault(t), newVault(t)
	ra, created, err := a.EnsureJournal(at("2026-10-26 09:00"))
	if err != nil || !created {
		t.Fatal(created, err)
	}
	rb, _, _ := b.EnsureJournal(at("2026-10-26 22:41"))
	if ra != "daily/2026/2026-10-26.md" || ra != rb || read(t, a, ra) != read(t, b, rb) {
		t.Fatalf("journals differ:\n%s\n---\n%s", read(t, a, ra), read(t, b, rb))
	}
	want := `---
title: Monday 26 October 2026
date: 2026-10-26
type: daily
---

# Monday 26 October 2026

[← 2026-10-25](2026-10-25.md) · [2026-10-27 →](2026-10-27.md) · [agents](../../agents/2026/2026-10-26/README.md)

## Log

## Notes
`
	if got := read(t, a, ra); got != want {
		t.Errorf("journal:\n%s\nwant:\n%s", got, want)
	}
	if _, created, _ := a.EnsureJournal(at("2026-10-26 10:00")); created {
		t.Error("an existing journal was created again")
	}
}

func TestAppendAndLink(t *testing.T) {
	v := newVault(t)
	if _, _, err := v.Append("first", at("2026-10-26 09:05")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := v.Append("second\nmore detail\n", at("2026-10-26 09:30")); err != nil {
		t.Fatal(err)
	}
	if _, err := v.LinkFromJournal("NAS [setup]", "tech/2026-10-26-nas.md", at("2026-10-26 10:00")); err != nil {
		t.Fatal(err)
	}
	if _, err := v.LinkFromJournal("NAS again", "tech/2026-10-26-nas.md", at("2026-10-26 10:00")); err != nil {
		t.Fatal(err)
	}
	got := read(t, v, "daily/2026/2026-10-26.md")
	want := `## Log

- 09:05 first
- 09:30 second
  more detail

## Notes

- [NAS \[setup\]](../../tech/2026-10-26-nas.md)
`
	if !strings.HasSuffix(got, want) {
		t.Errorf("journal ends:\n%s\nwant:\n%s", got[strings.Index(got, "## Log"):], want)
	}
	if _, _, err := v.Append("  \n", at("2026-10-26 09:30")); err == nil {
		t.Error("an empty append was accepted")
	}
}

func TestAddToSection(t *testing.T) {
	for _, tc := range []struct{ doc, want string }{
		{"# T\n\n## Log\n\n## Notes\n", "# T\n\n## Log\n\n- x\n\n## Notes\n"},
		{"# T\n\n## Log\n- a\n## Notes\n", "# T\n\n## Log\n- a\n- x\n\n## Notes\n"},
		{"# T\n\n## Log\n\n- a\n\n\n## Notes\n- n\n", "# T\n\n## Log\n\n- a\n- x\n\n## Notes\n- n\n"},
		{"# T\n\nwritten by hand\n", "# T\n\nwritten by hand\n\n## Log\n\n- x\n"},
		{"", "## Log\n\n- x\n"},
		{"## Log\n- a", "## Log\n- a\n- x\n"},
	} {
		if got := string(addToSection([]byte(tc.doc), "Log", "- x")); got != tc.want {
			t.Errorf("%q:\ngot  %q\nwant %q", tc.doc, got, tc.want)
		}
	}
}

func TestNewNote(t *testing.T) {
	v := newVault(t)
	now := at("2026-10-26 14:03")
	rel, err := v.NewNote("Q3: Budget report", "work/q3", now)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "work/q3/2026-10-26-q3-budget-report.md" {
		t.Errorf("path %s", rel)
	}
	if got := read(t, v, rel); got != "---\ntitle: \"Q3: Budget report\"\ndate: 2026-10-26\n---\n\n# Q3: Budget report\n\n" {
		t.Errorf("note:\n%s", got)
	}
	again, _ := v.NewNote("Q3: Budget report", "work/q3", now)
	if again != "work/q3/2026-10-26-q3-budget-report-2.md" {
		t.Errorf("second note with the same title: %s", again)
	}
	inbox, _ := v.NewNote("", "", now)
	if inbox != "inbox/2026-10-26-140300.md" {
		t.Errorf("untitled note in the default folder: %s", inbox)
	}
	if _, err := v.NewNote("x", ".git", now); err == nil {
		t.Error("a note in .git was created")
	}
	j := read(t, v, "daily/2026/2026-10-26.md")
	for _, l := range []string{"- [Q3: Budget report](../../work/q3/2026-10-26-q3-budget-report.md)", "(../../work/q3/2026-10-26-q3-budget-report-2.md)", "(../../inbox/2026-10-26-140300.md)"} {
		if !strings.Contains(j, l) {
			t.Errorf("journal lacks %s:\n%s", l, j)
		}
	}
}

func TestUniqueIgnoresCase(t *testing.T) {
	v := newVault(t)
	_ = WriteFile(v.Abs("tech/Note.md"), []byte("x"))
	if got := v.Unique("tech/note.md"); got != "tech/note-2.md" {
		t.Errorf("got %s", got)
	}
}

func TestRepoTemplateOverrides(t *testing.T) {
	v := newVault(t)
	_ = WriteFile(v.Abs(".nt/templates/note.md"), []byte("# {{.Title}} ({{.Folder}}, {{.Weekday}} {{.Time}}) {{.Rel \"README.md\"}}\n"))
	rel, err := v.NewNote("Plan", "tech/x", at("2026-10-26 14:03"))
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, v, rel); got != "# Plan (tech/x, Monday 14:03) ../../README.md\n" {
		t.Errorf("template output %q", got)
	}
	_ = WriteFile(v.Abs(".nt/templates/note.md"), []byte("{{.Nope}}"))
	if _, err := v.NewNote("Broken", "tech", at("2026-10-26 14:03")); err == nil || !strings.Contains(err.Error(), ".nt/templates/note.md") {
		t.Errorf("a broken template should name itself: %v", err)
	}
}

func TestParallelAppendsAllLand(t *testing.T) {
	v := newVault(t)
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := *v // its own Vault, as separate processes would have
			if _, _, err := w.Append(fmt.Sprintf("entry %02d", i), at("2026-10-26 09:00")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	j := read(t, v, "daily/2026/2026-10-26.md")
	for i := range 40 {
		if !strings.Contains(j, fmt.Sprintf("entry %02d\n", i)) {
			t.Errorf("entry %02d lost", i)
		}
	}
	if m, _ := filepath.Glob(v.Abs("daily/2026/.nt-tmp-*")); len(m) > 0 {
		t.Errorf("temp files left: %v", m)
	}
}

func TestOpenAndFormat(t *testing.T) {
	if _, err := Open(t.TempDir(), "h"); err == nil || !strings.Contains(err.Error(), "nt init") {
		t.Errorf("a folder without .git should point at nt init: %v", err)
	}
	v := newVault(t)
	_ = WriteFile(v.Abs(".nt/config.toml"), []byte("format = 2\n"))
	if _, err := Open(v.Root, "h"); err == nil || !strings.Contains(err.Error(), "update nt") {
		t.Errorf("a newer format should be refused: %v", err)
	}
	_ = WriteFile(v.Abs(".nt/config.toml"), []byte(config.RepoTemplate))
	w, err := Open(v.Root, "Home-MBP")
	if err != nil || w.Cfg.DefaultFolder != "inbox" || w.Host != "home-mbp" {
		t.Errorf("template config: %+v %v", w, err)
	}
}

func TestSaveAsset(t *testing.T) {
	v := newVault(t)
	now := at("2026-10-26 01:30") // before day_starts: still the 25th
	for _, tc := range []struct {
		note, name, ext, want string
	}{
		{"tech/2026-10-20-nas.md", "", ".png", "tech/assets/2026-10-25-image.png"},
		{"tech/2026-10-20-nas.md", "", ".png", "tech/assets/2026-10-25-image-2.png"},
		{"tech/2026-10-20-nas.md", "iptables rules", ".png", "tech/assets/2026-10-25-iptables-rules.png"},
		{"tech/2026-10-20-nas.md", "Screenshot 2026-10-24 at 10.12.03", ".PNG", "tech/assets/2026-10-25-screenshot-2026-10-24-at-10-12-03.png"},
		{"tech/2026-10-20-nas.md", "2026-10-24 rack", ".jpeg", "tech/assets/2026-10-24-rack.jpg"},
		{"daily/2026/2026-10-25.md", "whiteboard", ".png", "daily/2026/assets/2026-10-25-whiteboard.png"},
		{"README.md", "logo", ".svg", "assets/2026-10-25-logo.svg"},
	} {
		got, err := v.SaveAsset(tc.note, tc.name, tc.ext, []byte("data"), now)
		if err != nil || got != tc.want {
			t.Errorf("SaveAsset(%q, %q, %q) = %q, %v; want %q", tc.note, tc.name, tc.ext, got, err, tc.want)
			continue
		}
		if read(t, v, got) != "data" {
			t.Errorf("%s does not hold the data", got)
		}
	}
	if _, err := v.SaveAsset(".nt/templates/note.md", "x", ".png", nil, now); err == nil {
		t.Error("an attachment was written under .nt")
	}
}
