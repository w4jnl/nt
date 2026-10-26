// Package nvim carries nt's nvim plugin inside the binary and installs it where an nvim config
// can load it, so the plugin always matches the nt that runs it (docs/design.md §4, nvim).
package nvim

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed plugin
var files embed.FS

// Dir is where nt install --nvim puts the plugin: $XDG_DATA_HOME/nt/nvim, else
// ~/.local/share/nt/nvim.
func Dir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "nt", "nvim")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nt", "nvim")
}

// Install writes the plugin to dir, in place of whatever was there: into a new folder first,
// then renamed over the old one.
func Install(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".nt-tmp-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	err = fs.WalkDir(files, "plugin", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "plugin" {
			return err
		}
		dst := filepath.Join(tmp, filepath.FromSlash(p[len("plugin/"):]))
		if d.IsDir() {
			return os.Mkdir(dst, 0o755)
		}
		b, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(tmp, dir)
}

// Remove deletes the plugin.
func Remove(dir string) error { return os.RemoveAll(dir) }

// Installed reports whether dir holds a plugin, and whether it is exactly this binary's.
func Installed(dir string) (present, current bool) {
	if _, err := os.Stat(dir); err != nil {
		return false, false
	}
	want := map[string][]byte{}
	_ = fs.WalkDir(files, "plugin", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			want[p[len("plugin/"):]], _ = files.ReadFile(p)
		}
		return nil
	})
	n := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b, rerr := os.ReadFile(p)
		if w, ok := want[filepath.ToSlash(rel)]; !ok || rerr != nil || !bytes.Equal(b, w) {
			return fs.ErrInvalid
		}
		n++
		return nil
	})
	return true, err == nil && n == len(want)
}
