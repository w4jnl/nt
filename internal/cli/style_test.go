package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStylePrintsTheStyleNotesOnTheWay: nt style prints the root's style.md, then the folder's,
// each under its path and without its frontmatter; nothing when there is none.
func TestStylePrintsTheStyleNotesOnTheWay(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "notes")
	write := func(p, body string) {
		t.Helper()
		f := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".git/HEAD", "ref: refs/heads/main\n")
	write("../config.toml", "repo = \""+root+"\"\nhost = \"mbp\"\n[sync]\nauto = false\n")
	t.Setenv("NT_CONFIG", filepath.Join(base, "config.toml"))
	t.Setenv("NT_STATE", filepath.Join(base, "state"))
	t.Setenv("NT_REPO", root)

	if out, code := runMain(t, []string{"style", "kb/trading"}, ""); code != 0 || out != "" {
		t.Errorf("no style.md: %d %q", code, out)
	}
	if out, code := runMain(t, []string{"style", "--json"}, ""); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Errorf("no style.md, --json: %d %q", code, out)
	}
	write("style.md", "---\ntitle: Style\n---\n\nWrite in Dutch.\n")
	write("kb/trading/style.md", "Draw every chain.\n")
	out, code := runMain(t, []string{"style", "kb/trading"}, "")
	want := "<!-- style.md -->\nWrite in Dutch.\n\n<!-- kb/trading/style.md -->\nDraw every chain.\n"
	if code != 0 || out != want {
		t.Errorf("style kb/trading: %d\n%q\nwant\n%q", code, out, want)
	}
	if out, _ := runMain(t, []string{"style", "work"}, ""); out != "<!-- style.md -->\nWrite in Dutch.\n" {
		t.Errorf("style work: %q", out)
	}
	if out, _ := runMain(t, []string{"style", "--json", "kb/trading/2026-11-24-chain.md"}, ""); !strings.Contains(out, `"rel": "kb/trading/style.md"`) && !strings.Contains(out, `"rel":"kb/trading/style.md"`) {
		t.Errorf("style --json for a note: %s", out)
	}
	if out, code := runMain(t, []string{"style", "a", "b"}, ""); code == 0 || !strings.Contains(out, "name one folder") {
		t.Errorf("two folders: %d %q", code, out)
	}
}
