// Package termimg draws images inside the terminal with the kitty graphics protocol and its
// Unicode placeholders. An image is sent to the terminal once, under an id; the preview then
// carries rows of placeholder characters whose colour names that id. The placeholders are text,
// so tmux stores, scrolls and redraws them like any other cell, and the terminal paints the image
// over them. Ghostty and kitty implement this; elsewhere the preview keeps the alt text.
//
// See https://sw.kovidgoyal.net/kitty/graphics-protocol/#unicode-placeholders.
package termimg

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // decoders for image.Decode
	_ "image/jpeg"
	"image/png"
	"math/rand/v2"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

const (
	maxFile  = 20 << 20 // larger files stay links, as nt sync skips files over 20 MB
	maxSide  = 1600     // longer sides are scaled down before sending: enough for a preview pane
	maxCells = 255      // rows and columns a placement can address with the diacritics table
)

// Terminal sends images to one terminal. Its zero value is unusable; get one from Detect.
type Terminal struct {
	layers       int         // tmux servers between nt and the terminal: 0, 1, or 2 inside flok
	servers      []tmuxQuery // how to reach each of them, nearest first
	cellW, cellH int         // pixels per cell, for sizing; a 1:2 guess when the terminal does not say
	high         int         // the high byte of every image id: one per process, so nt in another pane keeps its own images
	next         int
	sent         []*Image
}

// Image is one picture loaded for the terminal.
type Image struct {
	id          int
	data        []byte // PNG, sent as is
	w, h        int    // pixels
	cols, rows  int    // the placement the terminal holds; 0 before it is sent
	low         int    // the image id's low byte, carried as the placeholder colour
	modTime     int64
	size        int64
	transmitted bool
}

// Detect decides whether images can be drawn, from mode ("auto", "on" or "off") and the
// environment. It returns nil when they cannot; why says what is missing and how to fix it,
// or is empty when there is nothing the person can change.
func Detect(mode string) (t *Terminal, why string) {
	var info tmuxInfo
	if os.Getenv("TMUX") != "" {
		info = tmuxQuery{pane: os.Getenv("TMUX_PANE")}.info()
		if isTmux(info) {
			if sock := outerSocket(); sock != "" {
				outer := tmuxQuery{args: []string{"-S", sock}}.info()
				info.outer = &outer
			}
		}
	}
	return detect(mode, os.Getenv, info, cellSize)
}

// tmuxQuery addresses one tmux server: the one nt runs in (no args, nt's pane), or another one
// by socket (-S path).
type tmuxQuery struct {
	args []string
	pane string
}

// display runs display-message -p with format on the server.
func (q tmuxQuery) display(format string) (string, error) {
	args := append(append([]string{}, q.args...), "display-message", "-p")
	if q.pane != "" {
		args = append(args, "-t", q.pane)
	}
	out, err := exec.Command("tmux", append(args, format)...).Output()
	return strings.TrimRight(string(out), "\n"), err
}

// info asks the server about its client terminal.
func (q tmuxQuery) info() tmuxInfo {
	out, err := q.display("#{client_termname}\t#{client_termtype}\t#{allow-passthrough}\t#{client_cell_width}\t#{client_cell_height}")
	if err != nil {
		return tmuxInfo{inside: true, query: q}
	}
	f := strings.Split(out, "\t")
	for len(f) < 5 {
		f = append(f, "")
	}
	cw, _ := strconv.Atoi(f[3])
	ch, _ := strconv.Atoi(f[4])
	return tmuxInfo{inside: true, query: q, termname: f[0], termtype: f[1], passthrough: f[2], cellW: cw, cellH: ch}
}

// tmuxInfo is what one tmux server knows about the terminal it runs in.
type tmuxInfo struct {
	inside       bool
	query        tmuxQuery
	termname     string // the client terminal's TERM, e.g. xterm-ghostty
	termtype     string // its XTVERSION answer, e.g. ghostty 1.3.1, or tmux 3.8 when tmux runs in tmux
	passthrough  string // allow-passthrough: off, on or all
	cellW, cellH int
	outer        *tmuxInfo // the server whose pane this server's client runs in (flok's outer tmux), when known
}

// outerSocket is the socket of the tmux server around this one, as flok publishes it in the
// inner server's global environment; "" when nothing does.
func outerSocket() string {
	out, err := exec.Command("tmux", "show-environment", "-g", "FLOK_OUTER_SOCKET").Output()
	if err != nil {
		return ""
	}
	return envValue(string(out), "FLOK_OUTER_SOCKET")
}

// envValue reads NAME=value from show-environment output; a removed variable prints -NAME.
func envValue(out, name string) string {
	v, ok := strings.CutPrefix(strings.TrimSpace(out), name+"=")
	if !ok {
		return ""
	}
	return v
}

// isTmux reports a tmux server whose client terminal is itself a tmux pane.
func isTmux(tm tmuxInfo) bool {
	return strings.HasPrefix(tm.termtype, "tmux ") || strings.HasPrefix(tm.termname, "tmux") || strings.HasPrefix(tm.termname, "screen")
}

func detect(mode string, env func(string) string, tm tmuxInfo, cells func() (int, int)) (*Terminal, string) {
	if mode == "off" {
		return nil, ""
	}
	// the servers between nt and the terminal, nearest first
	var servers []tmuxInfo
	if tm.inside {
		servers = append(servers, tm)
		if tm.outer != nil {
			servers = append(servers, *tm.outer)
		}
	}
	t := &Terminal{layers: len(servers), high: 1 + rand.IntN(255)}
	for _, sv := range servers {
		t.servers = append(t.servers, sv.query)
	}
	t.cellW, t.cellH = cells()
	for i := len(servers) - 1; i >= 0; i-- { // the nearest server that knows the cell size
		if servers[i].cellW > 0 && servers[i].cellH > 0 {
			t.cellW, t.cellH = servers[i].cellW, servers[i].cellH
		}
	}
	if t.cellW <= 0 || t.cellH <= 0 {
		t.cellW, t.cellH = 10, 20
	}
	if mode == "on" {
		return t, ""
	}
	if len(servers) == 0 {
		if kittyLike(env("TERM")) || kittyLike(env("TERM_PROGRAM")) || env("KITTY_WINDOW_ID") != "" {
			return t, ""
		}
		return nil, ""
	}
	last := servers[len(servers)-1] // the server whose client is the terminal itself
	if !kittyLike(last.termname) && !kittyLike(last.termtype) {
		return nil, ""
	}
	for i, s := range servers {
		if s.passthrough != "on" && s.passthrough != "all" {
			if i > 0 {
				return nil, "images: the outer tmux needs allow-passthrough"
			}
			return nil, "images: set -g allow-passthrough on"
		}
	}
	return t, ""
}

// kittyLike reports a terminal known to draw Unicode placeholders.
func kittyLike(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "ghostty") || strings.Contains(s, "kitty")
}

// Load reads an image file, ready to send. A file that changed since the last Load of the same
// Image is read again under the same id. PNG files up to maxSide are sent as they are; other
// formats and larger pictures are scaled and encoded as PNG first.
func (t *Terminal) Load(path string, prev *Image) (*Image, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if prev != nil && prev.modTime == st.ModTime().UnixNano() && prev.size == st.Size() {
		return prev, nil
	}
	if st.Size() > maxFile {
		return nil, fmt.Errorf("%s: larger than 20 MB", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	im := prev
	if im == nil {
		t.next++
		low := 1 + (t.next-1)%255
		im = &Image{id: t.high<<24 | low, low: low}
	}
	im.modTime, im.size, im.transmitted, im.cols, im.rows = st.ModTime().UnixNano(), st.Size(), false, 0, 0
	im.w, im.h, im.data = cfg.Width, cfg.Height, raw
	if format != "png" || max(cfg.Width, cfg.Height) > maxSide {
		m, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		m = shrink(m, maxSide)
		var b bytes.Buffer
		if err := png.Encode(&b, m); err != nil {
			return nil, err
		}
		im.data, im.w, im.h = b.Bytes(), m.Bounds().Dx(), m.Bounds().Dy()
	}
	return im, nil
}

// Fit returns the cells the image takes, keeping its shape: its own size in cells, but at most
// maxCols wide and maxRows tall.
func (t *Terminal) Fit(im *Image, maxCols, maxRows int) (cols, rows int) {
	if im.w <= 0 || im.h <= 0 {
		return 1, 1
	}
	maxCols, maxRows = clamp(maxCols, 1, maxCells), clamp(maxRows, 1, maxCells)
	cols = min(maxCols, ceilDiv(im.w, t.cellW))
	rows = ceilDiv(cols*t.cellW*im.h, im.w*t.cellH)
	if rows > maxRows {
		rows = maxRows
		cols = clamp(ceilDiv(rows*t.cellH*im.w, im.h*t.cellW), 1, maxCols)
	}
	return cols, max(rows, 1)
}

// Show returns what the terminal must receive so the image fills cols × rows cells: the whole
// image the first time, a new placement when only the size changed, nothing when it already
// holds that. The caller sends it, then draws Rows.
func (t *Terminal) Show(im *Image, cols, rows int) string {
	if im.transmitted && im.cols == cols && im.rows == rows {
		return ""
	}
	place := []string{"U=1", "i=" + strconv.Itoa(im.id), "c=" + strconv.Itoa(cols), "r=" + strconv.Itoa(rows), "q=2"}
	var out string
	if im.transmitted {
		out = t.wrap(ansi.KittyGraphics(nil, "a=d", "d=i", "i="+strconv.Itoa(im.id), "q=2")) +
			t.wrap(ansi.KittyGraphics(nil, append([]string{"a=p"}, place...)...))
	} else {
		out = t.transmit(im, append([]string{"a=T", "f=100", "t=d"}, place...))
		if !t.listed(im) {
			t.sent = append(t.sent, im)
		}
	}
	im.transmitted, im.cols, im.rows = true, cols, rows
	return out
}

// transmit sends the PNG in base64 chunks of at most 4096 bytes: the first chunk carries the
// keys, every chunk says whether more follow (m=1) or not (m=0).
func (t *Terminal) transmit(im *Image, keys []string) string {
	payload := base64.StdEncoding.EncodeToString(im.data)
	var b strings.Builder
	for first := true; first || payload != ""; first = false {
		n := min(len(payload), kitty.MaxChunkSize)
		chunk := payload[:n]
		payload = payload[n:]
		more := "m=0"
		if payload != "" {
			more = "m=1"
		}
		opts := []string{more, "q=2"}
		if first {
			opts = append(keys, more)
		}
		b.WriteString(t.wrap(ansi.KittyGraphics([]byte(chunk), opts...)))
	}
	return b.String()
}

// Rows draws the image as placeholder text, one string per row, cols cells wide. Each cell names
// its row and column with combining diacritics and the image id's high byte with a third one;
// the 256-colour foreground carries the low byte, so the id survives any colour profile.
func (t *Terminal) Rows(im *Image) []string {
	out := make([]string, im.rows)
	hi := kitty.Diacritic(im.id >> 24)
	for r := range im.rows {
		var b strings.Builder
		b.WriteString("\x1b[38;5;" + strconv.Itoa(im.low) + "m") // the image id, not a colour
		for c := range im.cols {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(r))
			b.WriteRune(kitty.Diacritic(c))
			b.WriteRune(hi)
		}
		b.WriteString("\x1b[39m")
		out[r] = b.String()
	}
	return out
}

// Discarded is how many bytes the tmux servers on the way have thrown away instead of passing
// them to their terminal, summed; -1 outside tmux. tmux drops its pending output, image data
// included, when the terminal falls behind, and later redraws only the text: a count that grew
// across a transfer means the images must be sent again.
func (t *Terminal) Discarded() int {
	if len(t.servers) == 0 {
		return -1
	}
	sum := 0
	for _, q := range t.servers {
		out, err := q.display("#{client_discarded}")
		if err != nil {
			return -1
		}
		n, _ := strconv.Atoi(out)
		sum += n
	}
	return sum
}

// Forget marks every image as not held by the terminal, so the next Show sends it again: the
// terminal may drop images when another program (the editor) takes the screen.
func (t *Terminal) Forget() {
	for _, im := range t.sent {
		im.transmitted = false
	}
}

// Clear deletes every image this process sent, data and placements, for when nt exits.
func (t *Terminal) Clear() string {
	var b strings.Builder
	for _, im := range t.sent {
		b.WriteString(t.wrap(ansi.KittyGraphics(nil, "a=d", "d=I", "i="+strconv.Itoa(im.id), "q=2")))
	}
	t.sent = nil
	return b.String()
}

func (t *Terminal) listed(im *Image) bool {
	for _, s := range t.sent {
		if s == im {
			return true
		}
	}
	return false
}

// wrap packs seq in tmux's passthrough once per server on the way: each server unpacks one
// layer and hands the rest to its client, so the terminal gets seq itself.
func (t *Terminal) wrap(seq string) string {
	for range t.layers {
		seq = ansi.TmuxPassthrough(seq)
	}
	return seq
}

// shrink scales m down so its longer side is at most side pixels, averaging each block of
// pixels; smaller images are returned as they are.
func shrink(m image.Image, side int) image.Image {
	b := m.Bounds()
	k := ceilDiv(max(b.Dx(), b.Dy()), side)
	if k <= 1 {
		return m
	}
	w, h := b.Dx()/k, b.Dy()/k
	out := image.NewNRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	for y := range out.Rect.Dy() {
		for x := range out.Rect.Dx() {
			var r, g, bl, a uint32
			for dy := range k {
				for dx := range k {
					pr, pg, pb, pa := m.At(b.Min.X+x*k+dx, b.Min.Y+y*k+dy).RGBA()
					r, g, bl, a = r+pr, g+pg, bl+pb, a+pa
				}
			}
			n := uint32(k * k)
			out.Set(x, y, color.RGBA64{uint16(r / n), uint16(g / n), uint16(bl / n), uint16(a / n)})
		}
	}
	return out
}

func ceilDiv(a, b int) int {
	if b <= 0 {
		return a
	}
	return (a + b - 1) / b
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }
