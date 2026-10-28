package agents

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/w4jnl/nt/internal/vault"
)

func testVault(t *testing.T) (*vault.Vault, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(root, "mbp")
	if err != nil {
		t.Fatal(err)
	}
	return v, t.TempDir()
}

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseEvent(t *testing.T) {
	for _, tc := range []struct {
		name, kind, payload, agent string
		want                       Event
		ok                         bool
	}{
		{"claude start", "start", `{"hook_event_name":"SessionStart","session_id":"00893aaf-19fa","cwd":"/p","source":"startup","model":"claude-opus-5-5"}`, "",
			Event{Kind: "start", Agent: "claude-code", Session: "00893aaf-19fa", Cwd: "/p", Model: "claude-opus-5-5"}, true},
		{"claude stop", "turn", `{"hook_event_name":"Stop","session_id":"s1","cwd":"/p","last_assistant_message":"Done.\nMore"}`, "",
			Event{Kind: "turn", Agent: "claude-code", Session: "s1", Cwd: "/p", Message: "Done.\nMore"}, true},
		{"claude end", "end", `{"hook_event_name":"SessionEnd","session_id":"s1","cwd":"/p","reason":"prompt_input_exit"}`, "",
			Event{Kind: "end", Agent: "claude-code", Session: "s1", Cwd: "/p", Reason: "prompt_input_exit"}, true},
		{"claude subagent", "turn", `{"hook_event_name":"SubagentStop","session_id":"s1","agent_id":"a1","cwd":"/p"}`, "", Event{}, false},
		{"copilot start", "start", `{"sessionId":"c1","timestamp":1761470000000,"cwd":"/q"}`, "",
			Event{Kind: "start", Agent: "copilot-cli", Session: "c1", Cwd: "/q"}, true},
		{"copilot stop", "turn", `{"sessionId":"c1","cwd":"/q","transcriptPath":"/t","stopReason":"end_turn"}`, "copilot-cli",
			Event{Kind: "turn", Agent: "copilot-cli", Session: "c1", Cwd: "/q", Reason: "end_turn"}, true},
		{"copilot end", "end", `{"sessionId":"c1","cwd":"/q","reason":"error"}`, "",
			Event{Kind: "end", Agent: "copilot-cli", Session: "c1", Cwd: "/q", Reason: "error"}, true},
		{"no session", "turn", `{"cwd":"/q"}`, "", Event{}, false},
	} {
		got, ok, err := ParseEvent(tc.kind, []byte(tc.payload), tc.agent)
		if err != nil || ok != tc.ok || ok && got != tc.want {
			t.Errorf("%s: got %+v, %v, %v; want %+v, %v", tc.name, got, ok, err, tc.want, tc.ok)
		}
	}
	if _, _, err := ParseEvent("turn", []byte("{not json"), ""); err == nil {
		t.Error("a broken payload is not an error")
	}
	if _, _, err := ParseEvent("stop", nil, ""); err == nil {
		t.Error("an unknown kind is not an error")
	}
}

func TestTemporary(t *testing.T) {
	home, _ := os.UserHomeDir()
	cases := map[string]bool{
		"/tmp":                       true,
		"/tmp/probe":                 true,
		"/var/tmp/scratch":           true,
		os.TempDir():                 true,
		filepath.Join(t.TempDir()):   true,
		"/tmpfiles/project":          false,
		filepath.Join(home, "notes"): false,
		"/Volumes/work/git/vuln":     false,
		"":                           false,
	}
	if runtime.GOOS == "darwin" {
		cases["/private/tmp/claude-501/x"] = true // /tmp is /private/tmp, as a session's cwd says
	}
	for dir, want := range cases {
		if got := Temporary(dir); got != want {
			t.Errorf("Temporary(%q) = %v, want %v", dir, got, want)
		}
	}
	// a session in a temporary folder gets no report, unless NT_REPORT_TEMP asks for one
	payload := []byte(`{"session_id":"s1","cwd":"/tmp/probe"}`)
	if _, ok, _ := ParseEvent("start", payload, ""); ok {
		t.Error("a throwaway session in /tmp is reported")
	}
	t.Setenv("NT_REPORT_TEMP", "1")
	if _, ok, _ := ParseEvent("start", payload, ""); !ok {
		t.Error("NT_REPORT_TEMP=1 does not report a session in /tmp")
	}
}

func TestLogLine(t *testing.T) {
	long := strings.Repeat("word ", 60)
	for in, want := range map[string]string{
		"## Summary\n\nAll   tests pass.\n": "Summary",
		"\n\n- built the thing\n":           "built the thing",
		"```go\ncode\n```":                  "code",
		long:                                strings.TrimSpace(long)[:199] + "…",
		"":                                  "",
		"> quoted **answer** with  spaces ": "quoted **answer** with spaces",
	} {
		if got := logLine(in); got != want {
			t.Errorf("logLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func read(t *testing.T, v *vault.Vault, rel string) *Report {
	t.Helper()
	b, err := os.ReadFile(v.Abs(rel))
	if err != nil {
		t.Fatal(err)
	}
	return ParseReport(b)
}

func TestSession(t *testing.T) {
	v, state := testVault(t)
	cwd := filepath.Join(t.TempDir(), "noteTaking")
	_ = os.Mkdir(cwd, 0o755)
	ev := func(kind, msg, reason string) Event {
		return Event{Kind: kind, Agent: "claude-code", Session: "00893AAF-19fa-41d2", Cwd: cwd, Message: msg, Reason: reason}
	}
	rel, err := Apply(v, state, ev("start", "", ""), at("2026-10-26 23:50"))
	if err != nil {
		t.Fatal(err)
	}
	if rel != "agents/2026/2026-10-26/mbp--claude-code--notetaking--00893aaf.md" {
		t.Fatalf("path %s", rel)
	}
	// past midnight the session stays in its first day's report
	if again, _ := Apply(v, state, ev("turn", "Built the digest.\nDetails follow.", ""), at("2026-10-27 05:10")); again != rel {
		t.Fatalf("the next day's turn went to %s", again)
	}
	if _, err := Apply(v, state, ev("end", "", "prompt_input_exit"), at("2026-10-27 05:20")); err != nil {
		t.Fatal(err)
	}
	// a late turn (an async Stop after the end) keeps ended and the status
	_, _ = Apply(v, state, ev("turn", "Late line.", ""), at("2026-10-27 05:19"))
	r := read(t, v, rel)
	for k, want := range map[string]string{
		"type": "agent-report", "schema": "1", "date": "2026-10-26", "agent": "claude-code", "host": "mbp",
		"session": "00893AAF-19fa-41d2", "project": "noteTaking", "cwd": cwd, "status": "done",
		"created": stamp(at("2026-10-26 23:50")), "ended": stamp(at("2026-10-27 05:20")), "updated": stamp(at("2026-10-27 05:20")),
	} {
		if got := r.Get(k); got != want {
			t.Errorf("%s: %q, want %q", k, got, want)
		}
	}
	log := r.Section("Log")
	for _, want := range []string{"- 23:50 started in " + cwd, "- 05:10 Built the digest.", "- 05:20 ended", "- 05:19 Late line."} {
		if !strings.Contains(log, want) {
			t.Errorf("the log lacks %q:\n%s", want, log)
		}
	}
	// resumed after it ended: a new report on the new day that continues the first
	next, _ := Apply(v, state, ev("start", "", ""), at("2026-10-27 09:00"))
	if next == rel || !strings.HasPrefix(next, "agents/2026/2026-10-27/") {
		t.Fatalf("the resumed session went to %s", next)
	}
	if got := read(t, v, next).Get("continues"); got != "../2026-10-26/mbp--claude-code--notetaking--00893aaf.md" {
		t.Errorf("continues: %q", got)
	}
}

func TestEndStatus(t *testing.T) {
	v, state := testVault(t)
	cwd := t.TempDir()
	start := func(s string) {
		_, _ = Apply(v, state, Event{Kind: "start", Agent: "copilot-cli", Session: s, Cwd: cwd}, at("2026-10-26 09:00"))
	}
	end := func(s, reason string) string {
		rel, _ := Apply(v, state, Event{Kind: "end", Agent: "copilot-cli", Session: s, Cwd: cwd, Reason: reason}, at("2026-10-26 10:00"))
		return read(t, v, rel).Get("status")
	}
	start("quiet")
	if got := end("quiet", "user_exit"); got != "abandoned" {
		t.Errorf("a session without a turn ended %s, want abandoned", got)
	}
	start("worked") // Copilot CLI's turns carry no message text
	rel, _ := Apply(v, state, Event{Kind: "turn", Agent: "copilot-cli", Session: "worked", Cwd: cwd, Reason: "end_turn"}, at("2026-10-26 09:30"))
	if log := read(t, v, rel).Section("Log"); !strings.Contains(log, "- 09:30 turn ended") {
		t.Errorf("a turn without text leaves no line in the log:\n%s", log)
	}
	// Copilot CLI sends several agentStops for one turn: the same line in the same minute is
	// written once, and a later turn gets its own
	for _, when := range []string{"2026-10-26 09:30", "2026-10-26 09:30", "2026-10-26 09:31"} {
		_, _ = Apply(v, state, Event{Kind: "turn", Agent: "copilot-cli", Session: "worked", Cwd: cwd}, at(when))
	}
	if log := read(t, v, rel).Section("Log"); strings.Count(log, "09:30 turn ended") != 1 || strings.Count(log, "09:31 turn ended") != 1 {
		t.Errorf("repeated turns are not folded into one line a minute:\n%s", log)
	}
	if got := end("worked", "complete"); got != "done" {
		t.Errorf("a session whose turns had no text ended %s, want done", got)
	}
	start("broken")
	_, _ = Apply(v, state, Event{Kind: "turn", Agent: "copilot-cli", Session: "broken", Cwd: cwd, Message: "trying"}, at("2026-10-26 09:30"))
	if got := end("broken", "error"); got != "failed" {
		t.Errorf("a session that ended on an error is %s, want failed", got)
	}
	start("held")
	if _, err := Write(v, state, Selector{Session: "held"}, "", "blocked", "Waiting on the API key", at("2026-10-26 09:40")); err != nil {
		t.Fatal(err)
	}
	if got := end("held", "complete"); got != "blocked" {
		t.Errorf("the end replaced the agent's own status: %s", got)
	}
}

func TestWrite(t *testing.T) {
	v, state := testVault(t)
	cwd := t.TempDir()
	rel, _ := Apply(v, state, Event{Kind: "start", Agent: "claude-code", Session: "s1", Cwd: cwd}, at("2026-10-26 09:00"))
	in := "Intro without a heading.\n\n## Decisions\n- merge, never rebase\n\n### open\n- the API key\n"
	got, err := Write(v, state, Selector{Cwd: filepath.Join(cwd, "sub")}, in, "done", "Digest and hooks\nsecond line", at("2026-10-26 09:30"))
	if err != nil || got != rel {
		t.Fatalf("Write: %s, %v; want %s", got, err, rel)
	}
	r := read(t, v, rel)
	if r.Section("Done") != "Intro without a heading." || r.Section("Decisions") != "- merge, never rebase" || r.Section("Open") != "- the API key" {
		t.Errorf("sections: %q / %q / %q", r.Section("Done"), r.Section("Decisions"), r.Section("Open"))
	}
	if r.Get("status") != "done" || r.Get("summary") != "Digest and hooks" || r.Title() != filepath.Base(cwd)+": Digest and hooks" {
		t.Errorf("status %q, summary %q, title %q", r.Get("status"), r.Get("summary"), r.Title())
	}
	if !strings.Contains(r.Section("Log"), "started in") {
		t.Error("write touched the log")
	}
	_, _ = Apply(v, state, Event{Kind: "end", Agent: "claude-code", Session: "s1", Cwd: cwd}, at("2026-10-26 10:00"))
	if _, err := Write(v, state, Selector{Session: "s1"}, "", "in-progress", "", at("2026-10-26 10:05")); err == nil {
		t.Error("an ended session went back to in-progress")
	}
	if _, err := Write(v, state, Selector{Session: "s1"}, "", "finished", "", at("2026-10-26 10:05")); err == nil {
		t.Error("an unknown status was taken")
	}
	// an agent without hooks: write makes its report
	other := t.TempDir()
	made, err := Write(v, state, Selector{Agent: "codex", Cwd: other}, "Did it.", "done", "Ran once", at("2026-10-26 11:00"))
	if err != nil || !strings.Contains(made, "--codex--") || read(t, v, made).Section("Done") != "Did it." {
		t.Fatalf("a report without hooks: %s, %v", made, err)
	}
}

func TestReportKeepsWhatItDoesNotKnow(t *testing.T) {
	in := "---\ntype: agent-report\nschema: 1\nproject: p\nsummary: \"a: b\"\nreviewer: jo # who\nstatus: done\n---\n\n# p: a: b\n\n## Done\n\nx\n\n## Notes\n\nmine\n\n## Log\n\n- 09:00 started\n"
	r := ParseReport([]byte(in))
	if r.Get("summary") != "a: b" || r.Get("reviewer") != "jo" || r.Section("Notes") != "mine" {
		t.Fatalf("parsed %q %q %q", r.Get("summary"), r.Get("reviewer"), r.Section("Notes"))
	}
	out := r.Bytes()
	if again := ParseReport(out).Bytes(); string(again) != string(out) {
		t.Errorf("a second round trip changed the report:\n%s\n---\n%s", out, again)
	}
	if !strings.Contains(string(out), "## Notes\n\nmine\n\n## Log") {
		t.Errorf("an unknown section moved or went:\n%s", out)
	}
}

func TestDigest(t *testing.T) {
	v, state := testVault(t)
	day := at("2026-10-26 12:00")
	mk := func(agent, session, cwd, status, summary string, when time.Time) {
		_ = os.MkdirAll(cwd, 0o755)
		_, _ = Apply(v, state, Event{Kind: "start", Agent: agent, Session: session, Cwd: cwd}, when)
		if status != "" {
			_, _ = Write(v, state, Selector{Session: session}, "", status, summary, when)
		}
	}
	base := t.TempDir()
	mk("copilot-cli", "7c1d0e2a", filepath.Join(base, "noteTaking"), "blocked", "Waiting on the API key", at("2026-10-26 10:00"))
	mk("claude-code", "00893aaf", filepath.Join(base, "noteTaking"), "done", "Phase 0 scaffold", at("2026-10-26 09:00"))
	mk("claude-code", "11111111", filepath.Join(base, "flok"), "", "", at("2026-10-26 11:00"))
	changed, err := Digest(v, day)
	if err != nil || !changed {
		t.Fatalf("Digest: %v, %v", changed, err)
	}
	got, _ := os.ReadFile(v.Abs("agents/2026/2026-10-26/README.md"))
	want := `# Agents · Monday 26 October 2026

[← journal](../../../daily/2026/2026-10-26.md)

## flok
- ◐ in-progress · claude-code · mbp · [report](mbp--claude-code--flok--11111111.md)

## noteTaking
- ✓ done · claude-code · mbp · Phase 0 scaffold · [report](mbp--claude-code--notetaking--00893aaf.md)
- ● blocked · copilot-cli · mbp · Waiting on the API key · [report](mbp--copilot-cli--notetaking--7c1d0e2a.md)
`
	if string(got) != want {
		t.Errorf("digest:\n%s\nwant:\n%s", got, want)
	}
	if changed, _ := Digest(v, day); changed {
		t.Error("the same reports wrote the digest again")
	}
	if days, err := Refresh(v); err != nil || len(days) != 0 {
		t.Errorf("Refresh with nothing new wrote %v, %v", days, err)
	}
	time.Sleep(10 * time.Millisecond) // a later modification time than the digest's
	_, _ = Write(v, state, Selector{Session: "11111111"}, "", "done", "Sidebar", at("2026-10-26 13:00"))
	if days, err := Refresh(v); err != nil || len(days) != 1 || days[0] != "2026-10-26" {
		t.Errorf("Refresh after a report changed wrote %v, %v", days, err)
	}
	got, _ = os.ReadFile(v.Abs("agents/2026/2026-10-26/README.md"))
	if !strings.Contains(string(got), "✓ done · claude-code · mbp · Sidebar") {
		t.Errorf("the refreshed digest lacks the change:\n%s", got)
	}
}

func TestClaudeHooks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	flok := `{"type":"command","command":"/opt/flok hook claude","async":true,"timeout":5}`
	_ = os.WriteFile(p, []byte(`{"model":"opus","hooks":{"Stop":[{"hooks":[`+flok+`]}],"PreToolUse":[{"matcher":"Bash","hooks":[`+flok+`]}]}}`), 0o600)
	changed, err := ClaudeHooks(p, "/Users/x/.local/bin/nt", false)
	if err != nil || !changed {
		t.Fatalf("install: %v, %v", changed, err)
	}
	if got := ClaudeHooked(p); strings.Join(got, " ") != "SessionEnd SessionStart Stop" {
		t.Fatalf("hooked events: %v", got)
	}
	b, _ := os.ReadFile(p)
	for _, want := range []string{`"model": "opus"`, `/opt/flok hook claude`, `"matcher": "Bash"`, `"/Users/x/.local/bin/nt agent turn"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("settings lack %s:\n%s", want, b)
		}
	}
	if changed, _ := ClaudeHooks(p, "/Users/x/.local/bin/nt", false); changed {
		t.Error("installing twice changed the file")
	}
	if changed, _ := ClaudeHooks(p, "/usr/local/bin/nt", false); !changed {
		t.Error("a new binary path did not update the hooks")
	}
	if b, _ := os.ReadFile(p); strings.Count(string(b), "agent turn") != 1 {
		t.Errorf("the hook is there more than once:\n%s", b)
	}
	if changed, err := ClaudeHooks(p, "", true); err != nil || !changed {
		t.Fatalf("remove: %v, %v", changed, err)
	}
	b, _ = os.ReadFile(p)
	if strings.Contains(string(b), "agent ") || strings.Count(string(b), "flok hook claude") != 2 || strings.Contains(string(b), "SessionStart") {
		t.Errorf("after removing, nt's hooks or empty events are left, or flok's went:\n%s", b)
	}
	if bak, _ := filepath.Glob(p + ".bak-*"); len(bak) == 0 {
		t.Error("no backup was written")
	}
}

func TestCopilotHooks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hooks", "nt.json")
	if changed, err := CopilotHooks(p, "/bin dir/nt", false); err != nil || !changed {
		t.Fatalf("install: %v, %v", changed, err)
	}
	b, _ := os.ReadFile(p)
	for _, want := range []string{`"sessionStart"`, `"agentStop"`, `"sessionEnd"`, `"'/bin dir/nt' agent turn --agent copilot-cli"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("hooks file lacks %s:\n%s", want, b)
		}
	}
	if changed, _ := CopilotHooks(p, "/bin dir/nt", false); changed {
		t.Error("writing the same hooks changed the file")
	}
	if changed, _ := CopilotHooks(p, "", true); !changed {
		t.Error("remove did nothing")
	}
	if _, err := os.Stat(p); err == nil {
		t.Error("the hooks file is still there")
	}
}
