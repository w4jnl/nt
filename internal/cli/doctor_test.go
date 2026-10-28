package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w4jnl/nt/internal/agents"
)

func TestHookChecks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(dir, "claude"))
	t.Setenv("COPILOT_HOME", filepath.Join(dir, "copilot"))
	nt := filepath.Join(dir, "bin", "nt")
	if err := os.MkdirAll(filepath.Dir(nt), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nt, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	check := func() string {
		var got []string
		hookChecks(func(name, state, format string, a ...any) {
			got = append(got, state+" "+name+" "+fmt.Sprintf(format, a...))
		})
		return strings.Join(got, "\n")
	}
	if got := check(); !strings.HasPrefix(got, "warn hooks agent sessions get no reports") {
		t.Errorf("no hooks: %s", got)
	}
	if _, err := agents.ClaudeHooks(agents.ClaudeSettingsPath(), nt, false); err != nil {
		t.Fatal(err)
	}
	if _, err := agents.CopilotHooks(agents.CopilotHooksPath(), nt, false); err != nil {
		t.Fatal(err)
	}
	if got := check(); got != "ok hooks Claude Code and Copilot CLI sessions get reports" {
		t.Errorf("both hooked: %s", got)
	}
	// the binary the hooks run goes away, as when ~/.local/bin/nt is removed for Homebrew's
	if err := os.Remove(nt); err != nil {
		t.Fatal(err)
	}
	if got := check(); !strings.HasPrefix(got, "fail hooks they run "+nt+", which is not there") {
		t.Errorf("a missing binary: %s", got)
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
