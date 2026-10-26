// Package tui is nt's terminal interface on Bubble Tea v2: the browser (two panes, folders and
// notes on the left, the rendered note on the right) and the one-note viewer, which share the
// preview (docs/design-system/README.md, Terminal UI).
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/editor"
	"github.com/w4jnl/nt/internal/termimg"
	"github.com/w4jnl/nt/internal/theme"
)

// ViewOptions configures the viewer. Mode is "auto" (ask the terminal), "dark" or "light".
// Images is "auto" (draw them when the terminal can), "on" or "off". Root is the folder links
// are shown from: the notes repository when the note is in it, else the working directory.
type ViewOptions struct {
	Path   string
	Root   string
	Mode   string
	Images string
}

// RunView shows one note until the person quits.
func RunView(o ViewOptions) error {
	m := newViewer(o)
	_, err := tea.NewProgram(m).Run()
	_, _ = os.Stdout.WriteString(m.p.clear()) // free the terminal's copies
	return err
}

type viewer struct {
	p       *preview
	mode    string
	known   bool // the background is known (answered, or forced by mode)
	st      theme.Styles
	pal     theme.Palette
	width   int
	height  int
	message string
}

type editedMsg struct{ err error }

func newViewer(o ViewOptions) *viewer {
	img, why := termimg.Detect(o.Images)
	m := &viewer{mode: o.Mode, known: o.Mode == "dark" || o.Mode == "light", message: why}
	m.p = newPreview(img, o.Mode != "light")
	m.setTheme(o.Mode != "light")
	m.p.open(o.Path, relDir(o.Root, o.Path))
	return m
}

func (m *viewer) setTheme(dark bool) {
	m.p.setTheme(dark)
	m.pal = theme.New(dark)
	m.st = m.pal.Styles()
}

func (m *viewer) Init() tea.Cmd {
	if m.known {
		return nil
	}
	return tea.RequestBackgroundColor
}

func (m *viewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		if !m.known {
			m.known = true
			m.setTheme(msg.IsDark())
			m.p.render()
		}
		return m, m.p.sendImages()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.p.setSize(m.width, m.height-2)
		m.p.render()
		return m, m.p.sendImages()
	case editedMsg:
		m.message = ""
		if msg.err != nil {
			m.message = "editor: " + msg.err.Error()
		}
		m.p.forget() // the editor had the screen; send the images again
		m.p.load()
		m.p.render()
		return m, m.p.sendImages()
	case imagesLostMsg:
		return m, m.p.resend(msg.try)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "e":
			argv := editor.Argv(m.p.path, m.p.sourceLine())
			return m, tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg { return editedMsg{err} })
		case "r":
			m.p.load()
			m.p.render()
			return m, m.p.sendImages()
		case "g", "home":
			m.p.vp.GotoTop()
			return m, nil
		case "G", "end":
			m.p.vp.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.p.vp, cmd = m.p.vp.Update(msg)
	return m, cmd
}

func (m *viewer) View() tea.View {
	v := tea.NewView(m.brandLine() + "\n" + m.p.View() + "\n" + m.footer())
	v.AltScreen = true
	v.WindowTitle = "nt · " + m.p.title
	return v
}

// brandLine is the top row: the [nt] wordmark and the note's title, the path muted at the right.
func (m *viewer) brandLine() string {
	left := " " + m.st.Wordmark() + "  " + m.st.Text.Render(m.p.title)
	right := m.st.Muted.Render(shortPath(m.p.path)) + " "
	return joinLR(left, right, m.width)
}

// footer is the bottom row: keys in accent with their verb in ink-muted, scroll position and the
// last editor error at the right.
func (m *viewer) footer() string {
	left := " " + hints(m.st, "e", "edit", "r", "reload", "g G", "top bottom", "q", "quit")
	right := m.st.Muted.Render(fmt.Sprintf("%3.0f%%", m.p.vp.ScrollPercent()*100)) + " "
	if m.message != "" {
		right = lipgloss.NewStyle().Foreground(m.pal.Alert).Render("◆ "+m.message) + "  " + right
	}
	return joinLR(left, right, m.width)
}

// hints renders key, verb pairs: the key in accent, the verb in ink-muted, two spaces apart.
func hints(st theme.Styles, kv ...string) string {
	var parts []string
	for i := 0; i+1 < len(kv); i += 2 {
		parts = append(parts, st.Key.Render(kv[i])+" "+st.Verb.Render(kv[i+1]))
	}
	return strings.Join(parts, "  ")
}

// joinLR puts left and right on one line of width cells, right-aligned, dropping right when the
// line is too narrow for both.
func joinLR(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, ok := strings.CutPrefix(p, home+"/"); ok {
			return "~/" + rel
		}
	}
	return p
}
