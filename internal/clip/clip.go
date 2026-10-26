// Package clip reads an image from the system clipboard for nt paste: AppKit through osascript
// on macOS, wl-paste or xclip on Linux. Nothing is installed or fetched; nt uses the OS's own
// tools (docs/design.md §4).
package clip

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ErrNoImage says the clipboard holds no image.
var ErrNoImage = errors.New("the clipboard holds no image: copy one first (on a Mac, Ctrl-Cmd-Shift-4 puts a screenshot there)")

// Image is what the clipboard held: a file copied in the file manager, kept as it is, or image
// data, written as PNG to the path Read was given.
type Image struct {
	File string // the copied file
	PNG  bool   // image data was written as PNG
}

// Read reads the clipboard. A copied file comes back as its path, whatever it is; image data is
// written to png.
func Read(png string) (Image, error) {
	switch runtime.GOOS {
	case "darwin":
		return readMac(png, "")
	case "linux":
		return readLinux(png)
	}
	return Image{}, fmt.Errorf("nt paste does not know the clipboard on %s", runtime.GOOS)
}

// macScript asks AppKit for the pasteboard: file URLs first (a file copied in Finder also
// carries its icon as an image), then PNG data, then TIFF converted to PNG. argv is the PNG path
// and a named pasteboard, "" for the clipboard.
const macScript = `ObjC.import('AppKit');
function run(argv) {
  const pb = argv[1] ? $.NSPasteboard.pasteboardWithName(argv[1]) : $.NSPasteboard.generalPasteboard;
  const opts = $.NSDictionary.dictionaryWithObjectForKey(true, $.NSPasteboardURLReadingFileURLsOnlyKey);
  const urls = pb.readObjectsForClassesOptions($.NSArray.arrayWithObject($.NSURL), opts);
  if (!urls.isNil() && urls.count > 0) return 'file\t' + ObjC.unwrap(urls.objectAtIndex(0).path);
  let data = pb.dataForType($.NSPasteboardTypePNG);
  if (data.isNil()) {
    const tiff = pb.dataForType($.NSPasteboardTypeTIFF);
    if (tiff.isNil()) return 'none';
    const rep = $.NSBitmapImageRep.imageRepWithData(tiff);
    if (rep.isNil()) return 'none';
    data = rep.representationUsingTypeProperties($.NSBitmapImageFileTypePNG, $.NSDictionary.dictionary);
  }
  return data.writeToFileAtomically(argv[0], true) ? 'png' : 'unwritable';
}`

func readMac(png, board string) (Image, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", macScript, png, board)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Image{}, fmt.Errorf("reading the clipboard: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	kind, file, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")
	switch kind {
	case "file":
		return Image{File: file}, nil
	case "png":
		return Image{PNG: true}, nil
	case "unwritable":
		return Image{}, fmt.Errorf("could not write the clipboard image to %s", png)
	}
	return Image{}, ErrNoImage
}

// readLinux uses wl-paste on Wayland, xclip on X11: a text/uri-list of files first, then
// image/png.
func readLinux(png string) (Image, error) {
	var get func(typ string) ([]byte, error)
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		if _, err := exec.LookPath("wl-paste"); err != nil {
			return Image{}, errors.New("nt paste needs wl-paste on Wayland: install wl-clipboard")
		}
		get = func(typ string) ([]byte, error) {
			if typ == "" {
				return exec.Command("wl-paste", "--list-types").Output()
			}
			return exec.Command("wl-paste", "--no-newline", "--type", typ).Output()
		}
	case os.Getenv("DISPLAY") != "":
		if _, err := exec.LookPath("xclip"); err != nil {
			return Image{}, errors.New("nt paste needs xclip on X11: install xclip")
		}
		get = func(typ string) ([]byte, error) {
			return exec.Command("xclip", "-selection", "clipboard", "-o", "-t", stringOr(typ, "TARGETS")).Output()
		}
	default:
		return Image{}, errors.New("no clipboard here: nt paste needs a desktop session (DISPLAY or WAYLAND_DISPLAY)")
	}
	types, err := get("")
	if err != nil {
		return Image{}, ErrNoImage
	}
	has := func(t string) bool {
		for _, l := range strings.Fields(string(types)) {
			if l == t {
				return true
			}
		}
		return false
	}
	if has("text/uri-list") {
		if b, err := get("text/uri-list"); err == nil {
			if f, ok := firstFile(string(b)); ok {
				return Image{File: f}, nil
			}
		}
	}
	if !has("image/png") {
		return Image{}, ErrNoImage
	}
	b, err := get("image/png")
	if err != nil || len(b) == 0 {
		return Image{}, ErrNoImage
	}
	if err := os.WriteFile(png, b, 0o644); err != nil {
		return Image{}, err
	}
	return Image{PNG: true}, nil
}

// ErrNoClipboard says this machine has no clipboard nt can write to: over ssh, or without
// wl-copy or xclip. Callers fall back to OSC 52, which the terminal carries.
var ErrNoClipboard = errors.New("no clipboard here")

// Write puts text on the clipboard: pbcopy on macOS, wl-copy on Wayland, xclip on X11. It works
// in tmux whatever its set-clipboard says, which OSC 52 does not.
func Write(text string) error {
	var argv []string
	switch {
	case runtime.GOOS == "darwin":
		argv = []string{"pbcopy"}
	case runtime.GOOS != "linux":
	case os.Getenv("WAYLAND_DISPLAY") != "":
		argv = []string{"wl-copy"}
	case os.Getenv("DISPLAY") != "":
		argv = []string{"xclip", "-selection", "clipboard", "-in"}
	}
	if argv == nil {
		return ErrNoClipboard
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return ErrNoClipboard
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// firstFile is the first file:// entry of a text/uri-list, as a path.
func firstFile(list string) (string, bool) {
	for _, l := range strings.Split(list, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if u, err := url.Parse(l); err == nil && u.Scheme == "file" && u.Path != "" {
			return u.Path, true
		}
	}
	return "", false
}

func stringOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
