package agents

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/w4jnl/nt/internal/vault"
)

// Statuses are the values of status, in the order a session moves through them.
var Statuses = []string{"in-progress", "blocked", "done", "failed", "abandoned"}

// sessionFile is where the state folder keeps a session's report path: the report flok's
// imports fill, until the session goes on in a new one (Import).
func sessionFile(stateDir, agent, session string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '_'
		}
		return r
	}, agent+"--"+session)
	return filepath.Join(stateDir, "sessions", clean)
}

// entry is a session's line in the state folder's map: its report's path, then what nt knows of
// the report, a line each. Lines it does not know, such as the "flok" of nt 0.7, are left out
// when it is written again.
type entry struct {
	rel         string
	agentStatus bool // the agent wrote its status with nt agent write: an import keeps it
	from        int  // the session's turns before the report's first: an earlier report's
}

func readEntry(file string) entry {
	b, err := os.ReadFile(file)
	if err != nil {
		return entry{}
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	e := entry{rel: strings.TrimSpace(lines[0])}
	for _, l := range lines[1:] {
		l = strings.TrimSpace(l)
		if l == "agent-status" {
			e.agentStatus = true
		} else if n, ok := strings.CutPrefix(l, "from "); ok {
			e.from, _ = strconv.Atoi(n)
		}
	}
	return e
}

func (e entry) writeTo(file string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	s := e.rel + "\n"
	if e.agentStatus {
		s += "agent-status\n"
	}
	if e.from > 0 {
		s += "from " + strconv.Itoa(e.from) + "\n"
	}
	return vault.WriteFile(file, []byte(s))
}

func readSession(stateDir, agent, session string) entry {
	return readEntry(sessionFile(stateDir, agent, session))
}

func (e entry) write(stateDir, agent, session string) error {
	return e.writeTo(sessionFile(stateDir, agent, session))
}

// reportPath is a new report's repository path: in the agents folder of day, named
// <host>--<agent>--<project>--<session>.md with the session's first 8 characters.
func reportPath(v *vault.Vault, day time.Time, host, agent, project, session string) string {
	sid := strings.ToLower(session)
	if len(sid) > 8 {
		sid = sid[:8]
	}
	name := strings.Join([]string{orWord(vault.Slug(host), "host"), orWord(vault.Slug(agent), "agent"),
		orWord(vault.Slug(project), "project"), orWord(vault.Slug(sid), "session")}, "--") + ".md"
	return v.Unique(path.Join(v.AgentsRel(day), name))
}

// start makes the report of a session nt has none for, in the agents folder of now's day, and
// remembers it in the sessions map, so flok's imports of the session fill the same file.
func start(v *vault.Vault, stateDir, agent, session, cwd string, now time.Time) (string, error) {
	unlock, err := v.Lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	project := projectName(cwd)
	rel := reportPath(v, v.Day(now), v.Host, agent, project, session)
	r := newReport()
	branch, head := gitState(cwd)
	for _, f := range []field{{"type", "agent-report"}, {"schema", "1"}, {"date", v.Day(now).Format("2006-01-02")},
		{"created", stamp(now)}, {"updated", stamp(now)}, {"agent", agent}, {"host", v.Host}, {"session", session},
		{"project", project}, {"cwd", cwd}, {"git_branch", branch}, {"git_head", head}, {"status", "in-progress"}} {
		r.Set(f.key, f.value)
	}
	where := cwd
	if branch != "" {
		where += " (" + branch + ")"
	}
	r.SetSection("Log", "- "+now.Format("15:04")+" started in "+where)
	if err := vault.WriteFile(v.Abs(rel), r.Bytes()); err != nil {
		return "", err
	}
	return rel, entry{rel: rel}.write(stateDir, agent, session)
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
	rel, _, err := find(v, stateDir, sel)
	return rel, err
}

// find is Find, with the map file that names the report.
func find(v *vault.Vault, stateDir string, sel Selector) (rel, file string, err error) {
	files, _ := filepath.Glob(filepath.Join(stateDir, "sessions", "*"))
	best, bestFile, bestRunning, bestAt := "", "", false, ""
	for _, f := range files {
		agent, session, ok := strings.Cut(filepath.Base(f), "--")
		if !ok || sel.Session != "" && session != sel.Session || sel.Agent != "" && sel.Session != "" && agent != sel.Agent {
			continue
		}
		rel := readEntry(f).rel
		if rel == "" {
			continue
		}
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
			best, bestFile, bestRunning, bestAt = rel, f, running, at
		}
	}
	if best == "" {
		return "", "", ErrNoReport
	}
	return best, bestFile, nil
}

// Write puts the agent's own account into a report: the Done, Decisions and Open sections of
// text (markdown under those headings; text without them is Done), and status and summary when
// given. With no report for the selector it makes one, so an agent flok does not follow can
// report too.
func Write(v *vault.Vault, stateDir string, sel Selector, text, status, summary string, now time.Time) (string, error) {
	if status != "" && !valid(status) {
		return "", fmt.Errorf("status is one of %s, not %q", strings.Join(Statuses, ", "), status)
	}
	rel, file, err := find(v, stateDir, sel)
	if errors.Is(err, ErrNoReport) {
		agent, session := orWord(sel.Agent, "agent"), sel.Session
		if session == "" {
			session = fmt.Sprintf("w%x", now.UnixNano())[:9]
		}
		if rel, err = start(v, stateDir, agent, session, sel.Cwd, now); err != nil {
			return "", err
		}
		file = sessionFile(stateDir, agent, session)
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
		// the agent's own status: flok's imports keep it
		if e := readEntry(file); !e.agentStatus {
			e.agentStatus = true
			if err := e.writeTo(file); err != nil {
				return "", err
			}
		}
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
