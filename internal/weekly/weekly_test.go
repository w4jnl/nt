package weekly

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w4jnl/nt/internal/vault"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

// repo is a notes repository in git with files, committed on committed.
func repo(t *testing.T, files map[string]string, committed string) *vault.Vault {
	t.Helper()
	root := t.TempDir()
	write(t, root, files)
	git(t, root, committed, "init", "-q", "-b", "main")
	git(t, root, committed, "add", "-A")
	git(t, root, committed, "commit", "-q", "-m", "notes")
	v, err := vault.Open(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for f, body := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// git runs git in root as if on date (YYYY-MM-DD HH:MM), for the commits' times.
func git(t *testing.T, root, date string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	stamp := at(date).Format(time.RFC3339)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@x", "GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func report(project, status, created string) string {
	return "---\ntype: agent-report\nproject: " + project + "\nagent: claude-code\nhost: h\nstatus: " + status +
		"\ncreated: " + created + "\n---\n\n# " + project + "\n"
}

func TestText(t *testing.T) {
	v := repo(t, map[string]string{
		"daily/2026/2026-10-26.md":                                "# Monday\n\n## Log\n\n- 09:12 Called the bank\n- 10:00 See [NAS](../../tech/2026-10-27-nas-setup.md)\n  continued line\n\n## Notes\n\n- [NAS setup](../../tech/2026-10-27-nas-setup.md)\n",
		"daily/2026/2026-10-28.md":                                "# Wednesday\n\n## Log\n\n## Notes\n",
		"daily/2026/2026-11-02.md":                                "# Next week\n\n## Log\n\n- 08:00 not this week\n",
		"tech/2026-10-27-nas-setup.md":                            "---\ntitle: NAS setup\n---\n\n# NAS setup\n",
		"tech/2026-10-30-nas-setup.conflict-b-20261030T100000.md": "# a conflict copy\n",
		"work/2026-10-01-q3.md":                                   "# Q3 review\n",
		"work/2026-10-02-quiet.md":                                "# Not touched\n",
		"todo.md": "# Todo\n\n## Inbox\n\n- [x] Pay the invoice done:2026-10-27\n- [ ] Renew passport due:2026-10-29\n" +
			"- [ ] Later due:2026-11-20\n- [x] Old done:2026-10-20\n",
		"agents/2026/2026-10-27/h--claude-code--nt--aaaa1111.md": report("nt", "done", "2026-10-27T09:00:00+01:00"),
		"agents/2026/2026-10-27/h--claude-code--nt--bbbb2222.md": report("nt", "blocked", "2026-10-27T10:00:00+01:00"),
	}, "2026-09-01 12:00")
	write(t, v.Root, map[string]string{"work/2026-10-01-q3.md": "# Q3 review\n\nNumbers in.\n", "todo.md": "# Todo\n\n## Inbox\n\n- [x] Pay the invoice done:2026-10-27\n- [ ] Renew passport due:2026-10-29\n- [ ] Later due:2026-11-20\n- [x] Old done:2026-10-20\n"})
	git(t, v.Root, "2026-10-28 12:00", "commit", "-q", "-am", "the week's changes")

	got, filled := Text(v, at("2026-10-29 10:00"))
	want := `---
title: Week 44, 2026
date: 2026-10-26
type: weekly
---

# Week 44 · 26 October – 1 November 2026

[← W43](2026-W43.md) · [W45 →](2026-W45.md)

Compiled by nt from the week's notes; it is written again when they change.

## Notes

- [NAS setup](../../tech/2026-10-27-nas-setup.md) · tech · new
- [Q3 review](../../work/2026-10-01-q3.md) · work · changed

## Todos

- ✓ Pay the invoice · Tue
- ○ Renew passport · due Thu 29 Oct

## Agents

- nt · 2 sessions: 1 blocked, 1 done

## Days

### Monday 26 October

- 09:12 Called the bank
- 10:00 See [NAS](../../tech/2026-10-27-nas-setup.md)
  continued line

[journal](../../daily/2026/2026-10-26.md)

### Tuesday 27 October

[2 agent sessions](../../agents/2026/2026-10-27/README.md)

### Wednesday 28 October

[journal](../../daily/2026/2026-10-28.md)
`
	if !filled || string(got) != want {
		t.Errorf("Text:\n%s\nwant:\n%s", got, want)
	}

	changed, err := Write(v, at("2026-10-26 10:00"))
	if err != nil || !changed {
		t.Fatalf("Write: %v, %v", changed, err)
	}
	if changed, _ := Write(v, at("2026-10-27 10:00")); changed {
		t.Error("the same notes wrote the digest again")
	}
	if b, _ := os.ReadFile(v.Abs("weekly/2026/2026-W44.md")); string(b) != want {
		t.Errorf("the file:\n%s", b)
	}
}

func TestRefresh(t *testing.T) {
	v := repo(t, map[string]string{
		"daily/2026/2026-10-28.md": "# Wednesday\n\n## Log\n\n- 09:00 this week\n",
		"weekly/2026/2026-W40.md":  "an old digest\n",
		"daily/2026/2026-10-06.md": "# W41\n\n## Log\n\n- 09:00 missed\n",
		"daily/2026/2026-10-14.md": "# W42\n\n## Log\n\n- 09:00 missed too\n",
	}, "2026-09-01 12:00")
	got, err := Refresh(v, at("2026-10-28 10:00"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "2026-W41 2026-W42 2026-W44" {
		t.Errorf("Refresh wrote %v; want the missed weeks and this one, not the empty W43", got)
	}
	if got, _ := Refresh(v, at("2026-10-28 10:02")); len(got) != 0 {
		t.Errorf("a second run wrote %v", got)
	}
	if b, _ := os.ReadFile(v.Abs("weekly/2026/2026-W40.md")); string(b) != "an old digest\n" {
		t.Error("an older week was written again")
	}
}

func TestISOMonday(t *testing.T) {
	for _, c := range []struct {
		y, w int
		want string
	}{{2026, 44, "2026-10-26"}, {2026, 1, "2025-12-29"}, {2026, 53, "2026-12-28"}, {2025, 1, "2024-12-30"}} {
		if got := isoMonday(c.y, c.w).Format("2006-01-02"); got != c.want {
			t.Errorf("isoMonday(%d, %d) = %s, want %s", c.y, c.w, got, c.want)
		}
	}
}

func TestBulkChanges(t *testing.T) {
	files := map[string]string{"daily/2026/2026-10-27.md": "# Day\n\n## Log\n\n- 09:00 a day\n"}
	for i := range 22 {
		files[fmt.Sprintf("old/2020-01-%02d-n.md", i+1)] = "# Old\n"
	}
	v := repo(t, files, "2026-09-01 12:00")
	edited := map[string]string{}
	for i := range 21 {
		edited[fmt.Sprintf("old/2020-01-%02d-n.md", i+1)] = "# Old\n\nmoved links\n"
	}
	write(t, v.Root, edited)
	git(t, v.Root, "2026-10-27 12:00", "commit", "-q", "-am", "a move rewrote links")
	write(t, v.Root, map[string]string{"old/2020-01-22-n.md": "# Old\n\nwritten\n"})
	git(t, v.Root, "2026-10-28 12:00", "commit", "-q", "-am", "an edit")
	got, _ := Text(v, at("2026-10-27 10:00"))
	want := "## Notes\n\n- [Old](../../old/2020-01-22-n.md) · old · changed\n- 21 more notes changed together, by an import or a move\n"
	if !strings.Contains(string(got), want) {
		t.Errorf("a bulk commit's notes are not counted apart:\n%s", got)
	}
}
