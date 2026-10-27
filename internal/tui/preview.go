package tui

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

// imagesLostMsg says tmux dropped output while images were on their way: send them again.
type imagesLostMsg struct{ try int }

type imagesSentMsg struct{}

// preview is one note rendered in a viewport, images drawn in place: the whole of nt view and
// the right pane of the browser.
type preview struct {
	path    string // the file
	rel     string // its folder's path from the root links are shown from ("" outside a repository)
	raw     []byte
	title   string
	err     error
	mod     time.Time
	vp      viewport.Model
	width   int
	height  int
	dark    bool
	pal     theme.Palette
	extra   string // shown after the note (the browser's "Linked from"), already styled
	body    string
	lines   []string                  // what the viewport shows, one rendered line each
	links   []linkSpot                // where the links are in lines (follow.go)
	hints   map[string]linkSpot       // f's labels on the links on screen, nil when off
	mark    *span                     // text selected with the mouse, nil when none
	img     *termimg.Terminal         // nil: images keep their alt text
	images  map[string]*termimg.Image // by file
	pending string                    // image data for the terminal, sent after the next frame
}

func newPreview(img *termimg.Terminal, dark bool) *preview {
	return &preview{img: img, images: map[string]*termimg.Image{}, vp: viewport.New(), dark: dark, pal: theme.New(dark)}
}

func (p *preview) setTheme(dark bool) { p.dark, p.pal = dark, theme.New(dark) }

func (p *preview) setSize(w, h int) {
	if max(1, w) != p.width {
		p.mark = nil // the note wraps anew
	}
	p.width, p.height = max(1, w), max(1, h)
	p.vp.SetWidth(p.width)
	p.vp.SetHeight(p.height)
}

// open shows the file at path; dir is its folder as links should be shown from. The previous
// note's images are deleted from the terminal.
func (p *preview) open(path, dir string) {
	if path != p.path && p.img != nil {
		p.pending += p.img.Clear()
		p.images = map[string]*termimg.Image{}
	}
	if path != p.path {
		p.vp.GotoTop()
		p.mark = nil
	}
	p.path, p.rel = path, dir
	p.load()
}

func (p *preview) load() {
	p.raw, p.err = os.ReadFile(p.path)
	p.title = render.Title(p.raw, p.path)
	if st, err := os.Stat(p.path); err == nil {
		p.mod = st.ModTime()
	}
}

// changed reports whether the file was written since it was loaded.
func (p *preview) changed() bool {
	if p.path == "" {
		return false
	}
	st, err := os.Stat(p.path)
	return err == nil && !st.ModTime().Equal(p.mod) || err != nil && p.err == nil
}

func (p *preview) render() {
	p.lines, p.links, p.hints = nil, nil, nil
	if p.width <= 1 || p.path == "" {
		p.vp.SetContent("")
		return
	}
	if p.err != nil {
		p.vp.SetContent(p.pal.Styles().Muted.Render("  " + p.err.Error()))
		return
	}
	var place render.Placer
	if p.img != nil {
		place = p.place
	}
	r, err := render.NewAt(min(p.width-2, previewMax), p.dark, p.rel)
	if err == nil {
		p.body, err = r.RenderImages(p.raw, place)
	}
	if err != nil {
		p.body = string(p.raw)
	}
	lines := strings.Split(strings.TrimRight(p.body+p.extra, "\n"), "\n")
	for i, l := range lines { // a pane never draws past its edge
		if ansi.StringWidth(l) > p.width {
			lines[i] = ansi.Truncate(l, p.width, "")
		}
	}
	p.links = findLinks(lines)
	p.markMissing(lines)
	p.lines = lines
	p.vp.SetContentLines(lines)
}

// place draws a local image as placeholder rows, at most width cells wide and one row shorter
// than the pane, so a whole image fits on the screen. A remote image stays a link: nt fetches
// nothing.
func (p *preview) place(dest string, width int) ([]string, bool) {
	f, ok := localImage(p.path, dest)
	if !ok {
		return nil, false
	}
	im, err := p.img.Load(f, p.images[f])
	if err != nil {
		return nil, false
	}
	p.images[f] = im
	cols, rows := p.img.Fit(im, min(width, p.width-2), max(1, p.height-1))
	p.pending += p.img.Show(im, cols, rows)
	return p.img.Rows(im), true
}

// forget marks the images as gone from the terminal, after another program had the screen.
func (p *preview) forget() {
	if p.img != nil {
		p.img.Forget()
	}
}

// clear deletes every image nt sent, for when it exits.
func (p *preview) clear() string {
	if p.img == nil {
		return ""
	}
	return p.img.Clear()
}

// sendImages hands the queued image data to the terminal after imageDelay, then checks after
// imageCheck that no tmux on the way discarded output meanwhile; if one did, the images go again.
func (p *preview) sendImages() tea.Cmd { return p.sendImagesTry(1) }

func (p *preview) sendImagesTry(try int) tea.Cmd {
	if p.pending == "" {
		return nil
	}
	seq, img := p.pending, p.img
	p.pending = ""
	before := -1
	send := tea.Tick(imageDelay, func(time.Time) tea.Msg {
		if img != nil {
			before = img.Discarded()
		}
		return tea.RawMsg{Msg: seq}
	})
	check := tea.Tick(imageCheck, func(time.Time) tea.Msg {
		if img != nil && try < imageTries && before >= 0 && img.Discarded() > before {
			return imagesLostMsg{try: try + 1}
		}
		return imagesSentMsg{}
	})
	return tea.Sequence(send, check) // in order: check reads before only after send wrote it
}

// resend draws the note again with every image sent anew.
func (p *preview) resend(try int) tea.Cmd {
	p.forget()
	p.render()
	return p.sendImagesTry(try)
}

// sourceLine estimates the note line at the top of the pane from the scroll fraction: Glamour
// keeps no map from rendered lines back to the source (the same estimate glow uses).
func (p *preview) sourceLine() int {
	total := p.vp.TotalLineCount()
	if total == 0 || p.vp.YOffset() == 0 {
		return 0
	}
	lines := strings.Count(string(p.raw), "\n") + 1
	return 1 + p.vp.YOffset()*lines/total
}

// showLine scrolls so the rendered text of source line n is near the top, by the same estimate.
func (p *preview) showLine(n int) {
	lines := strings.Count(string(p.raw), "\n") + 1
	if n <= 1 || lines == 0 {
		p.vp.GotoTop()
		return
	}
	p.vp.SetYOffset(max(0, (n-1)*p.vp.TotalLineCount()/lines-1))
}

// showText scrolls to the first rendered line that holds text (a heading, a search hit) and
// reports whether it found one.
func (p *preview) showText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for i, l := range strings.Split(p.body, "\n") {
		if strings.Contains(strings.ToLower(ansi.Strip(l)), text) {
			p.vp.SetYOffset(max(0, i-1))
			return true
		}
	}
	return false
}

// View is the visible part of the note, the selected text on accent-soft and f's labels over
// the links they follow.
func (p *preview) View() string {
	v := p.vp.View()
	if p.hints != nil {
		tag := lipgloss.NewStyle().Foreground(p.pal.OnAccent).Background(p.pal.Accent).Bold(true)
		rows := strings.Split(v, "\n")
		for label, s := range p.hints {
			i := s.line - p.vp.YOffset()
			if i < 0 || i >= len(rows) {
				continue
			}
			w := ansi.StringWidth(rows[i])
			rows[i] = ansi.Cut(rows[i], 0, s.from) + tag.Render(label) + ansi.Cut(rows[i], min(w, s.from+len(label)), w)
		}
		v = strings.Join(rows, "\n")
	}
	if p.mark == nil || p.mark.empty() {
		return v
	}
	a, b := p.mark.ordered()
	st := lipgloss.NewStyle().Foreground(p.pal.Ink).Background(p.pal.AccentSoft)
	rows := strings.Split(v, "\n")
	for i, row := range rows {
		n := p.vp.YOffset() + i
		if n < a.line || n > b.line || isImageRow(row) {
			continue
		}
		from, to := p.mark.cols(n, ansi.StringWidth(row))
		if from < to {
			rows[i] = ansi.Cut(row, 0, from) + st.Render(ansi.Strip(ansi.Cut(row, from, to))) + ansi.Cut(row, to, ansi.StringWidth(row))
		}
	}
	return strings.Join(rows, "\n")
}

// point is a place in the rendered note: a line and a cell in it.
type point struct{ line, col int }

// span is a selection from where the mouse went down to where it is now, either way round.
type span struct{ anchor, head point }

func (s *span) empty() bool { return s.anchor == s.head }

func (s *span) ordered() (point, point) {
	a, b := s.anchor, s.head
	if b.line < a.line || b.line == a.line && b.col < a.col {
		a, b = b, a
	}
	return a, b
}

// cols is the cells of line n inside the selection, as [from, to) of a line w cells wide: the
// first line from where it starts, the last up to and with where it ends, those between whole.
func (s *span) cols(n, w int) (int, int) {
	a, b := s.ordered()
	from, to := 0, w
	if n == a.line {
		from = a.col
	}
	if n == b.line {
		to = min(w, b.col+1)
	}
	return min(from, w), to
}

// selected is the selected text, plain: the note's margin and a code block's indent are taken
// off every line alike, spaces at the ends of lines dropped, and image rows left out.
func (p *preview) selected() string {
	if p.mark == nil || p.mark.empty() {
		return ""
	}
	a, b := p.mark.ordered()
	type frag struct {
		text   string
		from   int
		indent int
	}
	var frags []frag
	indent := -1
	for n := a.line; n <= b.line && n < len(p.lines); n++ {
		line := ansi.Strip(p.lines[n])
		if isImageRow(line) {
			continue
		}
		from, to := p.mark.cols(n, ansi.StringWidth(line))
		f := frag{text: line, from: from, indent: -1}
		if t := strings.TrimLeft(line, " "); strings.TrimSpace(ansi.Cut(line, from, to)) != "" {
			f.indent = len(line) - len(t)
			if indent < 0 || f.indent < indent {
				indent = f.indent
			}
		}
		f.text = ansi.Cut(line, 0, to)
		frags = append(frags, f)
	}
	var out []string
	for _, f := range frags {
		out = append(out, strings.TrimRight(ansi.Cut(f.text, max(f.from, indent), ansi.StringWidth(f.text)), " "))
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

// isImageRow reports a row of image placeholders, which are no text to select.
func isImageRow(s string) bool { return strings.ContainsRune(s, '\U0010EEEE') }

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

// relDir is the folder of a file as a path from root, "" when it is not under root.
func relDir(root, file string) string {
	if root == "" {
		return ""
	}
	r, err := filepath.Rel(root, filepath.Dir(file))
	if err != nil || strings.HasPrefix(r, "..") {
		return ""
	}
	return path.Clean(filepath.ToSlash(r))
}
