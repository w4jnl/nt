package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/w4jnl/nt/internal/clip"
	"github.com/w4jnl/nt/internal/gitsync"
	"github.com/w4jnl/nt/internal/vault"
)

// readClipboard is the clipboard, a variable so tests can stand in for it.
var readClipboard = clip.Read

// pasteTypes are the image files nt paste takes as they are: what GitHub and browsers show.
var pasteTypes = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true}

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
	note, err := inRepo(v, pos[0])
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
		if !pasteTypes[strings.ToLower(ext)] {
			return fail(fmt.Errorf("paste: %s not an image nt shows (png, jpg, gif, webp, svg)", what))
		}
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(file), ext)
		}
	}
	st, err := os.Stat(src)
	if err != nil {
		return fail(fmt.Errorf("paste: %w", err))
	}
	if st.Size() > gitsync.MaxFile {
		return fail(fmt.Errorf("paste: the image is %d MB, and sync leaves files over 20 MB out: make it smaller first", st.Size()>>20))
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fail(fmt.Errorf("paste: %w", err))
	}
	rel, err := v.SaveAsset(note, name, ext, data, v.Now())
	if err != nil {
		return fail(fmt.Errorf("paste: %w", err))
	}
	alt := strings.TrimSpace(name)
	if alt == "" {
		alt = "image"
	}
	line := "![" + strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(alt) + "](" + vault.RelLink(note, rel) + ")"
	if asJSON {
		return printJSON(map[string]any{"path": v.Abs(rel), "rel": rel, "markdown": line})
	}
	fmt.Println(line)
	return 0
}

// inRepo is the repository path of the note at p: p from the working directory, else p as a
// repository path. The note need not exist yet, its folder must.
func inRepo(v *vault.Vault, p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if !isFile(abs) && (isFile(v.Abs(p)) || !isDir(filepath.Dir(abs)) && isDir(filepath.Dir(v.Abs(p)))) {
		abs = v.Abs(p)
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("%s: no such folder", filepath.Dir(p))
	}
	root, err := filepath.EvalSymlinks(v.Root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, filepath.Join(dir, filepath.Base(abs)))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is not in the notes repository %s", p, tilde(v.Root))
	}
	return filepath.ToSlash(rel), nil
}
