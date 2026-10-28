package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/nvim"
	"github.com/w4jnl/nt/internal/ops"
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
		return printJSON(ops.IndexOf(v, config.StateDir()))
	case "save":
		return fail(errors.New("_editor save: name the note: nt _editor save <note> [--base <sha256>] [--keep]"))
	default:
		return fail(fmt.Errorf("_editor: unknown %q: use repo, hold, kick, day, index or save", args[0]))
	}
	return 0
}

// editorSave writes a note an editor changed: its text, read from in, through ops.Save, which
// says what is done when another writer changed it since. It prints {"saved", "sha", "text" when
// not saved, "copy" when one was made}.
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
	rel, err := ops.InRepo(v, pos[0])
	if err != nil {
		return fail(fmt.Errorf("_editor save: %w", err))
	}
	text, err := io.ReadAll(in)
	if err != nil {
		return fail(fmt.Errorf("_editor save: %w", err))
	}
	saved, err := ops.Save(v, rel, text, base, keep)
	if err != nil {
		return fail(err)
	}
	return printJSON(saved)
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
