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

func TestWrite(t *testing.T) {
	v, state := testVault(t)
	cwd := t.TempDir()
	// a session flok does not follow: write makes its report
	rel, err := Write(v, state, Selector{Agent: "claude-code", Session: "s1", Cwd: cwd}, "", "", "", at("2026-10-26 09:00"))
	if err != nil || rel != "agents/2026/2026-10-26/mbp--claude-code--"+vault.Slug(filepath.Base(cwd))+"--s1.md" {
		t.Fatalf("the report write makes: %s, %v", rel, err)
	}
	if r := read(t, v, rel); r.Get("status") != "in-progress" || r.Section("Log") != "- 09:00 started in "+cwd {
		t.Errorf("a new report: status %q, Log %q", r.Get("status"), r.Section("Log"))
	}
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
	// flok's import of the session fills the same report, and keeps the agent's status
	rec := Record{Version: 1, Agent: "claude-code", Session: "s1", Cwd: cwd, Created: stamp(at("2026-10-26 09:00")),
		Updated: stamp(at("2026-10-26 10:00")), Ended: stamp(at("2026-10-26 10:00")), Turns: 1,
		Log: []Turn{{Started: stamp(at("2026-10-26 09:01")), Ended: stamp(at("2026-10-26 09:20")), Text: "Built it."}}}
	if got, _, err := Import(v, state, rec, at("2026-10-26 10:00")); err != nil || got != rel {
		t.Fatalf("the import went to %s, %v; want %s", got, err, rel)
	}
	if r := read(t, v, rel); r.Get("status") != "done" || r.Section("Done") != "Intro without a heading." {
		t.Errorf("after the import: status %q, Done %q", r.Get("status"), r.Section("Done"))
	}
	if _, err := Write(v, state, Selector{Session: "s1"}, "", "in-progress", "", at("2026-10-26 10:05")); err == nil {
		t.Error("an ended session went back to in-progress")
	}
	if _, err := Write(v, state, Selector{Session: "s1"}, "", "finished", "", at("2026-10-26 10:05")); err == nil {
		t.Error("an unknown status was taken")
	}
	// no session named: write makes one of its own
	other := t.TempDir()
	made, err := Write(v, state, Selector{Agent: "codex", Cwd: other}, "Did it.", "done", "Ran once", at("2026-10-26 11:00"))
	if err != nil || !strings.Contains(made, "--codex--") || read(t, v, made).Section("Done") != "Did it." {
		t.Fatalf("a report of its own: %s, %v", made, err)
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
		if _, err := Write(v, state, Selector{Agent: agent, Session: session, Cwd: cwd}, "", status, summary, when); err != nil {
			t.Fatal(err)
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

func TestRemoveHooks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	flok := `{"type":"command","command":"/opt/flok hook claude","timeout":5}`
	nt := func(kind string) string {
		return `{"type":"command","command":"'/Users/a b/nt' agent ` + kind + `","timeout":10}`
	}
	_ = os.WriteFile(p, []byte(`{"model":"opus","hooks":{"Stop":[{"hooks":[`+flok+`,`+nt("turn")+`]}],"SessionStart":[{"hooks":[`+nt("start")+`]}],`+
		`"SessionEnd":[{"hooks":[`+nt("end")+`]}],"PreToolUse":[{"matcher":"Bash","hooks":[`+flok+`]}]}}`), 0o600)
	if !ClaudeHooked(p) {
		t.Fatal("nt's hooks are not found")
	}
	if changed, err := RemoveClaudeHooks(p); err != nil || !changed {
		t.Fatalf("remove: %v, %v", changed, err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "agent ") || strings.Count(string(b), "flok hook claude") != 2 || strings.Contains(string(b), "SessionStart") ||
		!strings.Contains(string(b), `"model": "opus"`) || !strings.Contains(string(b), `"matcher": "Bash"`) {
		t.Errorf("after removing, nt's hooks or empty events are left, or another hook or setting went:\n%s", b)
	}
	if ClaudeHooked(p) {
		t.Error("nt's hooks are still found")
	}
	if bak, _ := filepath.Glob(p + ".bak-*"); len(bak) != 1 {
		t.Errorf("backups: %v", bak)
	}
	if changed, _ := RemoveClaudeHooks(p); changed {
		t.Error("removing twice changed the file")
	}
	if changed, err := RemoveClaudeHooks(filepath.Join(t.TempDir(), "none.json")); changed || err != nil {
		t.Errorf("no settings: %v, %v", changed, err)
	}

	c := filepath.Join(t.TempDir(), "hooks", "nt.json")
	_ = os.MkdirAll(filepath.Dir(c), 0o755)
	_ = os.WriteFile(c, []byte(`{"version":1,"hooks":{"agentStop":[{"type":"command","bash":"/usr/local/bin/nt agent turn --agent copilot-cli"}]}}`), 0o644)
	if !CopilotHooked(c) {
		t.Fatal("Copilot's nt hooks are not found")
	}
	if changed, err := RemoveCopilotHooks(c); err != nil || !changed {
		t.Fatalf("remove Copilot's: %v, %v", changed, err)
	}
	if CopilotHooked(c) {
		t.Error("the hooks file is still there")
	}
	if changed, _ := RemoveCopilotHooks(c); changed {
		t.Error("removing a file that is gone changed something")
	}
}
