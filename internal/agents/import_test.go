package agents

import (
	"errors"
	"os"
	"path"
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

// TestImportContinues follows sessions that go on past their report: on a later day, or running
// again after their report ended. Each part gets a report of its own, in its day's folder, that
// links the one before; the one before is closed and links on.
func TestImportContinues(t *testing.T) {
	v, state := testVault(t)
	cwd := t.TempDir()
	read := func(rel string) *Report {
		t.Helper()
		b, err := os.ReadFile(v.Abs(rel))
		if err != nil {
			t.Fatal(err)
		}
		return ParseReport(b)
	}
	imp := func(rec Record, now string) string {
		t.Helper()
		rel, _, err := Import(v, state, rec, at(now))
		if err != nil {
			t.Fatal(err)
		}
		return rel
	}
	turn := func(day, start, end, text string) Turn {
		return Turn{Started: day + "T" + start + ":00+01:00", Ended: day + "T" + end + ":00+01:00", Text: text}
	}
	expect := func(r *Report, want map[string]string) {
		t.Helper()
		for k, w := range want {
			if got := r.Get(k); got != w {
				t.Errorf("%s = %q, want %q", k, got, w)
			}
		}
	}

	// a session on the 28th, left open, then resumed on the 30th: its start, before a turn
	rec := Record{Version: 1, Agent: "claude-code", Session: "8c3d0bdf-55ee", Cwd: cwd, Created: "2026-10-28T12:59:00+01:00",
		Updated: "2026-10-28T13:05:00+01:00", State: "idle", Turns: 1, Tools: 4, Log: []Turn{turn("2026-10-28", "13:00", "13:05", "Morning work.")}}
	first := imp(rec, "2026-10-28 13:05")
	rec.Updated = "2026-10-30T20:09:30+01:00"
	next := imp(rec, "2026-10-30 20:09")
	if !strings.HasPrefix(next, "agents/2026/2026-10-30/") || path.Base(next) != path.Base(first) {
		t.Fatalf("the 30th's report is %s, after %s", next, first)
	}
	expect(read(next), map[string]string{"date": "2026-10-30", "created": "2026-10-30T20:09:30+01:00", "status": "in-progress",
		"continues": "../2026-10-28/" + path.Base(first), "turns": "1", "tools": "4", "ended": ""})
	if got := read(next).Section("Log"); got != "- 20:09 started in "+cwd {
		t.Errorf("the 30th's Log: %q", got)
	}
	earlier := read(first)
	expect(earlier, map[string]string{"ended": "2026-10-28T13:05:00+01:00", "status": "done", "created": "2026-10-28T12:59:00+01:00"})
	if want := "- 12:59 started in " + cwd + "\n- 13:05 Morning work.\n- 13:05 continued in [2026-10-30](../2026-10-30/" + path.Base(next) + ")"; earlier.Section("Log") != want {
		t.Errorf("the 28th's Log:\n%s\nwant\n%s", earlier.Section("Log"), want)
	}
	closed, _ := os.ReadFile(v.Abs(first))

	// three records of the 30th's turns fill the one report, and leave the 28th's alone
	for _, end := range []string{"20:15", "20:40", "21:48"} {
		rec.Turns++
		rec.Log = append(rec.Log, turn("2026-10-30", "20:10", end, "Until "+end+"."))
		rec.Updated = "2026-10-30T" + end + ":00+01:00"
		if rel := imp(rec, "2026-10-30 "+end); rel != next {
			t.Fatalf("a record at %s went to %s", end, rel)
		}
	}
	if b, _ := os.ReadFile(v.Abs(first)); string(b) != string(closed) {
		t.Errorf("the 28th's report changed:\n%s", b)
	}
	if got, want := read(next).Section("Log"), "- 20:09 started in "+cwd+"\n- 20:15 Until 20:15.\n- 20:40 Until 20:40.\n- 21:48 Until 21:48."; got != want {
		t.Errorf("the 30th's Log:\n%s\nwant\n%s", got, want)
	}
	if got, err := Find(v, state, Selector{Cwd: cwd}); err != nil || got != next {
		t.Errorf("nt agent write finds %q, %v", got, err)
	}
	rec.Ended, rec.EndReason, rec.Updated = "2026-10-30T21:50:00+01:00", "exit", "2026-10-30T21:50:00+01:00"
	if rel := imp(rec, "2026-10-30 21:50"); rel != next || read(next).Get("status") != "done" || read(next).Get("ended") != rec.Ended {
		t.Errorf("the end: %s, status %q ended %q", rel, read(next).Get("status"), read(next).Get("ended"))
	}

	// flok first saw a session at its resume: nt's old hooks wrote the 28th's report
	old, err := start(v, state, "claude-code", "a1b2c3d4-hooks", cwd, at("2026-10-28 13:43"))
	if err != nil {
		t.Fatal(err)
	}
	r := read(old)
	r.SetSection("Log", r.Section("Log")+"\n- 13:50 From the hooks.")
	r.Set("updated", "2026-10-28T13:50:00+01:00")
	r.Set("model", "claude-fable-5")
	if err := os.WriteFile(v.Abs(old), r.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	hooked := Record{Version: 1, Agent: "claude-code", Session: "a1b2c3d4-hooks", Cwd: cwd, Created: "2026-10-30T20:09:30+01:00",
		Updated: "2026-10-30T20:20:00+01:00", Turns: 1, Log: []Turn{turn("2026-10-30", "20:10", "20:20", "Tonight.")}}
	resumed := imp(hooked, "2026-10-30 20:20")
	if !strings.HasPrefix(resumed, "agents/2026/2026-10-30/") {
		t.Fatalf("the resumed part is at %s", resumed)
	}
	expect(read(resumed), map[string]string{"created": "2026-10-30T20:09:30+01:00", "date": "2026-10-30", "model": "claude-fable-5"})
	if got := read(resumed).Section("Log"); got != "- 20:09 started in "+cwd+"\n- 20:20 Tonight." {
		t.Errorf("the resumed part's Log: %q", got)
	}
	earlier = read(old)
	expect(earlier, map[string]string{"created": stamp(at("2026-10-28 13:43")), "ended": "2026-10-28T13:50:00+01:00", "status": "done"})
	if !strings.Contains(earlier.Section("Log"), "\n- 13:50 From the hooks.\n- 13:50 continued in [2026-10-30]") {
		t.Errorf("the hooks' lines stay:\n%s", earlier.Section("Log"))
	}

	// the same day: flok still has the session ended, so a late turn adds its line; run again,
	// it goes on in a new report beside it
	for _, c := range []struct {
		name, ended string
		apart       bool
	}{{"ended000", "2026-10-29T14:06:00+01:00", false}, {"running0", "", true}} {
		rec := Record{Version: 1, Agent: "claude-code", Session: c.name + "-same-day", Cwd: cwd, Created: "2026-10-29T13:47:00+01:00",
			Updated: "2026-10-29T14:06:00+01:00", Ended: "2026-10-29T14:06:00+01:00", EndReason: "exit", Turns: 1,
			Log: []Turn{turn("2026-10-29", "13:48", "14:05", "Shipped.")}}
		rel := imp(rec, "2026-10-29 14:06")
		rec.Ended, rec.EndReason, rec.Updated, rec.Turns = c.ended, "", "2026-10-29T14:20:00+01:00", 2
		rec.Log = append(rec.Log, turn("2026-10-29", "14:10", "14:20", "Once more."))
		got := imp(rec, "2026-10-29 14:20")
		if (got != rel) != c.apart {
			t.Errorf("%s: %s after %s", c.name, got, rel)
		}
		if !c.apart {
			if !strings.Contains(read(rel).Section("Log"), "- 14:20 Once more.") || read(rel).Get("status") != "done" {
				t.Errorf("%s: the late turn:\n%s", c.name, read(rel).Section("Log"))
			}
			continue
		}
		if !strings.HasSuffix(got, "-2.md") || path.Dir(got) != path.Dir(rel) || read(got).Get("continues") != path.Base(rel) {
			t.Errorf("%s: %s continues %q", c.name, got, read(got).Get("continues"))
		}
		if l := read(got).Section("Log"); strings.Contains(l, "Shipped.") || !strings.Contains(l, "- 14:20 Once more.") {
			t.Errorf("%s: the new report's Log:\n%s", c.name, l)
		}
		if l := read(rel).Section("Log"); !strings.HasSuffix(l, "- 14:06 ended\n- 14:06 continued in [2026-10-29]("+path.Base(got)+")") {
			t.Errorf("%s: the earlier Log:\n%s", c.name, l)
		}
	}

	// past midnight, before day_starts, it is still the same day
	late := Record{Version: 1, Agent: "claude-code", Session: "late-night", Cwd: cwd, Created: "2026-10-28T23:00:00+01:00",
		Updated: "2026-10-28T23:30:00+01:00", Turns: 1, Log: []Turn{turn("2026-10-28", "23:00", "23:30", "Late.")}}
	rel := imp(late, "2026-10-28 23:30")
	late.Turns, late.Updated = 2, "2026-10-29T01:30:00+01:00"
	late.Log = append(late.Log, turn("2026-10-29", "01:00", "01:30", "Later."))
	if got := imp(late, "2026-10-29 01:30"); got != rel {
		t.Errorf("a turn at 01:00 went to %s, not %s", got, rel)
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
