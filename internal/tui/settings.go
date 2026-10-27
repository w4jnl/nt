package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/editor"
)

// The settings (docs/design.md §4, Settings): `,` shows every setting in the note's pane, this
// machine's first, then the repository's: its key, its value and what it does, the selected
// one's meaning in full on the footer line. j and k move, enter flips a true/false setting or
// asks for a new value, e opens the setting's file in $EDITOR, esc goes back to the note. A
// change is written as nt config set writes it, and takes effect here at once where it can: the
// theme, the pins, the image viewer, the browser's own sync. The folder list stays as it was.

// settingsView is the settings in the note's pane.
type settingsView struct {
	sel     int  // the selected setting, an index of config.Settings
	top     int  // the first line drawn
	clicked bool // the last click selected a row; a second one changes it
	full    bool // the pane was given the whole screen for it (narrow screens)
}

// openSettings shows the settings, keep selected.
func (b *browser) openSettings(keep string) tea.Cmd {
	b.reloadSettings()
	if b.set == nil {
		b.set = &settingsView{}
		if b.narrow() && !b.full {
			b.full, b.set.full = true, true
			b.layout()
		}
	}
	for i, s := range config.Settings {
		if s.Key == keep {
			b.set.sel = i
		}
	}
	b.settingMove(0)
	return nil
}

func (b *browser) closeSettings() tea.Cmd {
	if b.set.full {
		b.full = false
		b.layout()
	}
	b.set = nil
	b.p.render()
	return b.p.sendImages()
}

// reloadSettings reads both config files again; a file that does not load leaves what the
// browser has, and says why.
func (b *browser) reloadSettings() {
	if m, err := config.LoadMachine(); err != nil {
		b.say(err.Error(), true)
	} else {
		mode := b.o.Machine.Theme.Mode
		b.o.Machine = m
		if m.Theme.Mode != mode && m.Theme.Mode != "auto" {
			b.setTheme(m.Theme.Mode == "dark")
			b.p.render()
		}
	}
	if r, err := config.LoadRepo(b.v.Root); err != nil {
		b.say(err.Error(), true)
	} else {
		b.v.Cfg = r
	}
}

// settingSelected is the selected setting, while the settings are shown.
func (b *browser) settingSelected() (config.Setting, bool) {
	if b.set == nil || b.set.sel >= len(config.Settings) {
		return config.Setting{}, false
	}
	return config.Settings[b.set.sel], true
}

// settingMove moves the selection by d and says what the selected setting does.
func (b *browser) settingMove(d int) {
	b.set.sel = max(0, min(len(config.Settings)-1, b.set.sel+d))
	b.set.clicked = false
	b.say(config.Settings[b.set.sel].Help, false)
}

// settingsKey handles the keys while the settings are shown and reports whether k was one of
// them: all are but help, sync and quit, so no key acts on the folder list behind them.
func (b *browser) settingsKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "?", "s", "ctrl+c":
		return nil, false
	case "j", "down":
		b.settingMove(1)
	case "k", "up":
		b.settingMove(-1)
	case "g", "home":
		b.settingMove(-len(config.Settings))
	case "G", "end":
		b.settingMove(len(config.Settings))
	case "enter", "space", "right", "l":
		return b.changeSetting(), true
	case "e":
		s, _ := b.settingSelected()
		if s.File == "repo" {
			return b.editAt(".nt/config.toml", 0), true
		}
		argv := editor.Argv(config.MachinePath(), 0)
		return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg { return editedMsg{err} }), true
	case "esc", "q", "h", "left", ",":
		return b.closeSettings(), true
	}
	return nil, true
}

// changeSetting flips a true/false setting, or asks for the selected one's value on the footer
// line, prefilled, with its choices named.
func (b *browser) changeSetting() tea.Cmd {
	s, ok := b.settingSelected()
	if !ok {
		return nil
	}
	if s.Kind == "bool" {
		return b.setSetting(s, fmt.Sprint(s.Value(b.o.Machine, b.v.Cfg) != "true"))
	}
	label := s.Key + ": "
	if c := s.Choices(); len(c) > 0 {
		var words []string
		for _, w := range c {
			words = append(words, firstNonEmpty(w, "none"))
		}
		label = s.Key + " (" + strings.Join(words, ", ") + "): "
	}
	cur := s.Value(b.o.Machine, b.v.Cfg)
	b.prompt("setting", label)
	b.input.value, b.input.pos = []rune(cur), len([]rune(cur))
	return nil
}

// setSetting writes a setting typed as raw and shows the settings again.
func (b *browser) setSetting(s config.Setting, raw string) tea.Cmd {
	var err error
	if s.File == "machine" {
		_, err = config.SetMachine(s.Key, raw)
	} else {
		err = b.v.SetSetting(s.Key, raw)
	}
	cmd := b.openSettings(s.Key) // first: it says what the setting does, which this replaces
	if err != nil {
		b.say(err.Error(), true)
		return cmd
	}
	v := firstNonEmpty(s.Value(b.o.Machine, b.v.Cfg), "none")
	msg := s.Key + " = " + v
	switch {
	case strings.HasPrefix(s.Key, "sync."):
		msg += " · nt install --sync applies it to the timer"
	case s.Key == "inline_images" || s.Key == "repo" || s.Key == "host" || s.Key == "theme.mode" && v == "auto":
		msg += " · from the next start"
	}
	b.say(msg, false)
	return cmd
}

// settingsClick selects the setting on the pane's row, or changes it on a second click.
func (b *browser) settingsClick(row int) tea.Cmd {
	for i, line := range b.settingsLines() {
		if line.setting < 0 || i-b.set.top != row {
			continue
		}
		if line.setting == b.set.sel && b.set.clicked {
			return b.changeSetting()
		}
		b.set.sel = line.setting
		b.settingMove(0)
		b.set.clicked = true
		return nil
	}
	return nil
}

// settingsLine is one line of the settings pane: a heading, or the setting at an index of
// config.Settings.
type settingsLine struct {
	heading string
	setting int
}

func (b *browser) settingsLines() []settingsLine {
	lines := []settingsLine{{heading: "", setting: -1}}
	file := ""
	for i, s := range config.Settings {
		if s.File != file {
			if file != "" {
				lines = append(lines, settingsLine{setting: -1})
			}
			file = s.File
			lines = append(lines, settingsLine{heading: file, setting: -1})
		}
		lines = append(lines, settingsLine{setting: i})
	}
	return lines
}

// settingsPane draws the settings: a heading for each file, then a line for each setting: its
// key, its value in accent, and what it does, as much as fits.
func (b *browser) settingsPane(w, h int) string {
	keyW := 0
	for _, s := range config.Settings {
		keyW = max(keyW, len(s.Key))
	}
	valW := min(28, max(8, w/4))
	lines := b.settingsLines()
	at := 0
	for i, l := range lines {
		if l.setting == b.set.sel {
			at = i
		}
	}
	if at < b.set.top+1 {
		b.set.top = max(0, at-1)
	}
	if h > 0 && at >= b.set.top+h {
		b.set.top = at - h + 1
	}
	var rows []string
	for i := b.set.top; i < len(lines) && len(rows) < h; i++ {
		l := lines[i]
		switch {
		case l.setting < 0 && l.heading == "machine":
			rows = append(rows, "  "+b.st.Text.Render("this machine")+b.st.Faint.Render("  "+shortPath(config.MachinePath())))
		case l.setting < 0 && l.heading == "repo":
			rows = append(rows, "  "+b.st.Text.Render("the repository")+b.st.Faint.Render("  "+shortPath(b.v.Abs(".nt/config.toml"))+", shared by every machine"))
		case l.setting < 0:
			rows = append(rows, "")
		default:
			rows = append(rows, b.settingRow(config.Settings[l.setting], l.setting == b.set.sel, keyW, valW, w))
		}
	}
	return strings.Join(rows, "\n")
}

// settingRow draws a setting: the selection bar and surface when selected, the key, the value,
// what it does.
func (b *browser) settingRow(s config.Setting, selected bool, keyW, valW, w int) string {
	text, value, faint, lead := b.st.Text, b.st.Key, b.st.Faint, "  "
	if selected {
		bg := b.pal.Surface
		text, value, faint = text.Background(bg), value.Background(bg), faint.Background(bg)
		lead = " " + b.st.Bar.Render("▎")
	}
	v := s.Value(b.o.Machine, b.v.Cfg)
	vs := value
	if v == "" {
		v, vs = "none", faint
	}
	v = ansi.Truncate(v, valW, "…")
	line := lead + text.Render(fmt.Sprintf("%-*s  ", keyW, s.Key)) + vs.Render(v) + text.Render(strings.Repeat(" ", valW-ansi.StringWidth(v)+2)) + faint.Render(s.Help)
	if n := ansi.StringWidth(line); n > w {
		line = ansi.Truncate(line, w, "…")
	} else if selected {
		line += text.Render(strings.Repeat(" ", w-n))
	}
	return line
}
