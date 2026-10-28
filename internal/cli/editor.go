package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/gitsync"
	"github.com/w4jnl/nt/internal/nvim"
	"github.com/w4jnl/nt/internal/scan"
	"github.com/w4jnl/nt/internal/vault"
)

// nt _editor repo|hold|kick|day|index|save: what an editor needs from nt (internal/nvim, the
// Mac app).
//
//	repo   prints the notes repository's folder.
//	hold   holds .git/nt-edit (shared) until its input ends: sync commits but does not merge.
//	kick   starts a background sync when sync.auto is on, and returns at once.
//	day    prints the day as nt counts it (day_starts), YYYY-MM-DD, for a todo's done: date.
//	index  prints the repository as JSON: its folders, every note's title and the notes that
//	       link to each, the day, the pins and where status.json is.
//	save   <note> [--base <sha256>] [--keep] writes the note's text from its input (editorSave).
func runEditorHelper(args []string) int {
	if len(args) > 1 && args[0] == "save" {
		v, _, err := openVault()
		if err != nil {
			return fail(err)
		}
		return editorSave(v, args[1:], os.Stdin)
	}
	if len(args) != 1 {
		return fail(errors.New("_editor: use repo, hold, kick, day, index or save"))
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	switch args[0] {
	case "repo":
		fmt.Println(v.Root)
	case "hold":
		release, err := v.LockEdit()
		if err != nil {
			return fail(err)
		}
		_, _ = io.Copy(io.Discard, os.Stdin)
		release()
	case "kick":
		kickSync(m)
	case "day":
		fmt.Println(v.Day(v.Now()).Format("2006-01-02"))
	case "index":
		return printJSON(editorIndex(v))
	case "save":
		return fail(errors.New("_editor save: name the note: nt _editor save <note> [--base <sha256>] [--keep]"))
	default:
		return fail(fmt.Errorf("_editor: unknown %q: use repo, hold, kick, day, index or save", args[0]))
	}
	return 0
}

// editorIndex is the repository as an editor's sidebar and "Linked from" need it.
func editorIndex(v *vault.Vault) map[string]any {
	ix := scan.Build(v.Root)
	notes := ix.Notes
	if notes == nil {
		notes = []scan.Note{}
	}
	pins := v.Cfg.Pins
	if pins == nil {
		pins = []string{}
	}
	return map[string]any{
		"root":    v.Root,
		"state":   config.StateDir(),
		"day":     v.Day(v.Now()).Format("2006-01-02"),
		"pins":    pins,
		"folders": map[string]string{"daily": v.Cfg.Folders.Daily, "agents": v.Cfg.Folders.Agents, "weekly": v.Cfg.Folders.Weekly},
		"dirs":    dirsOf(v.Root),
		"notes":   notes,
		"back":    ix.Back,
	}
}

// editorSave writes a note an editor changed: its text, read from in, under the repository
// lock through a temp file (docs/format.md §10). With --base it writes only when the file on
// disk is still the one the editor read, whose SHA-256 is base. When another writer changed it
// since, nothing is written and the reply carries the file as it is now, for the editor to merge
// its changes into; --keep writes all the same, after saving that file beside the note as a
// conflict copy (§3). A note that is gone is written again. It prints
// {"saved", "sha", "text" when not saved, "copy" when one was made}.
func editorSave(v *vault.Vault, args []string, in io.Reader) int {
	args, keep := takeBool(args, "--keep")
	args, base, _, err := takeValue(args, "--base")
	if err != nil {
		return fail(fmt.Errorf("_editor save: %w", err))
	}
	pos, err := positional("_editor save", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) != 1 {
		return fail(errors.New("_editor save: name one note"))
	}
	rel, err := inRepo(v, pos[0])
	if err != nil {
		return fail(fmt.Errorf("_editor save: %w", err))
	}
	if !strings.HasSuffix(rel, ".md") || strings.HasPrefix(rel, ".") || strings.Contains(rel, "/.") {
		return fail(fmt.Errorf("_editor save: %s is not a note: notes are .md files outside folders starting with .", rel))
	}
	text, err := io.ReadAll(in)
	if err != nil {
		return fail(fmt.Errorf("_editor save: %w", err))
	}
	unlock, err := v.Lock()
	if err != nil {
		return fail(err)
	}
	defer unlock()
	path := v.Abs(rel)
	cur, err := os.ReadFile(path)
	gone := errors.Is(err, os.ErrNotExist)
	if err != nil && !gone {
		return fail(fmt.Errorf("_editor save: %w", err))
	}
	reply := map[string]any{"saved": true, "sha": sha256hex(text)}
	if !gone && base != "" && sha256hex(cur) != base && !bytes.Equal(cur, text) {
		if !keep {
			return printJSON(map[string]any{"saved": false, "sha": sha256hex(cur), "text": string(cur)})
		}
		cp := v.Unique(gitsync.ConflictName(rel, v.Host, v.Now()))
		if err := vault.WriteFile(v.Abs(cp), cur); err != nil {
			return fail(fmt.Errorf("_editor save: %w", err))
		}
		reply["copy"] = cp
	}
	if gone || !bytes.Equal(cur, text) {
		if err := vault.WriteFile(path, text); err != nil {
			return fail(fmt.Errorf("_editor save: %w", err))
		}
	}
	return printJSON(reply)
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// dirsOf lists every folder under root as a repository path, sorted, without hidden ones.
func dirsOf(root string) []string {
	dirs := []string{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(root, p)
		dirs = append(dirs, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(dirs)
	return dirs
}

// installNvim writes nt's nvim plugin, or removes it.
func installNvim(remove bool) error {
	dir := nvim.Dir()
	if remove {
		if err := nvim.Remove(dir); err != nil {
			return err
		}
		fmt.Println("removed the nvim plugin from " + tilde(dir))
		return nil
	}
	if err := nvim.Install(dir); err != nil {
		return err
	}
	fmt.Printf("installed the nvim plugin in %s; load it from your lazy.nvim specs with\n  { dir = %q, name = \"nt\", lazy = false }\n", tilde(dir), tilde(dir))
	return nil
}
