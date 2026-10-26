// Package mdparse reads what the browser needs from a note, with source line numbers: links,
// images and headings. It reads lines, skipping fenced code and inline code, which is enough for
// notes and keeps every item tied to the line it is on (Glamour keeps no map from its output
// back to the source).
package mdparse

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

// Link is a markdown link, an image, or a legacy [[wikilink]].
type Link struct {
	Text  string // the link text, or the alt text of an image
	Dest  string // as written: a URL, a relative path with an optional #anchor, or a wikilink target
	Line  int    // 1-based source line
	Image bool
	Wiki  bool // [[target]] or ![[target]]: Dest is a note name, not a path
}

// Heading is an ATX heading (# to ######).
type Heading struct {
	Level int
	Text  string
	Line  int
}

// Doc is what one note holds.
type Doc struct {
	Links    []Link
	Headings []Heading
}

var (
	mdLink   = regexp.MustCompile(`(!?)\[((?:[^\[\]]|\\[\[\]])*)\]\(\s*(<[^>]*>|[^)\s]+)(?:\s+"[^"]*")?\s*\)`)
	wikiLink = regexp.MustCompile(`(!?)\[\[([^\[\]|#]*)(#[^\[\]|]*)?(?:\|([^\[\]]*))?\]\]`)
	heading  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	imgTag   = regexp.MustCompile(`(?i)<img\b[^>]*\bsrc\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))[^>]*>`)
)

// Parse reads note (frontmatter included; line numbers count from the top of the file).
func Parse(note []byte) Doc {
	var d Doc
	lines := strings.Split(string(note), "\n")
	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if t := strings.TrimSpace(lines[i]); t == "---" || t == "..." {
				start = i + 1
				break
			}
		}
	}
	fence := ""
	for i := start; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = t[:3]
			continue
		}
		if m := heading.FindStringSubmatch(l); m != nil {
			d.Headings = append(d.Headings, Heading{Level: len(m[1]), Text: m[2], Line: i + 1})
		}
		for _, part := range outsideCode(l) {
			for _, m := range mdLink.FindAllStringSubmatch(part, -1) {
				dest := strings.TrimSuffix(strings.TrimPrefix(m[3], "<"), ">")
				d.Links = append(d.Links, Link{Text: unescape(m[2]), Dest: dest, Line: i + 1, Image: m[1] == "!"})
			}
			for _, m := range wikiLink.FindAllStringSubmatch(part, -1) {
				target := strings.TrimSpace(m[2]) + m[3]
				text := m[4]
				if text == "" {
					text = strings.TrimSpace(m[2] + m[3])
				}
				d.Links = append(d.Links, Link{Text: text, Dest: target, Line: i + 1, Image: m[1] == "!", Wiki: true})
			}
			for _, m := range imgTag.FindAllStringSubmatch(part, -1) {
				d.Links = append(d.Links, Link{Dest: m[1] + m[2] + m[3], Line: i + 1, Image: true})
			}
		}
	}
	return d
}

// outsideCode is the parts of a line that are not inline code.
func outsideCode(l string) []string {
	if !strings.Contains(l, "`") {
		return []string{l}
	}
	parts := strings.Split(l, "`")
	var out []string
	for i := 0; i < len(parts); i += 2 {
		out = append(out, parts[i])
	}
	return out
}

func unescape(s string) string { return strings.NewReplacer(`\[`, "[", `\]`, "]").Replace(s) }

// External reports a destination that leaves the repository: a URL with a scheme, mailto: and
// the like.
func External(dest string) bool {
	u, err := url.Parse(dest)
	return err == nil && u.Scheme != "" && len(u.Scheme) > 1 // c:\… is a Windows path, not a scheme
}

// Resolve turns a relative destination in the note at repository path from into a repository
// path and an anchor. ok is false for URLs, pure anchors and paths that leave the repository.
func Resolve(from, dest string) (rel, anchor string, ok bool) {
	if dest == "" || External(dest) {
		return "", "", false
	}
	p, anchor, _ := strings.Cut(dest, "#")
	if p == "" {
		return from, anchor, true // #heading in the same note
	}
	if d, err := url.PathUnescape(p); err == nil {
		p = d
	}
	if strings.HasPrefix(p, "/") {
		return "", "", false
	}
	rel = path.Clean(path.Join(path.Dir(from), p))
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", "", false
	}
	return rel, anchor, true
}
