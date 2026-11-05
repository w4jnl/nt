package render

import (
	"bytes"
	"regexp"
	"strings"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/w4jnl/nt/internal/theme"
)

// A fenced code block is drawn by nt, not by Glamour. Glamour wraps a block's lines at spaces
// and punctuation, as it wraps prose, so the columns of a command's output fall apart. nt breaks
// a line at the block's edge, every line at the same column, as vim does with wrap set: a table
// of output keeps its columns. The block is on surface-sunk across the preview's width, the
// language faint at its top right, the code coloured from the family palette (codeStyle).

// codeBlock is a fenced block at the top level of the body, fences included.
type codeBlock struct {
	lang, code string
	start, end int // its bytes in the body
}

var openFence = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})[^\n]*\n$")

// codeBlocks finds the fenced blocks at the top level of the body. One inside a list stays with
// Glamour.
func codeBlocks(body []byte) []codeBlock {
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(body))
	var out []codeBlock
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		f, ok := n.(*ast.FencedCodeBlock)
		if !ok || f.Lines().Len() == 0 {
			continue
		}
		lines := f.Lines()
		first, last := lines.At(0).Start, lines.At(lines.Len()-1).Stop
		if first == 0 {
			continue
		}
		start := bytes.LastIndexByte(body[:first-1], '\n') + 1 // the opening fence is the line before the code
		m := openFence.FindSubmatch(body[start:first])
		if m == nil {
			continue
		}
		var code strings.Builder
		for i := 0; i < lines.Len(); i++ {
			s := lines.At(i)
			code.Write(body[s.Start:s.Stop])
		}
		end := last
		if end > 0 && body[end-1] == '\n' {
			end--
		}
		if last < len(body) && body[last-1] == '\n' { // the closing fence, when the block has one
			line := body[last:]
			if nl := bytes.IndexByte(line, '\n'); nl >= 0 {
				line = line[:nl]
			}
			if t := strings.TrimSpace(string(line)); strings.HasPrefix(t, string(m[1])) && strings.Trim(t, string(m[1][:1])) == "" {
				end = last + len(line)
			}
		}
		out = append(out, codeBlock{lang: string(f.Language(body)), code: code.String(), start: start, end: end})
	}
	return out
}

// codeStyle is the palette as a chroma style, for the blocks nt draws itself.
func codeStyle(dark bool) *chroma.Style {
	p := palette(dark)
	entries := chroma.StyleEntries{}
	for tt, sp := range map[chroma.TokenType]ansi.StylePrimitive{
		chroma.Text: p.Text, chroma.Error: p.Error,
		chroma.Comment: p.Comment, chroma.CommentPreproc: p.CommentPreproc,
		chroma.Keyword: p.Keyword, chroma.KeywordReserved: p.KeywordReserved,
		chroma.KeywordNamespace: p.KeywordNamespace, chroma.KeywordType: p.KeywordType,
		chroma.Operator: p.Operator, chroma.Punctuation: p.Punctuation,
		chroma.Name: p.Name, chroma.NameBuiltin: p.NameBuiltin, chroma.NameTag: p.NameTag,
		chroma.NameAttribute: p.NameAttribute, chroma.NameClass: p.NameClass,
		chroma.NameConstant: p.NameConstant, chroma.NameDecorator: p.NameDecorator,
		chroma.NameFunction:  p.NameFunction,
		chroma.LiteralNumber: p.LiteralNumber, chroma.LiteralString: p.LiteralString,
		chroma.LiteralStringEscape: p.LiteralStringEscape,
		chroma.GenericDeleted:      p.GenericDeleted, chroma.GenericInserted: p.GenericInserted,
		chroma.GenericSubheading: p.GenericSubheading,
	} {
		var e []string
		if sp.Color != nil {
			e = append(e, *sp.Color)
		}
		if sp.Bold != nil && *sp.Bold {
			e = append(e, "bold")
		}
		if len(e) > 0 {
			entries[tt] = strings.Join(e, " ")
		}
	}
	st, err := chroma.NewStyle("nt", entries)
	if err != nil {
		return chroma.MustNewStyle("nt-plain", chroma.StyleEntries{})
	}
	return st
}

// codeRows draws a block width cells wide: a row with the language at the right when the fence
// names one, then the code, a cell of the background on each side, every line broken at the
// block's edge. Tabs are four cells; other control characters are left out.
func (r *Renderer) codeRows(b codeBlock, width int) []string {
	inner := max(1, width-2)
	bg := lipgloss.Color(theme.Hex("surface-sunk", r.dark))
	ground := lipgloss.NewStyle().Background(bg)
	pad := func(n int) string {
		if n <= 0 {
			return ""
		}
		return ground.Render(strings.Repeat(" ", n))
	}
	var rows []string
	if lang := strings.TrimSpace(b.lang); lang != "" {
		lang = xansi.Truncate(lang, inner, "")
		muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Hex("ink-muted", r.dark))).Background(bg)
		rows = append(rows, pad(1+inner-xansi.StringWidth(lang))+muted.Render(lang)+pad(1))
	}
	lex := lexers.Get(b.lang)
	if lex == nil {
		lex = lexers.Fallback
	}
	it, err := chroma.Coalesce(lex).Tokenise(nil, b.code)
	if err != nil {
		it = chroma.Literator(chroma.Token{Type: chroma.Text, Value: b.code})
	}
	ink := lipgloss.Color(theme.Hex("ink", r.dark))
	styles := map[chroma.TokenType]lipgloss.Style{}
	styleOf := func(tt chroma.TokenType) lipgloss.Style {
		if s, ok := styles[tt]; ok {
			return s
		}
		e := r.code.Get(tt)
		s := lipgloss.NewStyle().Foreground(ink).Background(bg)
		if e.Colour.IsSet() {
			s = s.Foreground(lipgloss.Color(e.Colour.String()))
		}
		if e.Bold == chroma.Yes {
			s = s.Bold(true)
		}
		styles[tt] = s
		return s
	}
	var line, run strings.Builder
	col := 0
	flush := func() {
		rows = append(rows, pad(1)+line.String()+pad(1+inner-col))
		line.Reset()
		col = 0
	}
	for tok := it(); tok != chroma.EOF; tok = it() {
		st := styleOf(tok.Type)
		endRun := func() {
			if run.Len() > 0 {
				line.WriteString(st.Render(run.String()))
				run.Reset()
			}
		}
		put := func(s string, w int) {
			if col+w > inner {
				endRun()
				flush()
			}
			run.WriteString(s)
			col += w
		}
		for _, c := range tok.Value {
			switch {
			case c == '\n':
				endRun()
				flush()
			case c == '\t':
				for n := 4 - col%4; n > 0; n-- {
					put(" ", 1)
				}
			case c < 0x20 || c == 0x7f:
			default:
				put(string(c), xansi.StringWidth(string(c)))
			}
		}
		endRun()
	}
	if col > 0 || line.Len() > 0 {
		flush()
	}
	return rows
}
