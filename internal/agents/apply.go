package agents

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/w4jnl/nt/internal/vault"
)

// Statuses are the values of status, in the order a session moves through them.
var Statuses = []string{"in-progress", "blocked", "done", "failed", "abandoned"}

// sessionFile is where the state folder keeps a session's report path: the path is fixed by
// the session's first event, so a session that runs past midnight stays in one file.
func sessionFile(stateDir, agent, session string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '_'
		}
		return r
	}, agent+"--"+session)
	return filepath.Join(stateDir, "sessions", clean)
}

func rememberSession(stateDir, agent, session, rel string) error {
	p := sessionFile(stateDir, agent, session)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return vault.WriteFile(p, []byte(rel+"\n"))
}

func recallSession(stateDir, agent, session string) string {
	b, err := os.ReadFile(sessionFile(stateDir, agent, session))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Apply records ev in its session's report at now and returns the report's repository path.
// The first event of a session makes the report; a start after the session ended, or on a later
// day, makes a new one that continues it. ended is final and updated never moves back.
func Apply(v *vault.Vault, stateDir string, ev Event, now time.Time) (string, error) {
	unlock, err := v.Lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	rel := recallSession(stateDir, ev.Agent, ev.Session)
	var r *Report
	if rel != "" {
		if b, err := os.ReadFile(v.Abs(rel)); err == nil {
			r = ParseReport(b)
		}
	}
	continues := ""
	if r != nil && ev.Kind == "start" && (r.Get("ended") != "" || r.Get("date") != v.Day(now).Format("2006-01-02")) {
		continues, r = rel, nil // resumed: a new report that links to the earlier one
	}
	created := r == nil
	if created {
		rel, r = newSession(v, ev, now)
		if continues != "" {
			r.Set("continues", vault.RelLink(rel, continues))
		}
	}
	clock := now.Format("15:04")
	switch ev.Kind {
	case "start":
		if created {
			where := ev.Cwd
			if b := r.Get("git_branch"); b != "" {
				where += " (" + b + ")"
			}
			r.Log("- " + clock + " started in " + where)
		}
	case "turn":
		l := logLine(ev.Message)
		if l == "" {
			l = "turn ended" // Copilot CLI's agentStop carries no text; the turn still counts
		}
		r.Log("- " + clock + " " + l)
	case "end":
		if r.Get("ended") == "" {
			r.Set("ended", stamp(now))
			if r.Get("status") == "in-progress" {
				r.Set("status", endStatus(r, ev.Reason))
			}
			branch, head := gitState(ev.Cwd)
			r.Set("git_branch", branch)
			r.Set("git_head", head)
			r.Log("- " + clock + " ended")
		}
	}
	if ev.Model != "" && r.Get("model") == "" {
		r.Set("model", ev.Model)
	}
	r.Set("updated", later(r.Get("updated"), now))
	if err := vault.WriteFile(v.Abs(rel), r.Bytes()); err != nil {
		return "", err
	}
	return rel, rememberSession(stateDir, ev.Agent, ev.Session, rel)
}

// newSession is a new report for ev's session, at a path in the agents folder of now's day.
func newSession(v *vault.Vault, ev Event, now time.Time) (string, *Report) {
	project := projectName(ev.Cwd)
	sid := strings.ToLower(ev.Session)
	if len(sid) > 8 {
		sid = sid[:8]
	}
	name := strings.Join([]string{orWord(vault.Slug(v.Host), "host"), orWord(vault.Slug(ev.Agent), "agent"),
		orWord(vault.Slug(project), "project"), orWord(vault.Slug(sid), "session")}, "--") + ".md"
	rel := v.Unique(path.Join(v.AgentsRel(v.Day(now)), name))
	r := newReport()
	branch, head := gitState(ev.Cwd)
	for _, f := range []field{{"type", "agent-report"}, {"schema", "1"}, {"date", v.Day(now).Format("2006-01-02")},
		{"created", stamp(now)}, {"updated", stamp(now)}, {"agent", ev.Agent}, {"host", v.Host}, {"session", ev.Session},
		{"project", project}, {"cwd", ev.Cwd}, {"git_branch", branch}, {"git_head", head}, {"status", "in-progress"}} {
		r.Set(f.key, f.value)
	}
	return rel, r
}

// endStatus is the status a session that ended without one of its own gets: abandoned when
// it never finished a turn, failed when it ended on an error, else done.
func endStatus(r *Report, reason string) string {
	switch {
	case r.LogLines() <= 1:
		return "abandoned"
	case reason == "error":
		return "failed"
	}
	return "done"
}

// projectName is the name of the git repository around cwd, else of cwd itself.
func projectName(cwd string) string {
	if out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output(); err == nil {
		if top := strings.TrimSpace(string(out)); top != "" {
			return filepath.Base(top)
		}
	}
	if cwd == "" {
		return "project"
	}
	return filepath.Base(cwd)
}

// gitState is the branch and short commit of the repository at cwd; "" outside one.
func gitState(cwd string) (branch, head string) {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", ""
	}
	if branch = strings.TrimSpace(string(out)); branch == "HEAD" {
		branch = ""
	}
	if out, err := exec.Command("git", "-C", cwd, "rev-parse", "--short", "HEAD").Output(); err == nil {
		head = strings.TrimSpace(string(out))
	}
	return branch, head
}

func stamp(t time.Time) string { return t.Format(time.RFC3339) }

// later is the later of the time in s and t, as a stamp: updated never moves backwards.
func later(s string, t time.Time) string {
	if old, err := time.Parse(time.RFC3339, s); err == nil && old.After(t) {
		return s
	}
	return stamp(t)
}

func orWord(s, word string) string {
	if s == "" {
		return word
	}
	return s
}

// Selector picks the report nt agent write fills: by session id, else the latest report whose
// session started in Cwd or a folder above it, preferring one still running.
type Selector struct {
	Agent, Session, Cwd string
}

// ErrNoReport says no report fits the selector.
var ErrNoReport = errors.New("no report for this session")

// Find is the repository path of the report sel picks.
func Find(v *vault.Vault, stateDir string, sel Selector) (string, error) {
	files, _ := filepath.Glob(filepath.Join(stateDir, "sessions", "*"))
	best, bestRunning, bestAt := "", false, ""
	for _, f := range files {
		agent, session, ok := strings.Cut(filepath.Base(f), "--")
		if !ok || sel.Session != "" && session != sel.Session || sel.Agent != "" && sel.Session != "" && agent != sel.Agent {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		rel := strings.TrimSpace(string(b))
		rb, err := os.ReadFile(v.Abs(rel))
		if err != nil {
			continue
		}
		r := ParseReport(rb)
		if cwd := r.Get("cwd"); sel.Session == "" && cwd != sel.Cwd && !strings.HasPrefix(sel.Cwd, strings.TrimSuffix(cwd, "/")+"/") {
			continue
		}
		running, at := r.Get("ended") == "", r.Get("updated")
		if best == "" || running && !bestRunning || running == bestRunning && at > bestAt {
			best, bestRunning, bestAt = rel, running, at
		}
	}
	if best == "" {
		return "", ErrNoReport
	}
	return best, nil
}

// Write puts the agent's own account into a report: the Done, Decisions and Open sections of
// text (markdown under those headings; text without them is Done), and status and summary when
// given. With no report for the selector it makes one, so an agent without hooks can report too.
func Write(v *vault.Vault, stateDir string, sel Selector, text, status, summary string, now time.Time) (string, error) {
	if status != "" && !valid(status) {
		return "", fmt.Errorf("status is one of %s, not %q", strings.Join(Statuses, ", "), status)
	}
	rel, err := Find(v, stateDir, sel)
	if errors.Is(err, ErrNoReport) {
		ev := Event{Kind: "start", Agent: orWord(sel.Agent, "agent"), Session: sel.Session, Cwd: sel.Cwd}
		if ev.Session == "" {
			ev.Session = fmt.Sprintf("w%x", now.UnixNano())[:9]
		}
		if rel, err = Apply(v, stateDir, ev, now); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	unlock, err := v.Lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	b, err := os.ReadFile(v.Abs(rel))
	if err != nil {
		return "", err
	}
	r := ParseReport(b)
	if status == "in-progress" && r.Get("ended") != "" {
		return "", errors.New("the session has ended: its status cannot go back to in-progress")
	}
	for name, t := range splitSections(text) {
		r.SetSection(name, t)
	}
	if status != "" {
		r.Set("status", status)
	}
	if s := strings.TrimSpace(strings.SplitN(summary, "\n", 2)[0]); s != "" {
		r.Set("summary", s)
	}
	r.Set("updated", later(r.Get("updated"), now))
	return rel, vault.WriteFile(v.Abs(rel), r.Bytes())
}

func valid(status string) bool {
	for _, s := range Statuses {
		if s == status {
			return true
		}
	}
	return false
}

// splitSections reads the agent's sections from markdown: the text under ## Done, ## Decisions
// and ## Open (any heading level, any case); text before the first of them is Done.
func splitSections(text string) map[string]string {
	out := map[string]string{}
	name := "" // before the first heading
	var cur []string
	flush := func() {
		t := strings.Trim(strings.Join(cur, "\n"), "\n")
		switch {
		case name != "":
			out[name] = t
		case t != "":
			out["Done"] = t
		}
		cur = nil
	}
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if h := strings.TrimLeft(l, "#"); h != l && strings.HasPrefix(h, " ") {
			switch w := strings.ToLower(strings.TrimSpace(h)); w {
			case "done", "decisions", "open":
				flush()
				name = strings.ToUpper(w[:1]) + w[1:]
				continue
			}
		}
		cur = append(cur, l)
	}
	flush()
	return out
}
