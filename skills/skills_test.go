package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstall(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "data", "nt", "skills", "nt")
	links := []string{filepath.Join(home, ".claude", "skills", "nt"), filepath.Join(home, ".agents", "skills", "nt")}
	if s := State(dir, links); s != "not installed" {
		t.Fatalf("before: %q", s)
	}
	if err := Install(dir, links); err != nil {
		t.Fatal(err)
	}
	if s := State(dir, links); s != "" {
		t.Fatalf("after the install: %q", s)
	}
	b, err := os.ReadFile(filepath.Join(links[0], "SKILL.md"))
	if err != nil || !strings.HasPrefix(string(b), "---\nname: nt\n") {
		t.Fatalf("the skill through the link: %v %q", err, b)
	}
	if err := Install(dir, links); err != nil {
		t.Fatalf("installing again: %v", err)
	}
	_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("old"), 0o644)
	if s := State(dir, links); s != "older than this nt" {
		t.Errorf("an old skill: %q", s)
	}
	other := filepath.Join(home, "elsewhere", "nt")
	_ = os.MkdirAll(other, 0o755)
	if err := os.Remove(links[1]); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(links[1], 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, links); err == nil {
		t.Error("a folder of the person's own was replaced by the link")
	}
	if err := Remove(dir, links); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(links[0]); err == nil {
		t.Error("the link is still there")
	}
	if _, err := os.Stat(links[1]); err != nil {
		t.Error("remove deleted a folder that was not nt's")
	}
}
