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
	"unicode"
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
	mdLink   = regexp.MustCompile(`(!?)\[((?:[^\[\]]|\\[\[\]])*)\]\(\s*(<[^>]*>|[^)\s]+)((?:\s+"[^"]*")?)\s*\)`)
	wikiLink = regexp.MustCompile(`(!?)\[\[([^\[\]|#]*)(#[^\[\]|]*)?(?:\|([^\[\]]*))?\]\]`)
	heading  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	imgTag   = regexp.MustCompile(`(?i)<img\b[^>]*\bsrc\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))[^>]*>`)
	refDef   = regexp.MustCompile(`^( {0,3}\[([^\]^][^\]]*)\]:[ \t]*)(<[^>]*>|\S+)(.*)$`)
)

// Parse reads note (frontmatter included; line numbers count from the top of the file).
func Parse(note []byte) Doc {
	var d Doc
	lines := strings.Split(string(note), "\n")
	inText := text(lines)
	for i, l := range lines {
		if !inText(i) {
			continue
		}
		if m := heading.FindStringSubmatch(l); m != nil {
			d.Headings = append(d.Headings, Heading{Level: len(m[1]), Text: m[2], Line: i + 1})
		}
		if m := refDef.FindStringSubmatch(l); m != nil {
			d.Links = append(d.Links, Link{Text: m[2], Dest: strings.Trim(m[3], "<>"), Line: i + 1})
			continue
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

// text returns whether line i of lines is text: not frontmatter at the top, not inside a fenced
// code block or its fence lines. It is called with i rising.
func text(lines []string) func(i int) bool {
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
	return func(i int) bool {
		if i < start {
			return false
		}
		t := strings.TrimSpace(lines[i])
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			return false
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = t[:3]
			return false
		}
		return true
	}
}

// Rewrite gives fn every link Parse finds, in order, and writes back what fn changes: the text
// and destination of a markdown link or image, the destination of a reference definition or an
// <img> tag. A [[wikilink]] fn changes becomes a markdown link to l.Dest. Frontmatter, code and
// everything else stay byte for byte. n is how many links changed.
func Rewrite(note []byte, fn func(l *Link) bool) (out []byte, n int) {
	lines := strings.Split(string(note), "\n")
	inText := text(lines)
	for i, l := range lines {
		if !inText(i) {
			continue
		}
		if m := refDef.FindStringSubmatch(l); m != nil {
			k := Link{Text: m[2], Dest: strings.Trim(m[3], "<>"), Line: i + 1}
			if fn(&k) {
				lines[i] = m[1] + k.Dest + m[4]
				n++
			}
			continue
		}
		lines[i] = mapOutsideCode(l, func(s string) string {
			s = mdLink.ReplaceAllStringFunc(s, func(all string) string {
				m := mdLink.FindStringSubmatch(all)
				k := Link{Text: unescape(m[2]), Dest: strings.TrimSuffix(strings.TrimPrefix(m[3], "<"), ">"), Line: i + 1, Image: m[1] == "!"}
				was := k
				if !fn(&k) {
					return all
				}
				n++
				text, dest := m[2], m[3]
				if k.Text != was.Text {
					text = escape(k.Text)
				}
				if k.Dest != was.Dest {
					dest = k.Dest
				}
				return m[1] + "[" + text + "](" + dest + m[4] + ")"
			})
			s = wikiLink.ReplaceAllStringFunc(s, func(all string) string {
				m := wikiLink.FindStringSubmatch(all)
				k := Link{Text: m[4], Dest: strings.TrimSpace(m[2]) + m[3], Line: i + 1, Image: m[1] == "!", Wiki: true}
				if k.Text == "" {
					k.Text = strings.TrimSpace(m[2] + m[3])
				}
				if !fn(&k) {
					return all
				}
				n++
				return m[1] + "[" + escape(k.Text) + "](" + k.Dest + ")"
			})
			return imgTag.ReplaceAllStringFunc(s, func(all string) string {
				m := imgTag.FindStringSubmatchIndex(all)
				g := 2
				for m[g] < 0 {
					g += 2
				}
				k := Link{Dest: all[m[g]:m[g+1]], Line: i + 1, Image: true}
				if !fn(&k) {
					return all
				}
				n++
				return all[:m[g]] + k.Dest + all[m[g+1]:]
			})
		})
	}
	return []byte(strings.Join(lines, "\n")), n
}

// mapOutsideCode applies fn to the parts of a line that are not inline code.
func mapOutsideCode(l string, fn func(string) string) string {
	if !strings.Contains(l, "`") {
		return fn(l)
	}
	parts := strings.Split(l, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = fn(parts[i])
	}
	return strings.Join(parts, "`")
}

// Anchor is GitHub's anchor for a heading: lowercase, spaces to "-", other punctuation dropped.
func Anchor(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
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
func escape(s string) string   { return strings.NewReplacer("[", `\[`, "]", `\]`).Replace(s) }

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
