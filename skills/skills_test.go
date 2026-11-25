package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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
	if d, err := os.ReadFile(filepath.Join(links[0], "drawing.svg")); err != nil || !strings.HasPrefix(string(d), "<svg ") {
		t.Fatalf("the drawing through the link: %v %q", err, d)
	}
	if err := Install(dir, links); err != nil {
		t.Fatalf("installing again: %v", err)
	}
	_ = os.Remove(filepath.Join(dir, "drawing.svg"))
	if s := State(dir, links); s != "older than this nt" {
		t.Errorf("a skill without its drawing: %q", s)
	}
	if err := Install(dir, links); err != nil {
		t.Fatal(err)
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

// TestDrawingUsesTheTokens: every colour in the pattern for a drawing is one of the design
// system's, and the dark ones come under prefers-color-scheme.
func TestDrawingUsesTheTokens(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "docs", "design-system", "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Color struct {
			Tokens []struct {
				Value map[string]string `json:"value"`
			} `json:"tokens"`
		} `json:"color"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	known := map[string]map[string]bool{"light": {}, "dark": {}}
	for _, tok := range doc.Color.Tokens {
		for theme, v := range tok.Value {
			known[theme][strings.ToUpper(v)] = true
		}
	}
	light, dark, ok := strings.Cut(Drawing, "@media (prefers-color-scheme:dark)")
	if !ok {
		t.Fatal("the drawing has no dark colours")
	}
	dark, rest, _ := strings.Cut(dark, "</style>")
	hex := regexp.MustCompile(`#[0-9A-Fa-f]{6}\b`)
	for theme, text := range map[string]string{"light": light + rest, "dark": dark} {
		found := hex.FindAllString(text, -1)
		if len(found) == 0 {
			t.Errorf("%s: no colours", theme)
		}
		for _, c := range found {
			if !known[theme][strings.ToUpper(c)] {
				t.Errorf("%s: %s is not a token of the design system", theme, c)
			}
		}
	}
}
