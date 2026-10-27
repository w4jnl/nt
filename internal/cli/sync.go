package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/gitsync"
	"github.com/w4jnl/nt/internal/theme"
)

// nt sync: commit, merge and push once. --background is the timer's form: no waiting for a run
// already going, progress into the sync log, an OS notification for new conflict copies.
func runSync(args []string) int {
	args, background := takeBool(args, "--background")
	args, asJSON := takeBool(args, "--json")
	if _, err := positional("sync", args); err != nil {
		return fail(err)
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	o := gitsync.Options{Wait: !background, Digest: m.Digest, Weekly: m.Weekly}
	if background {
		o.Log, o.Notify = stamped{os.Stdout}, gitsync.Notify
	} else if !asJSON {
		o.Log = stderrLog{cliStyles(m)}
	}
	st := gitsync.Run(v, o)
	if err := gitsync.WriteStatus(config.StateDir(), st); err != nil {
		fmt.Fprintln(os.Stderr, "nt: status:", err)
	}
	code := 0
	if st.State == "error" {
		code = 1
	}
	if asJSON {
		if c := printJSON(st); c != 0 {
			return c
		}
		return code
	}
	line := statusLine(st, cliPalette(m))
	if background {
		fmt.Fprintln(stamped{os.Stdout}, ansi.Strip(line)) // a log file, not a terminal
	} else {
		lipgloss.Println(line)
	}
	return code
}

type stderrLog struct{ st theme.Styles }

func (l stderrLog) Write(b []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		lipgloss.Fprintln(os.Stderr, l.st.Muted.Render(line))
	}
	return len(b), nil
}

// syncState maps a status to the theme's sync state, for its glyph and colour.
func syncState(s string) theme.State {
	switch s {
	case "synced", "local":
		return theme.Synced
	case "unpushed":
		return theme.Unpushed
	case "offline":
		return theme.Offline
	default:
		return theme.Conflict
	}
}

// statusLine is "● synced · 3 files committed · merged", the glyph and word first.
func statusLine(st gitsync.Status, pal theme.Palette) string {
	sty := pal.Styles()
	state := syncState(st.State)
	word := st.State
	if st.State == "unpushed" {
		word = fmt.Sprintf("unpushed %d", st.Ahead)
	}
	head := lipgloss.NewStyle().Foreground(pal.Color(state)).Render(theme.Glyph(state, 0) + " " + word)
	var parts []string
	if st.Committed > 0 {
		parts = append(parts, plural(st.Committed, "file")+" committed")
	}
	if st.Merged {
		parts = append(parts, "merged")
	}
	if n := len(st.NewConflicts); n > 0 {
		parts = append(parts, plural(n, "new conflict copy")+": "+strings.Join(st.NewConflicts, ", "))
	} else if n := len(st.Conflicts); n > 0 {
		parts = append(parts, plural(n, "conflict copy")+": "+strings.Join(st.Conflicts, ", "))
	}
	if n := len(st.Skipped); n > 0 {
		parts = append(parts, plural(n, "file")+" over 20 MB not committed: "+strings.Join(st.Skipped, ", "))
	}
	if st.Message != "" && st.State != "conflict" {
		parts = append(parts, st.Message)
	}
	if len(parts) == 0 {
		return head
	}
	return head + sty.Muted.Render(" · "+strings.Join(parts, " · "))
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	if strings.HasSuffix(what, "copy") {
		return fmt.Sprintf("%d %s", n, strings.TrimSuffix(what, "copy")+"copies")
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// nt install --sync|--nvim|--hooks|--skill: the background sync timer, the nvim plugin, the
// agent hooks and the agent skill; nt uninstall takes the same flags and removes them.
func runInstall(args []string, remove bool) int {
	args, sync := takeBool(args, "--sync")
	args, withNvim := takeBool(args, "--nvim")
	args, hooks := takeBool(args, "--hooks")
	args, skill := takeBool(args, "--skill")
	if _, err := positional("install", args); err != nil {
		return fail(err)
	}
	if !sync && !withNvim && !hooks && !skill {
		return fail(errors.New("install: use --sync, --nvim, --hooks or --skill (nt uninstall takes the same)"))
	}
	if withNvim {
		if err := installNvim(remove); err != nil {
			return fail(fmt.Errorf("install: nvim plugin: %w", err))
		}
	}
	if hooks {
		if err := installHooks(remove); err != nil {
			return fail(fmt.Errorf("install: hooks: %w", err))
		}
	}
	if skill {
		if err := installSkill(remove); err != nil {
			return fail(fmt.Errorf("install: skill: %w", err))
		}
	}
	if !sync {
		return 0
	}
	m, err := config.LoadMachine()
	if err != nil {
		return fail(err)
	}
	t, err := timer(m)
	if err != nil {
		return fail(err)
	}
	if remove {
		if err := t.Remove(); err != nil {
			return fail(err)
		}
		lipgloss.Println("removed " + gitsync.Label + ": nt sync runs only when you ask")
		return 0
	}
	if !m.Sync.Auto {
		return fail(fmt.Errorf("install: sync.auto is false in %s: set it to true first", tilde(config.MachinePath())))
	}
	if err := t.Install(); err != nil {
		return fail(err)
	}
	lipgloss.Println(fmt.Sprintf("installed %s: %s sync every %s (%s)", gitsync.Label, tilde(t.Bin), short(t.Interval), tilde(t.Path())))
	return 0
}

func timer(m config.Machine) (gitsync.Timer, error) {
	iv, err := m.SyncInterval()
	if err != nil {
		return gitsync.Timer{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return gitsync.Timer{}, err
	}
	bin, err := stableBinary()
	if err != nil {
		return gitsync.Timer{}, err
	}
	return gitsync.Timer{Bin: bin, Interval: iv, StateDir: config.StateDir(), Home: home}, nil
}

// stableBinary is the path the timer runs: the nt on PATH when it is this binary (a Homebrew
// symlink survives upgrades, the Cellar path does not), else this binary itself.
func stableBinary() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	selfReal, _ := filepath.EvalSymlinks(self)
	if onPath, err := exec.LookPath("nt"); err == nil {
		if abs, err := filepath.Abs(onPath); err == nil {
			if real, _ := filepath.EvalSymlinks(abs); real == selfReal {
				return abs, nil
			}
		}
	}
	return self, nil
}

// short writes a duration without empty units: 2m, 90s, 1h30m.
func short(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}
