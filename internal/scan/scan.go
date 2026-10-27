// Package scan builds the in-memory picture of a notes repository the browser needs: every
// note's title and which notes link to which. Nothing is written to disk: the files are the
// truth and the index is rebuilt when they change (docs/design.md §4).
package scan

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/w4jnl/nt/internal/mdparse"
	"github.com/w4jnl/nt/internal/render"
)

// Note is one note in the index.
type Note struct {
	Path  string // repository path
	Title string
}

// Index is the repository's notes and links.
type Index struct {
	Notes []Note              // every note, by path
	Back  map[string][]string // note → notes that link to it, sorted
	title map[string]string
	stems map[string][]string // lowercased file stem and title → paths, for legacy wikilinks
}

// Build reads every note under root, skipping folders that start with a dot.
func Build(root string) *Index {
	ix := &Index{Back: map[string][]string{}, title: map[string]string{}, stems: map[string][]string{}}
	notes := map[string][]byte{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && p != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		notes[rel] = b
		t := render.Title(b, rel)
		ix.Notes = append(ix.Notes, Note{Path: rel, Title: t})
		ix.title[rel] = t
		stem := strings.ToLower(strings.TrimSuffix(path.Base(rel), ".md"))
		ix.stems[stem] = append(ix.stems[stem], rel)
		if lt := strings.ToLower(t); lt != stem {
			ix.stems[lt] = append(ix.stems[lt], rel)
		}
		return nil
	})
	sort.Slice(ix.Notes, func(i, j int) bool { return ix.Notes[i].Path < ix.Notes[j].Path })
	for from, b := range notes {
		seen := map[string]bool{}
		for _, l := range mdparse.Parse(b).Links {
			to, ok := ix.Target(from, l)
			if !ok || to == from || seen[to] || ix.title[to] == "" {
				continue
			}
			seen[to] = true
			ix.Back[to] = append(ix.Back[to], from)
		}
	}
	for k := range ix.Back {
		sort.Strings(ix.Back[k])
	}
	return ix
}

// Title is a note's title, or "" for a path the index does not hold.
func (ix *Index) Title(rel string) string { return ix.title[rel] }

// Target is the repository path a link in the note at from points to: a relative path, or for
// a legacy wikilink the note whose file stem or title matches, nearest to from.
func (ix *Index) Target(from string, l mdparse.Link) (string, bool) {
	if !l.Wiki {
		rel, _, ok := mdparse.Resolve(from, l.Dest)
		return rel, ok
	}
	name, _, _ := strings.Cut(l.Dest, "#")
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), ".md"))
	if name == "" {
		return from, true
	}
	if _, ok := ix.title[name+".md"]; ok {
		return name + ".md", true
	}
	cands := ix.stems[path.Base(name)]
	if len(cands) == 0 {
		return "", false
	}
	best, score := cands[0], -1
	for _, c := range cands {
		if s := shared(path.Dir(from), path.Dir(c)); s > score {
			best, score = c, s
		}
	}
	return best, true
}

func shared(a, b string) int {
	pa, pb := strings.Split(a, "/"), strings.Split(b, "/")
	n := 0
	for n < len(pa) && n < len(pb) && pa[n] == pb[n] {
		n++
	}
	return n
}

// BrokenLink is a link in a note that points at nothing.
type BrokenLink struct {
	From string `json:"from"` // the note it is in
	Line int    `json:"line"`
	Dest string `json:"dest"` // as written
}

// BrokenLinks lists the links in the notes under root that point at nothing: relative links and
// images to files that are not there, and legacy wikilinks no note answers to. Code, URLs, links
// to a heading in the same note and conflict copies are left out, and so are the targets skip
// says may be missing by design, such as the journal of a day that has none. A reference
// definition counts only when its destination looks like a path, so "[Note]: call Bob" is prose.
func BrokenLinks(root string, skip func(rel string) bool) []BrokenLink {
	ix := Build(root)
	var out []BrokenLink
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && p != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") || strings.HasPrefix(d.Name(), ".") || strings.Contains(d.Name(), ".conflict-") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		from := filepath.ToSlash(rel)
		for _, l := range mdparse.Parse(b).Links {
			switch {
			case l.Dest == "" || mdparse.External(l.Dest) || strings.HasPrefix(l.Dest, "#"):
				continue
			case l.Ref && !strings.ContainsAny(l.Dest, "./"):
				continue
			case l.Wiki:
				if _, ok := ix.Target(from, l); !ok && !l.Image {
					out = append(out, BrokenLink{from, l.Line, "[[" + l.Dest + "]]"})
				}
				continue
			}
			to, _, ok := mdparse.Resolve(from, l.Dest)
			if !ok || skip != nil && skip(to) {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(to))); err != nil {
				out = append(out, BrokenLink{from, l.Line, l.Dest})
			}
		}
		return nil
	})
	return out
}
