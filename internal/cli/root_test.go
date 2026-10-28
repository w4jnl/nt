package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTextFile: nt <file> shows text, and leaves a picture or another binary file to the OS.
func TestTextFile(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"note.md":   "# Note\n",
		"list.txt":  "milk\neggs\n",
		"empty.txt": "",
		"logo.png":  "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]bool{"note.md": true, "list.txt": true, "empty.txt": true, "logo.png": false, ".": false, "none.md": false} {
		if got := textFile(filepath.Join(dir, name)); got != want {
			t.Errorf("%s: text %v, want %v", name, got, want)
		}
	}
}
