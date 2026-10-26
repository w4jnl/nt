// Package theme is the Go form of docs/design-system/tokens.json: the w4jnl family colours in a
// dark and a light theme, nt's state colours and glyphs, and the Lip Gloss styles built from them.
// tokens.json is the source of truth; TestTokensMatchDesignSystem keeps this file equal to it.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// pair is one token's value in the dark and the light theme, as "#RRGGBB".
type pair struct{ dark, light string }

// tokens holds every colour token by its name in tokens.json, references ("{accent}") resolved.
var tokens = map[string]pair{
	"ground":       {"#14181A", "#F6F8F8"},
	"surface":      {"#1A2023", "#FFFFFF"},
	"surface-sunk": {"#0F1214", "#EEF2F2"},
	"line":         {"#263034", "#DCE3E4"},
	"line-strong":  {"#6B7A7D", "#7E8D90"},
	"ink":          {"#F2F5F5", "#14181A"},
	"ink-muted":    {"#8A999C", "#5A686B"},
	"ink-faint":    {"#5E6D70", "#98A5A7"},
	"accent":       {"#3FD0D4", "#0B7A7E"},
	"on-accent":    {"#0B1214", "#FFFFFF"},
	"accent-soft":  {"#173437", "#E8F6F6"},
	"focus":        {"#3FD0D4", "#0B7A7E"},
	"mark":         {"#3FD0D4", "#12999D"},
	"brand":        {"#12999D", "#12999D"},
	"ok":           {"#5FC47A", "#1F7A3A"},
	"ok-soft":      {"#16301F", "#E3F3E8"},
	"warn":         {"#E8963C", "#9A5A0E"},
	"warn-soft":    {"#33240F", "#FBEEDC"},
	"alert":        {"#F0645A", "#C0342B"},
	"alert-soft":   {"#3A1A18", "#FBE4E2"},
	"on-alert":     {"#1C0B09", "#FFFFFF"},
	"synced":       {"#5FC47A", "#1F7A3A"},
	"syncing":      {"#3FD0D4", "#0B7A7E"},
	"unpushed":     {"#8A999C", "#5A686B"},
	"offline":      {"#E8963C", "#9A5A0E"},
	"conflict":     {"#F0645A", "#C0342B"},
	"in-progress":  {"#3FD0D4", "#0B7A7E"},
	"done":         {"#5FC47A", "#1F7A3A"},
	"blocked":      {"#E8963C", "#9A5A0E"},
	"failed":       {"#F0645A", "#C0342B"},
	"abandoned":    {"#8A999C", "#5A686B"},
}

// Hex returns a token's value for the theme, "" for an unknown name.
func Hex(name string, dark bool) string {
	p, ok := tokens[name]
	if !ok {
		return ""
	}
	if dark {
		return p.dark
	}
	return p.light
}

// Palette is one theme's colours, named as in tokens.json.
type Palette struct {
	Dark bool

	Ground, Surface, SurfaceSunk, Line, LineStrong color.Color
	Ink, InkMuted, InkFaint                        color.Color
	Accent, OnAccent, AccentSoft, Mark, Brand      color.Color
	OK, Warn, Alert                                color.Color
}

// New returns the dark or the light palette.
func New(dark bool) Palette {
	c := func(name string) color.Color { return lipgloss.Color(Hex(name, dark)) }
	return Palette{
		Dark:   dark,
		Ground: c("ground"), Surface: c("surface"), SurfaceSunk: c("surface-sunk"),
		Line: c("line"), LineStrong: c("line-strong"),
		Ink: c("ink"), InkMuted: c("ink-muted"), InkFaint: c("ink-faint"),
		Accent: c("accent"), OnAccent: c("on-accent"), AccentSoft: c("accent-soft"),
		Mark: c("mark"), Brand: c("brand"),
		OK: c("ok"), Warn: c("warn"), Alert: c("alert"),
	}
}

// State is a sync or agent-report state; its token name in tokens.json is the string.
type State string

const (
	Synced   State = "synced"
	Syncing  State = "syncing"
	Unpushed State = "unpushed"
	Offline  State = "offline"
	Conflict State = "conflict"

	InProgress State = "in-progress"
	Done       State = "done"
	Blocked    State = "blocked"
	Failed     State = "failed"
	Abandoned  State = "abandoned"
)

// spinner is flok's working spinner; static output (the digest, a pipe) uses its first frame.
var spinner = []string{"◐", "◓", "◑", "◒"}

// Glyph is the state's shape, so the state reads without colour. frame animates the spinning
// states (syncing, in-progress); pass 0 for static output.
func Glyph(s State, frame int) string {
	switch s {
	case Synced:
		return "●"
	case Syncing, InProgress:
		return spinner[frame%len(spinner)]
	case Unpushed:
		return "↑"
	case Offline:
		return "◌" // dashed ring: the link is broken; ◐ is the spinner's first frame
	case Conflict, Failed:
		return "◆"
	case Done:
		return "✓"
	case Blocked:
		return "●"
	case Abandoned:
		return "○"
	}
	return "·"
}

// Color is the state's colour in the palette's theme.
func (p Palette) Color(s State) color.Color {
	if h := Hex(string(s), p.Dark); h != "" {
		return lipgloss.Color(h)
	}
	return p.InkMuted
}

// Styles are the Lip Gloss styles every screen of nt shares.
type Styles struct {
	Text, Muted, Faint lipgloss.Style
	Key                lipgloss.Style // a key in the footer: accent
	Verb               lipgloss.Style // the verb after it: ink-muted
	Frame, Name        lipgloss.Style // the [nt] wordmark: brackets in mark, name in ink
	Rule               lipgloss.Style // pane borders and rules: line
	Selected           lipgloss.Style // the selected row: surface background
	Bar                lipgloss.Style // the selection bar ▎: accent
}

// Styles builds the shared styles for the palette.
func (p Palette) Styles() Styles {
	s := lipgloss.NewStyle
	return Styles{
		Text:     s().Foreground(p.Ink),
		Muted:    s().Foreground(p.InkMuted),
		Faint:    s().Foreground(p.InkFaint),
		Key:      s().Foreground(p.Accent).Bold(true),
		Verb:     s().Foreground(p.InkMuted),
		Frame:    s().Foreground(p.Mark).Bold(true),
		Name:     s().Foreground(p.Ink).Bold(true),
		Rule:     s().Foreground(p.Line),
		Selected: s().Background(p.Surface).Foreground(p.Ink),
		Bar:      s().Foreground(p.Accent).Background(p.Surface),
	}
}

// Wordmark is "[nt]": the brackets in mark, the name in ink, bold, as in flok's sidebar.
func (s Styles) Wordmark() string {
	return s.Frame.Render("[") + s.Name.Render("nt") + s.Frame.Render("]")
}
