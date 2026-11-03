package cli

import (
	"fmt"
	"time"

	"github.com/w4jnl/nt/internal/ops"
)

// runHistory prints when a note changed, newest first, each change with the machine whose nt
// committed it: what sync wrote into the commit's subject. A change not yet synced is not there.
func runHistory(args []string) int {
	args, asJSON := takeBool(args, "--json")
	pos, err := positional("history", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) != 1 {
		return fail(fmt.Errorf("history: name one note: nt history <note>"))
	}
	v, _, err := openVault()
	if err != nil {
		return fail(err)
	}
	changes, err := ops.History(v, nil, pos[0])
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return printJSON(changes)
	}
	if len(changes) == 0 {
		fmt.Println("no change synced yet")
		return 0
	}
	for _, c := range changes {
		at := c.At
		if t, err := time.Parse(time.RFC3339, c.At); err == nil {
			at = t.Local().Format("2006-01-02 15:04")
		}
		if c.By == "" {
			fmt.Println(at)
		} else {
			fmt.Printf("%s  %s\n", at, c.By)
		}
	}
	return 0
}
