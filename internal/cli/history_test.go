package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHistoryListsTheSyncedChanges: nt history prints each commit that touched the note, newest
// first, with the host sync named in its subject; --json carries the times as RFC 3339.
func TestHistoryListsTheSyncedChanges(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "notes")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	git("init", "--quiet", "-b", "main")
	write := func(p, body string) {
		if err := os.WriteFile(filepath.Join(root, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("work/net.md", "# Network\n")
	git("add", "-A")
	git("commit", "--quiet", "-m", "nt: mbp · 1 file (work)")
	write("work/net.md", "# Network\n\nVLAN 10.\n")
	write("work/other.md", "# Other\n")
	git("add", "-A")
	git("commit", "--quiet", "-m", "nt: iphone · 2 files (work)")
	write("work/other.md", "# Other\n\nnot nt\n")
	git("commit", "--quiet", "-am", "by hand")
	config := filepath.Join(base, "config.toml")
	write("../config.toml", "repo = \""+root+"\"\nhost = \"mbp\"\n[sync]\nauto = false\n")
	t.Setenv("NT_CONFIG", config)
	t.Setenv("NT_STATE", filepath.Join(base, "state"))
	t.Setenv("NT_REPO", root)

	out, code := runMain(t, []string{"history", "work/net.md"}, "")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != 0 || len(lines) != 2 || !strings.HasSuffix(lines[0], "  iphone") || !strings.HasSuffix(lines[1], "  mbp") {
		t.Errorf("history: %d\n%s", code, out)
	}
	out, code = runMain(t, []string{"history", "--json", "work/other.md"}, "")
	if code != 0 || !strings.Contains(out, `"at": "`) || strings.Count(out, `"by"`) != 1 || !strings.Contains(out, `"by": "iphone"`) {
		t.Errorf("history --json: a change by hand has no host; got %d\n%s", code, out)
	}
	out, code = runMain(t, []string{"_editor", "index"}, "")
	if code != 0 || !strings.Contains(out, `"by": "iphone"`) || !strings.Contains(out, `"changed": "`) {
		t.Errorf("the index carries changed and by: %d\n%s", code, out)
	}
	if out, code := runMain(t, []string{"history"}, ""); code == 0 || !strings.Contains(out, "name one note") {
		t.Errorf("no note: %d %s", code, out)
	}
}
