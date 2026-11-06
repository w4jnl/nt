package render

import (
	"bytes"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/w4jnl/nt/internal/theme"
)

// An alert is GitHub's note box (docs/format.md): a block quote whose first line is a tag,
// `> [!NOTE]`, `> [!TIP]`, `> [!IMPORTANT]`, `> [!WARNING]` or `> [!CAUTION]`. GitHub draws it
// as a box with a title; any other viewer shows a quote with the tag, so nothing is lost. nt
// draws it as a rule down the left in the kind's colour, the kind's word bold in that colour
// as a title, and the body in ink, rendered as the rest of the note is (lists, code, links and
// images inside it included).

// alertBlock is an alert at the top level of the body.
type alertBlock struct {
	kind       string // note, tip, important, warning, caution
	body       []byte // the quote's markdown without its marks and its tag line
	start, end int    // its bytes in the body
}

var (
	alertTag  = regexp.MustCompile(`^\[!(?i:(NOTE|TIP|IMPORTANT|WARNING|CAUTION))\]\s*$`)
	quoteMark = regexp.MustCompile(`^ {0,3}> ?`)
)

// alertKinds maps a kind to its colour token and the title it is drawn with.
var alertKinds = map[string]struct{ token, title string }{
	"note":      {"accent", "Note"},
	"tip":       {"ok", "Tip"},
	"important": {"accent", "Important"},
	"warning":   {"warn", "Warning"},
	"caution":   {"alert", "Caution"},
}

// alertBlocks finds the alerts at the top level of the body. A quote inside a list, or one with
// no tag on its first line, stays with Glamour.
func alertBlocks(body []byte) []alertBlock {
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(body))
	var out []alertBlock
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		q, ok := n.(*ast.Blockquote)
		if !ok {
			continue
		}
		p, ok := q.FirstChild().(*ast.Paragraph)
		if !ok || p.Lines().Len() == 0 {
			continue
		}
		first := p.Lines().At(0)
		m := alertTag.FindSubmatch(bytes.TrimSpace(body[first.Start:first.Stop]))
		if m == nil {
			continue
		}
		start := bytes.LastIndexByte(body[:first.Start], '\n') + 1
		end := first.Stop
		_ = ast.Walk(q, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering || c.Type() != ast.TypeBlock {
				return ast.WalkContinue, nil
			}
			if lines := c.Lines(); lines != nil && lines.Len() > 0 {
				end = max(end, lines.At(lines.Len()-1).Stop)
			}
			return ast.WalkContinue, nil
		})
		end = lineEnd(body, end)
		for end < len(body) { // lines still marked as the quote's, like a code block's closing fence
			next := lineEnd(body, end+1)
			if !quoteMark.Match(body[end+1 : next]) {
				break
			}
			end = next
		}
		var inner []string
		for i, l := range strings.Split(string(body[start:end]), "\n") {
			if i == 0 {
				continue // the tag
			}
			inner = append(inner, quoteMark.ReplaceAllString(l, ""))
		}
		out = append(out, alertBlock{kind: strings.ToLower(string(m[1])), body: []byte(strings.Join(inner, "\n") + "\n"), start: start, end: end})
	}
	return out
}

// alertRows draws an alert width cells wide: the title, then the body rendered at the width
// inside the rule, each row behind "│ " in the kind's colour.
func (r *Renderer) alertRows(a alertBlock, width int, place Placer) []string {
	k, ok := alertKinds[a.kind]
	if !ok {
		k = alertKinds["note"]
	}
	tint := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Hex(k.token, r.dark)))
	rule := tint.Render("│ ")
	rows := []string{rule + tint.Bold(true).Render(k.title)}
	sub, err := NewAt(max(1, width-2)+2*r.margin, r.dark, r.dir)
	if err != nil {
		return rows
	}
	out, err := sub.RenderImages(a.body, place)
	if err != nil {
		return rows
	}
	lines := strings.Split(strings.Trim(out, "\n"), "\n")
	margin := strings.Repeat(" ", r.margin)
	for _, l := range lines {
		rows = append(rows, strings.TrimRight(rule+strings.TrimPrefix(l, margin), " "))
	}
	return rows
}
