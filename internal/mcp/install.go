package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The Claude app and Copilot CLI each start the servers their config file names under
// "mcpServers". nt install --mcp writes nt's entry there, and leaves the rest of the file as it
// is.

// name is nt's entry in a client's config.
const name = "nt"

// Client is a program that starts nt mcp.
type Client struct {
	Name  string // as nt install --mcp=<name> takes it
	Title string // as messages name it
	Path  string // its config file
	Apply string // what makes it read the file again
	Fix   string // where the person mends a file that is not JSON
	Need  string // what to do when its folder is not there
	shell bool   // started from a shell, with its PATH: the entry carries none
	typ   string // the entry's "type", where the client wants one
	tools bool   // the entry lists the tools the client may call
}

// Clients are the programs nt install --mcp knows, the first the one it takes when none is
// named.
func Clients() []Client {
	return []Client{
		{Name: "claude", Title: "the Claude app", Path: ConfigPath(),
			Apply: "quit and open the app to apply",
			Fix:   "fix it in the Claude app, Settings › Developer › Edit Config",
			Need:  "install the Claude app and open it once"},
		{Name: "copilot", Title: "Copilot CLI", Path: CopilotPath(),
			Apply: "a new copilot session applies it (/mcp show lists its servers)",
			Fix:   "fix it by hand, or with copilot's /mcp",
			Need:  "install Copilot CLI and run it once",
			shell: true, typ: "local", tools: true},
	}
}

// Pick is the clients a comma list names: "" the first, "all" every one.
func Pick(list string) ([]Client, error) {
	all := Clients()
	var names []string
	for _, c := range all {
		names = append(names, c.Name)
	}
	if list = strings.TrimSpace(list); list == "" {
		return all[:1], nil
	}
	if list == "all" {
		return all, nil
	}
	var out []Client
	seen := map[string]bool{}
next:
	for _, n := range strings.Split(list, ",") {
		n = strings.TrimSpace(n)
		for _, c := range all {
			if c.Name == n {
				if !seen[n] {
					seen[n] = true
					out = append(out, c)
				}
				continue next
			}
		}
		return nil, fmt.Errorf("--mcp=%s: nt knows %s, or all", n, strings.Join(names, ", "))
	}
	return out, nil
}

// CopilotPath is Copilot CLI's list of servers: $COPILOT_HOME/mcp-config.json, else
// ~/.copilot/mcp-config.json.
func CopilotPath() string {
	base := os.Getenv("COPILOT_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".copilot")
	}
	return filepath.Join(base, "mcp-config.json")
}

// ConfigPath is the Claude app's config file: in Application Support on macOS, else under
// $XDG_CONFIG_HOME or ~/.config.
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "Claude", "claude_desktop_config.json")
	}
	return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
}

// Entry is a server as a client's config names it. Copilot CLI wants the type "local" and the
// tools it may call; the Claude app takes neither.
type Entry struct {
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
	Tools   []string          `json:"tools,omitempty"`
}

// ReadOnly says whether the entry starts a server that only reads.
func (e Entry) ReadOnly() bool {
	for _, a := range e.Args {
		if a == "--read-only" {
			return true
		}
	}
	return false
}

// read is the config file's keys and its servers; a missing or empty file has none.
func read(c Client) (top, servers map[string]json.RawMessage, err error) {
	top, servers = map[string]json.RawMessage{}, map[string]json.RawMessage{}
	path := c.Path
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || err == nil && len(bytes.TrimSpace(data)) == 0 {
		return top, servers, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, nil, fmt.Errorf("%s is not JSON nt can change (%w): %s", path, err, c.Fix)
	}
	if raw, ok := top["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, nil, fmt.Errorf("%s: mcpServers is not a table of servers: %w", path, err)
		}
	}
	return top, servers, nil
}

func write(path string, top, servers map[string]json.RawMessage) error {
	if len(servers) == 0 {
		delete(top, "mcpServers")
	} else {
		raw, err := json.Marshal(servers)
		if err != nil {
			return err
		}
		top["mcpServers"] = raw
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".nt-tmp-*")
	if err != nil {
		return err
	}
	_, werr := f.Write(append(out, '\n'))
	if err := errors.Join(werr, f.Close()); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}

// Install names bin as the client's nt server in its config file, which is made when the
// client's folder is there and the file is not. The Claude app starts the server without the
// shell's PATH, so its entry gives one to find git and rg on; Copilot CLI runs in a shell and
// hands its own on.
func Install(c Client, bin string, readOnly bool) error {
	path := c.Path
	if st, err := os.Stat(filepath.Dir(path)); err != nil || !st.IsDir() {
		return fmt.Errorf("%s is not there: %s", filepath.Dir(path), c.Need)
	}
	top, servers, err := read(c)
	if err != nil {
		return err
	}
	e := Entry{Type: c.typ, Command: bin, Args: []string{"mcp"}}
	if !c.shell {
		e.Env = map[string]string{
			"PATH": strings.Join(dedupe([]string{filepath.Dir(bin), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"}), ":"),
		}
	}
	if c.tools {
		e.Tools = []string{"*"}
	}
	if readOnly {
		e.Args = append(e.Args, "--read-only")
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	servers[name] = raw
	return write(path, top, servers)
}

// Remove takes nt's entry out of the client's config file, and says whether it had one.
func Remove(c Client) (bool, error) {
	path := c.Path
	top, servers, err := read(c)
	if err != nil {
		return false, err
	}
	if _, ok := servers[name]; !ok {
		return false, nil
	}
	delete(servers, name)
	return true, write(path, top, servers)
}

// Installed is nt's entry in the client's config file, and whether it has one.
func Installed(c Client) (Entry, bool, error) {
	path := c.Path
	_, servers, err := read(c)
	if err != nil {
		return Entry{}, false, err
	}
	raw, ok := servers[name]
	if !ok {
		return Entry{}, false, nil
	}
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return Entry{}, true, fmt.Errorf("%s: the nt server is not as nt wrote it: nt install --mcp=%s", path, c.Name)
	}
	return e, true, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
