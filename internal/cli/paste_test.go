package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w4jnl/nt/internal/clip"
)

// pasteRepo is a notes repository with one note, nt pointed at it, and the clipboard replaced
// by clipboard.
func pasteRepo(t *testing.T, clipboard func(png string) (clip.Image, error)) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{".git", "tech"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tech", "2026-10-20-nas.md"), []byte("# NAS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NT_REPO", root)
	t.Setenv("NT_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	old := readClipboard
	readClipboard = clipboard
	t.Cleanup(func() { readClipboard = old })
	return root
}

// paste runs nt paste with args and returns its exit code and what it printed.
func paste(t *testing.T, args ...string) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	code := runPaste(args)
	os.Stdout = stdout
	w.Close()
	out, _ := io.ReadAll(r)
	return code, strings.TrimSpace(string(out))
}

func TestPaste(t *testing.T) {
	day := time.Now().Add(-4 * time.Hour).Format("2006-01-02") // day_starts 04:00
	pngData := func(png string) (clip.Image, error) {
		return clip.Image{PNG: true}, os.WriteFile(png, []byte("png"), 0o644)
	}
	root := pasteRepo(t, pngData)

	code, out := paste(t, "tech/2026-10-20-nas.md")
	if want := "![image](assets/" + day + "-image.png)"; code != 0 || out != want {
		t.Fatalf("a repository path: %d %q, want %q", code, out, want)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "tech", "assets", day+"-image.png")); string(b) != "png" {
		t.Fatalf("the image was not saved beside the note: %q", b)
	}
	code, out = paste(t, filepath.Join(root, "tech", "2026-10-20-nas.md"), "--name", "rack [front]")
	if want := `![rack \[front\]](assets/` + day + "-rack-front.png)"; code != 0 || out != want {
		t.Fatalf("an absolute path with --name: %d %q, want %q", code, out, want)
	}

	shot := filepath.Join(t.TempDir(), "Rack photo.JPEG")
	if err := os.WriteFile(shot, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	readClipboard = func(string) (clip.Image, error) { return clip.Image{File: shot}, nil }
	code, out = paste(t, "tech/new-note.md")
	if want := "![Rack photo](assets/" + day + "-rack-photo.jpg)"; code != 0 || out != want {
		t.Fatalf("a copied file, for a note not written yet: %d %q, want %q", code, out, want)
	}

	readClipboard = func(string) (clip.Image, error) { return clip.Image{File: "/etc/hosts"}, nil }
	if code, _ := paste(t, "tech/2026-10-20-nas.md"); code == 0 {
		t.Error("a copied file that is not an image was pasted")
	}
	readClipboard = func(string) (clip.Image, error) { return clip.Image{}, clip.ErrNoImage }
	if code, _ := paste(t, "tech/2026-10-20-nas.md"); code == 0 {
		t.Error("an empty clipboard pasted something")
	}
	readClipboard = pngData
	if code, _ := paste(t, filepath.Join(t.TempDir(), "elsewhere.md")); code == 0 {
		t.Error("a note outside the repository got an attachment")
	}
	if code, _ := paste(t); code == 0 {
		t.Error("paste without a note did something")
	}
}
