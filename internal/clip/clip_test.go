package clip

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFirstFile(t *testing.T) {
	for _, tc := range []struct {
		list, want string
		ok         bool
	}{
		{"file:///home/me/shot.png\n", "/home/me/shot.png", true},
		{"# copied\r\nfile:///home/me/a%20b.png\r\nfile:///x.png\r\n", "/home/me/a b.png", true},
		{"https://example.com/x.png\n", "", false},
		{"", "", false},
	} {
		got, ok := firstFile(tc.list)
		if got != tc.want || ok != tc.ok {
			t.Errorf("firstFile(%q) = %q, %v; want %q, %v", tc.list, got, ok, tc.want, tc.ok)
		}
	}
}

// writeScript fills a named pasteboard, so the test never touches the person's clipboard: argv
// is the pasteboard, then png, tiff or file and a path, or nothing to leave it empty.
const writeScript = `ObjC.import('AppKit');
function run(argv) {
  const pb = $.NSPasteboard.pasteboardWithName(argv[0]);
  pb.clearContents;
  if (argv[1] === 'file') return pb.writeObjects($.NSArray.arrayWithObject($.NSURL.fileURLWithPath(argv[2])));
  if (argv[1] === 'text') return pb.setStringForType('just text', $.NSPasteboardTypeString);
  if (!argv[1]) return true;
  const data = $.NSData.dataWithContentsOfFile(argv[2]);
  return pb.setDataForType(data, argv[1] === 'tiff' ? $.NSPasteboardTypeTIFF : $.NSPasteboardTypePNG);
}`

func TestReadMac(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the pasteboard is macOS's")
	}
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(1, 1, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "in.png")
	if err := os.WriteFile(in, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	tiff := filepath.Join(dir, "in.tiff")
	if out, err := exec.Command("sips", "-s", "format", "tiff", in, "--out", tiff).CombinedOutput(); err != nil {
		t.Fatalf("sips: %v: %s", err, out)
	}
	board := "nt-test-" + filepath.Base(dir)
	t.Cleanup(func() {
		_ = exec.Command("osascript", "-l", "JavaScript", "-e", "ObjC.import('AppKit'); function run(argv) { $.NSPasteboard.pasteboardWithName(argv[0]).releaseGlobally; }", board).Run()
	})
	fill := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("osascript", append([]string{"-l", "JavaScript", "-e", writeScript, board}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("filling the pasteboard: %v: %s", err, out)
		}
	}
	for _, tc := range []struct {
		name string
		fill []string
		want Image
		err  error
	}{
		{"png data", []string{"png", in}, Image{PNG: true}, nil},
		{"tiff data", []string{"tiff", tiff}, Image{PNG: true}, nil},
		{"a copied file", []string{"file", in}, Image{File: in}, nil},
		{"text only", []string{"text"}, Image{}, ErrNoImage},
		{"empty", nil, Image{}, ErrNoImage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fill(tc.fill...)
			out := filepath.Join(dir, tc.name+".png")
			got, err := readMac(out, board)
			if err != tc.err || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v, %v", got, err, tc.want, tc.err)
			}
			if !got.PNG {
				return
			}
			f, err := os.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			back, err := png.Decode(f)
			if err != nil {
				t.Fatalf("the written file is not a PNG: %v", err)
			}
			if back.Bounds().Dx() != 4 || back.Bounds().Dy() != 3 {
				t.Fatalf("the written image is %v, want 4x3", back.Bounds())
			}
		})
	}
}
