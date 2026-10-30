package agents

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRecord(t *testing.T) {
	if rec, err := ParseRecord([]byte(`{"version":1,"agent":"claude-code","session":"s1","future":{"x":1},"log":[{"text":"hi","later":true}]}`)); err != nil || rec.Session != "s1" || rec.Log[0].Text != "hi" {
		t.Errorf("unknown fields are ignored: %+v %v", rec, err)
	}
	if _, err := ParseRecord([]byte(`{"version":2,"agent":"claude-code","session":"s1"}`)); !errors.Is(err, ErrNewerRecord) {
		t.Errorf("a newer version is refused as newer: %v", err)
	}
	for _, bad := range []string{`{"agent":"claude-code","session":"s1"}`, `{"version":1,"session":"s1"}`, `{"version":1,"agent":"a"}`, `not json`} {
		if _, err := ParseRecord([]byte(bad)); err == nil || errors.Is(err, ErrNewerRecord) {
			t.Errorf("%s: %v", bad, err)
		}
	}
}

// TestImport follows one session through flok's records: the report made at the first, its
// frontmatter and Log rewritten from each, the agent's own account and status kept, the status
// at the end, and turns flok no longer holds kept.
func TestImport(t *testing.T) {
	v, state := testVault(t)
	cwd := t.TempDir()
	rec := Record{Version: 1, Agent: "claude-code", Session: "F0CECA08-e88a-4f70", Cwd: cwd, Branch: "main", Head: "1553630",
		Model: "claude-fable-5-1", Created: "2026-10-29T13:47:44.512036+01:00", Updated: "2026-10-29T13:47:44.512036+01:00", State: "working"}
	rel, ended, err := Import(v, state, rec, at("2026-10-29 13:48"))
	if err != nil || ended {
		t.Fatal(err, ended)
	}
	if want := "agents/2026/2026-10-29/mbp--claude-code--" + filepath.Base(cwd) + "--f0ceca08.md"; rel != strings.ToLower(want) && rel != want {
		t.Fatalf("the report is at %s, want %s", rel, want)
	}
	read := func() *Report {
		b, err := os.ReadFile(v.Abs(rel))
		if err != nil {
			t.Fatal(err)
		}
		return ParseReport(b)
	}
	r := read()
	for k, want := range map[string]string{"date": "2026-10-29", "created": "2026-10-29T13:47:44+01:00", "host": "mbp", "status": "in-progress",
		"model": "claude-fable-5-1", "git_branch": "main", "git_head": "1553630", "turns": "0", "tools": "0", "blocked_minutes": ""} {
		if got := r.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if got := r.Section("Log"); got != "- 13:47 started in "+cwd+" (main)" {
		t.Errorf("the first Log: %q", got)
	}

	// a turn, then a block in the next one while it runs
	rec.Turns, rec.Tools, rec.BlockedS, rec.State, rec.Updated = 2, 15, 95, "blocked", "2026-10-29T14:01:00+01:00"
	rec.Log = []Turn{
		{Started: "2026-10-29T13:47:50+01:00", Ended: "2026-10-29T13:52:10+01:00", Text: "Yes, rework it.\nThe nt side agrees.", Tools: map[string]int{"Read": 3, "Bash": 12},
			Events: []TurnEvent{{At: "2026-10-29T13:48:02+01:00", Kind: "question", Text: "Which   branch?"}, {At: "2026-10-29T13:51:00+01:00", Kind: "wait", Text: "2 agents + shell", Seconds: 710},
				{At: "2026-10-29T13:51:30+01:00", Kind: "compaction"}, {At: "2026-10-29T13:51:40+01:00", Kind: "someday"}}},
		{Started: "2026-10-29T14:00:00+01:00", Events: []TurnEvent{{At: "2026-10-29T14:00:30+01:00", Kind: "blocked", Text: "permission: Bash"}}},
	}
	if _, _, err := Import(v, state, rec, at("2026-10-29 14:01")); err != nil {
		t.Fatal(err)
	}
	r = read()
	want := "- 13:47 started in " + cwd + " (main)\n" +
		"- 13:52 Yes, rework it.\n" +
		"  - Bash ×12, Read ×3\n" +
		"  - 13:48 asked: Which branch?\n" +
		"  - 13:51 waited 12 min for 2 agents + shell\n" +
		"  - 13:51 compacted\n" +
		"- 14:00\n" +
		"  - 14:00 blocked: permission: Bash"
	if got := r.Section("Log"); got != want {
		t.Errorf("Log:\n%s\nwant\n%s", got, want)
	}
	if r.Get("status") != "blocked" || r.Get("turns") != "2" || r.Get("tools") != "15" || r.Get("blocked_minutes") != "2" {
		t.Errorf("while blocked: status %s turns %s tools %s blocked %s", r.Get("status"), r.Get("turns"), r.Get("tools"), r.Get("blocked_minutes"))
	}
	if b, _ := os.ReadFile(v.Abs(rel)); !strings.Contains(string(b), "\nturns: 2\n") {
		t.Errorf("numbers are written unquoted:\n%s", b)
	}

	// the agent writes its own account and status: an import keeps both
	if _, err := Write(v, state, Selector{Cwd: cwd}, "## Done\n\n- reworked it\n", "done", "Reworked the reports", at("2026-10-29 14:02")); err != nil {
		t.Fatal(err)
	}
	rec.State, rec.Updated = "working", "2026-10-29T14:03:00+01:00"
	if _, _, err := Import(v, state, rec, at("2026-10-29 14:03")); err != nil {
		t.Fatal(err)
	}
	r = read()
	if r.Section("Done") != "- reworked it" || r.Get("summary") != "Reworked the reports" || r.Get("status") != "done" {
		t.Errorf("the agent's own: done %q summary %q status %q", r.Section("Done"), r.Get("summary"), r.Get("status"))
	}

	// flok drops the oldest turn: its line stays; the end comes
	rec.Log = []Turn{{Started: "2026-10-29T14:00:00+01:00", Ended: "2026-10-29T14:05:00+01:00", Text: "Shipped."}}
	rec.Ended, rec.EndReason, rec.Updated = "2026-10-29T14:06:00+01:00", "", "2026-10-29T14:06:00+01:00"
	_, ended, err = Import(v, state, rec, at("2026-10-29 14:06"))
	if err != nil || !ended {
		t.Fatal("the end", err, ended)
	}
	want = "- 13:47 started in " + cwd + " (main)\n" +
		"- 13:52 Yes, rework it.\n" +
		"  - Bash ×12, Read ×3\n" +
		"  - 13:48 asked: Which branch?\n" +
		"  - 13:51 waited 12 min for 2 agents + shell\n" +
		"  - 13:51 compacted\n" +
		"- 14:05 Shipped.\n" +
		"- 14:06 ended"
	r = read()
	if got := r.Section("Log"); got != want {
		t.Errorf("Log after the end:\n%s\nwant\n%s", got, want)
	}
	if r.Get("ended") != "2026-10-29T14:06:00+01:00" || r.Get("status") != "done" {
		t.Errorf("ended %q status %q", r.Get("ended"), r.Get("status"))
	}
	if _, ended, _ := Import(v, state, rec, at("2026-10-29 14:07")); ended {
		t.Error("the same end again says the session ended again")
	}
	if got, err := Find(v, state, Selector{Cwd: cwd}); err != nil || got != rel {
		t.Errorf("nt agent write finds %q, %v", got, err)
	}
}

func TestImportStatusAndHost(t *testing.T) {
	v, state := testVault(t)
	for _, c := range []struct {
		name, reason string
		turns        int
		want         string
		log          []Turn
	}{{"error", "error", 3, "failed", nil}, {"no turn", "", 0, "abandoned", nil}, {"cleared", "clear", 2, "done", nil},
		{"closed in its first turn", "exit", 1, "abandoned", []Turn{{Started: "2026-10-28T23:11:00+01:00"}}},
		{"one turn ended", "exit", 1, "done", []Turn{{Started: "2026-10-28T23:11:00+01:00", Ended: "2026-10-28T23:12:00+01:00"}}}} {
		rec := Record{Version: 1, Agent: "copilot-cli", Session: "s-" + c.name, Host: "macmini", Cwd: "/Users/jaro/work/api",
			Created: "2026-10-28T23:10:00+01:00", Ended: "2026-10-29T00:20:00+01:00", EndReason: c.reason, Turns: c.turns, Log: c.log}
		rel, _, err := Import(v, state, rec, at("2026-10-29 00:21"))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(v.Abs(rel))
		r := ParseReport(b)
		if r.Get("status") != c.want {
			t.Errorf("%s: status %q, want %q", c.name, r.Get("status"), c.want)
		}
		// a remote host's alias is the host, the created day the folder
		if !strings.HasPrefix(rel, "agents/2026/2026-10-28/macmini--copilot-cli--api--") || r.Get("host") != "macmini" {
			t.Errorf("%s: %s, host %q", c.name, rel, r.Get("host"))
		}
	}
}

func TestMinutes(t *testing.T) {
	for s, want := range map[int]string{0: "", 1: "", 29: "", 30: "1", 89: "1", 95: "2", 710: "12"} {
		if got := minutes(s); got != want {
			t.Errorf("minutes(%d) = %q, want %q", s, got, want)
		}
	}
}

func TestMarkImported(t *testing.T) {
	state := t.TempDir()
	if _, ok := LastImport(state); ok {
		t.Error("an import before any")
	}
	if err := MarkImported(state, at("2026-10-29 14:05")); err != nil {
		t.Fatal(err)
	}
	if got, ok := LastImport(state); !ok || !got.Equal(at("2026-10-29 14:05")) {
		t.Errorf("the last import: %v %v", got, ok)
	}
}
