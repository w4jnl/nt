package cli

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/w4jnl/nt/internal/ops"
)

// TestAppsReadOneJSON: the command lines the apps run nt with print the same, run by nt itself
// (the Mac app) or by ops.Runner in the process (the iOS app), on two repositories that start
// the same. Paths and times of day are the only differences allowed.
func TestAppsReadOneJSON(t *testing.T) {
	type side struct{ root, config, state string }
	make := func(name string) side {
		base := t.TempDir()
		s := side{root: filepath.Join(base, "notes"), config: filepath.Join(base, "config.toml"), state: filepath.Join(base, "state")}
		for _, d := range []string{".git", "work", "homelab"} {
			if err := os.MkdirAll(filepath.Join(s.root, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		write := map[string]string{
			filepath.Join(s.root, "homelab", "2026-10-21-nas.md"): "# NAS setup\n\n- [ ] Replace the fan due:2026-10-25\n\nSee [the network](../work/net.md).\n",
			filepath.Join(s.root, "work", "net.md"):               "# Network\n\nThe NAS is on VLAN 10.\n",
			filepath.Join(s.root, "work", "diagram.png"):          "\x89PNG\r\n\x1a\n",
			s.config: "repo = \"" + s.root + "\"\nhost = \"mbp\"\n[sync]\nauto = false\n",
		}
		for p, body := range write {
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return s
	}
	use := func(s side) {
		t.Setenv("NT_CONFIG", s.config)
		t.Setenv("NT_STATE", s.state)
		t.Setenv("NT_REPO", s.root)
	}
	clock := regexp.MustCompile(`\b\d\d:\d\d\b`)
	norm := func(s side, out string) string {
		out = strings.ReplaceAll(out, s.root, "ROOT")
		out = strings.ReplaceAll(out, s.config, "CONFIG")
		out = strings.ReplaceAll(out, s.state, "STATE")
		return clock.ReplaceAllString(out, "HH:MM")
	}
	cli, app := make("cli"), make("app")
	journal := func(s side) string {
		matches, _ := filepath.Glob(filepath.Join(s.root, "daily", "*", "*.md"))
		if len(matches) == 0 {
			return filepath.Join(s.root, "daily", "none.md")
		}
		return matches[0]
	}
	steps := []struct {
		args  func(s side) []string
		stdin string
	}{
		{args: func(side) []string { return []string{"t", "--json"} }},
		{args: func(side) []string { return []string{"a", "--json", "--", "Called the ISP about the static IP"} }},
		{args: func(side) []string { return []string{"n", "--json", "--", "Q4 planning", "work"} }},
		{args: func(side) []string { return []string{"todo", "add", "--json", "--", "Renew the domain due:2026-11-06"} }},
		{args: func(side) []string { return []string{"todo", "--all", "--json"} }},
		{args: func(side) []string { return []string{"todo", "done", "Renew the domain", "--json"} }},
		{args: func(side) []string { return []string{"todo", "due", "homelab/2026-10-21-nas.md:3", "none", "--json"} }},
		{args: func(side) []string { return []string{"_editor", "index"} }},
		{args: func(side) []string { return []string{"_editor", "day"} }},
		{args: func(side) []string { return []string{"s", "--json", "vlan"} }},
		{args: func(side) []string { return []string{"s", "--json", "nothing like this"} }},
		{args: func(side) []string { return []string{"config", "--json"} }},
		{args: func(s side) []string { return []string{"_editor", "save", journal(s), "--base", "not the sha"} }, stdin: "# Overwritten\n"},
		{args: func(s side) []string {
			return []string{"paste", filepath.Join(s.root, "homelab", "2026-10-21-nas.md"), "--from", filepath.Join(s.root, "work", "diagram.png"), "--json"}
		}},
		{args: func(side) []string { return []string{"mkdir", "--json", "projects"} }},
		{args: func(side) []string { return []string{"rename", "--json", "work/net.md", "Network plan"} }},
		{args: func(side) []string { return []string{"mv", "--json", "homelab/2026-10-21-nas.md", "projects/"} }},
		{args: func(side) []string { return []string{"rm", "--dry-run", "--json", "projects/2026-10-21-nas.md"} }},
		{args: func(side) []string { return []string{"rm", "--json", "projects/2026-10-21-nas.md"} }},
		{args: func(side) []string { return []string{"rm", "--json", "work"} }},
	}
	for _, st := range steps {
		use(cli)
		wantOut, wantCode := runMain(t, st.args(cli), st.stdin)
		use(app)
		out, code, err := ops.Runner{}.Run(st.args(app), []byte(st.stdin))
		if err != nil {
			out = []byte("nt: " + err.Error() + "\n")
		}
		name := strings.Join(st.args(cli)[:2], " ")
		if strings.TrimSpace(wantOut) == "" {
			t.Errorf("%s: nt printed nothing", name)
		}
		if got, want := norm(app, string(out)), norm(cli, wantOut); got != want || code != wantCode {
			t.Errorf("%s: the app's runner says (%d)\n%s\nnt says (%d)\n%s", name, code, got, wantCode, want)
		}
	}
}

// runMain runs nt's command line in the process, with stdin, and returns what it printed on
// stdout (stderr after it when it failed) and its exit status.
func runMain(t *testing.T, args []string, stdin string) (string, int) {
	t.Helper()
	inR, inW, _ := os.Pipe()
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = inR, outW, errW
	go func() { _, _ = io.WriteString(inW, stdin); inW.Close() }()
	done := make(chan [2]string)
	go func() {
		o, _ := io.ReadAll(outR)
		e, _ := io.ReadAll(errR)
		done <- [2]string{string(o), string(e)}
	}()
	code := Main(args)
	outW.Close()
	errW.Close()
	os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr
	got := <-done
	if code != 0 && got[0] == "" {
		return got[1], code
	}
	return got[0], code
}
