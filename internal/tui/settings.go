package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/editor"
)

// The settings list (docs/design.md §4, Settings): `,` shows every setting with its value in the
// list pane, the selected one's meaning on the footer line. enter flips a true/false setting or
// asks for a new value, e opens the setting's file in $EDITOR. A change is written as nt config
// set writes it, and takes effect here at once where it can: the theme, the pins, the image
// viewer, the browser's own sync.

// openSettings shows the settings, keep selected.
func (b *browser) openSettings(keep string) tea.Cmd {
	b.reloadSettings()
	w := 0
	for _, s := range config.Settings {
		w = max(w, len(s.Key))
	}
	var items []pickItem
	sel := 0
	for i, s := range config.Settings {
		v := s.Value(b.o.Machine, b.v.Cfg)
		if v == "" {
			v = "none"
		}
		where := "this machine"
		if s.File == "repo" {
			where = "repository"
		}
		items = append(items, pickItem{label: fmt.Sprintf("%-*s  %s", w, s.Key, v), hint: where, kind: "setting", target: s.Key})
		if s.Key == keep {
			sel = i
		}
	}
	b.pick = &picker{title: "settings · enter changes · e edits the file", items: items, settings: true}
	return b.pickMove(sel)
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

// settingSelected is the setting on the selected row.
func (b *browser) settingSelected() (config.Setting, bool) {
	p := b.pick
	if p == nil || !p.settings || p.sel >= len(p.items) {
		return config.Setting{}, false
	}
	return config.FindSetting(p.items[p.sel].target)
}

// settingKeys handles the settings list's own keys and reports whether k was one of them.
func (b *browser) settingKeys(k tea.KeyPressMsg) (tea.Cmd, bool) {
	s, ok := b.settingSelected()
	if !ok {
		return nil, false
	}
	switch k.String() {
	case "enter", "space", "right", "l":
		if s.Kind == "bool" {
			return b.setSetting(s, fmt.Sprint(s.Value(b.o.Machine, b.v.Cfg) != "true")), true
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
		return nil, true
	case "e":
		if s.File == "repo" {
			return b.editAt(".nt/config.toml", 0), true
		}
		p := config.MachinePath()
		argv := editor.Argv(p, 0)
		return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg { return editedMsg{err} }), true
	}
	return nil, false
}

// setSetting writes the selected setting typed as raw and shows the list again.
func (b *browser) setSetting(s config.Setting, raw string) tea.Cmd {
	var err error
	if s.File == "machine" {
		_, err = config.SetMachine(s.Key, raw)
	} else {
		err = b.v.SetSetting(s.Key, raw)
	}
	if err != nil {
		cmd := b.openSettings(s.Key) // first: it shows the setting's meaning, which the error replaces
		b.say(err.Error(), true)
		return cmd
	}
	cmd := b.openSettings(s.Key)
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
