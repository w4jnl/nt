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

// The Claude app starts the servers its config file names under "mcpServers". nt install --mcp
// writes nt's entry there, and leaves the rest of the file as it is.

// name is nt's entry in the Claude app's config.
const name = "nt"

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

// Entry is a server as the Claude app's config names it.
type Entry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
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
func read(path string) (top, servers map[string]json.RawMessage, err error) {
	top, servers = map[string]json.RawMessage{}, map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || err == nil && len(bytes.TrimSpace(data)) == 0 {
		return top, servers, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, nil, fmt.Errorf("%s is not JSON nt can change (%w): fix it in the Claude app, Settings › Developer › Edit Config", path, err)
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

// Install names bin as the Claude app's nt server in the config file at path, which is made
// when the app's folder is there and the file is not. The server finds git and rg on the PATH
// the entry gives it: the app starts it without the shell's.
func Install(path, bin string, readOnly bool) error {
	if st, err := os.Stat(filepath.Dir(path)); err != nil || !st.IsDir() {
		return fmt.Errorf("%s is not there: install the Claude app and open it once", filepath.Dir(path))
	}
	top, servers, err := read(path)
	if err != nil {
		return err
	}
	e := Entry{Command: bin, Args: []string{"mcp"}, Env: map[string]string{
		"PATH": strings.Join(dedupe([]string{filepath.Dir(bin), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"}), ":"),
	}}
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

// Remove takes nt's entry out of the config file at path.
func Remove(path string) error {
	top, servers, err := read(path)
	if err != nil {
		return err
	}
	if _, ok := servers[name]; !ok {
		return nil
	}
	delete(servers, name)
	return write(path, top, servers)
}

// Installed is nt's entry in the config file at path, and whether it has one.
func Installed(path string) (Entry, bool, error) {
	_, servers, err := read(path)
	if err != nil {
		return Entry{}, false, err
	}
	raw, ok := servers[name]
	if !ok {
		return Entry{}, false, nil
	}
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return Entry{}, true, fmt.Errorf("%s: the nt server is not as nt wrote it: nt install --mcp", path)
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
