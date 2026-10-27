package tui

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/mdparse"
)

// Following links in the preview (docs/design.md §4): a click on a link follows it; f labels the
// links on screen with letters, and typing one follows it; b goes back to the note before.
// Glamour writes every link as an OSC 8 hyperlink with its target as written in the note, so the
// rendered lines say which cells belong to which link.

// linkSpot is a link's cells on one rendered line, [from, to), and where it points; text is the
// bytes of the line between its hyperlink's start and end.
type linkSpot struct {
	line, from, to int
	dest           string
	text           [2]int
}

// findLinks reads the OSC 8 hyperlinks in the rendered lines: the cells each one covers. A link
// wrapped over two lines is a spot on each.
func findLinks(lines []string) []linkSpot {
	var out []linkSpot
	for n, l := range lines {
		col, start, bstart, dest := 0, 0, 0, ""
		end := func(at int) {
			if dest != "" && col > start {
				out = append(out, linkSpot{n, start, col, dest, [2]int{bstart, at}})
			}
		}
		for i := 0; i < len(l); {
			if l[i] == 0x1b && i+1 < len(l) {
				switch l[i+1] {
				case ']': // OSC, up to BEL or ST
					j := i + 2
					for j < len(l) && l[j] != 0x07 && !(l[j] == 0x1b && j+1 < len(l) && l[j+1] == '\\') {
						j++
					}
					osc := l[i+2 : j]
					end(i)
					i = j + 1
					if j < len(l) && l[j] == 0x1b {
						i = j + 2
					}
					if rest, ok := strings.CutPrefix(osc, "8;"); ok {
						_, uri, _ := strings.Cut(rest, ";")
						dest, start, bstart = uri, col, i
					}
				case '[': // CSI, up to its final byte
					j := i + 2
					for j < len(l) && (l[j] < 0x40 || l[j] > 0x7e) {
						j++
					}
					i = j + 1
				default:
					i += 2
				}
				continue
			}
			r, size := utf8.DecodeRuneInString(l[i:])
			col += ansi.StringWidth(string(r))
			i += size
		}
		end(len(l))
	}
	return out
}

// linkAt is the link under a cell of the rendered note.
func (p *preview) linkAt(at point) (linkSpot, bool) {
	for _, s := range p.links {
		if s.line == at.line && at.col >= s.from && at.col < s.to {
			return s, true
		}
	}
	return linkSpot{}, false
}

// hintKeys are the letters f labels links with, the home row first.
const hintKeys = "asdfghjklqwertyuiopzxcvbnm"

// hintLabels are n labels of one letter, or of two when there are more links than letters, so
// no label starts another.
func hintLabels(n int) []string {
	keys := strings.Split(hintKeys, "")
	if n <= len(keys) {
		return keys[:n]
	}
	var out []string
	for _, a := range keys {
		for _, b := range keys {
			if len(out) == n {
				return out
			}
			out = append(out, a+b)
		}
	}
	return out
}

// startHints labels each link on screen, one label per target.
func (b *browser) startHints() tea.Cmd {
	if b.showing == "" {
		return nil
	}
	top := b.p.vp.YOffset()
	seen := map[string]bool{}
	var spots []linkSpot
	for _, s := range b.p.links {
		if s.line >= top && s.line < top+b.p.height && !seen[s.dest] {
			seen[s.dest] = true
			spots = append(spots, s)
		}
	}
	if len(spots) == 0 {
		b.say("no links on screen", false)
		return nil
	}
	b.p.hints, b.hinted = map[string]linkSpot{}, ""
	for i, l := range hintLabels(len(spots)) {
		b.p.hints[l] = spots[i]
	}
	return nil
}

// hintKey takes the letters of a label: a whole one follows its link, esc or a letter no label
// goes on with cancels.
func (b *browser) hintKey(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if s == "esc" || len(s) != 1 || !strings.Contains(hintKeys, s) {
		b.p.hints = nil
		return nil
	}
	b.hinted += s
	if spot, ok := b.p.hints[b.hinted]; ok {
		b.p.hints = nil
		return b.follow(spot.dest)
	}
	for l := range b.p.hints {
		if strings.HasPrefix(l, b.hinted) {
			return nil
		}
	}
	b.say("no link has the letters "+b.hinted, false)
	b.p.hints = nil
	return nil
}

// visit is a note the browser showed, and where it was scrolled to, for b.
type visit struct {
	rel string
	top int
}

const historyMax = 50

// follow goes where a link in the note on screen points, dest as written in it: a note, at the
// heading its anchor names; a folder; an image or another file in the repository, opened as the
// i key opens images; a URL, opened by the OS. The note on screen is remembered, so b comes back
// to it.
func (b *browser) follow(dest string) tea.Cmd {
	if mdparse.External(dest) {
		b.say("opened "+dest, false)
		openDetached(osOpener(), dest)
		return nil
	}
	p, anchor, ok := mdparse.Resolve(b.showing, dest)
	if !ok {
		b.say(dest+": the link points outside the notes", true)
		return nil
	}
	if p == b.showing {
		b.p.showText(strings.ReplaceAll(anchor, "-", " "))
		return nil
	}
	st, err := os.Stat(b.v.Abs(p))
	switch {
	case err != nil:
		b.say(p+": the link points to nothing", true)
		return nil
	case st.IsDir():
		b.remember()
		b.cal = nil
		b.focus, b.full = onList, false
		b.layout()
		return b.openFolder(p)
	case strings.HasSuffix(strings.ToLower(p), ".md"):
		b.remember()
		b.cal = nil
		cmd := b.reveal(p)
		if anchor != "" {
			b.p.showText(strings.ReplaceAll(anchor, "-", " "))
		}
		return cmd
	case imageExt[strings.ToLower(path.Ext(p))]:
		return b.openImage(b.v.Abs(p))
	}
	b.say("opened "+p, false)
	openDetached(osOpener(), b.v.Abs(p))
	return nil
}

var imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true}

// remember keeps the note on screen, and how far it was scrolled, for b.
func (b *browser) remember() {
	if b.showing == "" {
		return
	}
	b.history = append(b.history, visit{b.showing, b.p.vp.YOffset()})
	if len(b.history) > historyMax {
		b.history = b.history[1:]
	}
}

// back shows the note a link was followed from, scrolled as it was.
func (b *browser) back() tea.Cmd {
	for len(b.history) > 0 {
		v := b.history[len(b.history)-1]
		b.history = b.history[:len(b.history)-1]
		if !isFile(b.v.Abs(v.rel)) {
			continue // gone since: the one before it
		}
		b.cal = nil
		cmd := b.reveal(v.rel)
		b.p.vp.SetYOffset(v.top)
		return cmd
	}
	b.say("no note to go back to", false)
	return nil
}

// markMissing draws the links whose file is not there in ink-faint, struck through: they show
// as broken before a click says so (docs/design-system, link states).
func (p *preview) markMissing(lines []string) {
	gone := lipgloss.NewStyle().Foreground(p.pal.InkFaint).Strikethrough(true)
	missing := map[string]bool{}
	for i := len(p.links) - 1; i >= 0; i-- { // from the end, so the byte offsets before stay true
		s := p.links[i]
		m, ok := missing[s.dest]
		if !ok {
			m = p.missing(s.dest)
			missing[s.dest] = m
		}
		if !m {
			continue
		}
		l := lines[s.line]
		a, b := s.text[0], min(s.text[1], len(l))
		lines[s.line] = l[:a] + gone.Render(ansi.Strip(l[a:b])) + l[b:]
	}
}

// missing reports a link, as written in the note, to a local file that is not there. URLs and
// links to a heading in the note are never missing.
func (p *preview) missing(dest string) bool {
	if mdparse.External(dest) {
		return false
	}
	f, _, _ := strings.Cut(dest, "#")
	if f == "" {
		return false
	}
	if d, err := url.PathUnescape(f); err == nil {
		f = d
	}
	if !filepath.IsAbs(f) {
		f = filepath.Join(filepath.Dir(p.path), filepath.FromSlash(f))
	}
	_, err := os.Stat(f)
	return err != nil
}
