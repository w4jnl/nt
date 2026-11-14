package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// How a client's nt server is written.
const (
	inJSON   = iota // an entry in a JSON file, under the client's key
	inTOML          // a table in Codex CLI's config.toml
	byClaude        // Claude Code writes it itself: claude mcp add-json
)

// home is the folder a client keeps its config in: the environment variable env when it is set,
// else dir in the home folder.
func home(env, dir string) string {
	if env != "" {
		if p := os.Getenv(env); p != "" {
			return p
		}
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, dir)
}

// ClaudeCodePath is where Claude Code keeps the servers of every project:
// $CLAUDE_CONFIG_DIR/.claude.json, else ~/.claude.json. nt reads it; Claude Code writes it.
func ClaudeCodePath() string {
	if p := os.Getenv("CLAUDE_CONFIG_DIR"); p != "" {
		return filepath.Join(p, ".claude.json")
	}
	return home("", ".claude.json")
}

// Present says whether the client is on this machine: its folder is there, or for Claude Code
// its command.
func Present(c Client) bool {
	if c.kind == byClaude {
		_, err := exec.LookPath("claude")
		return err == nil
	}
	st, err := os.Stat(filepath.Dir(c.Path))
	return err == nil && st.IsDir()
}

// claudeCode runs claude mcp with args, and gives what it said when it fails.
func claudeCode(args ...string) error {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return errors.New("claude is not on PATH: install Claude Code")
	}
	out, err := exec.Command(bin, append([]string{"mcp"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude mcp %s: %s", args[0], strings.TrimSpace(string(out)))
	}
	return nil
}

// tableOf matches the header of nt's table in a config.toml, and of the tables under it.
var tableOf = regexp.MustCompile(`^\s*\[\s*mcp_servers\s*\.\s*("nt"|'nt'|nt)\s*(\.[^\]]*)?\]\s*(#.*)?$`)

// withoutTable is a config.toml without nt's table: its lines from the header to the next
// table's.
func withoutTable(text string) string {
	var out []string
	in := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case tableOf.MatchString(line):
			in = true
			for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
				out = out[:len(out)-1]
			}
			continue
		case in && strings.HasPrefix(strings.TrimSpace(line), "["):
			in = false
			out = append(out, "")
		}
		if !in {
			out = append(out, line)
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}

// tomlServers reads the servers a config.toml names.
func tomlServers(c Client, text string) (map[string]Entry, error) {
	var cfg struct {
		Servers map[string]Entry `toml:"mcp_servers"`
	}
	if _, err := toml.Decode(text, &cfg); err != nil {
		return nil, fmt.Errorf("%s is not TOML nt can change (%w): %s", c.Path, err, c.Fix)
	}
	return cfg.Servers, nil
}

func readTOML(c Client) (string, map[string]Entry, error) {
	data, err := os.ReadFile(c.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	servers, err := tomlServers(c, string(data))
	return string(data), servers, err
}

// putTOML writes the config.toml with nt's table as e, or without it when e is nil. A file that
// names nt in another form than a table is left alone.
func putTOML(c Client, e *Entry) (had bool, err error) {
	text, servers, err := readTOML(c)
	if err != nil {
		return false, err
	}
	_, had = servers[name]
	if !had && e == nil {
		return false, nil
	}
	next := withoutTable(text)
	if strings.TrimSpace(next) == "" {
		next = ""
	}
	if e != nil {
		cmd, _ := json.Marshal(e.Command) // a JSON string is a TOML basic string
		args, _ := json.Marshal(e.Args)
		if next != "" {
			next += "\n"
		}
		next += fmt.Sprintf("[mcp_servers.%s]\ncommand = %s\nargs = %s\n", name, cmd, bytes.ReplaceAll(args, []byte(","), []byte(", ")))
	}
	after, err := tomlServers(c, next)
	if _, still := after[name]; err != nil || still != (e != nil) {
		return had, fmt.Errorf("%s names the nt server in a form nt does not change: %s", c.Path, c.Fix)
	}
	return had, writeFile(c.Path, []byte(next))
}
