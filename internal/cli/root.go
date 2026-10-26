// Package cli dispatches nt subcommands.
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/tui"
)

// Version is stamped at build time: -ldflags "-X github.com/w4jnl/nt/internal/cli.Version=v0.1.0".
var Version = "dev"

// Tagline is nt's one-line description, as in the brand lockup.
const Tagline = "plain markdown notes, synced by git"

const usageText = `nt — plain markdown notes, synced by git

usage: nt <command>

  view        <file> [--theme auto|dark|light] [--images auto|on|off]: show one note rendered,
              e to edit it in $EDITOR; images are drawn in Ghostty and kitty, also inside tmux
  theme       --glamour [dir]: write nt's preview style for glow (glamour-dark.json and
              glamour-light.json, in ~/.config/nt by default); use it with glow -s <file>
  version     print the version
`

func usage() { fmt.Fprint(os.Stderr, usageText) }

// Main runs the CLI and returns the exit code.
func Main(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "view":
		return runView(args[1:])
	case "theme":
		return runTheme(args[1:])
	case "version", "--version", "-V":
		if stdoutIsTerminal() { // the lockup for people; "nt X.Y.Z" for scripts and the brew test
			fmt.Println(Lockup(Version))
		} else {
			fmt.Println("nt", strings.TrimPrefix(Version, "v"))
		}
		return 0
	case "help", "-h", "--help":
		usage()
		return 0
	}
	fmt.Fprintf(os.Stderr, "nt: unknown command %q\n\n", args[0])
	usage()
	return 2
}

// Lockup is the one-line ASCII brand lockup from assets/brand/README.md: the mark as "[≡▁]"
// (bracket, two text lines, cursor), then the name, version and tagline.
func Lockup(version string) string {
	return "[≡▁] nt " + strings.TrimPrefix(version, "v") + " · " + Tagline
}

func runView(args []string) int {
	o := tui.ViewOptions{Mode: "auto", Images: "auto"}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--theme" && i+1 < len(args):
			i++
			o.Mode = args[i]
		case strings.HasPrefix(a, "--theme="):
			o.Mode = strings.TrimPrefix(a, "--theme=")
		case a == "--images" && i+1 < len(args):
			i++
			o.Images = args[i]
		case strings.HasPrefix(a, "--images="):
			o.Images = strings.TrimPrefix(a, "--images=")
		case strings.HasPrefix(a, "-"):
			return fail(fmt.Errorf("view: unknown flag %s", a))
		default:
			o.Path = a
		}
	}
	if o.Mode != "auto" && o.Mode != "dark" && o.Mode != "light" {
		return fail(fmt.Errorf("view: --theme is auto, dark or light, not %q", o.Mode))
	}
	if o.Images != "auto" && o.Images != "on" && o.Images != "off" {
		return fail(fmt.Errorf("view: --images is auto, on or off, not %q", o.Images))
	}
	if o.Path == "" {
		return fail(errors.New("view: name a note, e.g. nt view daily/2026/2026-10-26.md"))
	}
	if _, err := os.Stat(o.Path); err != nil {
		return fail(fmt.Errorf("view: %w", err))
	}
	return report(tui.RunView(o))
}

func runTheme(args []string) int {
	if len(args) == 0 || args[0] != "--glamour" {
		return fail(errors.New("theme: use nt theme --glamour [dir]"))
	}
	dir := ""
	if len(args) > 1 {
		dir = args[1]
	} else {
		cfg, err := os.UserConfigDir()
		if home, herr := os.UserHomeDir(); herr == nil {
			cfg, err = filepath.Join(home, ".config"), nil // ~/.config on macOS too, as flok
		}
		if err != nil {
			return fail(err)
		}
		dir = filepath.Join(cfg, "nt")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(err)
	}
	for _, v := range []struct {
		name string
		dark bool
	}{{"glamour-dark.json", true}, {"glamour-light.json", false}} {
		b, err := render.JSON(v.dark)
		if err != nil {
			return fail(err)
		}
		p := filepath.Join(dir, v.name)
		if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
			return fail(err)
		}
		fmt.Println("wrote", p)
	}
	fmt.Printf("use it with: glow -s %s <file>\n", filepath.Join(dir, "glamour-dark.json"))
	return 0
}

// stdoutIsTerminal reports whether a person, not a pipe, is reading stdout.
func stdoutIsTerminal() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "nt:", err)
	return 2
}

func report(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "nt:", err)
		return 1
	}
	return 0
}
