package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Event is one hook call for a report: a session starting, a turn ending, or the session ending.
type Event struct {
	Kind    string // start, turn, end
	Agent   string // claude-code, copilot-cli, or what the caller named
	Session string
	Cwd     string
	Message string // the agent's last message, for a turn
	Reason  string // why the session ended
	Model   string
}

// Kinds are the events nt agent takes, and the hook events they stand for.
var Kinds = map[string]struct{ Claude, Copilot string }{
	"start": {"SessionStart", "sessionStart"},
	"turn":  {"Stop", "agentStop"},
	"end":   {"SessionEnd", "sessionEnd"},
}

// ParseEvent reads a hook payload in Claude Code's shape (snake_case: session_id,
// last_assistant_message) or Copilot CLI's (camelCase: sessionId). agent names the agent when
// the caller knows; else the shape tells. ok is false for an event that belongs to no report: a
// subagent's, one without a session, or a throwaway session's in a temporary folder.
func ParseEvent(kind string, payload []byte, agent string) (Event, bool, error) {
	if _, known := Kinds[kind]; !known {
		return Event{}, false, fmt.Errorf("%q is not start, turn or end", kind)
	}
	m := map[string]any{}
	if len(bytes.TrimSpace(payload)) > 0 {
		if err := json.Unmarshal(payload, &m); err != nil {
			return Event{}, false, fmt.Errorf("the hook payload is not JSON: %w", err)
		}
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if s, ok := m[k].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	if str("agent_id", "agentId") != "" {
		return Event{}, false, nil // a subagent's turn: the session's report is the parent's
	}
	ev := Event{Kind: kind, Agent: agent, Session: str("session_id", "sessionId"), Cwd: str("cwd"),
		Message: str("last_assistant_message", "lastAssistantMessage"), Reason: str("reason", "stopReason"), Model: str("model")}
	if ev.Agent == "" {
		switch {
		case str("session_id") != "":
			ev.Agent = "claude-code"
		case str("sessionId") != "":
			ev.Agent = "copilot-cli"
		default:
			ev.Agent = "agent"
		}
	}
	if ev.Cwd == "" {
		ev.Cwd, _ = os.Getwd()
	}
	if Temporary(ev.Cwd) && os.Getenv("NT_REPORT_TEMP") == "" {
		return Event{}, false, nil
	}
	return ev, ev.Session != "", nil
}

// Temporary says whether dir is in a temporary folder (/tmp, /var/tmp, $TMPDIR), where a
// session is a throwaway: a script's `claude -p` in a scratch folder, a test. Such sessions get
// no report; NT_REPORT_TEMP=1 reports them all the same.
func Temporary(dir string) bool {
	if dir == "" {
		return false
	}
	paths := []string{filepath.Clean(dir)}
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		paths = append(paths, r)
	}
	for _, root := range []string{os.TempDir(), "/tmp", "/var/tmp"} {
		roots := []string{filepath.Clean(root)}
		if r, err := filepath.EvalSymlinks(root); err == nil {
			roots = append(roots, r)
		}
		for _, p := range paths {
			for _, r := range roots {
				if r != "/" && (p == r || strings.HasPrefix(p, r+string(filepath.Separator))) {
					return true
				}
			}
		}
	}
	return false
}

// logLine is what a turn adds to ## Log: the first line of the agent's message with text in it,
// spaces folded, at most 200 characters. Long answers and secrets stay out of the report.
func logLine(msg string) string {
	for _, l := range strings.Split(msg, "\n") {
		l = strings.Join(strings.Fields(strings.TrimLeft(l, "#>*- \t")), " ")
		if l == "" || strings.HasPrefix(l, "```") {
			continue
		}
		if utf8.RuneCountInString(l) > 200 {
			l = string([]rune(l)[:199]) + "…"
		}
		return l
	}
	return ""
}
