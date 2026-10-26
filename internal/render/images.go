package render

import (
	"bytes"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// Placer draws an image in the preview. It gets the image's link destination as written in the
// note and the widest it may be, in cells, and returns the rows that stand for it, or false to
// keep Glamour's own rendering (the alt text and the link).
type Placer func(dest string, width int) (rows []string, ok bool)

// imageBlock is an image that is the whole of a top-level block: a paragraph holding only
// ![alt](path), or an HTML block holding only one <img src> (wrapper tags like <p align="center">
// allowed). These are the images the preview draws in place; an image inside a sentence, a list
// or a table keeps its alt text.
type imageBlock struct {
	dest, alt  string
	start, end int // the paragraph's bytes in the body
}

func imageBlocks(body []byte) []imageBlock {
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(body))
	var out []imageBlock
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		lines := n.Lines()
		if n.Type() != ast.TypeBlock || lines == nil || lines.Len() == 0 {
			continue
		}
		start, end := lines.At(0).Start, lines.At(lines.Len()-1).Stop
		switch b := n.(type) {
		case *ast.Paragraph:
			if c, ok := b.FirstChild().(*ast.Image); ok && b.ChildCount() == 1 {
				out = append(out, imageBlock{dest: string(c.Destination), alt: plainText(c, body), start: start, end: end})
			} else if onlyHTML(b, body) {
				// inline tags only, like <a href="…"><img src="…"></a>
				if dest, alt, ok := htmlImage(body[start:end]); ok {
					out = append(out, imageBlock{dest: dest, alt: alt, start: start, end: end})
				}
			}
		case *ast.HTMLBlock:
			if b.HasClosure() {
				end = b.ClosureLine.Stop
			}
			if dest, alt, ok := htmlImage(body[start:end]); ok {
				out = append(out, imageBlock{dest: dest, alt: alt, start: start, end: end})
			}
		}
	}
	return out
}

var (
	imgTag  = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	anyTag  = regexp.MustCompile(`(?s)<[^>]*>`)
	imgAttr = regexp.MustCompile(`(?is)\b(src|alt)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
)

// onlyHTML reports a paragraph made of inline HTML tags and blanks, nothing else.
func onlyHTML(p ast.Node, src []byte) bool {
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.RawHTML:
		case *ast.Text:
			if strings.TrimSpace(string(t.Segment.Value(src))) != "" {
				return false
			}
		default:
			return false
		}
	}
	return p.ChildCount() > 0
}

// htmlImage reads the one <img> in a piece of HTML that holds nothing else but tags around it.
func htmlImage(h []byte) (dest, alt string, ok bool) {
	tags := imgTag.FindAll(h, -1)
	if len(tags) != 1 || strings.TrimSpace(html.UnescapeString(string(anyTag.ReplaceAll(h, nil)))) != "" {
		return "", "", false
	}
	for _, m := range imgAttr.FindAllSubmatch(tags[0], -1) {
		v := html.UnescapeString(string(m[2]) + string(m[3]) + string(m[4]))
		if strings.EqualFold(string(m[1]), "src") {
			dest = v
		} else {
			alt = v
		}
	}
	return dest, alt, dest != ""
}

func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); entering && ok {
			b.Write(t.Segment.Value(src))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// RenderImages renders a note like Render, and draws each image that stands alone in its
// paragraph with place. The paragraph becomes a marker word before Glamour runs; afterwards the
// line that holds the marker is replaced by the image's rows, at the marker's indent.
func (r *Renderer) RenderImages(note []byte, place Placer) (string, error) {
	_, body := SplitFrontmatter(note)
	if place == nil {
		return r.tr.Render(string(body))
	}
	width := max(1, r.width-2*r.margin)
	type drawn struct {
		marker, alt string
		rows        []string
	}
	var marks []drawn
	nonce := markerNonce(body)
	src := body
	blocks := imageBlocks(body)
	for i := len(blocks) - 1; i >= 0; i-- { // from the end, so earlier offsets stay valid
		b := blocks[i]
		rows, ok := place(b.dest, width)
		if !ok || len(rows) == 0 {
			continue
		}
		m := drawn{marker: nonce + strconv.Itoa(i) + "x", alt: b.alt, rows: rows}
		marks = append(marks, m)
		// keep the newlines the block ends with, so it stays a block of its own
		repl := m.marker + strings.Repeat("\n", len(src[b.start:b.end])-len(bytes.TrimRight(src[b.start:b.end], "\n")))
		src = append(append(append([]byte{}, src[:b.start]...), repl...), src[b.end:]...)
	}
	out, err := r.tr.Render(string(src))
	if err != nil || len(marks) == 0 {
		return out, err
	}
	lines := strings.Split(out, "\n")
	var res []string
	for _, l := range lines {
		plain := ansi.Strip(l)
		hit := false
		for _, m := range marks {
			if strings.TrimSpace(plain) != m.marker {
				continue
			}
			indent := strings.Repeat(" ", ansi.StringWidth(plain[:strings.Index(plain, m.marker)]))
			for _, row := range m.rows {
				res = append(res, indent+row)
			}
			hit = true
			break
		}
		if !hit {
			res = append(res, l)
		}
	}
	out = strings.Join(res, "\n")
	for _, m := range marks { // a marker Glamour moved off a line of its own falls back to the alt text
		out = strings.ReplaceAll(out, m.marker, m.alt)
	}
	return out, nil
}

// markerNonce is a word that does not occur in the note, so a marker never matches its text.
func markerNonce(body []byte) string {
	n := "ntimage"
	for bytes.Contains(body, []byte(n)) {
		n += "x"
	}
	return n
}
