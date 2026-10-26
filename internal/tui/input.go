package tui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/theme"
)

// lineInput is the one-line prompt at the bottom of the browser: add to today, a new note's
// title, a search. Enter and Escape belong to the browser; this handles the text.
type lineInput struct {
	prompt string
	value  []rune
	pos    int
}

func (in *lineInput) Value() string { return string(in.value) }

// key edits the text and reports whether it changed.
func (in *lineInput) key(k tea.KeyPressMsg) bool {
	before := string(in.value)
	switch k.String() {
	case "backspace", "ctrl+h":
		if in.pos > 0 {
			in.value = append(in.value[:in.pos-1], in.value[in.pos:]...)
			in.pos--
		}
	case "delete", "ctrl+d":
		if in.pos < len(in.value) {
			in.value = append(in.value[:in.pos], in.value[in.pos+1:]...)
		}
	case "left", "ctrl+b":
		in.pos = max(0, in.pos-1)
	case "right", "ctrl+f":
		in.pos = min(len(in.value), in.pos+1)
	case "home", "ctrl+a":
		in.pos = 0
	case "end", "ctrl+e":
		in.pos = len(in.value)
	case "ctrl+u":
		in.value, in.pos = in.value[in.pos:], 0
	case "ctrl+k":
		in.value = in.value[:in.pos]
	case "ctrl+w", "alt+backspace":
		i := in.pos
		for i > 0 && unicode.IsSpace(in.value[i-1]) {
			i--
		}
		for i > 0 && !unicode.IsSpace(in.value[i-1]) {
			i--
		}
		in.value, in.pos = append(in.value[:i], in.value[in.pos:]...), i
	default:
		if k.Text != "" && k.Mod&^tea.ModShift == 0 {
			in.insert(k.Text)
		}
	}
	return string(in.value) != before
}

// paste inserts pasted text as one line: newlines and tabs become single spaces.
func (in *lineInput) paste(text string) {
	in.insert(strings.Join(strings.Fields(text), " "))
}

// insert puts text at the cursor.
func (in *lineInput) insert(text string) {
	r := []rune(text)
	in.value = append(in.value[:in.pos], append(r, in.value[in.pos:]...)...)
	in.pos += len(r)
}

// View is the prompt in accent and the text on surface-sunk, the cursor a reversed cell.
func (in *lineInput) View(st theme.Styles, pal theme.Palette, width int) string {
	text := st.Text.Background(pal.SurfaceSunk)
	cursor := lipgloss.NewStyle().Foreground(pal.OnAccent).Background(pal.Accent)
	before, at, after := string(in.value[:in.pos]), " ", ""
	if in.pos < len(in.value) {
		at, after = string(in.value[in.pos]), string(in.value[in.pos+1:])
	}
	room := width - ansi.StringWidth(in.prompt) - 3
	for ansi.StringWidth(before)+1 > room && before != "" { // keep the cursor on screen
		before = string([]rune(before)[1:])
	}
	line := st.Key.Background(pal.Surface).Render(in.prompt) + text.Render(before) + cursor.Render(at) + text.Render(after)
	return ansi.Truncate(line, width-1, "")
}
