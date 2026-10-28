package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEditorIndex(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		".nt/config.toml":                   "format = 1\npins = [\"homelab\"]\n",
		"homelab/2026-10-21-nas-setup.md":   "---\ntitle: NAS setup\n---\n\nSee [backups](2026-10-22-backups.md).\n",
		"homelab/2026-10-22-backups.md":     "# Backups\n\nFrom the [NAS](2026-10-21-nas-setup.md) and [nothing](gone.md).\n",
		"homelab/assets/2026-10-21-nas.png": "png",
		"daily/2026/2026-10-21.md":          "# Wednesday\n\n- [NAS setup](../../homelab/2026-10-21-nas-setup.md)\n",
		"work/.gitkeep":                     "",
		".hidden/note.md":                   "# Hidden\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	t.Setenv("NT_REPO", root)
	t.Setenv("NT_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("NT_STATE", state)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	code := runEditorHelper([]string{"index"})
	os.Stdout = stdout
	w.Close()
	out, _ := io.ReadAll(r)
	if code != 0 {
		t.Fatalf("_editor index: exit %d: %s", code, out)
	}

	var got struct {
		Root    string            `json:"root"`
		State   string            `json:"state"`
		Day     string            `json:"day"`
		Pins    []string          `json:"pins"`
		Folders map[string]string `json:"folders"`
		Dirs    []string          `json:"dirs"`
		Notes   []struct {
			Path  string `json:"path"`
			Title string `json:"title"`
		} `json:"notes"`
		Back map[string][]string `json:"back"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.Root == "" || got.State != state || len(got.Day) != len("2026-10-27") {
		t.Errorf("root %q, state %q, day %q", got.Root, got.State, got.Day)
	}
	if !reflect.DeepEqual(got.Pins, []string{"homelab"}) || got.Folders["daily"] != "daily" {
		t.Errorf("pins %v, folders %v", got.Pins, got.Folders)
	}
	if want := []string{"daily", "daily/2026", "homelab", "homelab/assets", "work"}; !reflect.DeepEqual(got.Dirs, want) {
		t.Errorf("dirs %v, want %v", got.Dirs, want)
	}
	titles := map[string]string{}
	for _, n := range got.Notes {
		titles[n.Path] = n.Title
	}
	want := map[string]string{
		"daily/2026/2026-10-21.md":        "Wednesday",
		"homelab/2026-10-21-nas-setup.md": "NAS setup",
		"homelab/2026-10-22-backups.md":   "Backups",
	}
	if !reflect.DeepEqual(titles, want) {
		t.Errorf("titles %v, want %v", titles, want)
	}
	if b := got.Back["homelab/2026-10-21-nas-setup.md"]; !reflect.DeepEqual(b, []string{"daily/2026/2026-10-21.md", "homelab/2026-10-22-backups.md"}) {
		t.Errorf("linked from %v", b)
	}
}
