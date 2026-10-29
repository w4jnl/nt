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

func TestHookChecks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NT_STATE", filepath.Join(dir, "state"))
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
	if got := check(); !strings.HasPrefix(got, "warn reports agent sessions get no reports: no report imported yet") {
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
	// flok imports reports: the hooks leave its sessions to it, and without them all is well
	if err := agents.MarkImported(filepath.Join(dir, "state"), time.Date(2026, 10, 29, 14, 5, 0, 0, time.Local)); err != nil {
		t.Fatal(err)
	}
	if got := check(); got != "ok reports imported by flok, last at Thu 29 Oct 14:05\nok hooks Claude Code and Copilot CLI; they leave the sessions flok imports to flok" {
		t.Errorf("imported and hooked: %s", got)
	}
	for _, remove := range []func() error{
		func() error { _, err := agents.ClaudeHooks(agents.ClaudeSettingsPath(), nt, true); return err },
		func() error { _, err := agents.CopilotHooks(agents.CopilotHooksPath(), nt, true); return err },
	} {
		if err := remove(); err != nil {
			t.Fatal(err)
		}
	}
	if got := check(); got != "ok reports imported by flok, last at Thu 29 Oct 14:05" {
		t.Errorf("imported, no hooks: %s", got)
	}
	if _, err := agents.ClaudeHooks(agents.ClaudeSettingsPath(), nt, false); err != nil {
		t.Fatal(err)
	}
	if _, err := agents.CopilotHooks(agents.CopilotHooksPath(), nt, false); err != nil {
		t.Fatal(err)
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
