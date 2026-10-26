package render

import (
	"bytes"
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

// imageBlock is an image that is the whole of a top-level paragraph: the only images the preview
// draws in place. An image inside a sentence, a list or a table keeps its alt text.
type imageBlock struct {
	dest, alt  string
	start, end int // the paragraph's bytes in the body
}

func imageBlocks(body []byte) []imageBlock {
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(body))
	var out []imageBlock
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		p, ok := n.(*ast.Paragraph)
		if !ok || p.ChildCount() != 1 || p.Lines().Len() == 0 {
			continue
		}
		img, ok := p.FirstChild().(*ast.Image)
		if !ok {
			continue
		}
		lines := p.Lines()
		out = append(out, imageBlock{
			dest:  string(img.Destination),
			alt:   plainText(img, body),
			start: lines.At(0).Start,
			end:   lines.At(lines.Len() - 1).Stop,
		})
	}
	return out
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
		src = append(append(append([]byte{}, src[:b.start]...), m.marker...), src[b.end:]...)
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
