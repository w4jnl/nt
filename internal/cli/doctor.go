package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/editor"
	"github.com/w4jnl/nt/internal/gitsync"
	"github.com/w4jnl/nt/internal/termimg"
	"github.com/w4jnl/nt/internal/theme"
)

// check is one line of nt doctor: ok, warn (works, but something is missing) or fail (nt will
// not work as it should).
type check struct {
	Check  string `json:"check"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// nt doctor: check the repository, git, the sync timer, the tools nt uses and images.
func runDoctor(args []string) int {
	args, asJSON := takeBool(args, "--json")
	if _, err := positional("doctor", args); err != nil {
		return fail(err)
	}
	m, merr := config.LoadMachine()
	var checks []check
	add := func(name, state, format string, a ...any) {
		checks = append(checks, check{name, state, fmt.Sprintf(format, a...)})
	}
	if merr != nil {
		add("config", "fail", "%v", merr)
	} else {
		add("config", "ok", "%s · host %s", tilde(config.MachinePath()), m.Host)
	}
	v, _, verr := openVault()
	if verr != nil {
		add("repository", "fail", "%v", verr)
	} else {
		add("repository", "ok", "%s (format %d)", tilde(v.Root), v.Cfg.Format)
		gitChecks(v.Root, add)
	}
	syncChecks(m, add)
	toolChecks(add)
	switch t, why := termimg.Detect(m.InlineImages); {
	case t != nil:
		add("images", "ok", "drawn in the preview")
	case why != "":
		add("images", "warn", "%s", why)
	case m.InlineImages == "off":
		add("images", "ok", "inline_images is off: images show their alt text")
	default:
		add("images", "ok", "this terminal does not draw images: notes show their alt text")
	}
	failed := false
	for _, c := range checks {
		failed = failed || c.State == "fail"
	}
	if asJSON {
		root := ""
		if v != nil {
			root = v.Root
		}
		code := printJSON(map[string]any{"version": Version, "repo": root, "checks": checks})
		if code == 0 && failed {
			return 1
		}
		return code
	}
	pal := cliPalette(m)
	st := pal.Styles()
	lipgloss.Println(Lockup(Version))
	lipgloss.Println()
	for _, c := range checks {
		s := map[string]theme.State{"ok": theme.Done, "warn": theme.Blocked, "fail": theme.Failed}[c.State]
		mark := lipgloss.NewStyle().Foreground(pal.Color(s)).Render(fmt.Sprintf("%s %-4s", theme.Glyph(s, 0), c.State))
		lipgloss.Println(mark + "  " + st.Text.Render(fmt.Sprintf("%-11s", c.Check)) + st.Muted.Render(c.Detail))
	}
	if failed {
		return 1
	}
	return 0
}

func gitChecks(root string, add func(name, state, format string, a ...any)) {
	g := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
		return strings.TrimSpace(string(out)), err
	}
	branch, err := g("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		add("branch", "fail", "HEAD is detached: git -C %s switch main", tilde(root))
	} else {
		add("branch", "ok", "%s", branch)
	}
	switch url, err := g("remote", "get-url", "origin"); {
	case err != nil:
		add("remote", "warn", "no remote: git -C %s remote add origin git@github.com:<you>/notes.git", tilde(root))
	case strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://"):
		add("remote", "warn", "%s: use the SSH form (git@github.com:…) so background sync never asks for a password", url)
	default:
		add("remote", "ok", "%s", url)
	}
	name, _ := g("config", "user.name")
	email, _ := g("config", "user.email")
	if name == "" || email == "" {
		add("identity", "fail", "git has no user.name or user.email, so sync cannot commit: git config --global user.name …")
	} else {
		add("identity", "ok", "%s <%s>", name, email)
	}
	attrs, _ := os.ReadFile(filepath.Join(root, ".gitattributes"))
	ignore, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if !strings.Contains(string(attrs), "merge=union") || !strings.Contains(string(ignore), ".nt-tmp-*") {
		add("files", "warn", ".gitattributes or .gitignore lacks nt's lines: nt init %s adds them", tilde(root))
	} else {
		add("files", "ok", ".gitattributes and .gitignore")
	}
}

func syncChecks(m config.Machine, add func(name, state, format string, a ...any)) {
	if !m.Sync.Auto {
		add("timer", "ok", "sync.auto is off: nt sync runs when you ask")
	} else if t, err := timer(m); err != nil {
		add("timer", "fail", "%v", err)
	} else if !t.Loaded() {
		add("timer", "warn", "not running: nt install --sync")
	} else {
		add("timer", "ok", "%s every %s", gitsync.Label, short(t.Interval))
	}
	st, ok := gitsync.ReadStatus(config.StateDir())
	if !ok {
		add("last sync", "warn", "never: nt sync")
		return
	}
	age := time.Since(st.At).Round(time.Second)
	detail := fmt.Sprintf("%s %s ago", st.State, age)
	if st.Message != "" {
		detail += ": " + st.Message
	}
	switch st.State {
	case "synced", "local", "unpushed", "busy":
		add("last sync", "ok", "%s", detail)
	case "offline", "conflict":
		add("last sync", "warn", "%s", detail)
	default:
		add("last sync", "fail", "%s", detail)
	}
}

func toolChecks(add func(name, state, format string, a ...any)) {
	if p, err := exec.LookPath("git"); err != nil {
		add("git", "fail", "git is not installed")
	} else {
		out, _ := exec.Command(p, "--version").Output()
		add("git", "ok", "%s", strings.TrimSpace(strings.TrimPrefix(string(out), "git version ")))
	}
	ed := editor.Argv("x", 0)[0]
	if _, err := exec.LookPath(ed); err != nil {
		add("editor", "fail", "%s is not on PATH: set $EDITOR", ed)
	} else {
		add("editor", "ok", "%s", ed)
	}
	for _, t := range []struct{ bin, why, pkg string }{
		{"rg", "nt s and nt f need it", "ripgrep"},
		{"fzf", "nt f needs it", "fzf"},
	} {
		if p, err := exec.LookPath(t.bin); err != nil {
			add(t.bin, "warn", "not installed (%s): brew install %s", t.why, t.pkg)
		} else {
			add(t.bin, "ok", "%s", tilde(p))
		}
	}
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("chafa"); err != nil {
			add("chafa", "warn", "not installed: images cannot open over ssh (apt install chafa)")
		}
	}
}
