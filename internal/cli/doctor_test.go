package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w4jnl/nt/internal/agents"
)

func TestReportChecks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NT_STATE", filepath.Join(dir, "state"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(dir, "claude"))
	t.Setenv("COPILOT_HOME", filepath.Join(dir, "copilot"))
	check := func() string {
		var got []string
		reportChecks(func(name, state, format string, a ...any) {
			got = append(got, state+" "+name+" "+fmt.Sprintf(format, a...))
		})
		return strings.Join(got, "\n")
	}
	if got := check(); got != "warn reports agent sessions get no reports: none imported yet, is flok's [notes] on?" {
		t.Errorf("nothing imported: %s", got)
	}
	if err := agents.MarkImported(filepath.Join(dir, "state"), time.Date(2026, 10, 29, 14, 5, 0, 0, time.Local)); err != nil {
		t.Fatal(err)
	}
	if got := check(); got != "ok reports imported by flok, last at Thu 29 Oct 14:05" {
		t.Errorf("imported: %s", got)
	}
	// nt's hooks of nt 0.7 and before, left behind
	for path, body := range map[string]string{
		agents.ClaudeSettingsPath(): `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"/opt/homebrew/bin/nt agent turn"}]}]}}`,
		agents.CopilotHooksPath():   `{"version":1,"hooks":{"agentStop":[{"type":"command","bash":"nt agent turn --agent copilot-cli"}]}}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := "warn hooks nt's old hooks are still in " + filepath.Join(dir, "claude", "settings.json") + " and " + filepath.Join(dir, "copilot", "hooks", "nt.json") +
		"; flok writes the reports now: nt uninstall --hooks\nok reports imported by flok, last at Thu 29 Oct 14:05"
	if got := check(); got != want {
		t.Errorf("hooks left:\n%s\nwant\n%s", got, want)
	}
}

func TestRuns(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "nt")
	_ = os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755)
	plain := filepath.Join(t.TempDir(), "plain")
	_ = os.WriteFile(plain, []byte("x"), 0o644)
	for bin, want := range map[string]bool{
		"":                      true,
		exe:                     true,
		plain:                   false,
		filepath.Dir(exe):       false,
		"/no/such/nt":           false,
		"sh":                    true,
		"no-such-tool-anywhere": false,
	} {
		if got := runs(bin); got != want {
			t.Errorf("runs(%q) = %v, want %v", bin, got, want)
		}
	}
}
