package vault

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Delete removes the note at rel, the attachments in files (repository paths) and an assets
// folder they leave empty, and takes the line that links the note out of the journal of the
// day in its name, all under the repository lock. Sync commits the deletion like any change,
// so git still has every file.
func (v *Vault) Delete(rel string, files []string) error {
	unlock, err := v.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err := os.Remove(v.Abs(rel)); err != nil {
		return err
	}
	dirs := map[string]bool{}
	for _, f := range files {
		if err := os.Remove(v.Abs(f)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		dirs[path.Dir(f)] = true
	}
	for d := range dirs {
		if path.Base(d) == "assets" {
			_ = os.Remove(v.Abs(d)) // only when empty
		}
	}
	day := datePrefix.FindString(path.Base(rel))
	if day == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return nil
	}
	journal := v.JournalRel(t)
	old, err := os.ReadFile(v.Abs(journal))
	if journal == rel || err != nil {
		return nil
	}
	if out := dropLinkLines(old, "]("+RelLink(journal, rel)+")"); !bytes.Equal(out, old) {
		return WriteFile(v.Abs(journal), out)
	}
	return nil
}

// dropLinkLines takes out the list items that are only a link ending in tail.
func dropLinkLines(doc []byte, tail string) []byte {
	lines := strings.SplitAfter(string(doc), "\n")
	var out strings.Builder
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "- [") && strings.HasSuffix(t, tail) {
			continue
		}
		out.WriteString(l)
	}
	return []byte(out.String())
}

// Unshared is the files among images (repository paths) that no note but rel names: what can
// go with it. A note names a file when its text holds the file's name.
func (v *Vault) Unshared(rel string, images []string) []string {
	if len(images) == 0 {
		return nil
	}
	used := map[string]bool{}
	_ = filepath.WalkDir(v.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && p != v.Root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") || v.RelPath(p) == rel {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for _, img := range images {
			if bytes.Contains(b, []byte(path.Base(img))) {
				used[img] = true
			}
		}
		return nil
	})
	var out []string
	for _, img := range images {
		if !used[img] {
			out = append(out, img)
		}
	}
	return out
}

// DeleteFolder removes the folder at rel when it holds no notes: nothing but perhaps an empty
// assets folder.
func (v *Vault) DeleteFolder(rel string) error {
	if !v.FolderEmpty(rel) {
		return fmt.Errorf("%s/ is not empty: delete what is in it first", rel)
	}
	unlock, err := v.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	_ = os.Remove(v.Abs(path.Join(rel, "assets")))
	return os.Remove(v.Abs(rel))
}

// FolderEmpty reports whether the folder at rel holds nothing but perhaps an empty assets folder.
func (v *Vault) FolderEmpty(rel string) bool {
	if strings.Trim(rel, "/") == "" {
		return false
	}
	des, err := os.ReadDir(v.Abs(rel))
	if err != nil {
		return false
	}
	for _, d := range des {
		if d.Name() != "assets" || !d.IsDir() {
			return false
		}
		if inner, err := os.ReadDir(v.Abs(path.Join(rel, "assets"))); err != nil || len(inner) > 0 {
			return false
		}
	}
	return true
}
