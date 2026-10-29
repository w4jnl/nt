package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionScriptsParse(t *testing.T) {
	for shell, script := range map[string]string{"bash": bashCompletion(), "zsh": zshCompletion()} {
		if _, err := exec.LookPath(shell); err != nil {
			t.Logf("%s not installed, skipping its syntax check", shell)
			continue
		}
		cmd := exec.Command(shell, "-n")
		cmd.Stdin = strings.NewReader(script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s -n: %v\n%s", shell, err, out)
		}
	}
	for _, c := range commands {
		for _, n := range c.names {
			if !strings.Contains(bashCompletion(), n) || !strings.Contains(zshCompletion(), "'"+n+":") {
				t.Errorf("command %s is missing from a script", n)
			}
		}
	}
}

// TestBashCompletion runs the bash script against a stand-in nt that knows two folders and a note.
func TestBashCompletion(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	probe := bashCompletion() + `
nt() { case $2 in folders) printf 'tech\nwork\n' ;; notes) printf 'tech/2026-10-20-nas.md\n' ;; esac; }
c() { COMP_WORDS=("$@"); COMP_CWORD=$(( ${#COMP_WORDS[@]} - 1 )); _nt; echo "${COMPREPLY[*]}"; }
c nt te
c nt l
c nt ls ""
c nt n "a title" w
c nt n ""
c nt view t
c nt view --theme ""
c nt view --theme dark ""
c nt paste --name shot t
c nt install --
c nt completion ""
c nt import ""
c nt agent ""
c nt agent write --status b
c nt todo ""
c nt todo add --due f
c nt todo add --in t
c nt mv t
c nt mv tech/2026-10-20-nas.md w
c nt rename --
c nt config ""
c nt config set s
`
	out, err := exec.Command("bash", "-c", probe).CombinedOutput()
	if err != nil {
		t.Fatalf("bash: %v\n%s", err, out)
	}
	want := []string{
		"tech",                   // nt <folder> opens the browser there
		"ls",                     // a command
		"tech work",              // ls takes a folder
		"work",                   // n's second argument is a folder
		"",                       // its first is a title
		"tech/2026-10-20-nas.md", // view takes a note
		"auto dark light",
		"tech/2026-10-20-nas.md", // the flag's value is not an argument
		"tech/2026-10-20-nas.md",
		"--sync --nvim --hooks --skill",
		"bash zsh",
		"obsidian",
		"import write start turn end",
		"blocked",
		"add done undo due rm",
		"fri",
		"tech",                        // --in takes a folder
		"tech tech/2026-10-20-nas.md", // mv moves a note or a folder
		"work",                        // into a folder
		"--dry-run --json",
		"get set edit", // config's first word
		"summary.tool summary.model sync.auto sync.interval", // then a setting
	}
	got := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), out)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i+1, got[i], want[i])
		}
	}
}

func TestComplete(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{".git/HEAD", ".nt/config.toml", "tech/2026-10-20-nas.md", "tech/assets/x.png", "tech/old/a.md", "work/b.md", "README.md", "tech/notes.txt"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NT_REPO", root)
	t.Setenv("NT_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	for kind, want := range map[string]string{
		"folders": "tech\ntech/old\nwork",
		"notes":   "README.md\ntech/2026-10-20-nas.md\ntech/old/a.md\nwork/b.md",
	} {
		r, w, _ := os.Pipe()
		stdout := os.Stdout
		os.Stdout = w
		code := runComplete([]string{kind})
		os.Stdout, _ = stdout, w.Close()
		b := make([]byte, 4096)
		n, _ := r.Read(b)
		if got := strings.TrimSpace(string(b[:n])); code != 0 || got != want {
			t.Errorf("_complete %s: %d %q, want %q", kind, code, got, want)
		}
	}
}
