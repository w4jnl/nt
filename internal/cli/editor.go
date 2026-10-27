package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/nvim"
	"github.com/w4jnl/nt/internal/scan"
	"github.com/w4jnl/nt/internal/vault"
)

// nt _editor repo|hold|kick|day|index: what an editor needs from nt (internal/nvim, the Mac app).
//
//	repo   prints the notes repository's folder.
//	hold   holds .git/nt-edit (shared) until its input ends: sync commits but does not merge.
//	kick   starts a background sync when sync.auto is on, and returns at once.
//	day    prints the day as nt counts it (day_starts), YYYY-MM-DD, for a todo's done: date.
//	index  prints the repository as JSON: its folders, every note's title and the notes that
//	       link to each, the day, the pins and where status.json is.
func runEditorHelper(args []string) int {
	if len(args) != 1 {
		return fail(errors.New("_editor: use repo, hold, kick, day or index"))
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
	default:
		return fail(fmt.Errorf("_editor: unknown %q: use repo, hold, kick, day or index", args[0]))
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
