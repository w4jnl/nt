package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/w4jnl/nt/internal/nvim"
)

// nt _editor repo|hold|kick: what an editor plugin needs from nt (internal/nvim).
//
//	repo  prints the notes repository's folder.
//	hold  holds .git/nt-edit (shared) until its input ends: sync commits but does not merge.
//	kick  starts a background sync when sync.auto is on, and returns at once.
func runEditorHelper(args []string) int {
	if len(args) != 1 {
		return fail(errors.New("_editor: use repo, hold or kick"))
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
	default:
		return fail(fmt.Errorf("_editor: unknown %q: use repo, hold or kick", args[0]))
	}
	return 0
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
