package style

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, text string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestForAndAll(t *testing.T) {
	root := t.TempDir()
	write(t, root, "style.md", "---\ntitle: Style\n---\n\n# Style\n\nWrite in Dutch.\n")
	write(t, root, "kb/trading/style.md", "Draw every chain.\n")
	write(t, root, "kb/trading/2026-11-24-chain.md", "# Chain\n")
	write(t, root, "work/style.md", "---\ntitle: empty\n---\n")
	write(t, root, ".git/style.md", "not a note\n")

	rels := func(ns []Note) []string {
		var out []string
		for _, n := range ns {
			out = append(out, n.Rel)
		}
		return out
	}
	for _, tc := range []struct {
		at   string
		want []string
	}{
		{"", []string{"style.md"}},
		{"inbox", []string{"style.md"}},
		{"kb", []string{"style.md"}},
		{"kb/trading", []string{"style.md", "kb/trading/style.md"}},
		{"kb/trading/deep/er", []string{"style.md", "kb/trading/style.md"}},
		{"kb/trading/2026-11-24-chain.md", []string{"style.md", "kb/trading/style.md"}},
		{"work", []string{"style.md"}}, // an empty one says nothing
		{"../kb/trading", []string{"style.md", "kb/trading/style.md"}},
	} {
		if got := rels(For(root, tc.at)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("For(%q) = %v, want %v", tc.at, got, tc.want)
		}
	}
	if n := For(root, "")[0]; n.Text != "# Style\n\nWrite in Dutch." {
		t.Errorf("the root's text, without its frontmatter: %q", n.Text)
	}
	if got, want := All(root), []string{"style.md", "kb/trading/style.md", "work/style.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("All = %v, want %v", got, want)
	}
	if Folder("style.md") != "" || Folder("kb/trading/style.md") != "kb/trading" {
		t.Errorf("Folder: %q, %q", Folder("style.md"), Folder("kb/trading/style.md"))
	}

	write(t, root, "long/style.md", strings.Repeat("word ", Max))
	if n := For(root, "long")[1]; len(n.Text) > Max+200 || !strings.Contains(n.Text, "is not shown") {
		t.Errorf("a long style note is cut: %d bytes", len(n.Text))
	}
}
