package agents

// nt's own hooks, which nt 0.7 and before installed and flok has replaced: nt uninstall --hooks
// takes them out, and nt doctor warns while some are left.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// RemoveClaudeHooks takes nt's hooks out of the Claude Code settings at path, from every event,
// keeping every other hook and setting. A changed file is backed up first. It reports whether
// the file changed.
func RemoveClaudeHooks(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	settings := map[string]any{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, fmt.Errorf("%s is not JSON: %w", path, err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	changed := false
	for event, v := range hooks {
		list, _ := v.([]any)
		var keep []any
		removed := false
		for _, item := range list {
			entry, _ := item.(map[string]any)
			inner, _ := entry["hooks"].([]any)
			var rest []any
			for _, hi := range inner {
				h, _ := hi.(map[string]any)
				if cmd, _ := h["command"].(string); ours.MatchString(cmd) {
					removed = true
				} else {
					rest = append(rest, hi)
				}
			}
			switch {
			case len(rest) == len(inner):
				keep = append(keep, item)
			case len(rest) > 0:
				entry["hooks"] = rest
				keep = append(keep, entry)
			} // else the entry held only nt's hooks
		}
		if !removed {
			continue
		}
		changed = true
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
	}
	_ = os.WriteFile(path+".bak-"+time.Now().Format("20060102-150405"), data, 0o600)
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o600)
}

// RemoveCopilotHooks deletes nt's hooks file for Copilot CLI. It reports whether there was one.
func RemoveCopilotHooks(path string) (bool, error) {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// ClaudeHooked says whether the Claude Code settings at path still run one of nt's hooks.
func ClaudeHooked(path string) bool {
	var s struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &s) != nil {
		return false
	}
	for _, entries := range s.Hooks {
		for _, e := range entries {
			for _, h := range e.Hooks {
				if ours.MatchString(h.Command) {
					return true
				}
			}
		}
	}
	return false
}

// CopilotHooked says whether Copilot CLI's hooks file at path still runs one of nt's hooks.
func CopilotHooked(path string) bool {
	var f struct {
		Hooks map[string][]struct {
			Bash string `json:"bash"`
		} `json:"hooks"`
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &f) != nil {
		return false
	}
	for _, hooks := range f.Hooks {
		for _, h := range hooks {
			if ours.MatchString(h.Bash) {
				return true
			}
		}
	}
	return false
}
