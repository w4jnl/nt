package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/mcp"
)

// nt mcp [--read-only]: nt as a Model Context Protocol server on stdin and stdout, for the
// Claude app (docs/design.md §4, The Claude app). Only protocol messages go to stdout; what goes
// wrong goes to stderr, which the app keeps in its log.
func runMCP(args []string) int {
	args, readOnly := takeBool(args, "--read-only")
	if _, err := positional("mcp", args); err != nil {
		return fail(err)
	}
	v, _, err := openVault()
	if err != nil {
		return fail(fmt.Errorf("mcp: %w", err))
	}
	// paths are the repository's, whatever folder the app started nt in
	if err := os.Chdir(v.Root); err != nil {
		return fail(fmt.Errorf("mcp: %w", err))
	}
	s := &mcp.Server{Version: Version, ReadOnly: readOnly, Log: os.Stderr, Kick: func() {
		if m, err := config.LoadMachine(); err == nil {
			kickSync(m)
		}
	}}
	if err := s.Serve(os.Stdin, os.Stdout); err != nil {
		return fail(fmt.Errorf("mcp: %w", err))
	}
	return 0
}

// takeMCP takes --mcp and --mcp=<clients> out of args: whether it was there, and the clients
// it names ("" when none).
func takeMCP(args []string) (rest []string, on bool, list string) {
	for _, a := range args {
		switch v, ok := strings.CutPrefix(a, "--mcp="); {
		case a == "--mcp":
			on = true
		case ok:
			on, list = true, v
		default:
			rest = append(rest, a)
		}
	}
	return rest, on, list
}

// installMCP names this nt as the nt server of the clients list names, or takes it out. "all"
// is every client on this machine: the others are named and left.
func installMCP(list string, remove, readOnly bool) error {
	clients, err := mcp.Pick(list)
	if err != nil {
		return err
	}
	all := strings.TrimSpace(list) == "all"
	if all && !remove {
		var here []mcp.Client
		var away []string
		for _, c := range clients {
			if mcp.Present(c) {
				here = append(here, c)
			} else {
				away = append(away, c.Name)
			}
		}
		if len(here) == 0 {
			return fmt.Errorf("none of %s is on this machine", joinAnd(away))
		}
		if len(away) > 0 {
			defer fmt.Printf("not on this machine, left: %s\n", joinAnd(away))
		}
		clients = here
	}
	if remove {
		for _, c := range clients {
			had, err := mcp.Remove(c)
			if err != nil {
				return err
			}
			if had {
				fmt.Printf("removed nt from %s's servers (%s); %s\n", c.Title, tilde(c.Path), c.Apply)
			} else if !all {
				fmt.Printf("%s has no nt server (%s)\n", c.Title, tilde(c.Path))
			}
		}
		return nil
	}
	bin, err := stableBinary()
	if err != nil {
		return err
	}
	how := "reads and writes the notes"
	if readOnly {
		how = "only reads the notes"
	}
	for _, c := range clients {
		if err := mcp.Install(c, bin, readOnly); err != nil {
			if list == "" {
				return fmt.Errorf("%w; nt install --mcp=all takes the tools that are here (nt knows %s)", err, known())
			}
			return err
		}
		fmt.Printf("%s runs %s mcp, which %s (%s); %s\n", c.Title, tilde(bin), how, tilde(c.Path), c.Apply)
	}
	return nil
}

// known lists the clients' names.
func known() string {
	var names []string
	for _, c := range mcp.Clients() {
		names = append(names, c.Name)
	}
	return strings.Join(names, ", ")
}

// mcpCheck says which clients run nt, with which binary, and what they are kept from.
func mcpCheck(m config.Machine, add func(name, state, format string, a ...any)) {
	for _, c := range mcp.Clients() {
		const line = "mcp" // one line a client; the detail names it
		e, ok, err := mcp.Installed(c)
		switch {
		case err != nil:
			add(line, "warn", "%v", err)
		case !ok: // not asked for
		case !runs(e.Command):
			add(line, "fail", "%s runs %s, which is not there, so it has no notes: nt install --mcp=%s", c.Title, tilde(e.Command), c.Name)
		default:
			what := "reads and writes the notes"
			if e.ReadOnly() {
				what = "only reads the notes"
			}
			if h := mcp.Hidden(m); len(h) > 0 {
				what += fmt.Sprintf(", kept from %s (mcp.hide)", joinAnd(h))
			}
			add(line, "ok", "%s runs %s mcp, which %s", c.Title, tilde(e.Command), what)
		}
	}
}

func joinAnd(s []string) string {
	switch len(s) {
	case 0:
		return ""
	case 1:
		return s[0]
	}
	out := ""
	for i, x := range s[:len(s)-1] {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out + " and " + s[len(s)-1]
}
