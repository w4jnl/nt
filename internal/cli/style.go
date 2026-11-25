package cli

import (
	"fmt"

	"github.com/w4jnl/nt/internal/style"
)

// runStyle prints the person's own rules for writing and drawing that hold for a folder or a
// note (docs/format.md §15): the root's style.md, then each folder's on the way down, the
// nearest last. An agent reads them before it writes. Nothing is printed when there are none.
func runStyle(args []string) int {
	args, asJSON := takeBool(args, "--json")
	pos, err := positional("style", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) > 1 {
		return fail(fmt.Errorf("style: name one folder or note: nt style [folder]"))
	}
	v, _, err := openVault()
	if err != nil {
		return fail(err)
	}
	at := ""
	if len(pos) == 1 {
		at = pos[0]
	}
	notes := style.For(v.Root, at)
	if asJSON {
		if notes == nil {
			notes = []style.Note{}
		}
		return printJSON(notes)
	}
	for i, n := range notes {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("<!-- %s -->\n%s\n", n.Rel, n.Text)
	}
	return 0
}
