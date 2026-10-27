package weekly

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/w4jnl/nt/internal/agents"
	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/vault"
)

// The weekly summary (docs/format.md §7, Weekly summaries): once a week is over, each machine
// whose config names a tool ([summary] tool = "claude" or "copilot") has that tool write a few
// sentences on what was written on this machine that week: its journal lines, the notes it made
// or changed, the todos it added or checked, its agent sessions. nt's commits name their machine
// ("nt: <host> · …") and the reports carry their host, so a week written at work never reaches
// the tool at home, and the other way round. Each summary is a file of its own, written once by
// its machine, weekly/GGGG/GGGG-Www-<host>.md; the weekly digest shows them all.

// Tool is the agent CLI that writes a machine's summaries, and the model it uses.
type Tool struct {
	Name  string // claude or copilot
	Model string
}

// Tools are the CLIs a summary can be written with.
var Tools = []string{"claude", "copilot"}

// SummaryRel is the summary of host for the week day is in.
func SummaryRel(v *vault.Vault, day time.Time, host string) string {
	return strings.TrimSuffix(v.WeekRel(day), ".md") + "-" + host + ".md"
}

// the limits on what goes to the tool: per note, per agent report, and in all
const (
	noteChars   = 1500
	reportChars = 1200
	inputChars  = 40000
	summaryMax  = 1500 // the summary's own length, cut at a sentence
)

// Input is what was written on host in the week that starts on monday, as the tool reads it, or
// "" when nothing was.
func Input(v *vault.Vault, host string, monday time.Time) string {
	monday = vault.WeekStart(monday)
	sunday := monday.AddDate(0, 0, 6)
	var journal, todos, notes, sessions []string
	files, bulk := addedBy(v, host, monday, monday.AddDate(0, 0, 7))
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		f := files[p]
		if day, ok := journalDay(v, p); ok {
			for _, l := range f.lines {
				if logTime.MatchString(l) {
					journal = append(journal, day.Format("Mon")+" "+strings.TrimPrefix(l, "- "))
				}
			}
		}
		for _, l := range f.lines {
			if m := task.FindStringSubmatch(l); m != nil {
				what := "added"
				if m[1] != " " {
					what = "done"
				}
				todos = append(todos, what+": "+clean(m[2]))
			}
		}
		if _, ok := journalDay(v, p); ok || path.Base(p) == "todo.md" {
			continue
		}
		head, err := readHead(v.Abs(p))
		if err != nil {
			continue // deleted since
		}
		how := "changed"
		if f.made {
			how = "new"
		}
		lines := f.lines
		if f.made { // a new note: its text as it is now, without the frontmatter
			if b, err := os.ReadFile(v.Abs(p)); err == nil {
				_, body := render.SplitFrontmatter(b)
				lines = strings.Split(string(body), "\n")
			}
		}
		var text []string
		for _, l := range lines {
			if t := strings.TrimSpace(l); t != "" && task.FindStringSubmatch(l) == nil {
				text = append(text, t)
			}
		}
		if len(text) == 0 && !f.made {
			continue // only todos changed, and they are listed already
		}
		notes = append(notes, fmt.Sprintf("- %s (%s, %s):\n%s", render.Title(head, p), p, how, indent(cut(strings.Join(text, "\n"), noteChars))))
	}
	if bulk > 0 {
		notes = append(notes, fmt.Sprintf("- and %d notes changed together, by an import or a move", bulk))
	}
	for i := range 7 {
		for _, l := range agents.Day(v, monday.AddDate(0, 0, i)) {
			if l.Host != host {
				continue
			}
			s := fmt.Sprintf("- %s, %s with %s: %s", orWord(l.Project, "no project"), l.Status, l.Agent, l.Summary)
			if b, err := os.ReadFile(v.Abs(l.Rel)); err == nil {
				r := agents.ParseReport(b)
				var parts []string
				for _, sec := range []string{"Done", "Decisions", "Open"} {
					if t := strings.TrimSpace(r.Section(sec)); t != "" {
						parts = append(parts, sec+":\n"+t)
					}
				}
				if len(parts) > 0 {
					s += "\n" + indent(cut(strings.Join(parts, "\n"), reportChars))
				}
			}
			sessions = append(sessions, s)
		}
	}
	if len(journal)+len(todos)+len(notes)+len(sessions) == 0 {
		return ""
	}
	var b strings.Builder
	_, w := monday.ISOWeek()
	fmt.Fprintf(&b, "Week %d (%s), what was written on the machine %s.\n", w, span(monday, sunday), host)
	for _, s := range []struct {
		name  string
		items []string
	}{{"Journal lines", journal}, {"Todos", todos}, {"Notes made or changed, with the lines added", notes}, {"Agent sessions", sessions}} {
		if len(s.items) > 0 {
			b.WriteString("\n" + s.name + ":\n" + strings.Join(s.items, "\n") + "\n")
		}
	}
	return cut(b.String(), inputChars)
}

// prompt is what the tool is asked; the week follows it.
const prompt = `You write the weekly summary in a personal notes repository. The text that follows lists what
was written on one machine in one week: journal lines, todos added or done, notes made or changed
with the lines added, and coding-agent sessions with what they did. Write one paragraph of three
to six sentences in English: what got done, the decisions made, and what is still open. Name the
projects and notes it is about. Say only what the text says. Leave out passwords, tokens, keys,
and personal data such as phone numbers and addresses. Plain text: no heading, no list, no
markdown.`

// Summarize has tool write the summary of the week that starts on monday, for this machine
// (v.Host), and writes it. It returns the summary's path, or "" when nothing was written on this
// machine that week.
func Summarize(ctx context.Context, v *vault.Vault, tool Tool, monday time.Time) (string, error) {
	monday = vault.WeekStart(monday)
	in := Input(v, v.Host, monday)
	if in == "" {
		return "", nil
	}
	text, err := run(ctx, tool, in)
	if err != nil {
		return "", err
	}
	y, w := monday.ISOWeek()
	rel := SummaryRel(v, monday, v.Host)
	front := fmt.Sprintf("---\ntitle: %s\ndate: %s\ntype: weekly-summary\nhost: %s\ntool: %s\n",
		vault.YAMLString(fmt.Sprintf("Week %d, %d · %s", w, y, v.Host)), monday.Format("2006-01-02"), v.Host, tool.Name)
	if tool.Model != "" {
		front += "model: " + vault.YAMLString(tool.Model) + "\n"
	}
	body := front + "---\n\n" + text + "\n"
	return rel, v.Update(rel, func([]byte) ([]byte, error) { return []byte(body), nil })
}

// Due is the weeks this machine has a summary to write for: weeks that are over, back four
// weeks, with no summary of its own yet and something written on it.
func Due(v *vault.Vault, now time.Time) []time.Time {
	cur := vault.WeekStart(v.Day(now))
	var out []time.Time
	for i := 4; i >= 1; i-- {
		m := cur.AddDate(0, 0, -7*i)
		if !isFile(v.Abs(SummaryRel(v, m, v.Host))) && Input(v, v.Host, m) != "" {
			out = append(out, m)
		}
	}
	return out
}

// RefreshSummaries writes the summaries this machine owes, as sync runs it: a week whose tool
// failed (offline, logged out) is tried again an hour later, not on every run. stateDir keeps
// when each week was last tried. It returns the summaries it wrote.
func RefreshSummaries(ctx context.Context, v *vault.Vault, tool Tool, stateDir string, now time.Time) ([]string, error) {
	var wrote []string
	var errs []error
	for _, m := range Due(v, now) {
		mark := filepath.Join(stateDir, "summary-"+path.Base(strings.TrimSuffix(SummaryRel(v, m, v.Host), ".md"))+".tried")
		if st, err := os.Stat(mark); err == nil && time.Since(st.ModTime()) < time.Hour { // the clock, not now: the mark's time is real
			continue
		}
		_ = os.MkdirAll(stateDir, 0o755)
		_ = os.WriteFile(mark, nil, 0o644)
		rel, err := Summarize(ctx, v, tool, m)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_ = os.Remove(mark)
		if rel != "" {
			wrote = append(wrote, rel)
		}
	}
	return wrote, errors.Join(errs...)
}

// run asks the tool. claude reads the week on stdin, with no tools, no hooks and no session
// kept; copilot gets it in the prompt. NT_NO_REPORT keeps nt's own hooks from reporting the call
// as an agent session. The answer becomes one paragraph.
func run(ctx context.Context, tool Tool, in string) (string, error) {
	var args []string
	stdin := in
	switch tool.Name {
	case "claude":
		args = []string{"-p", "--tools", "", "--settings", `{"disableAllHooks":true}`, "--no-session-persistence",
			"--strict-mcp-config", "--output-format", "text"}
		if tool.Model != "" {
			args = append(args, "--model", tool.Model)
		}
		args = append(args, prompt)
	case "copilot":
		args = []string{"-p", prompt + "\n\n" + in, "--silent"}
		if tool.Model != "" {
			args = append(args, "--model", tool.Model)
		}
		stdin = ""
	default:
		return "", fmt.Errorf("summary: tool %q is not one of %s", tool.Name, strings.Join(Tools, ", "))
	}
	bin, err := LookTool(tool.Name)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "nt-summary-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	ctx, stop := context.WithTimeout(ctx, 5*time.Minute)
	defer stop()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir // not the notes: no AGENTS.md, nothing to read but the week
	cmd.Env = append(os.Environ(), "NT_NO_REPORT=1")
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("summary: %s: %v: %s", tool.Name, err, firstLine(errOut.String()))
	}
	text := paragraph(out.String())
	if text == "" {
		return "", fmt.Errorf("summary: %s answered nothing", tool.Name)
	}
	return text, nil
}

// LookTool finds a tool's binary: on PATH, else where Claude Code and Copilot CLI install.
func LookTool(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	for _, d := range []string{filepath.Join(home, ".local", "bin"), "/opt/homebrew/bin", "/usr/local/bin",
		filepath.Join(home, ".npm-global", "bin"), filepath.Join(home, ".claude", "local")} {
		if p := filepath.Join(d, name); isFile(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("summary: %s is not installed (not on PATH, nor in ~/.local/bin or Homebrew's bin)", name)
}

// paragraph makes the tool's answer one paragraph of plain text, at most summaryMax characters,
// cut after a sentence: no heading, list or task line can come of it.
func paragraph(s string) string {
	var words []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "#>*-•"))
		if l != "" {
			words = append(words, strings.Fields(l)...)
		}
	}
	p := strings.Join(words, " ")
	if len(p) <= summaryMax {
		return p
	}
	p = p[:summaryMax]
	if i := strings.LastIndexAny(p, ".!?"); i > summaryMax/2 {
		return p[:i+1]
	}
	return strings.TrimSpace(p) + "…"
}

// file is the lines one machine's commits added to a file in the week, and whether they made it.
type file struct {
	lines []string
	made  bool
}

// addedBy reads git's log of the week for the commits nt made on host, without merges: the lines
// they added to each note. The agent reports and the weekly files are left out; a commit that
// changed more than bulkNotes notes was an import or a move, and its notes are only counted.
func addedBy(v *vault.Vault, host string, from, to time.Time) (map[string]*file, int) {
	start, _ := v.Cfg.DayStart()
	c := v.Cfg.Folders
	cmd := exec.Command("git", "-C", v.Root, "log", "--no-merges", "--reverse", "-M", "--unified=0", "-p",
		"--format=%x00", "--grep=^nt: "+regexp.QuoteMeta(host)+" ",
		"--since="+from.Add(start).Format(time.RFC3339), "--until="+to.Add(start).Format(time.RFC3339),
		"--", ".", ":(exclude)"+c.Agents, ":(exclude)"+c.Weekly)
	out, err := cmd.Output()
	files := map[string]*file{}
	if err != nil {
		return files, 0
	}
	bulk := map[string]bool{}
	for _, commit := range strings.Split(string(out), "\x00") {
		perFile := map[string]*file{}
		var cur *file
		made := false
		for _, l := range strings.Split(commit, "\n") {
			switch {
			case strings.HasPrefix(l, "diff --git "):
				cur, made = nil, false
			case strings.HasPrefix(l, "--- /dev/null"):
				made = true
			case strings.HasPrefix(l, "+++ "):
				cur = nil
				p, ok := strings.CutPrefix(l, "+++ b/")
				base := path.Base(p)
				if ok && (journalOK(v, p) || !hidden(p) && !reserved(v, p) && (note(base) || base == "todo.md")) {
					cur = &file{made: made}
					perFile[p] = cur
				}
			case cur != nil && strings.HasPrefix(l, "+"):
				cur.lines = append(cur.lines, strings.TrimPrefix(l, "+"))
			}
		}
		if len(perFile) > bulkNotes {
			for p := range perFile {
				bulk[p] = true
			}
			continue
		}
		for p, f := range perFile {
			if have := files[p]; have != nil {
				have.lines = append(have.lines, f.lines...)
				have.made = have.made || f.made
			} else {
				files[p] = f
			}
		}
	}
	for p := range files {
		delete(bulk, p)
	}
	return files, len(bulk)
}

var (
	logTime = regexp.MustCompile(`^- \d{2}:\d{2}\b`)
	task    = regexp.MustCompile(`^\s*(?:[-*+]|\d{1,9}[.)])[ \t]+\[([ xX])\][ \t]+(.*)$`)
	stamps  = regexp.MustCompile(`\s+(?:due|done):\d{4}-\d{2}-\d{2}`)
)

// journalDay is the day of a journal at rel.
func journalDay(v *vault.Vault, rel string) (time.Time, bool) {
	rest, ok := strings.CutPrefix(rel, v.Cfg.Folders.Daily+"/")
	if !ok {
		return time.Time{}, false
	}
	d, err := time.ParseInLocation("2006-01-02", strings.TrimSuffix(path.Base(rest), ".md"), time.Local)
	return d, err == nil
}

func journalOK(v *vault.Vault, rel string) bool { _, ok := journalDay(v, rel); return ok }

func clean(todo string) string { return strings.TrimSpace(stamps.ReplaceAllString(todo, "")) }

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + " …"
}

func indent(s string) string {
	if s == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

// summaries is the weekly summaries of monday's week, by host: what the digest shows.
func summaries(v *vault.Vault, monday time.Time) []string {
	prefix := strings.TrimSuffix(v.WeekRel(monday), ".md") + "-"
	des, _ := os.ReadDir(v.Abs(path.Dir(prefix)))
	var out []string
	for _, d := range des {
		rel := path.Join(path.Dir(prefix), d.Name())
		if d.IsDir() || !strings.HasPrefix(rel, prefix) || !strings.HasSuffix(rel, ".md") {
			continue
		}
		b, err := os.ReadFile(v.Abs(rel))
		if err != nil {
			continue
		}
		front, body := render.SplitFrontmatter(b)
		host, tool := frontValue(front, "host"), frontValue(front, "tool")
		if host == "" || strings.TrimSpace(string(body)) == "" {
			continue
		}
		head := "### " + host
		if tool != "" {
			head += " · " + tool
		}
		out = append(out, head+"\n\n"+strings.TrimSpace(string(body)))
	}
	return out
}

func frontValue(front []byte, key string) string {
	for _, l := range strings.Split(string(front), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), key+":"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}
