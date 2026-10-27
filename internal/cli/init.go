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
	attributeLines = []string{"daily/** merge=union", "todo.md merge=union"}
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

// agentsGuide is the repository's AGENTS.md: how an agent working in the notes repository finds
// its way and writes to it. It is the person's file once written.
const agentsGuide = `# Notes

Plain markdown notes kept by nt (github.com/w4jnl/nt) and synced by git. This file is for an
agent working in this repository.

## Layout

- ` + "`daily/YYYY/YYYY-MM-DD.md`" + `: one journal per day. Add to it with ` + "`nt a \"text\"`" + `.
- ` + "`agents/YYYY/YYYY-MM-DD/`" + `: agent session reports, one file per session, and the day's
  ` + "`README.md`" + `, which is generated from them: do not edit it.
- ` + "`weekly/GGGG/GGGG-Www.md`" + `: the week's digest, compiled from the notes: read it, do not edit it.
- ` + "`todo.md`" + `: the todo list; a folder can have its own. A todo is a task line in any note,
  ` + "`- [ ] text due:YYYY-MM-DD`" + `; nt marks one done as ` + "`- [x] text done:YYYY-MM-DD`" + `. Add one with
  ` + "`nt todo add \"text\" --due fri`" + ` (` + "`--in folder`" + ` for a folder's list).
- Every other folder holds topic notes named ` + "`YYYY-MM-DD-<slug>.md`" + `, with attachments in an
  ` + "`assets/`" + ` folder beside them.

## Rules

- Do not run git here. ` + "`nt sync`" + ` is the only git writer; it runs on a timer.
- Write with nt where it can: ` + "`nt a`, `nt n \"Title\" folder`, `nt paste`, `nt agent write`" + `. It takes
  the repository lock and writes atomically.
- Move and rename with ` + "`nt mv <note|folder> <to>` and `nt rename <note> \"New title\"`" + `, never ` + "`mv`" + `:
  nt fixes the links to and in what moves.
- GitHub-flavoured markdown only: ` + "`[Title](relative/path.md)`" + ` links, ` + "`![alt](assets/x.png)`" + `
  images, flat YAML frontmatter (` + "`title`, `date`, `tags`" + `). No wikilinks, embeds or callouts.
- Your session's report: ` + "`nt agent write --status done --summary \"one line\"`" + ` with ` + "`## Done`" + `,
  ` + "`## Decisions` and `## Open`" + ` on stdin. No secrets and no transcripts: this repository is
  pushed to GitHub.
- Find things with ` + "`nt s \"words\" --json` and `nt ls folder --json`" + `; the open todos with
  ` + "`nt todo --json`" + `.

The full format is docs/format.md in github.com/w4jnl/nt.
`

// ensureRepoFiles writes .nt/config.toml when missing and adds missing lines to .gitignore and
// .gitattributes, keeping what is there. It returns the files it changed.
func ensureRepoFiles(dir string) ([]string, error) {
	var written []string
	for _, f := range []struct{ name, text string }{{".nt/config.toml", config.RepoTemplate}, {"AGENTS.md", agentsGuide}} {
		if p := filepath.Join(dir, filepath.FromSlash(f.name)); !isFile(p) {
			if err := vault.WriteFile(p, []byte(f.text)); err != nil {
				return nil, err
			}
			written = append(written, f.name)
		}
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
