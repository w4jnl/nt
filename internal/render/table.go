package render

import (
	"bytes"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	astext "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/w4jnl/nt/internal/theme"
)

// A table is laid out by nt, not by Glamour. Glamour's table (lipgloss) takes cells from the
// widest column with no notion of a word, so a narrow column loses a letter to the next line
// ("Hig" / "h") while the prose columns keep slack. nt sizes columns as a browser does: a
// column is at least its longest word wide, and what is left goes to the columns with the most
// to wrap. The cells are drawn in Glamour's own look: the same ink, the same separators, inline
// markdown rendered as it renders it.

// tableBlock is a GFM table at the top level of the body.
type tableBlock struct {
	cells      [][]string // the header row first; each cell its markdown
	aligns     []astext.Alignment
	start, end int // its bytes in the body
}

// tableBlocks finds the tables at the top level of the body. One inside a list stays with
// Glamour.
func tableBlocks(body []byte) []tableBlock {
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(body))
	var out []tableBlock
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		t, ok := n.(*astext.Table)
		if !ok {
			continue
		}
		b := tableBlock{aligns: t.Alignments, start: -1}
		for row := t.FirstChild(); row != nil; row = row.NextSibling() {
			var cells []string
			for c := row.FirstChild(); c != nil; c = c.NextSibling() {
				var s strings.Builder
				lines := c.Lines()
				for i := 0; i < lines.Len(); i++ {
					seg := lines.At(i)
					s.Write(body[seg.Start:seg.Stop])
					if b.start < 0 || seg.Start < b.start {
						b.start = seg.Start
					}
					b.end = max(b.end, seg.Stop)
				}
				cells = append(cells, s.String())
			}
			b.cells = append(b.cells, cells)
		}
		if b.start < 0 {
			continue
		}
		b.start = bytes.LastIndexByte(body[:b.start], '\n') + 1 // the header's whole line
		b.end = lineEnd(body, b.end)
		if len(b.cells) == 1 && b.end < len(body) { // a header alone: the delimiter row is the next line
			b.end = lineEnd(body, b.end+1)
		}
		out = append(out, b)
	}
	return out
}

// lineEnd is the offset of the newline that ends the line holding at, or the body's end.
func lineEnd(body []byte, at int) int {
	if nl := bytes.IndexByte(body[at:], '\n'); nl >= 0 {
		return at + nl
	}
	return len(body)
}

// cell renders a cell's markdown as Glamour renders inline text: one styled line, unwrapped.
// A cell holds inline markdown only (GFM), but alone on a line "#" or "- a" would be a block,
// so the cell is rendered behind a word, then cut off it.
func (r *Renderer) cell(md string) string {
	if strings.TrimSpace(md) == "" {
		return ""
	}
	out, err := r.cells.Render("x " + md)
	if err != nil {
		return md
	}
	out = strings.ReplaceAll(out, base, "")
	out = strings.Trim(out, "\n")
	out = strings.TrimPrefix(out, strings.Repeat(" ", r.margin))
	if i := strings.IndexByte(out, '\n'); i >= 0 {
		out = out[:i]
	}
	out = xansi.Cut(out, 2, xansi.StringWidth(out))
	return strings.TrimRight(out, " ")
}

// wordCap is the longest word that keeps a column whole when the words of a table do not all
// fit: a date, a state, a short name. A longer one (a path, a long token) breaks at the
// column's edge, as a code block's line does, so the prose columns keep room to read.
const wordCap = 10

// words is a cell's text in the runs its wrapper keeps together: split at spaces, not at the
// non-breaking spaces around inline code.
func words(s string) []string {
	return strings.FieldsFunc(xansi.Strip(s), func(r rune) bool { return r == ' ' || r == '\t' })
}

// tableWidths gives each column its width within avail: every column its widest cell when they
// all fit, else at least its longest word, with the room left shared by how much each has to
// wrap. When even the words do not fit, a word longer than wordCap breaks at the edge instead;
// when the short words still do not fit, the widest columns give a cell at a time, down to one.
func tableWidths(maxW, minW []int, avail int) []int {
	n := len(maxW)
	w := make([]int, n)
	lo := make([]int, n)
	sumMax, sumMin := 0, 0
	for j := range w {
		sumMax += maxW[j]
		sumMin += minW[j]
	}
	sumLo := sumMin
	copy(lo, minW)
	if sumMin > avail {
		sumLo = 0
		for j := range lo {
			lo[j] = min(minW[j], wordCap)
			sumLo += lo[j]
		}
	}
	switch {
	case sumMax <= avail:
		copy(w, maxW)
	case sumLo >= avail:
		copy(w, lo)
		for over := sumLo - avail; over > 0; over-- {
			k := -1
			for j := range w {
				if w[j] > 1 && (k < 0 || w[j] > w[k]) {
					k = j
				}
			}
			if k < 0 {
				break
			}
			w[k]--
		}
	default:
		extra, room, given := avail-sumLo, sumMax-sumLo, 0
		for j := range w {
			w[j] = lo[j] + extra*(maxW[j]-lo[j])/room
			given += w[j] - lo[j]
		}
		for rest := extra - given; rest > 0; rest-- { // the remainder, a cell to each column with the most left to wrap
			k := -1
			for j := range w {
				if w[j] < maxW[j] && (k < 0 || maxW[j]-w[j] > maxW[k]-w[k]) {
					k = j
				}
			}
			if k < 0 {
				break
			}
			w[k]++
		}
	}
	return w
}

// tableRows draws a table width cells wide, as Glamour draws one: a cell of space on each
// side of a cell, "│" between columns, a rule of "─" and "┼" under the header, no rule between
// rows. A cell wraps at its spaces to its column's width; a lone word wider than the column
// breaks at the edge.
func (r *Renderer) tableRows(t tableBlock, width int) []string {
	ncol := len(t.aligns)
	for _, row := range t.cells {
		ncol = max(ncol, len(row))
	}
	if ncol == 0 {
		return nil
	}
	cells := make([][]string, len(t.cells))
	maxW, minW := make([]int, ncol), make([]int, ncol)
	for i, row := range t.cells {
		cells[i] = make([]string, ncol)
		for j := range ncol {
			if j >= len(row) {
				continue
			}
			s := r.cell(row[j])
			cells[i][j] = s
			maxW[j] = max(maxW[j], xansi.StringWidth(s))
			for _, word := range words(s) {
				minW[j] = max(minW[j], xansi.StringWidth(word))
			}
		}
	}
	for j := range ncol {
		minW[j] = max(1, minW[j])
		maxW[j] = max(maxW[j], minW[j])
	}
	widths := tableWidths(maxW, minW, max(ncol, width-3*ncol+1)) // a space each side of a cell, "│" between
	ink := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Hex("ink", r.dark)))
	sep := ink.Render(" │ ")
	align := func(j int) astext.Alignment {
		if j < len(t.aligns) {
			return t.aligns[j]
		}
		return astext.AlignNone
	}
	var rows []string
	for i, row := range cells {
		lines := make([][]string, ncol)
		height := 1
		for j, s := range row {
			if s == "" {
				continue
			}
			lines[j] = strings.Split(xansi.Wrap(s, widths[j], ""), "\n")
			height = max(height, len(lines[j]))
		}
		for k := range height {
			var b strings.Builder
			b.WriteString(" ")
			for j := range ncol {
				if j > 0 {
					b.WriteString(sep)
				}
				s := ""
				if k < len(lines[j]) {
					s = lines[j][k]
				}
				b.WriteString(alignCell(s, widths[j], align(j)))
			}
			rows = append(rows, strings.TrimRight(b.String(), " "))
		}
		if i == 0 {
			var b strings.Builder
			for j, w := range widths {
				if j > 0 {
					b.WriteString("┼")
				}
				b.WriteString(strings.Repeat("─", w+2))
			}
			rows = append(rows, ink.Render(b.String()))
		}
	}
	return rows
}

// alignCell pads s to w cells, the text at the left, the right or the middle.
func alignCell(s string, w int, a astext.Alignment) string {
	gap := w - xansi.StringWidth(s)
	if gap <= 0 {
		return s
	}
	switch a {
	case astext.AlignRight:
		return strings.Repeat(" ", gap) + s
	case astext.AlignCenter:
		return strings.Repeat(" ", gap/2) + s + strings.Repeat(" ", gap-gap/2)
	}
	return s + strings.Repeat(" ", gap)
}

// drawn is a block nt draws in place of Glamour: its marker in the markdown, the rows that
// replace the marker's line, and the text that stands in when Glamour moved the marker off a
// line of its own.
type drawn struct {
	start, end  int
	marker, alt string
	rows        []string
}

// ownBlocks is every block nt draws itself (code blocks, tables, alerts), last first, so
// splicing one leaves the earlier offsets as they are.
func (r *Renderer) ownBlocks(body []byte, nonce string, width int, place Placer) []drawn {
	var out []drawn
	for i, a := range alertBlocks(body) {
		out = append(out, drawn{start: a.start, end: a.end, marker: nonce + itoa(i) + "a", alt: strings.TrimSpace(string(body[a.start:a.end])), rows: r.alertRows(a, width, place)})
	}
	for i, b := range codeBlocks(body) {
		out = append(out, drawn{start: b.start, end: b.end, marker: nonce + itoa(i) + "c", alt: strings.TrimRight(b.code, "\n"), rows: r.codeRows(b, width)})
	}
	for i, t := range tableBlocks(body) {
		out = append(out, drawn{start: t.start, end: t.end, marker: nonce + itoa(i) + "t", alt: strings.TrimSpace(string(body[t.start:t.end])), rows: r.tableRows(t, width)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start > out[j].start })
	return out
}
