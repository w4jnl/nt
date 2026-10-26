package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/vault"
)

// openVault finds and opens the notes repository with the machine config.
func openVault() (*vault.Vault, config.Machine, error) {
	m, err := config.LoadMachine()
	if err != nil {
		return nil, m, err
	}
	root := vault.Find(m)
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil, m, fmt.Errorf("no notes repository at %s: run nt init %s, or set repo in %s", tilde(root), tilde(root), tilde(config.MachinePath()))
	}
	v, err := vault.Open(root, m.Host)
	return v, m, err
}

// takeBool removes the flag name from args and reports whether it was there.
func takeBool(args []string, name string) ([]string, bool) {
	out, found := args[:0:0], false
	for _, a := range args {
		if a == name {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

// takeValue removes "name value" or "name=value" from args.
func takeValue(args []string, name string) ([]string, string, bool, error) {
	var out []string
	val, found := "", false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == name:
			if i+1 >= len(args) {
				return nil, "", false, fmt.Errorf("%s needs a value", name)
			}
			i++
			val, found = args[i], true
		case strings.HasPrefix(a, name+"="):
			val, found = strings.TrimPrefix(a, name+"="), true
		default:
			out = append(out, a)
		}
	}
	return out, val, found, nil
}

// positional checks no unknown flags are left; "--" ends the flags.
func positional(cmd string, args []string) ([]string, error) {
	var out []string
	for i, a := range args {
		if a == "--" {
			return append(out, args[i+1:]...), nil
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			return nil, fmt.Errorf("%s: unknown flag %s", cmd, a)
		}
		out = append(out, a)
	}
	return out, nil
}

func printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fail(err)
	}
	return 0
}

// cliStyles are the theme's styles for command output.
func cliStyles(m config.Machine) theme.Styles { return cliPalette(m).Styles() }

// cliPalette is the theme for command output. A command cannot ask the terminal for its
// background without taking it over, so auto means dark unless COLORFGBG says light.
func cliPalette(m config.Machine) theme.Palette {
	dark := true
	switch m.Theme.Mode {
	case "light":
		dark = false
	case "auto":
		if f := os.Getenv("COLORFGBG"); f != "" {
			bg := f[strings.LastIndexByte(f, ';')+1:]
			dark = !(bg == "7" || bg == "15")
		}
	}
	return theme.New(dark)
}

func stdinIsTerminal() bool { return isTerminal(os.Stdin) }

// isTerminal reports a terminal, not just any character device: /dev/null is one too.
func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// edit opens rel in the editor at line, marking the repository as being edited so sync does not
// merge underneath it, then starts a background sync.
func edit(v *vault.Vault, m config.Machine, path string, line int) int {
	release, err := v.LockEdit()
	if err != nil {
		return fail(err)
	}
	err = runEditor(path, line)
	release()
	kickSync(m)
	return report(err)
}

// kickSync starts `nt sync --background` detached when automatic sync is on: the editor closed,
// so the change goes out now instead of at the next timer tick.
func kickSync(m config.Machine) {
	if !m.Sync.Auto {
		return
	}
	bin, err := os.Executable()
	if err != nil {
		return
	}
	logf, err := openLog()
	if err != nil {
		return
	}
	defer logf.Close()
	c := exec.Command(bin, "sync", "--background")
	c.Stdout, c.Stderr = logf, logf
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if c.Start() == nil {
		_ = c.Process.Release()
	}
}

// openLog opens the sync log for appending, starting a new one past 1 MB.
func openLog() (*os.File, error) {
	dir := config.StateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "sync.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 1<<20 {
		_ = os.Rename(p, p+".1")
	}
	return os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

// stamped prefixes each line written to w with the time, for the sync log.
type stamped struct{ w io.Writer }

func (s stamped) Write(b []byte) (int, error) {
	for _, l := range strings.SplitAfter(string(b), "\n") {
		if l != "" {
			fmt.Fprintf(s.w, "%s %s", time.Now().Format("2006-01-02 15:04:05"), l)
		}
	}
	return len(b), nil
}

// tilde shortens a path under the home directory to ~/….
func tilde(p string) string {
	if h, err := os.UserHomeDir(); err == nil {
		if rest, ok := strings.CutPrefix(p, h+"/"); ok {
			return "~/" + rest
		}
		if p == h {
			return "~"
		}
	}
	return p
}
