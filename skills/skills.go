// Package skills carries nt's agent skill (skills/nt/SKILL.md) inside the binary and installs
// it once, in nt's data folder, linked into Claude Code's and Copilot CLI's personal skill
// folders, so both agents read the same file and it always matches the nt that runs it.
package skills

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed nt/SKILL.md
var skill []byte

// Dir is where the skill is installed: $XDG_DATA_HOME/nt/skills/nt, else
// ~/.local/share/nt/skills/nt.
func Dir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "nt", "skills", "nt")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nt", "skills", "nt")
}

// Links are where the agents look for personal skills: Claude Code's skills folder
// ($CLAUDE_CONFIG_DIR/skills, else ~/.claude/skills) and ~/.agents/skills, which Copilot CLI
// reads.
func Links() []string {
	home, _ := os.UserHomeDir()
	claude := filepath.Join(home, ".claude")
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		claude = d
	}
	return []string{filepath.Join(claude, "skills", "nt"), filepath.Join(home, ".agents", "skills", "nt")}
}

// Install writes the skill to dir and links it from every path in links. A link path that
// is something else already is left alone, and reported.
func Install(dir string, links []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), skill, 0o644); err != nil {
		return err
	}
	for _, l := range links {
		if t, err := os.Readlink(l); err == nil {
			if t == dir {
				continue
			}
			if err := os.Remove(l); err != nil {
				return err
			}
		} else if _, err := os.Lstat(l); err == nil {
			return fmt.Errorf("%s exists and is not nt's link: move it away first", l)
		}
		if err := os.MkdirAll(filepath.Dir(l), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(dir, l); err != nil {
			return err
		}
	}
	return nil
}

// Remove deletes the links that point at dir, and dir.
func Remove(dir string, links []string) error {
	for _, l := range links {
		if t, err := os.Readlink(l); err == nil && t == dir {
			if err := os.Remove(l); err != nil {
				return err
			}
		}
	}
	return os.RemoveAll(dir)
}

// State says how installed the skill is: "" when every link points at dir and the file is this
// binary's, else what is missing.
func State(dir string, links []string) string {
	b, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return "not installed"
	}
	for _, l := range links {
		if t, err := os.Readlink(l); err != nil || t != dir {
			return l + " does not link to it"
		}
	}
	if string(b) != string(skill) {
		return "older than this nt"
	}
	return ""
}
