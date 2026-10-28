// Package tui is nt's terminal interface on Bubble Tea v2: the browser (two panes, folders and
// notes on the left, the rendered note on the right) and the one-note viewer, which share the
// preview (docs/design-system/README.md, Terminal UI).
package tui

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/editor"
	"github.com/w4jnl/nt/internal/mdparse"
	"github.com/w4jnl/nt/internal/termimg"
	"github.com/w4jnl/nt/internal/theme"
)

// ViewOptions configures the viewer. Mode is "auto" (ask the terminal), "dark" or "light".
// Images is "auto" (draw them when the terminal can), "on" or "off". Root is the folder links
// are shown from: the notes repository when the note is in it, else the working directory.
// ImageViewer and Bin open an image as the browser does (image_viewer, and nt for chafa).
type ViewOptions struct {
	Path        string
	Root        string
	Mode        string
	Images      string
	ImageViewer string
	Bin         string
}

// RunView shows one file until the person quits: a note rendered, any other text as written.
func RunView(o ViewOptions) error {
	m := newViewer(o)
	_, err := tea.NewProgram(m).Run()
	_, _ = os.Stdout.WriteString(m.p.clear()) // free the terminal's copies
	return err
}

// viewer is nt view: one file on the whole screen, with the browser's reading keys. A link to a
// note or a text file opens in it, b comes back; o and i are pickers in the note's place.
type viewer struct {
	o         ViewOptions
	p         *preview
	mode      string
	known     bool // the background is known (answered, or forced by mode)
	st        theme.Styles
	pal       theme.Palette
	width     int
	height    int
	message   string
	alert     bool
	history   []visit // the files links were followed from, for b; rel is the file's path
	hinted    string  // the letters typed after f so far
	selecting bool    // the mouse is down on the note, selecting text
	pick      *picker // the outline or the images, in the note's place
}

type editedMsg struct{ err error }

type viewTickMsg struct{}

func newViewer(o ViewOptions) *viewer {
	img, why := termimg.Detect(o.Images)
	m := &viewer{o: o, mode: o.Mode, known: o.Mode == "dark" || o.Mode == "light", message: why, alert: why != ""}
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

func (m *viewer) say(msg string, alert bool) { m.message, m.alert = msg, alert }

func (m *viewer) tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return viewTickMsg{} })
}

func (m *viewer) Init() tea.Cmd {
	if m.known {
		return m.tick()
	}
	return tea.Batch(m.tick(), tea.RequestBackgroundColor)
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
	case viewTickMsg: // the file written meanwhile, by an editor or a sync: show it as it is now
		if m.p.changed() && m.p.mark == nil {
			top := m.p.vp.YOffset()
			m.p.load()
			m.p.render()
			m.p.vp.SetYOffset(top)
			return m, tea.Batch(m.tick(), m.p.sendImages())
		}
		return m, m.tick()
	case editedMsg:
		m.message = ""
		if msg.err != nil {
			m.say("editor: "+msg.err.Error(), true)
		}
		m.p.forget() // the editor had the screen; send the images again
		m.p.load()
		m.p.render()
		return m, m.p.sendImages()
	case imagesLostMsg:
		return m, m.p.resend(msg.try)
	case tea.KeyPressMsg:
		m.message = ""
		if m.p.hints != nil {
			return m, m.hintKey(msg)
		}
		if m.pick != nil {
			return m, m.pickKey(msg)
		}
		if cmd, ok := m.key(msg); ok {
			return m, cmd
		}
	case tea.MouseMsg:
		return m, m.mouse(msg)
	}
	var cmd tea.Cmd
	m.p.vp, cmd = m.p.vp.Update(msg)
	return m, cmd
}

// key acts on a key the viewer has; the others scroll, as the viewport takes them.
func (m *viewer) key(k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch k.String() {
	case "esc":
		if m.p.mark != nil {
			m.p.mark = nil
			return nil, true
		}
		return tea.Quit, true
	case "q", "ctrl+c":
		return tea.Quit, true
	case "e":
		argv := editor.Argv(m.p.path, m.p.sourceLine())
		return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg { return editedMsg{err} }), true
	case "r":
		m.p.load()
		m.p.render()
		return m.p.sendImages(), true
	case "g", "home":
		m.p.vp.GotoTop()
		return nil, true
	case "G", "end":
		m.p.vp.GotoBottom()
		return nil, true
	case "f":
		if !m.p.label() {
			m.say("no links on screen", false)
		}
		m.hinted = ""
		return nil, true
	case "b":
		return m.back(), true
	case "o":
		return m.outline(), true
	case "i":
		return m.images(), true
	case "y":
		if text := m.p.selected(); text != "" {
			m.p.mark = nil
			m.say("copied "+plural(strings.Count(text, "\n")+1, "line"), false)
			return copyText(text), true
		}
		m.say("copied "+shortPath(m.p.path), false)
		return copyText(m.p.path), true
	}
	return nil, false
}

// hintKey takes the letters of a label: a whole one follows its link, esc or a letter no label
// goes on with cancels.
func (m *viewer) hintKey(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	m.hinted += s
	dest, more := m.p.typed(m.hinted, s)
	switch {
	case dest != "":
		return m.follow(dest)
	case !more && len(s) == 1 && strings.Contains(hintKeys, s):
		m.say("no link has the letters "+m.hinted, false)
	}
	return nil
}

// follow goes where a link points, dest as written in the file: a heading of it; a note or a
// text file, shown here (b comes back); an image, opened as i opens it; any other file, a
// folder or a URL, opened by the OS.
func (m *viewer) follow(dest string) tea.Cmd {
	if mdparse.External(dest) {
		m.say("opened "+dest, false)
		openDetached(osOpener(), dest)
		return nil
	}
	f, anchor, _ := strings.Cut(dest, "#")
	anchor = strings.ReplaceAll(anchor, "-", " ")
	if f == "" {
		m.p.showText(anchor)
		return nil
	}
	if d, err := url.PathUnescape(f); err == nil {
		f = d
	}
	if !filepath.IsAbs(f) {
		f = filepath.Join(filepath.Dir(m.p.path), filepath.FromSlash(f))
	}
	st, err := os.Stat(f)
	switch {
	case err != nil:
		m.say(dest+": the link points to nothing", true)
		return nil
	case st.Mode().IsRegular() && (markdown(f) || strings.EqualFold(filepath.Ext(f), ".txt")):
		m.history = append(m.history, visit{m.p.path, m.p.vp.YOffset()})
		if len(m.history) > historyMax {
			m.history = m.history[1:]
		}
		m.p.open(f, relDir(m.o.Root, f))
		m.p.render()
		if anchor != "" {
			m.p.showText(anchor)
		}
		return m.p.sendImages()
	case imageExt[strings.ToLower(path.Ext(f))]:
		return openImage(m.o.ImageViewer, m.o.Bin, f)
	}
	m.say("opened "+shortPath(f), false)
	openDetached(osOpener(), f)
	return nil
}

// back shows the file a link was followed from, scrolled as it was.
func (m *viewer) back() tea.Cmd {
	for len(m.history) > 0 {
		v := m.history[len(m.history)-1]
		m.history = m.history[:len(m.history)-1]
		if !isFile(v.rel) {
			continue // gone since: the one before it
		}
		m.p.open(v.rel, relDir(m.o.Root, v.rel))
		m.p.render()
		m.p.vp.SetYOffset(v.top)
		return m.p.sendImages()
	}
	m.say("no file to go back to", false)
	return nil
}

func (m *viewer) outline() tea.Cmd {
	var items []pickItem
	for _, h := range mdparse.Parse(m.p.raw).Headings {
		items = append(items, pickItem{label: strings.Repeat("  ", h.Level-1) + h.Text, kind: "heading", text: h.Text, line: h.Line})
	}
	if len(items) == 0 || !markdown(m.p.path) {
		m.say("no headings here", false)
		return nil
	}
	m.pick = &picker{title: "outline · " + plural(len(items), "heading"), items: items}
	return nil
}

func (m *viewer) images() tea.Cmd {
	var items []pickItem
	for _, l := range mdparse.Parse(m.p.raw).Links {
		if !l.Image {
			continue
		}
		it := pickItem{label: firstNonEmpty(l.Text, path.Base(l.Dest)), hint: l.Dest}
		if mdparse.External(l.Dest) {
			it.kind, it.target = "url", l.Dest
		} else if f, ok := localImage(m.p.path, l.Dest); ok {
			it.kind, it.target = "image", f
		}
		items = append(items, it)
	}
	if len(items) == 0 || !markdown(m.p.path) {
		m.say("no images here", false)
		return nil
	}
	m.pick = &picker{title: "images · " + plural(len(items), "image"), items: items}
	return nil
}

func (m *viewer) pickKey(k tea.KeyPressMsg) tea.Cmd {
	p := m.pick
	switch k.String() {
	case "esc", "q", "left", "h":
		m.pick = nil
		return m.p.sendImages()
	case "j", "down":
		m.pickMove(1)
	case "k", "up":
		m.pickMove(-1)
	case "g", "home":
		m.pickMove(-len(p.items))
	case "G", "end":
		m.pickMove(len(p.items))
	case "enter", "right":
		return m.choose()
	case "ctrl+c":
		return tea.Quit
	}
	return nil
}

func (m *viewer) pickMove(d int) {
	p := m.pick
	p.sel = max(0, min(len(p.items)-1, p.sel+d))
	h := m.p.height - 1 // under the picker's title
	if p.sel < p.top {
		p.top = p.sel
	}
	if h > 0 && p.sel >= p.top+h {
		p.top = p.sel - h + 1
	}
}

// choose goes to the picked heading, or opens the picked image, and gives the note back.
func (m *viewer) choose() tea.Cmd {
	p := m.pick
	m.pick = nil
	if p.sel >= len(p.items) {
		return nil
	}
	switch it := p.items[p.sel]; it.kind {
	case "heading":
		if !m.p.showText(it.text) {
			m.p.showLine(it.line)
		}
	case "url":
		m.say("opened "+it.target, false)
		openDetached(osOpener(), it.target)
	case "image":
		return openImage(m.o.ImageViewer, m.o.Bin, it.target)
	}
	return m.p.sendImages()
}

// mouse: dragging over the note selects its text, a click on a link follows it, the wheel
// scrolls; in a picker a click selects a row and a second click picks it.
func (m *viewer) mouse(msg tea.MouseMsg) tea.Cmd {
	ms := msg.Mouse()
	row := ms.Y - 1 // under the brand line
	switch msg.(type) {
	case tea.MouseWheelMsg:
		switch {
		case m.pick != nil:
			m.pickMove(map[tea.MouseButton]int{tea.MouseWheelDown: 1, tea.MouseWheelUp: -1}[ms.Button])
		case ms.Button == tea.MouseWheelDown:
			m.p.vp.ScrollDown(3)
		case ms.Button == tea.MouseWheelUp:
			m.p.vp.ScrollUp(3)
		}
		return nil
	case tea.MouseMotionMsg:
		if m.selecting {
			m.p.mark.head = m.notePoint(ms.X, row)
		}
		return nil
	case tea.MouseReleaseMsg:
		if !m.selecting {
			return nil
		}
		m.selecting = false
		if m.p.mark.empty() { // a click, not a drag: on a link, it follows it
			at := m.p.mark.anchor
			m.p.mark = nil
			if s, ok := m.p.linkAt(at); ok {
				return m.follow(s.dest)
			}
		}
		return nil
	}
	if _, ok := msg.(tea.MouseClickMsg); !ok || ms.Button != tea.MouseLeft || row < 0 || row >= m.p.height {
		return nil
	}
	m.message = ""
	if p := m.pick; p != nil {
		i := p.top + row - 1 // under the picker's title
		switch {
		case row == 0 || i >= len(p.items):
		case i == p.sel:
			return m.choose()
		default:
			m.pickMove(i - p.sel)
		}
		return nil
	}
	at := m.notePoint(ms.X, row)
	m.p.mark, m.selecting = &span{anchor: at, head: at}, true
	return nil
}

// notePoint is the place in the rendered file under the cell x of row; above or below the pane
// the file scrolls a line, so a selection can reach past what is on screen.
func (m *viewer) notePoint(x, row int) point {
	switch {
	case row < 0:
		m.p.vp.ScrollUp(1)
		row = 0
	case row >= m.p.height:
		m.p.vp.ScrollDown(1)
		row = m.p.height - 1
	}
	return point{line: m.p.vp.YOffset() + row, col: max(0, x)}
}

func (m *viewer) View() tea.View {
	body := m.p.View()
	if p := m.pick; p != nil {
		rows := []string{" " + m.st.Muted.Render(p.title)}
		for i := p.top; i < len(p.items) && len(rows) < m.p.height; i++ {
			rows = append(rows, pickRow(m.st, m.pal, p.items[i], i == p.sel, m.width))
		}
		for len(rows) < m.p.height {
			rows = append(rows, "")
		}
		body = strings.Join(rows, "\n")
	}
	v := tea.NewView(m.brandLine() + "\n" + body + "\n" + m.footer())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion // clicks, the wheel, and motion while a button is held
	v.WindowTitle = "nt · " + m.p.title
	return v
}

// brandLine is the top row: the [nt] wordmark and the file's title, the path muted at the right.
func (m *viewer) brandLine() string {
	left := " " + m.st.Wordmark() + "  " + m.st.Text.Render(m.p.title)
	right := m.st.Muted.Render(shortPath(m.p.path)) + " "
	return joinLR(left, right, m.width)
}

// footer is the bottom row: keys in accent with their verb in ink-muted, and at the right the
// last message and the scroll position.
func (m *viewer) footer() string {
	var left string
	switch {
	case m.p.hints != nil:
		left = hints(m.st, "letters", "follow the link they label", "esc", "cancel")
	case m.pick != nil:
		left = hints(m.st, "enter", "go", "j k", "move", "esc", "back")
	case m.p.mark != nil && !m.p.mark.empty():
		left = hints(m.st, "y", "copy", "esc", "clear", "drag", "select more")
	default:
		left = hints(m.st, "f", "follow", "b", "back", "o", "outline", "i", "images", "y", "copy", "e", "edit", "q", "quit")
	}
	left = " " + left
	right := m.st.Muted.Render(fmt.Sprintf("%3.0f%%", m.p.vp.ScrollPercent()*100)) + " "
	switch {
	case m.message != "" && m.alert:
		right = lipgloss.NewStyle().Foreground(m.pal.Alert).Render("◆ "+m.message) + "  " + right
	case m.message != "":
		right = m.st.Muted.Render(m.message) + "  " + right
	}
	if m.message != "" && lipgloss.Width(left)+lipgloss.Width(right) > m.width {
		return right // a message matters more than the hints it covers
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
