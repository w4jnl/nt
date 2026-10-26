package termimg

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func TestDetect(t *testing.T) {
	cells := func() (int, int) { return 9, 18 }
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	ghosttyTmux := tmuxInfo{inside: true, termname: "xterm-ghostty", termtype: "ghostty 1.3.1", passthrough: "on", cellW: 8, cellH: 18}
	// flok: nt's tmux runs in a pane of flok's outer tmux, which runs in Ghostty
	inFlok := func(inner, outer string) tmuxInfo {
		o := tmuxInfo{inside: true, termname: "xterm-ghostty", termtype: "ghostty 1.3.1", passthrough: outer, cellW: 8, cellH: 18}
		return tmuxInfo{inside: true, termname: "tmux-256color", termtype: "tmux 3.8", passthrough: inner, outer: &o}
	}
	for _, tc := range []struct {
		name, mode string
		env        map[string]string
		tmux       tmuxInfo
		ok         bool
		layers     int
		why        string
	}{
		{"off wins", "off", map[string]string{"TERM": "xterm-ghostty"}, tmuxInfo{}, false, 0, ""},
		{"ghostty", "auto", map[string]string{"TERM": "xterm-ghostty"}, tmuxInfo{}, true, 0, ""},
		{"kitty", "auto", map[string]string{"TERM": "xterm-kitty"}, tmuxInfo{}, true, 0, ""},
		{"kitty by window id", "auto", map[string]string{"KITTY_WINDOW_ID": "1"}, tmuxInfo{}, true, 0, ""},
		{"apple terminal", "auto", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "Apple_Terminal"}, tmuxInfo{}, false, 0, ""},
		{"ghostty in tmux", "auto", nil, ghosttyTmux, true, 1, ""},
		{"ghostty in tmux, all", "auto", nil, tmuxInfo{inside: true, termname: "xterm-ghostty", passthrough: "all"}, true, 1, ""},
		{"ghostty in tmux by version", "auto", nil, tmuxInfo{inside: true, termname: "xterm-256color", termtype: "ghostty 1.3.1", passthrough: "on"}, true, 1, ""},
		{"passthrough off", "auto", nil, tmuxInfo{inside: true, termname: "xterm-ghostty", passthrough: "off"}, false, 0, "allow-passthrough on"},
		{"tmux in terminal.app", "auto", nil, tmuxInfo{inside: true, termname: "xterm-256color", passthrough: "on"}, false, 0, ""},
		{"tmux in tmux, outer unknown", "auto", nil, tmuxInfo{inside: true, termname: "tmux-256color", termtype: "tmux 3.8", passthrough: "on"}, false, 0, ""},
		{"inside flok", "auto", nil, inFlok("on", "on"), true, 2, ""},
		{"inside flok, inner passthrough off", "auto", nil, inFlok("off", "on"), false, 0, "set -g allow-passthrough on"},
		{"inside flok, outer passthrough off", "auto", nil, inFlok("on", "off"), false, 0, "outer tmux needs allow-passthrough"},
		{"inside flok in terminal.app", "auto", nil, func() tmuxInfo {
			tm := inFlok("on", "on")
			tm.outer.termname, tm.outer.termtype = "xterm-256color", ""
			return tm
		}(), false, 0, ""},
		{"on forces through flok", "on", nil, inFlok("off", "off"), true, 2, ""},
		{"on forces", "on", nil, tmuxInfo{inside: true, passthrough: "off"}, true, 1, ""},
	} {
		got, why := detect(tc.mode, env(tc.env), tc.tmux, cells)
		layers := -1
		if got != nil {
			layers = got.layers
		}
		if (got != nil) != tc.ok || (got != nil && layers != tc.layers) || !strings.Contains(why, tc.why) || (tc.why == "" && why != "") {
			t.Errorf("%s: got %v (layers %d) %q, want ok %v layers %d %q", tc.name, got != nil, layers, why, tc.ok, tc.layers, tc.why)
		}
	}
	if got, _ := detect("auto", env(nil), ghosttyTmux, cells); got.cellW != 8 || got.cellH != 18 {
		t.Errorf("tmux cell size not used: %dx%d", got.cellW, got.cellH)
	}
	if got, _ := detect("auto", env(nil), inFlok("on", "on"), cells); got.cellW != 8 || got.cellH != 18 {
		t.Errorf("the outer tmux's cell size not used when the inner one does not know: %dx%d", got.cellW, got.cellH)
	}
	if got, _ := detect("auto", env(map[string]string{"TERM": "xterm-ghostty"}), tmuxInfo{}, func() (int, int) { return 0, 0 }); got.cellW != 10 || got.cellH != 20 {
		t.Errorf("unknown cell size should fall back to 10x20, got %dx%d", got.cellW, got.cellH)
	}
}

func TestFit(t *testing.T) {
	term := &Terminal{cellW: 10, cellH: 20}
	for _, tc := range []struct {
		w, h, maxC, maxR, cols, rows int
	}{
		{100, 40, 80, 30, 10, 2},        // small: its own size
		{1600, 800, 80, 30, 80, 20},     // wide: the full width, shape kept (80*10/2 = 400 px = 20 rows)
		{800, 1600, 80, 30, 30, 30},     // tall: capped by rows, width follows (30*20/2 = 300 px = 30 cols)
		{4000, 10, 80, 30, 80, 1},       // a thin strip still takes a row
		{1000, 1000, 400, 400, 100, 50}, // cells never exceed the image's own pixels
	} {
		im := &Image{w: tc.w, h: tc.h}
		if c, r := term.Fit(im, tc.maxC, tc.maxR); c != tc.cols || r != tc.rows {
			t.Errorf("%dx%d in %dx%d: got %dx%d cells, want %dx%d", tc.w, tc.h, tc.maxC, tc.maxR, c, r, tc.cols, tc.rows)
		}
	}
}

func TestRowsEncodeTheImage(t *testing.T) {
	term := &Terminal{cellW: 10, cellH: 20, high: 7}
	im := &Image{id: 7<<24 | 3, low: 3, cols: 4, rows: 2}
	rows := term.Rows(im)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for r, row := range rows {
		if w := ansi.StringWidth(row); w != 4 {
			t.Errorf("row %d is %d cells wide, want 4", r, w)
		}
		if !strings.HasPrefix(row, "\x1b[38;5;3m") {
			t.Errorf("row %d does not carry the id's low byte as its colour: %q", r, row)
		}
		cells := []rune(ansi.Strip(row))
		for c := range 4 {
			got := cells[c*4 : c*4+4]
			want := []rune{kitty.Placeholder, kitty.Diacritic(r), kitty.Diacritic(c), kitty.Diacritic(7)}
			if string(got) != string(want) {
				t.Errorf("cell %d,%d = %U, want %U", r, c, got, want)
			}
		}
	}
}

func TestShowSendsOncePlacesOnResize(t *testing.T) {
	term := &Terminal{cellW: 10, cellH: 20, high: 9, layers: 1}
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	writePNG(t, p, 30, 20)
	im, err := term.Load(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	first := term.Show(im, 3, 1)
	if !strings.Contains(first, "a=T,f=100,t=d,U=1,i="+strconv.Itoa(im.id)+",c=3,r=1,q=2,m=0;") {
		t.Errorf("first show does not transmit with a virtual placement: %q", first)
	}
	if !strings.HasPrefix(first, "\x1bPtmux;\x1b\x1b_G") {
		t.Errorf("inside tmux the sequence is not wrapped for passthrough: %q", first[:min(20, len(first))])
	}
	if again := term.Show(im, 3, 1); again != "" {
		t.Errorf("same size sent again: %q", again)
	}
	resized := term.Show(im, 2, 1)
	if !strings.Contains(resized, "a=d,d=i,i=") || !strings.Contains(resized, "a=p,U=1,i=") || strings.Contains(resized, "a=T") {
		t.Errorf("a resize should replace the placement, not the data: %q", resized)
	}
	term.Forget()
	if again := term.Show(im, 2, 1); !strings.Contains(again, "a=T") {
		t.Errorf("after Forget the data is not sent again: %q", again)
	}
	if c := term.Clear(); strings.Count(c, "a=d,d=I,") != 1 {
		t.Errorf("Clear should delete the one image once: %q", c)
	}
	if again, _ := term.Load(p, im); again != im {
		t.Error("an unchanged file was loaded again")
	}
}

func TestTransmitChunks(t *testing.T) {
	term := &Terminal{cellW: 10, cellH: 20, high: 1}
	data := bytes.Repeat([]byte{0xAB}, 9000) // 12000 base64 bytes: three chunks
	seq := term.transmit(&Image{id: 1<<24 | 1, data: data}, []string{"a=T", "f=100"})
	re := regexp.MustCompile("\x1b_G([^;]*);([^\x1b]*)\x1b\\\\")
	ms := re.FindAllStringSubmatch(seq, -1)
	if len(ms) != 3 {
		t.Fatalf("got %d chunks, want 3", len(ms))
	}
	var payload strings.Builder
	for i, m := range ms {
		if len(m[2]) > kitty.MaxChunkSize {
			t.Errorf("chunk %d is %d bytes", i, len(m[2]))
		}
		wantMore := "m=1"
		if i == len(ms)-1 {
			wantMore = "m=0"
		}
		if !strings.Contains(m[1], wantMore) || (i == 0) != strings.Contains(m[1], "a=T") {
			t.Errorf("chunk %d keys %q", i, m[1])
		}
		payload.WriteString(m[2])
	}
	if got, _ := base64.StdEncoding.DecodeString(payload.String()); !bytes.Equal(got, data) {
		t.Error("chunks do not reassemble into the data")
	}
}

func TestLoadConvertsAndShrinks(t *testing.T) {
	term := &Terminal{cellW: 10, cellH: 20, high: 1}
	dir := t.TempDir()
	jp := filepath.Join(dir, "photo.jpg")
	f, _ := os.Create(jp)
	_ = jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 3300, 1650)), nil)
	f.Close()
	im, err := term.Load(jp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(im.data, []byte("\x89PNG")) || max(im.w, im.h) > maxSide || im.w != 1100 || im.h != 550 {
		t.Errorf("jpeg not sent as a shrunk PNG: %dx%d png=%v", im.w, im.h, bytes.HasPrefix(im.data, []byte("\x89PNG")))
	}
	if _, err := term.Load(filepath.Join(dir, "missing.png"), nil); err == nil {
		t.Error("a missing file loaded")
	}
	bad := filepath.Join(dir, "notes.png")
	_ = os.WriteFile(bad, []byte("not an image"), 0o644)
	if _, err := term.Load(bad, nil); err == nil {
		t.Error("a non-image loaded")
	}
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			m.Set(x, y, color.NRGBA{uint8(x * 8), uint8(y * 12), 160, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, m); err != nil {
		t.Fatal(err)
	}
}

func TestWrapOncePerServer(t *testing.T) {
	seq := ansi.KittyGraphics([]byte("QUJD"), "a=T", "m=0")
	for layers := range 3 {
		got := (&Terminal{layers: layers}).wrap(seq)
		for range layers { // each tmux server unpacks one layer, as tmux does
			inner, ok := strings.CutPrefix(got, "\x1bPtmux;")
			if !ok || !strings.HasSuffix(inner, "\x1b\\") {
				t.Fatalf("layers %d: not a tmux passthrough: %q", layers, got)
			}
			got = strings.ReplaceAll(strings.TrimSuffix(inner, "\x1b\\"), "\x1b\x1b", "\x1b")
		}
		if got != seq {
			t.Errorf("layers %d: the terminal would get %q, want %q", layers, got, seq)
		}
	}
}

func TestEnvValue(t *testing.T) {
	for _, tc := range []struct{ out, want string }{
		{"FLOK_OUTER_SOCKET=/private/tmp/tmux-501/flok\n", "/private/tmp/tmux-501/flok"},
		{"-FLOK_OUTER_SOCKET\n", ""},
		{"", ""},
		{"OTHER=x\n", ""},
	} {
		if got := envValue(tc.out, "FLOK_OUTER_SOCKET"); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.out, got, tc.want)
		}
	}
}
