package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/vault"
)

// nt config [--json]: every setting, its value, its file and what it does; nt config get <key>,
// nt config set <key> <value…>, nt config edit [--repo] (docs/format.md §9).
func runConfig(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "get":
			return runConfigGet(args[1:])
		case "set":
			return runConfigSet(args[1:])
		case "edit":
			return runConfigEdit(args[1:])
		}
	}
	args, asJSON := takeBool(args, "--json")
	if pos, err := positional("config", args); err != nil {
		return fail(err)
	} else if len(pos) > 0 {
		return fail(fmt.Errorf("config: %q is not get, set or edit", pos[0]))
	}
	m, r, repoPath, err := loadBoth()
	if err != nil {
		return fail(err)
	}
	if asJSON {
		var out []map[string]any
		for _, s := range config.Settings {
			p := config.MachinePath()
			if s.File == "repo" {
				p = repoPath
			}
			out = append(out, map[string]any{"key": s.Key, "value": s.Value(m, r), "file": s.File, "path": p,
				"kind": s.Kind, "choices": s.Choices(), "help": s.Help})
		}
		return printJSON(map[string]any{"settings": out})
	}
	st := cliStyles(m)
	w := 0
	for _, s := range config.Settings {
		w = max(w, len(s.Key))
	}
	file := ""
	for _, s := range config.Settings {
		if s.File != file {
			if file != "" {
				lipgloss.Println()
			}
			file = s.File
			if file == "machine" {
				lipgloss.Println(st.Text.Render("this machine") + st.Muted.Render("  "+tilde(config.MachinePath())))
			} else {
				lipgloss.Println(st.Text.Render("the repository") + st.Muted.Render("  "+tilde(repoPath)+", shared by every machine"))
			}
		}
		v := s.Value(m, r)
		if v == "" {
			v = st.Faint.Render("none")
		} else {
			v = st.Key.Render(v)
		}
		lipgloss.Println("  " + st.Text.Render(fmt.Sprintf("%-*s", w, s.Key)) + "  " + v)
		lipgloss.Println("  " + strings.Repeat(" ", w) + "  " + st.Faint.Render(s.Help))
	}
	return 0
}

// loadBoth reads this machine's config and the repository's, and where the repository's is.
func loadBoth() (config.Machine, config.Repo, string, error) {
	m, err := config.LoadMachine()
	if err != nil {
		return m, config.Repo{}, "", err
	}
	root := vault.Find(m)
	r, err := config.LoadRepo(root)
	return m, r, filepath.Join(root, ".nt", "config.toml"), err
}

func runConfigGet(args []string) int {
	pos, err := positional("config get", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) != 1 {
		return fail(errors.New("config get: name one setting; nt config lists them"))
	}
	s, ok := config.FindSetting(pos[0])
	if !ok {
		return fail(fmt.Errorf("config get: %q is not a setting; nt config lists them", pos[0]))
	}
	m, r, _, err := loadBoth()
	if err != nil {
		return fail(err)
	}
	fmt.Println(s.Value(m, r))
	return 0
}

// nt config set <key> <value…>: one setting, in the file it lives in. The words of the value
// are one value: nt config set pins tech work.
func runConfigSet(args []string) int {
	pos, err := positional("config set", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) < 1 {
		return fail(errors.New("config set: use nt config set <key> <value>; nt config lists the keys"))
	}
	s, ok := config.FindSetting(pos[0])
	if !ok {
		return fail(fmt.Errorf("config set: %q is not a setting; nt config lists them", pos[0]))
	}
	raw := strings.Join(pos[1:], " ")
	where, err := SetSetting(s, raw)
	if err != nil {
		return fail(fmt.Errorf("config set: %w", err))
	}
	m, r, _, _ := loadBoth()
	st := cliStyles(m)
	v := s.Value(m, r)
	if v == "" {
		v = "none"
	}
	lipgloss.Println(st.Text.Render(s.Key) + st.Muted.Render(" = ") + st.Key.Render(v) + st.Faint.Render("  "+where))
	if hint := SettingHint(s); hint != "" {
		lipgloss.Println(st.Muted.Render(hint))
	}
	return 0
}

// SetSetting writes one setting typed as raw: this machine's in its config file, the
// repository's in .nt/config.toml under the repository lock, for sync to share. It says where.
func SetSetting(s config.Setting, raw string) (string, error) {
	if s.File == "machine" {
		if _, err := config.SetMachine(s.Key, raw); err != nil {
			return "", err
		}
		return "this machine: " + tilde(config.MachinePath()), nil
	}
	v, _, err := openVault()
	if err != nil {
		return "", err
	}
	if err := v.SetSetting(s.Key, raw); err != nil {
		return "", err
	}
	return "the repository: " + tilde(v.Abs(".nt/config.toml")) + ", shared by sync", nil
}

// SettingHint is what a change needs to take effect, when it is more than the next command.
func SettingHint(s config.Setting) string {
	switch {
	case strings.HasPrefix(s.Key, "sync."):
		return "the timer keeps its old settings until nt install --sync"
	case strings.HasPrefix(s.Key, "folders."):
		return "the files in the old folder stay where they are"
	case s.Key == "repo":
		return "nt works on that repository from now on; the sync timer too after nt install --sync"
	}
	return ""
}

// nt config edit [--repo]: the whole file in $EDITOR, checked when the editor closes.
func runConfigEdit(args []string) int {
	args, repo := takeBool(args, "--repo")
	if _, err := positional("config edit", args); err != nil {
		return fail(err)
	}
	if repo {
		v, m, err := openVault()
		if err != nil {
			return fail(err)
		}
		p := v.Abs(".nt/config.toml")
		if code := edit(v, m, p, 0); code != 0 {
			return code
		}
		return checked(p, config.CheckRepo)
	}
	p := config.MachinePath()
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		if _, err := config.SetMachine("repo", "~/notes"); err != nil { // the template, to start from
			return fail(err)
		}
	}
	if err := runEditor(p, 0); err != nil {
		return fail(fmt.Errorf("config edit: %w", err))
	}
	return checked(p, config.CheckMachine)
}

// checked says whether the file at p loads, after an edit.
func checked(p string, check func([]byte) error) int {
	b, err := os.ReadFile(p)
	if err == nil {
		err = check(b)
	}
	if err != nil {
		return fail(fmt.Errorf("%s does not load: %w; nt config edit opens it again", tilde(p), err))
	}
	fmt.Println(tilde(p) + ": ok")
	return 0
}
