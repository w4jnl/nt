// Package render turns a note into styled terminal text with Glamour, in a style built from the
// design tokens (docs/design-system/tokens.json) so the preview, `glow -s` and the rest of nt
// read as one thing. Glamour gets the note without its frontmatter: it would draw the YAML as a
// rule and a heading.
package render

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"

	"github.com/w4jnl/nt/internal/theme"
)

// Style is nt's Glamour style for the dark or the light theme: Glamour's own layout (margins,
// prefixes, tables) with every colour taken from the tokens.
func Style(dark bool) ansi.StyleConfig {
	base := styles.LightStyleConfig
	if dark {
		base = styles.DarkStyleConfig
	}
	s := clone(base)
	c := func(name string) *string { v := theme.Hex(name, dark); return &v }
	t, f := true, false

	s.Document.Color = c("ink")
	s.Document.BackgroundColor = nil
	s.BlockQuote.Color = c("ink-muted")
	s.BlockQuote.IndentToken = ptr("│ ")
	s.Heading.Color, s.Heading.Bold = c("accent"), &t
	s.H1.Color, s.H1.BackgroundColor, s.H1.Bold = c("on-accent"), c("accent"), &t
	s.H6.Color, s.H6.Bold = c("ink-muted"), &f
	s.HorizontalRule.Color = c("line-strong")
	s.Link.Color, s.Link.Underline = c("accent"), &t
	s.LinkText.Color, s.LinkText.Bold = c("accent"), &t
	s.Image.Color, s.Image.Underline = c("accent"), &t
	s.ImageText.Color = c("ink-muted")
	s.Code.Color, s.Code.BackgroundColor = c("ink"), c("surface-sunk")
	s.CodeBlock.Color = c("ink")
	s.CodeBlock.Chroma = chroma(dark)
	s.Task.Ticked, s.Task.Unticked = "[✓] ", "[ ] "
	s.Item.BlockPrefix = "• "
	s.Table.Color = c("ink")
	s.DefinitionTerm.Color = c("accent")
	return s
}

// chroma colours code blocks from the family palette: keywords in accent, strings in ok,
// numbers in warn, comments muted, errors in alert. Few colours on purpose: code in a note is
// read, not edited.
func chroma(dark bool) *ansi.Chroma {
	p := func(name string, bold bool) ansi.StylePrimitive {
		v := theme.Hex(name, dark)
		sp := ansi.StylePrimitive{Color: &v}
		if bold {
			sp.Bold = &bold
		}
		return sp
	}
	bg := theme.Hex("surface-sunk", dark)
	return &ansi.Chroma{
		Text:                p("ink", false),
		Error:               p("alert", false),
		Comment:             p("ink-muted", false),
		CommentPreproc:      p("ink-muted", false),
		Keyword:             p("accent", true),
		KeywordReserved:     p("accent", true),
		KeywordNamespace:    p("accent", false),
		KeywordType:         p("accent", false),
		Operator:            p("ink-muted", false),
		Punctuation:         p("ink-muted", false),
		Name:                p("ink", false),
		NameBuiltin:         p("accent", false),
		NameTag:             p("accent", false),
		NameAttribute:       p("ink", false),
		NameClass:           p("ink", true),
		NameConstant:        p("warn", false),
		NameDecorator:       p("warn", false),
		NameFunction:        p("ink", true),
		LiteralNumber:       p("warn", false),
		LiteralString:       p("ok", false),
		LiteralStringEscape: p("warn", false),
		GenericDeleted:      p("alert", false),
		GenericInserted:     p("ok", false),
		GenericSubheading:   p("ink-muted", false),
		Background:          ansi.StylePrimitive{BackgroundColor: &bg},
	}
}

// JSON is the style as a Glamour style file, for `glow -s <file>` (nt theme --glamour).
func JSON(dark bool) ([]byte, error) {
	return json.MarshalIndent(Style(dark), "", "  ")
}

// Renderer renders notes at one width in one theme. Make a new one when either changes.
type Renderer struct {
	tr            *glamour.TermRenderer
	width, margin int
}

// base is the fake URL scheme a note's folder is given, so Glamour resolves a relative link
// like ../tech/x.md against it; the scheme is cut from the output, leaving the repository path
// tech/x.md. Without a base Glamour resolves against nothing and prints /tech/x.md.
const base = "ntrepo:///" // no hyphen: Glamour wraps long lines at hyphens

// New returns a renderer that wraps at width columns.
func New(width int, dark bool) (*Renderer, error) { return NewAt(width, dark, "") }

// NewAt is New for a note in folder dir (slash-separated, relative to the repository or the
// working directory): relative links show as paths from that root.
func NewAt(width int, dark bool, dir string) (*Renderer, error) {
	s := Style(dark)
	if dir = strings.Trim(dir, "/"); dir != "" && dir != "." {
		dir += "/"
	} else {
		dir = ""
	}
	tr, err := glamour.NewTermRenderer(
		glamour.WithStyles(s),
		glamour.WithWordWrap(width),
		glamour.WithInlineTableLinks(true),
		glamour.WithBaseURL(base+dir),
	)
	if err != nil {
		return nil, err
	}
	margin := 0
	if s.Document.Margin != nil {
		margin = int(*s.Document.Margin)
	}
	return &Renderer{tr: tr, width: width, margin: margin}, nil
}

// Render styles a note: frontmatter stripped, the body rendered, images as their alt text.
func (r *Renderer) Render(note []byte) (string, error) { return r.RenderImages(note, nil) }

// Field is the value of a top-level key in the note's frontmatter, unquoted; "" when the note
// has no such key.
func Field(note []byte, key string) string {
	front, _ := SplitFrontmatter(note)
	for _, l := range strings.Split(string(front), "\n") {
		if v, ok := strings.CutPrefix(l, key+":"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// Title is a note's title (docs/format.md §4): the frontmatter title, then the first "# "
// heading, then the file name without its extension. A date at the start of a title is left
// out (the date is shown on its own), and a title that is nothing but a date and time, as
// templates that copy the file name leave behind, does not count.
func Title(note []byte, path string) string {
	front, body := SplitFrontmatter(note)
	for _, l := range strings.Split(string(front), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "title:"); ok {
			if t := undated(strings.Trim(strings.TrimSpace(v), `"'`)); t != "" {
				return t
			}
		}
	}
	for _, l := range strings.Split(string(body), "\n") {
		if t, ok := strings.CutPrefix(l, "# "); ok {
			if t := undated(strings.TrimSpace(t)); t != "" {
				return t
			}
		}
	}
	base := path[strings.LastIndexAny(path, "/\\")+1:]
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		base = base[:i]
	}
	if t := undated(base); t != "" {
		return t
	}
	return base
}

var (
	leadingDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[ _T-]*`)
	clock       = regexp.MustCompile(`^\d{4,6}$`)
)

// undated is t without a leading date; "" when nothing but a date or a time is left.
func undated(t string) string {
	rest := strings.TrimSpace(leadingDate.ReplaceAllString(t, ""))
	if rest == t {
		return t
	}
	if rest == "" || clock.MatchString(rest) {
		return ""
	}
	return rest
}

// SplitFrontmatter separates a leading YAML block ("---" line, YAML, "---" or "..." line) from
// the body. A note without one returns nil and the note unchanged.
func SplitFrontmatter(note []byte) (front, body []byte) {
	rest, ok := cutLine(note, "---")
	if !ok {
		return nil, note
	}
	for off := 0; off < len(rest); {
		end := bytes.IndexByte(rest[off:], '\n')
		line := rest[off:]
		next := len(rest)
		if end >= 0 {
			line, next = rest[off:off+end], off+end+1
		}
		if l := strings.TrimRight(string(line), " \t\r"); l == "---" || l == "..." {
			return rest[:off], rest[next:]
		}
		off = next
	}
	return nil, note // an opening line with no closing one is a rule, not frontmatter
}

// cutLine removes a first line equal to want (trailing spaces and \r ignored).
func cutLine(b []byte, want string) ([]byte, bool) {
	end := bytes.IndexByte(b, '\n')
	if end < 0 || strings.TrimRight(string(b[:end]), " \t\r") != want {
		return nil, false
	}
	return b[end+1:], true
}

func clone(s ansi.StyleConfig) ansi.StyleConfig {
	var out ansi.StyleConfig
	b, _ := json.Marshal(s)
	_ = json.Unmarshal(b, &out)
	return out
}

func ptr[T any](v T) *T { return &v }
