package cli

import (
	"fmt"
	"os"

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

// installMCP names this nt as the Claude app's nt server, or takes it out.
func installMCP(remove, readOnly bool) error {
	p := mcp.ConfigPath()
	if remove {
		if err := mcp.Remove(p); err != nil {
			return err
		}
		fmt.Println("removed nt from the Claude app's servers (" + tilde(p) + "); quit and open the app to apply")
		return nil
	}
	bin, err := stableBinary()
	if err != nil {
		return err
	}
	if err := mcp.Install(p, bin, readOnly); err != nil {
		return err
	}
	how := "reads and writes the notes"
	if readOnly {
		how = "only reads the notes"
	}
	fmt.Printf("the Claude app runs %s mcp, which %s (%s); quit and open the app to apply\n", tilde(bin), how, tilde(p))
	return nil
}

// mcpCheck says whether the Claude app runs nt, with which binary, and what it is kept from.
func mcpCheck(m config.Machine, add func(name, state, format string, a ...any)) {
	e, ok, err := mcp.Installed(mcp.ConfigPath())
	switch {
	case err != nil:
		add("claude app", "warn", "%v", err)
	case !ok:
		return // not asked for
	case !runs(e.Command):
		add("claude app", "fail", "it runs %s, which is not there, so the app has no notes: nt install --mcp", tilde(e.Command))
	default:
		what := "reads and writes the notes"
		if e.ReadOnly() {
			what = "only reads the notes"
		}
		if h := mcp.Hidden(m); len(h) > 0 {
			what += fmt.Sprintf(", kept from %s (mcp.hide)", joinAnd(h))
		}
		add("claude app", "ok", "%s mcp %s", tilde(e.Command), what)
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
