package cli

import (
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
	"github.com/w4jnl/nt/internal/ops"
	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/vault"
)

// openVault finds and opens the notes repository with the machine config.
func openVault() (*vault.Vault, config.Machine, error) { return ops.Open() }

// takeBool, takeValue and positional read a command's arguments, as the apps' runner does.
func takeBool(args []string, name string) ([]string, bool) { return ops.Flag(args, name) }

func takeValue(args []string, name string) ([]string, string, bool, error) {
	return ops.Value(args, name)
}

func positional(cmd string, args []string) ([]string, error) { return ops.Positional(cmd, args) }

// printJSON prints v as every --json does (ops.JSON).
func printJSON(v any) int {
	b, err := ops.JSON(v)
	if err != nil {
		return fail(err)
	}
	_, _ = os.Stdout.Write(b)
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
func tilde(p string) string { return ops.Tilde(p) }
