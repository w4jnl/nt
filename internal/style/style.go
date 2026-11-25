// Package style finds the person's own rules for writing and drawing (docs/format.md §15): a
// note named style.md in the root of the notes holds everywhere, one in a folder for that
// folder and the folders under it. Agents read them before they write, over the style nt's
// skill and MCP server recommend.
package style

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Name is the file name of a style note.
const Name = "style.md"

// Max is how much of one style note is read, in bytes: it goes into an agent's context.
const Max = 16 << 10

// Note is one style note: its repository path and its text, without the frontmatter.
type Note struct {
	Rel  string `json:"rel"`
	Text string `json:"text"`
}

// For is the style notes that hold for at, a folder or a note as a repository path ("" the
// root): the root's first, then each folder's on the way down, so the last one is the nearest.
func For(root, at string) []Note {
	at = strings.Trim(path.Clean("/"+filepath.ToSlash(at)), "/")
	if strings.HasSuffix(at, ".md") {
		at = strings.Trim(path.Dir(at), "./")
	}
	dirs := []string{""}
	if at != "" {
		for _, p := range strings.Split(at, "/") {
			dirs = append(dirs, path.Join(dirs[len(dirs)-1], p))
		}
	}
	var out []Note
	for _, d := range dirs {
		rel := path.Join(d, Name)
		if n, ok := read(root, rel); ok {
			out = append(out, n)
		}
	}
	return out
}

// All is the repository path of every style note, the root's first, then by path. Folders
// whose name starts with a dot are not looked in.
func All(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == Name && d.Type().IsRegular() {
			if rel, err := filepath.Rel(root, p); err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if a, b := strings.Count(out[i], "/"), strings.Count(out[j], "/"); (a == 0) != (b == 0) {
			return a == 0
		}
		return out[i] < out[j]
	})
	return out
}

// Folder is the folder a style note holds for, "" the root.
func Folder(rel string) string {
	return strings.Trim(path.Dir(rel), "./")
}

func read(root, rel string) (Note, bool) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	if st, err := os.Lstat(p); err != nil || !st.Mode().IsRegular() {
		return Note{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Note{}, false
	}
	text := strings.TrimSpace(body(string(b)))
	if text == "" {
		return Note{}, false
	}
	if len(text) > Max {
		text = strings.ToValidUTF8(text[:Max], "") + "\n\n(the rest of " + rel + " is not shown: read the note)"
	}
	return Note{Rel: rel, Text: text}, true
}

// body is a note's text without its frontmatter.
func body(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return s
	}
	if i := strings.Index(s[4:], "\n---\n"); i >= 0 {
		return s[4+i+5:]
	}
	if strings.HasSuffix(s, "\n---") {
		return ""
	}
	return s
}
