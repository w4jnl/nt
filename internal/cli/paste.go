package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/w4jnl/nt/internal/clip"
	"github.com/w4jnl/nt/internal/ops"
)

// readClipboard is the clipboard, a variable so tests can stand in for it.
var readClipboard = clip.Read

// nt paste <note> [--name words] [--from file]: save the clipboard's image beside the note, as
// assets/YYYY-MM-DD-<name>.<ext> in its folder, and print the markdown line that shows it, for
// the editor to insert. Image data becomes PNG; a copied image file keeps its type and, without
// --name, its name. --from takes the image from a file instead, as when one is dropped on a note.
func runPaste(args []string) int {
	args, asJSON := takeBool(args, "--json")
	args, name, _, err := takeValue(args, "--name")
	if err != nil {
		return fail(fmt.Errorf("paste: %w", err))
	}
	args, from, _, err := takeValue(args, "--from")
	if err != nil {
		return fail(fmt.Errorf("paste: %w", err))
	}
	pos, err := positional("paste", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) != 1 {
		return fail(errors.New("paste: name the note the image goes with: nt paste <note> [--name words]"))
	}
	v, _, err := openVault()
	if err != nil {
		return fail(err)
	}
	note, err := ops.InRepo(v, pos[0])
	if err != nil {
		return fail(fmt.Errorf("paste: %w", err))
	}
	file, what := from, filepath.Base(from)+" is"
	var src, ext string
	if from == "" {
		tmp, err := os.CreateTemp("", "nt-paste-*.png")
		if err != nil {
			return fail(err)
		}
		tmp.Close()
		defer os.Remove(tmp.Name())
		im, err := readClipboard(tmp.Name())
		if err != nil {
			return fail(fmt.Errorf("paste: %w", err))
		}
		src, ext = tmp.Name(), ".png"
		file, what = im.File, "the clipboard holds "+filepath.Base(im.File)+","
	}
	if file != "" {
		src, ext = file, filepath.Ext(file)
		if !ops.ImageTypes[strings.ToLower(ext)] {
			return fail(fmt.Errorf("paste: %s not an image nt shows (png, jpg, gif, webp, svg)", what))
		}
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(file), ext)
		}
	}
	p, err := ops.Paste(v, note, name, src, ext)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return printJSON(p)
	}
	fmt.Println(p.Markdown)
	return 0
}
