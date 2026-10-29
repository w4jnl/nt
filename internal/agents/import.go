package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/w4jnl/nt/internal/vault"
)

// RecordVersion is the highest version of flok's session record nt reads.
const RecordVersion = 1

// ErrNewerRecord says a record is of a version this nt does not read: flok is newer than nt.
var ErrNewerRecord = errors.New("the record is newer than this nt")

// Record is one session as flok knows it, the whole of it on every call (flok's docs/notes.md,
// "The record"): nt writes the session's report from it, so a missed call is mended by the next.
type Record struct {
	Version   int    `json:"version"`
	Agent     string `json:"agent"`
	Session   string `json:"session"`
	Host      string `json:"host"` // "": this machine, nt's own host; else flok's alias for a remote one
	Cwd       string `json:"cwd"`
	Branch    string `json:"branch"`
	Head      string `json:"head"`
	Model     string `json:"model"`
	Created   string `json:"created"`
	Updated   string `json:"updated"`
	Ended     string `json:"ended"`
	EndReason string `json:"end_reason"` // absent while it runs; error, clear, logout, exit, other
	State     string `json:"state"`      // working, blocked, waiting, idle, done
	Turns     int    `json:"turns"`
	Tools     int    `json:"tools"`
	BlockedS  int    `json:"blocked_s"`
	WaitedS   int    `json:"waited_s"`
	Log       []Turn `json:"log"` // the turns, oldest first; the oldest may have been dropped
}

// Turn is one turn of a record: the first line of the agent's reply, the tools it ran and what
// happened on the way. Ended is "" while it runs.
type Turn struct {
	Started string         `json:"started"`
	Ended   string         `json:"ended"`
	Text    string         `json:"text"`
	Tools   map[string]int `json:"tools"`
	Events  []TurnEvent    `json:"events"`
}

// TurnEvent is a block, a question, a wait, an error or a compaction in a turn.
type TurnEvent struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	Seconds int    `json:"seconds"`
}

// ParseRecord reads a record; fields nt does not know are left alone, a newer version refused.
func ParseRecord(b []byte) (Record, error) {
	var rec Record
	if err := json.Unmarshal(b, &rec); err != nil {
		return rec, fmt.Errorf("the record is not JSON: %w", err)
	}
	switch {
	case rec.Version > RecordVersion:
		return rec, fmt.Errorf("%w: version %d, and nt reads up to %d: upgrade nt", ErrNewerRecord, rec.Version, RecordVersion)
	case rec.Version < 1:
		return rec, errors.New("the record has no version")
	case rec.Agent == "" || rec.Session == "":
		return rec, errors.New("the record names no agent or session")
	}
	return rec, nil
}

// Import writes rec's session's report: made the first time, in the agents folder of the day it
// was created, then its frontmatter and ## Log rewritten from the record on every call. The
// agent's own sections, its summary and a status it wrote stay. The sessions map marks the
// session as flok's, so nt's own hooks leave it alone and nt agent write finds it. ended says the
// session ended with this call.
func Import(v *vault.Vault, stateDir string, rec Record, now time.Time) (rel string, ended bool, err error) {
	unlock, err := v.Lock()
	if err != nil {
		return "", false, err
	}
	defer unlock()
	created := parseTime(rec.Created, now)
	host := rec.Host
	if host == "" {
		host = v.Host
	}
	e := readSession(stateDir, rec.Agent, rec.Session)
	var r *Report
	if e.rel != "" {
		if b, err := os.ReadFile(v.Abs(e.rel)); err == nil {
			r = ParseReport(b)
		}
	}
	if r == nil {
		project := filepath.Base(rec.Cwd)
		if rec.Host == "" {
			project = projectName(rec.Cwd) // the cwd is on this machine
		}
		e.rel, r = newImported(v, rec, host, project, created)
	}
	wasEnded := r.Get("ended") != ""
	for _, f := range []field{{"type", "agent-report"}, {"schema", "1"}, {"agent", rec.Agent}, {"host", host},
		{"session", rec.Session}, {"cwd", rec.Cwd}, {"git_branch", rec.Branch}, {"git_head", rec.Head}, {"model", rec.Model}} {
		if f.value != "" || f.key == "git_branch" || f.key == "git_head" {
			r.Set(f.key, f.value)
		}
	}
	if rec.Created != "" {
		r.Set("created", rec.Created)
	}
	if r.Get("ended") == "" && rec.Ended != "" {
		r.Set("ended", rec.Ended) // set once: a session resumed after its end still adds its turns
	}
	r.Set("updated", later(r.Get("updated"), parseTime(rec.Updated, now)))
	if !e.agentStatus {
		r.Set("status", importStatus(rec, r.Get("ended") != ""))
	}
	r.Set("turns", strconv.Itoa(rec.Turns))
	r.Set("tools", strconv.Itoa(rec.Tools))
	r.Set("blocked_minutes", minutes(rec.BlockedS))
	r.Set("waited_minutes", minutes(rec.WaitedS))
	r.SetSection("Log", importLog(r, rec))
	if err := vault.WriteFile(v.Abs(e.rel), r.Bytes()); err != nil {
		return "", false, err
	}
	e.flok = true
	if err := e.write(stateDir, rec.Agent, rec.Session); err != nil {
		return "", false, err
	}
	return e.rel, !wasEnded && r.Get("ended") != "", nil
}

// newImported is a new report for rec's session, in the agents folder of the day it was created.
func newImported(v *vault.Vault, rec Record, host, project string, created time.Time) (string, *Report) {
	sid := strings.ToLower(rec.Session)
	if len(sid) > 8 {
		sid = sid[:8]
	}
	name := strings.Join([]string{orWord(vault.Slug(host), "host"), orWord(vault.Slug(rec.Agent), "agent"),
		orWord(vault.Slug(project), "project"), orWord(vault.Slug(sid), "session")}, "--") + ".md"
	day := v.Day(created)
	rel := v.Unique(path.Join(v.AgentsRel(day), name))
	r := newReport()
	for _, f := range []field{{"type", "agent-report"}, {"schema", "1"}, {"date", day.Format("2006-01-02")},
		{"created", stamp(created)}, {"project", project}} {
		r.Set(f.key, f.value)
	}
	return rel, r
}

// importStatus is the status the record gives, when the agent wrote none of its own: blocked
// while it waits on a person, else in progress; at the end failed on an error, abandoned when no
// turn ended (a session closed in its first turn), else done.
func importStatus(rec Record, ended bool) string {
	switch {
	case !ended && rec.State == "blocked":
		return "blocked"
	case !ended:
		return "in-progress"
	case rec.EndReason == "error":
		return "failed"
	case !turnEnded(rec):
		return "abandoned"
	}
	return "done"
}

// turnEnded says whether any turn of the session ended: one flok no longer sends did, since
// only the oldest are dropped and turns counts every turn opened.
func turnEnded(rec Record) bool {
	if rec.Turns > len(rec.Log) {
		return true
	}
	for _, t := range rec.Log {
		if t.Ended != "" {
			return true
		}
	}
	return false
}

// minutes is seconds as whole minutes for the frontmatter, at least 1 when there were any; ""
// (no key) for none.
func minutes(s int) string {
	if s <= 0 {
		return ""
	}
	return strconv.Itoa(max(1, (s+30)/60))
}

var (
	startLine = regexp.MustCompile(`^- (\d\d:\d\d )?started in `)
	endLine   = regexp.MustCompile(`^- (\d\d:\d\d )?ended$`)
)

// importLog is ## Log from the record: the start, one line per turn with its tools and events
// under it, and the end. The turns the record no longer holds (flok keeps the latest) keep the
// lines they were given: as many of the oldest turn lines as turns - len(log).
func importLog(r *Report, rec Record) string {
	var turns [][]string // the turn lines already there, each with the lines under it
	for _, l := range strings.Split(r.Section("Log"), "\n") {
		switch {
		case l == "":
		case strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t"):
			if len(turns) > 0 {
				turns[len(turns)-1] = append(turns[len(turns)-1], l)
			}
		case startLine.MatchString(l) || endLine.MatchString(l):
		default:
			turns = append(turns, []string{l})
		}
	}
	where := rec.Cwd
	if rec.Branch != "" {
		where += " (" + rec.Branch + ")"
	}
	out := []string{line(clock(r.Get("created")) + "started in " + where)}
	for _, t := range turns[:min(len(turns), max(0, rec.Turns-len(rec.Log)))] {
		out = append(out, t...)
	}
	for _, t := range rec.Log {
		out = append(out, turnLines(t)...)
	}
	if e := r.Get("ended"); e != "" {
		out = append(out, line(clock(e)+"ended"))
	}
	return strings.Join(out, "\n")
}

// turnLines is a turn in ## Log: when the reply came and its first line, then the tools it ran
// and its events, a line each. A turn still running shows only once something happened in it.
func turnLines(t Turn) []string {
	open := t.Ended == ""
	if open && len(t.Events) == 0 {
		return nil
	}
	at, text := t.Ended, logLine(t.Text)
	if open {
		at = t.Started
	} else if text == "" {
		text = "turn ended"
	}
	out := []string{line(strings.TrimSpace(clock(at) + text))}
	if s := toolsLine(t.Tools); s != "" {
		out = append(out, "  "+line(s))
	}
	for _, ev := range t.Events {
		if s := eventLine(ev); s != "" {
			out = append(out, "  "+line(clock(ev.At)+s))
		}
	}
	return out
}

// toolsLine is a turn's tools, the most run first: Bash ×12, Read ×3.
func toolsLine(tools map[string]int) string {
	names := make([]string, 0, len(tools))
	for n, c := range tools {
		if c > 0 && strings.TrimSpace(n) != "" {
			names = append(names, n)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if tools[names[i]] != tools[names[j]] {
			return tools[names[i]] > tools[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fold(n) + " ×" + strconv.Itoa(tools[n])
	}
	return strings.Join(parts, ", ")
}

// eventLine is what an event says in the log; one nt does not know says nothing.
func eventLine(ev TurnEvent) string {
	text := fold(ev.Text)
	with := func(s string) string {
		if text == "" {
			return s
		}
		return s + ": " + text
	}
	switch ev.Kind {
	case "blocked":
		return with("blocked")
	case "question":
		return with("asked")
	case "wait":
		w := "waiting"
		if ev.Seconds > 0 {
			w = "waited " + duration(ev.Seconds)
		}
		if text != "" {
			w += " for " + text
		}
		return w
	case "error":
		return with("error")
	case "compaction":
		return "compacted"
	}
	return ""
}

// duration is a wait in words: 45 s, 12 min.
func duration(s int) string {
	if s < 60 {
		return strconv.Itoa(s) + " s"
	}
	return strconv.Itoa((s+30)/60) + " min"
}

// line is a log line: "- " and the text, at most 200 characters.
func line(s string) string {
	s = "- " + s
	if utf8.RuneCountInString(s) > 200 {
		s = string([]rune(s)[:199]) + "…"
	}
	return s
}

// clock is a stamp's time of day with a space after it, in the offset it carries; "" when it is
// no time.
func clock(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	return t.Format("15:04") + " "
}

// fold is text on one line, spaces folded.
func fold(s string) string { return strings.Join(strings.Fields(s), " ") }

// parseTime is the time in a stamp, or else.
func parseTime(s string, or time.Time) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return or
}

// importedFile stamps the last import in the state folder, for nt doctor.
func importedFile(stateDir string) string { return filepath.Join(stateDir, "flok-import") }

// MarkImported notes that flok imported a report at now.
func MarkImported(stateDir string, now time.Time) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	return vault.WriteFile(importedFile(stateDir), []byte(stamp(now)+"\n"))
}

// LastImport is when flok last imported a report on this machine; ok is false when it never did.
func LastImport(stateDir string) (time.Time, bool) {
	b, err := os.ReadFile(importedFile(stateDir))
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	return t, err == nil
}
