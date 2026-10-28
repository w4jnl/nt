package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/w4jnl/nt/internal/vault"
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

func TestEditorSave(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{".git", "homelab"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	note := filepath.Join(root, "homelab", "2026-10-21-nas.md")
	read := func() string { b, _ := os.ReadFile(note); return string(b) }
	write := func(s string) {
		if err := os.WriteFile(note, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	v, err := vault.Open(root, "mbp")
	if err != nil {
		t.Fatal(err)
	}

	type reply struct {
		Saved bool   `json:"saved"`
		SHA   string `json:"sha"`
		Text  string `json:"text"`
		Copy  string `json:"copy"`
	}
	save := func(text string, args ...string) (int, reply) {
		t.Helper()
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		stdout := os.Stdout
		os.Stdout = w
		code := editorSave(v, args, strings.NewReader(text))
		os.Stdout = stdout
		w.Close()
		out, _ := io.ReadAll(r)
		var got reply
		if code == 0 {
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("not JSON: %v\n%s", err, out)
			}
		}
		return code, got
	}

	write("# NAS\n")
	code, got := save("# NAS\n\nFour disks.\n", "homelab/2026-10-21-nas.md", "--base", sum("# NAS\n"))
	if code != 0 || !got.Saved || got.SHA != sum("# NAS\n\nFour disks.\n") || read() != "# NAS\n\nFour disks.\n" {
		t.Fatalf("the base on disk: %d %+v, file %q", code, got, read())
	}

	// Another writer appended a line since the editor read the note: nothing is written, and the
	// reply carries the file for the editor to merge.
	write("# NAS\n\nFour disks.\n- 09:12 fan\n")
	code, got = save("# NAS\n\nFour disks, RAID-Z1.\n", note, "--base", sum("# NAS\n\nFour disks.\n"))
	if code != 0 || got.Saved || got.Text != "# NAS\n\nFour disks.\n- 09:12 fan\n" || got.SHA != sum(got.Text) {
		t.Fatalf("a changed base: %d %+v", code, got)
	}
	if read() != "# NAS\n\nFour disks.\n- 09:12 fan\n" {
		t.Fatalf("a changed base was written over: %q", read())
	}

	// --keep writes all the same and keeps the other version as a conflict copy.
	code, got = save("mine\n", note, "--base", sum("# NAS\n"), "--keep")
	if code != 0 || !got.Saved || read() != "mine\n" || !strings.HasPrefix(got.Copy, "homelab/2026-10-21-nas.conflict-mbp-") {
		t.Fatalf("--keep: %d %+v, file %q", code, got, read())
	}
	if b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(got.Copy))); string(b) != "# NAS\n\nFour disks.\n- 09:12 fan\n" {
		t.Fatalf("the conflict copy holds %q", b)
	}

	// The same text as on disk is no conflict, and a note that is gone is written again.
	if code, got = save("mine\n", note, "--base", sum("old")); code != 0 || !got.Saved || got.Copy != "" {
		t.Fatalf("the text on disk with another base: %d %+v", code, got)
	}
	os.Remove(note)
	if code, got = save("back\n", note, "--base", sum("mine\n")); code != 0 || !got.Saved || read() != "back\n" {
		t.Fatalf("a note that is gone: %d %+v, file %q", code, got, read())
	}

	for _, bad := range []string{"homelab/assets/x.png", ".nt/templates/note.md", ".git/config.md", filepath.Join(t.TempDir(), "out.md")} {
		if code, _ := save("x", bad); code == 0 {
			t.Errorf("_editor save wrote %s", bad)
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "homelab", ".nt-tmp-*")); len(matches) > 0 {
		t.Errorf("temp files left: %v", matches)
	}
}
