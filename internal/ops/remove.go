package ops

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/w4jnl/nt/internal/mdparse"
	"github.com/w4jnl/nt/internal/scan"
	"github.com/w4jnl/nt/internal/vault"
)

// Removal is what nt rm deletes, worked out first: a note with the images no other note uses,
// or an empty folder; and how many notes link to the note, leaving out the line in the journal
// of its day, which goes with it.
type Removal struct {
	Rel    string   `json:"rel"`
	Folder bool     `json:"folder"`
	Title  string   `json:"title,omitempty"`
	Images []string `json:"images,omitempty"`
	Links  int      `json:"links"`
	DryRun bool     `json:"dry_run"`
}

// Remove deletes the note or the empty folder at p, a repository path or an absolute one, as
// the browser's D does (docs/design.md, the browser): the files go, sync commits it, git keeps
// them. With dry it only says what it would delete.
func Remove(v *vault.Vault, p string, dry bool) (Removal, error) {
	rel := strings.Trim(filepath.ToSlash(p), "/")
	if filepath.IsAbs(p) {
		rel = v.RelPath(p)
	}
	info, err := os.Stat(v.Abs(rel))
	if rel == "" || strings.HasPrefix(rel, "../") || err != nil {
		return Removal{}, fmt.Errorf("%s is not in the notes", p)
	}
	if info.IsDir() {
		if !v.FolderEmpty(rel) {
			return Removal{}, fmt.Errorf("%s/ is not empty: move or delete what is in it first", rel)
		}
		r := Removal{Rel: rel, Folder: true, DryRun: dry}
		if dry {
			return r, nil
		}
		return r, v.DeleteFolder(rel)
	}
	if !strings.HasSuffix(rel, ".md") {
		return Removal{}, fmt.Errorf("%s is not a note", rel)
	}
	raw, err := os.ReadFile(v.Abs(rel))
	if err != nil {
		return Removal{}, err
	}
	var images []string
	for _, l := range mdparse.Parse(raw).Links {
		if !l.Image || l.Wiki {
			continue
		}
		if f, ok := localFile(v.Abs(rel), l.Dest); ok {
			if r := v.RelPath(f); !strings.HasPrefix(r, "../") && strings.Contains("/"+r, "/assets/") && isFile(f) {
				images = append(images, r)
			}
		}
	}
	r := Removal{Rel: rel, Images: v.Unshared(rel, images), DryRun: dry}
	ix := scan.Build(v.Root)
	for _, n := range ix.Notes {
		if n.Path == rel {
			r.Title = n.Title
		}
	}
	journal := ""
	if day, err := time.ParseInLocation("2006-01-02", firstDate(path.Base(rel)), time.Local); err == nil {
		journal = v.JournalRel(day)
	}
	for _, from := range ix.Back[rel] {
		if from != journal {
			r.Links++
		}
	}
	if dry {
		return r, nil
	}
	return r, v.Delete(rel, r.Images)
}

// Source is the repository path of a note or folder named on a command line: a path from the
// root of the notes, else from the working directory (absolute then).
func Source(v *vault.Vault, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if _, err := os.Stat(v.Abs(p)); err == nil {
		return p
	}
	if abs, err := filepath.Abs(p); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return p
}

// localFile is the file an image's destination names beside a note, when it names one.
func localFile(note, dest string) (string, bool) {
	if dest == "" || strings.Contains(dest, "://") || strings.HasPrefix(dest, "data:") {
		return "", false
	}
	if d, err := url.PathUnescape(dest); err == nil {
		dest = d
	}
	if filepath.IsAbs(dest) {
		return dest, true
	}
	return filepath.Join(filepath.Dir(note), dest), true
}

// firstDate is the YYYY-MM-DD a file name starts with, "" when it starts with none.
func firstDate(name string) string {
	if len(name) >= 10 {
		if _, err := time.Parse("2006-01-02", name[:10]); err == nil {
			return name[:10]
		}
	}
	return ""
}
