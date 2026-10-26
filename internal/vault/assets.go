package vault

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
)

var datePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// SaveAsset writes an attachment for the note at noteRel: assets/YYYY-MM-DD-<slug><ext> in the
// note's folder (docs/format.md, Attachments), named after name, or "image" when name has no
// letters or digits. A name that already starts with a date keeps it instead of getting a second.
// It returns the attachment's repository path; the note links it as assets/<file>.
func (v *Vault) SaveAsset(noteRel, name, ext string, data []byte, t time.Time) (string, error) {
	slug := Slug(name)
	if slug == "" {
		slug = "image"
	}
	if !datePrefix.MatchString(slug) {
		slug = v.Day(t).Format("2006-01-02") + "-" + slug
	}
	ext = strings.ToLower(ext)
	if ext == ".jpeg" {
		ext = ".jpg"
	}
	dir := path.Join(path.Dir(noteRel), "assets")
	if strings.HasPrefix(dir, ".") || strings.Contains(dir, "/.") {
		return "", fmt.Errorf("%s: notes in folders starting with . get no attachments", noteRel)
	}
	unlock, err := v.Lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	rel := v.Unique(dir + "/" + slug + ext)
	return rel, WriteFile(v.Abs(rel), data)
}
