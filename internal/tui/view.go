// Package tui is nt's terminal interface on Bubble Tea v2. This first piece is the note viewer:
// the brand line, the rendered note in a viewport and the footer, with the editor hand-off. It
// becomes the preview pane of the two-pane screen (docs/design-system/README.md, Terminal UI).
package tui

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/termimg"
	"github.com/w4jnl/nt/internal/theme"
)

// previewMax is the widest a note is wrapped (tokens.json terminal/preview-max): longer lines
// are harder to read, so wider panes keep the extra width as margin.
const previewMax = 100

// imageDelay holds image data back until the frame that enters the alternate screen is drawn:
// outside tmux the terminal keeps each screen's images apart. Bubble Tea draws a frame every
// 1/60 s, so three frames is enough.
const imageDelay = 50 * time.Millisecond

// imageCheck is how long after sending images nt asks tmux whether it threw output away, and
// imageTries how often it sends them before giving up.
const (
	imageCheck = 250 * time.Millisecond
	imageTries = 3
)

// ViewOptions configures the viewer. Mode is "auto" (ask the terminal), "dark" or "light".
// Images is "auto" (draw them when the terminal can), "on" or "off".
type ViewOptions struct {
	Path   string
	Mode   string
	Images string
}

// RunView shows one note until the person quits.
func RunView(o ViewOptions) error {
	m := newViewer(o)
	m.load()
	_, err := tea.NewProgram(m).Run()
	if m.img != nil {
		_, _ = os.Stdout.WriteString(m.img.Clear()) // free the terminal's copies
	}
	return err
}

type viewer struct {
	path    string
	mode    string
	dark    bool
	known   bool // the background is known (answered, or forced by mode)
	pal     theme.Palette
	st      theme.Styles
	vp      viewport.Model
	width   int
	height  int
	raw     []byte
	body    string
	title   string
	err     error
	edits   int
	message string
	img     *termimg.Terminal         // nil: images keep their alt text
	images  map[string]*termimg.Image // by file
	pending string                    // image data for the terminal, sent after the next frame
}

type editedMsg struct{ err error }

// imagesLostMsg says tmux dropped output while images were on their way: send them again.
type imagesLostMsg struct{ try int }

type imagesSentMsg struct{}

func newViewer(o ViewOptions) *viewer {
	m := &viewer{path: o.Path, mode: o.Mode, dark: o.Mode != "light", known: o.Mode == "dark" || o.Mode == "light"}
	m.img, m.message = termimg.Detect(o.Images)
	m.images = map[string]*termimg.Image{}
	m.setTheme(m.dark)
	m.vp = viewport.New()
	return m
}

func (m *viewer) setTheme(dark bool) {
	m.dark = dark
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
			m.rerender()
		}
		return m, m.sendImages()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp.SetWidth(m.width)
		m.vp.SetHeight(max(1, m.height-2))
		m.rerender()
		return m, m.sendImages()
	case editedMsg:
		m.edits++
		if msg.err != nil {
			m.message = "editor: " + msg.err.Error()
		} else {
			m.message = ""
		}
		if m.img != nil {
			m.img.Forget() // the editor had the screen; send the images again
		}
		m.load()
		m.rerender()
		return m, m.sendImages()
	case imagesLostMsg:
		if m.img != nil {
			m.img.Forget()
			m.rerender()
			return m, m.sendImagesTry(msg.try)
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "e":
			return m, m.edit()
		case "r":
			m.load()
			m.rerender()
			return m, m.sendImages()
		case "g", "home":
			m.vp.GotoTop()
			return m, nil
		case "G", "end":
			m.vp.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// edit hands the terminal to $VISUAL or $EDITOR (nvim by default) and reloads the note when the
// editor exits. Bubble Tea suspends itself for the editor's lifetime.
func (m *viewer) edit() tea.Cmd {
	argv := strings.Fields(firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "nvim"))
	if line := m.sourceLine(); line > 0 && isVi(argv[0]) {
		argv = append(argv, fmt.Sprintf("+%d", line))
	}
	c := exec.Command(argv[0], append(argv[1:], m.path)...)
	return tea.ExecProcess(c, func(err error) tea.Msg { return editedMsg{err: err} })
}

// sourceLine estimates the note line at the top of the viewport from the scroll fraction:
// Glamour keeps no map from rendered lines back to the source (the same estimate glow uses).
func (m *viewer) sourceLine() int {
	total := m.vp.TotalLineCount()
	if total == 0 || m.vp.YOffset() == 0 {
		return 0
	}
	lines := strings.Count(string(m.raw), "\n") + 1
	return 1 + m.vp.YOffset()*lines/total
}

func (m *viewer) load() {
	m.raw, m.err = os.ReadFile(m.path)
	m.title = titleOf(m.raw, m.path)
}

func (m *viewer) rerender() {
	if m.width == 0 {
		return
	}
	if m.err != nil {
		m.vp.SetContent(m.pal.Styles().Muted.Render("  " + m.err.Error()))
		return
	}
	var place render.Placer
	if m.img != nil {
		place = m.place
	}
	r, err := render.New(min(m.width-4, previewMax), m.dark)
	if err == nil {
		m.body, err = r.RenderImages(m.raw, place)
	}
	if err != nil {
		m.body = string(m.raw)
	}
	m.vp.SetContent(m.body)
}

// place draws a local image as placeholder rows, at most width cells wide and one row shorter
// than the viewport, so a whole image fits on the screen. A remote image stays a link: nt
// fetches nothing.
func (m *viewer) place(dest string, width int) ([]string, bool) {
	p, ok := localImage(m.path, dest)
	if !ok {
		return nil, false
	}
	im, err := m.img.Load(p, m.images[p])
	if err != nil {
		return nil, false
	}
	m.images[p] = im
	cols, rows := m.img.Fit(im, width, max(1, m.vp.Height()-1))
	m.pending += m.img.Show(im, cols, rows)
	return m.img.Rows(im), true
}

// sendImages hands the queued image data to the terminal after imageDelay, then checks after
// imageCheck that no tmux on the way discarded output meanwhile; if one did, the images go again.
func (m *viewer) sendImages() tea.Cmd { return m.sendImagesTry(1) }

func (m *viewer) sendImagesTry(try int) tea.Cmd {
	if m.pending == "" {
		return nil
	}
	seq, img := m.pending, m.img
	m.pending = ""
	before := -1
	send := tea.Tick(imageDelay, func(time.Time) tea.Msg {
		before = img.Discarded()
		return tea.RawMsg{Msg: seq}
	})
	check := tea.Tick(imageCheck, func(time.Time) tea.Msg {
		if try < imageTries && before >= 0 && img.Discarded() > before {
			return imagesLostMsg{try: try + 1}
		}
		return imagesSentMsg{}
	})
	return tea.Sequence(send, check) // in order: check reads before only after send wrote it
}

// localImage resolves an image destination against the note's folder; false for a URL.
func localImage(note, dest string) (string, bool) {
	if dest == "" || strings.Contains(dest, "://") || strings.HasPrefix(dest, "data:") {
		return "", false
	}
	if d, err := url.PathUnescape(dest); err == nil {
		dest = d
	}
	if filepath.IsAbs(dest) {
		return dest, true
	}
	return filepath.Join(filepath.Dir(note), dest), true
}

func (m *viewer) View() tea.View {
	v := tea.NewView(m.brandLine() + "\n" + m.vp.View() + "\n" + m.footer())
	v.AltScreen = true
	v.WindowTitle = "nt · " + m.title
	return v
}

// brandLine is the top row: the [nt] wordmark and the note's title, the path muted at the right.
func (m *viewer) brandLine() string {
	left := " " + m.st.Wordmark() + "  " + m.st.Text.Render(m.title)
	right := m.st.Muted.Render(shortPath(m.path)) + " "
	return joinLR(left, right, m.width)
}

// footer is the bottom row: keys in accent with their verb in ink-muted, scroll position and the
// last editor error at the right.
func (m *viewer) footer() string {
	hint := func(k, v string) string { return m.st.Key.Render(k) + " " + m.st.Verb.Render(v) }
	left := " " + strings.Join([]string{hint("e", "edit"), hint("r", "reload"), hint("g G", "top bottom"), hint("q", "quit")}, "  ")
	right := m.st.Muted.Render(fmt.Sprintf("%3.0f%%", m.vp.ScrollPercent()*100)) + " "
	if m.message != "" {
		right = lipgloss.NewStyle().Foreground(m.pal.Alert).Render("◆ "+m.message) + "  " + right
	}
	return joinLR(left, right, m.width)
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

// titleOf finds the note's title: frontmatter title, then the first "# " heading, then the
// file name (docs/format.md §4).
func titleOf(note []byte, path string) string {
	front, body := render.SplitFrontmatter(note)
	for _, l := range strings.Split(string(front), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "title:"); ok {
			if t := strings.Trim(strings.TrimSpace(v), `"'`); t != "" {
				return t
			}
		}
	}
	for _, l := range strings.Split(string(body), "\n") {
		if t, ok := strings.CutPrefix(l, "# "); ok {
			return strings.TrimSpace(t)
		}
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, ok := strings.CutPrefix(p, home+"/"); ok {
			return "~/" + rel
		}
	}
	return p
}

func isVi(bin string) bool {
	switch filepath.Base(bin) {
	case "nvim", "vim", "vi", "hx", "nano", "micro", "kak":
		return true
	}
	return false
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
