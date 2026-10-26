package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/vault"
)

// The files every notes repository has (docs/format.md §11).
var (
	ignoreLines    = []string{".nt-tmp-*", "*~", "4913", ".*.sw?", ".DS_Store", ".obsidian/"}
	attributeLines = []string{"daily/** merge=union"}
)

// nt init [dir] [--clone url]: make dir a notes repository, or clone one. Either way the
// repository gets nt's files, and a machine without a config gets one pointing at dir.
func runInit(args []string) int {
	args, asJSON := takeBool(args, "--json")
	args, url, cloning, err := takeValue(args, "--clone")
	if err != nil {
		return fail(err)
	}
	pos, err := positional("init", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) > 1 {
		return fail(errors.New("init: name one folder"))
	}
	m, err := config.LoadMachine()
	if err != nil {
		return fail(err)
	}
	dir := m.Repo
	if len(pos) == 1 {
		dir = config.Expand(pos[0])
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return fail(err)
	}
	isRepo := isFile(filepath.Join(dir, ".nt", "config.toml")) && isDir(filepath.Join(dir, ".git"))
	switch {
	case cloning:
		if !emptyOrMissing(dir) {
			return fail(fmt.Errorf("init: %s exists and is not empty; clone into a new folder", tilde(dir)))
		}
		c := exec.Command("git", "clone", url, dir)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stderr, os.Stderr
		if err := c.Run(); err != nil {
			return fail(fmt.Errorf("init: git clone %s: %w", url, err))
		}
	case isRepo:
	case !emptyOrMissing(dir):
		return fail(fmt.Errorf("init: %s exists and is not empty, so it is not made a notes repository; "+
			"pick a new folder (on macOS ~/notes and ~/Notes are the same folder)", tilde(dir)))
	default:
		if out, err := exec.Command("git", "init", "--quiet", "-b", "main", dir).CombinedOutput(); err != nil {
			return fail(fmt.Errorf("init: git init: %s", strings.TrimSpace(string(out))))
		}
	}
	written, err := ensureRepoFiles(dir)
	if err != nil {
		return fail(err)
	}
	machine := ""
	if _, err := os.Stat(config.MachinePath()); errors.Is(err, os.ErrNotExist) {
		if err := vault.WriteFile(config.MachinePath(), fmt.Appendf(nil, config.MachineTemplate, tilde(dir))); err != nil {
			return fail(err)
		}
		machine = config.MachinePath()
	}
	remote := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Run() == nil
	if asJSON {
		return printJSON(map[string]any{"repo": dir, "written": written, "machine_config": machine, "remote": remote})
	}
	st := cliStyles(m)
	verb := "created"
	if isRepo {
		verb = "checked"
	} else if cloning {
		verb = "cloned"
	}
	lipgloss.Println(st.Text.Render(verb+" "+tilde(dir)) + st.Muted.Render(written2(written)))
	if machine != "" {
		lipgloss.Println(st.Muted.Render("wrote " + tilde(machine) + " with repo = " + tilde(dir)))
	} else if m.Repo != dir && os.Getenv("NT_REPO") == "" {
		lipgloss.Println(st.Muted.Render("set repo = \"" + tilde(dir) + "\" in " + tilde(config.MachinePath()) + " to make it the default"))
	}
	lipgloss.Println()
	if !remote {
		lipgloss.Println(st.Key.Render("next") + st.Verb.Render("  add a remote: git -C "+tilde(dir)+" remote add origin git@github.com:<you>/notes.git"))
	}
	lipgloss.Println(st.Key.Render("next") + st.Verb.Render("  sync in the background: nt install --sync"))
	lipgloss.Println(st.Key.Render("next") + st.Verb.Render("  check the setup: nt doctor"))
	return 0
}

func written2(w []string) string {
	if len(w) == 0 {
		return ""
	}
	return " · wrote " + strings.Join(w, ", ")
}

// ensureRepoFiles writes .nt/config.toml when missing and adds missing lines to .gitignore and
// .gitattributes, keeping what is there. It returns the files it changed.
func ensureRepoFiles(dir string) ([]string, error) {
	var written []string
	cfg := filepath.Join(dir, ".nt", "config.toml")
	if !isFile(cfg) {
		if err := vault.WriteFile(cfg, []byte(config.RepoTemplate)); err != nil {
			return nil, err
		}
		written = append(written, ".nt/config.toml")
	}
	for _, f := range []struct {
		name  string
		lines []string
	}{{".gitignore", ignoreLines}, {".gitattributes", attributeLines}} {
		changed, err := ensureLines(filepath.Join(dir, f.name), f.lines)
		if err != nil {
			return nil, err
		}
		if changed {
			written = append(written, f.name)
		}
	}
	return written, nil
}

func ensureLines(p string, want []string) (bool, error) {
	b, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	s := string(b)
	changed := false
	for _, l := range want {
		if !have[l] {
			if s != "" && !strings.HasSuffix(s, "\n") {
				s += "\n"
			}
			s += l + "\n"
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	return true, vault.WriteFile(p, []byte(s))
}

func emptyOrMissing(dir string) bool {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return err == nil && len(entries) == 0
}

func isFile(p string) bool { st, err := os.Stat(p); return err == nil && st.Mode().IsRegular() }
func isDir(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }
