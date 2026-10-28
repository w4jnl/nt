package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ClaudeSettingsPath is Claude Code's user settings: $CLAUDE_CONFIG_DIR/settings.json, else
// ~/.claude/settings.json.
func ClaudeSettingsPath() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "settings.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

// CopilotHooksPath is nt's hooks file for Copilot CLI: $COPILOT_HOME/hooks/nt.json, else
// ~/.copilot/hooks/nt.json.
func CopilotHooksPath() string {
	base := os.Getenv("COPILOT_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".copilot")
	}
	return filepath.Join(base, "hooks", "nt.json")
}

// ours matches the command of an nt hook, whatever the binary's path.
var ours = regexp.MustCompile(`(^|[/'" ])nt'? agent (start|turn|end)( |$)`)

func hookCommand(bin, kind string) string { return quote(bin) + " agent " + kind }

func quote(s string) string {
	if regexp.MustCompile(`^[A-Za-z0-9_./~+-]+$`).MatchString(s) {
		return s
	}
	return "'" + regexp.MustCompile(`'`).ReplaceAllString(s, `'\''`) + "'"
}

func kinds() []string {
	k := make([]string, 0, len(Kinds))
	for kind := range Kinds {
		k = append(k, kind)
	}
	sort.Strings(k)
	return k
}

// ClaudeHooks adds nt's hooks to the Claude Code settings at path, or with remove takes them
// out, keeping every other hook and setting. The hooks run in order, not in the background, so a
// session's events reach its report in the order they happened; each takes milliseconds. A
// changed file is backed up first. It reports whether the file changed.
func ClaudeHooks(path, bin string, remove bool) (bool, error) {
	settings := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, fmt.Errorf("%s is not JSON: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
		if remove {
			return false, nil
		}
	default:
		return false, err
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	changed := false
	for _, kind := range kinds() {
		event := Kinds[kind].Claude
		want := hookCommand(bin, kind)
		list, _ := hooks[event].([]any)
		var keep []any
		found := false
		for _, item := range list {
			entry, _ := item.(map[string]any)
			inner, _ := entry["hooks"].([]any)
			var rest []any
			for _, hi := range inner {
				h, _ := hi.(map[string]any)
				cmd, _ := h["command"].(string)
				if !ours.MatchString(cmd) {
					rest = append(rest, hi)
					continue
				}
				if remove || found {
					changed = true
					continue
				}
				found = true
				if cmd != want || h["async"] != nil {
					h["command"], h["timeout"] = want, float64(10)
					delete(h, "async")
					changed = true
				}
				rest = append(rest, h)
			}
			if len(rest) == 0 && len(inner) > 0 {
				continue // the entry held only nt's hook
			}
			entry["hooks"] = rest
			keep = append(keep, entry)
		}
		if !found && !remove {
			keep = append(keep, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": want, "timeout": float64(10)}}})
			changed = true
		}
		if len(keep) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = keep
		}
	}
	if !changed {
		return false, nil
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}
	if data != nil {
		_ = os.WriteFile(path+".bak-"+time.Now().Format("20060102-150405"), data, 0o600)
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o600)
}

// ClaudeHooked lists the events of the Claude Code settings at path that have nt's hook, and
// the binaries those hooks run.
func ClaudeHooked(path string) (events, bins []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	var s struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if json.Unmarshal(data, &s) != nil {
		return nil, nil
	}
	for _, kind := range kinds() {
		event := Kinds[kind].Claude
		for _, e := range s.Hooks[event] {
			for _, h := range e.Hooks {
				if ours.MatchString(h.Command) {
					events = append(events, event)
					bins = append(bins, hookBinary(h.Command))
				}
			}
		}
	}
	return events, bins
}

// CopilotHooked lists the binaries the nt hooks in Copilot CLI's hooks file at path run; none
// when there is no such file.
func CopilotHooked(path string) (bins []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f struct {
		Hooks map[string][]struct {
			Bash string `json:"bash"`
		} `json:"hooks"`
	}
	if json.Unmarshal(data, &f) != nil {
		return nil
	}
	for _, kind := range kinds() {
		for _, h := range f.Hooks[Kinds[kind].Copilot] {
			if ours.MatchString(h.Bash) {
				bins = append(bins, hookBinary(h.Bash))
			}
		}
	}
	return bins
}

// hookBinary is the binary a hook command of nt's runs: what comes before " agent ", unquoted
// as quote quotes it.
func hookBinary(cmd string) string {
	i := strings.LastIndex(cmd, " agent ")
	if i < 0 {
		return ""
	}
	b := strings.TrimSpace(cmd[:i])
	if len(b) >= 2 && b[0] == '\'' && b[len(b)-1] == '\'' {
		b = strings.ReplaceAll(b[1:len(b)-1], `'\''`, `'`)
	}
	return b
}

// CopilotHooks writes nt's hooks file for Copilot CLI, or with remove deletes it. Copilot's
// payloads do not name the agent or the event, so the commands do. It reports whether
// anything changed.
func CopilotHooks(path, bin string, remove bool) (bool, error) {
	if remove {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return err == nil, err
	}
	hooks := map[string]any{}
	for _, kind := range kinds() {
		hooks[Kinds[kind].Copilot] = []any{map[string]any{"type": "command", "bash": hookCommand(bin, kind) + " --agent copilot-cli", "timeoutSec": 10}}
	}
	out, err := json.MarshalIndent(map[string]any{"version": 1, "hooks": hooks}, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	if old, err := os.ReadFile(path); err == nil && string(old) == string(out) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}
