// Package weekly compiles the week's digest (docs/format.md §7): weekly/GGGG/GGGG-Www.md, made
// from the week's journals, the notes made or changed that week, its todos and its agent
// sessions. One machine compiles it (weekly = true), before each sync commits, and writes it only
// when the bytes change; it reads git's history but never changes the repository through git.
package weekly

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/w4jnl/nt/internal/agents"
	"github.com/w4jnl/nt/internal/mdparse"
	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/todo"
	"github.com/w4jnl/nt/internal/vault"
)

// Text is the digest of the week that starts on monday, as of now, and whether the week has
// anything in it. The same notes give the same bytes: nothing in it depends on the time it is
// made but the open todos, which are listed by their dates.
func Text(v *vault.Vault, monday time.Time) ([]byte, bool) {
	monday = vault.WeekStart(monday)
	rel := v.WeekRel(monday)
	sunday := monday.AddDate(0, 0, 6)
	in := func(date string) bool {
		return date >= monday.Format("2006-01-02") && date <= sunday.Format("2006-01-02")
	}
	y, w := monday.ISOWeek()
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntitle: Week %d, %d\ndate: %s\ntype: weekly\n---\n\n", w, y, monday.Format("2006-01-02"))
	fmt.Fprintf(&b, "# Week %d · %s\n\n", w, span(monday, sunday))
	prev, next := monday.AddDate(0, 0, -7), monday.AddDate(0, 0, 7)
	_, pw := prev.ISOWeek()
	_, nw := next.ISOWeek()
	fmt.Fprintf(&b, "[← W%02d](%s) · [W%02d →](%s)\n\n", pw, vault.RelLink(rel, v.WeekRel(prev)), nw, vault.RelLink(rel, v.WeekRel(next)))
	b.WriteString("Compiled by nt from the week's notes; it is written again when they change.\n")
	filled := false

	if s := summaries(v, monday); len(s) > 0 {
		b.WriteString("\n## Summary\n\n" + strings.Join(s, "\n\n") + "\n")
		filled = true
	}

	if notes := notesOf(v, monday, sunday, rel); len(notes) > 0 {
		b.WriteString("\n## Notes\n\n" + strings.Join(notes, "\n") + "\n")
		filled = true
	}

	var done, open []todo.Item
	for _, it := range todo.Collect(v) {
		switch {
		case it.Done && in(it.DoneOn):
			done = append(done, it)
		case !it.Done && it.Due != "" && it.Due <= sunday.Format("2006-01-02"):
			open = append(open, it)
		}
	}
	sort.SliceStable(done, func(i, j int) bool { return done[i].DoneOn < done[j].DoneOn })
	sort.SliceStable(open, func(i, j int) bool { return open[i].Due < open[j].Due })
	if len(done)+len(open) > 0 {
		b.WriteString("\n## Todos\n\n")
		for _, it := range done {
			b.WriteString("- " + theme.Glyph(theme.Done, 0) + " " + it.Text + " · " + weekday(it.DoneOn) + "\n")
		}
		for _, it := range open {
			b.WriteString("- ○ " + it.Text + " · due " + shortDate(it.Due) + "\n")
		}
		filled = true
	}

	type tally struct {
		sessions int
		states   map[string]int
	}
	projects := map[string]*tally{}
	perDay := make([][]agents.Line, 7)
	for i := range 7 {
		perDay[i] = agents.Day(v, monday.AddDate(0, 0, i))
		for _, l := range perDay[i] {
			p := l.Project
			if projects[p] == nil {
				projects[p] = &tally{states: map[string]int{}}
			}
			projects[p].sessions++
			projects[p].states[l.Status]++
		}
	}
	if len(projects) > 0 {
		b.WriteString("\n## Agents\n\n")
		names := make([]string, 0, len(projects))
		for p := range projects {
			names = append(names, p)
		}
		sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
		for _, p := range names {
			t := projects[p]
			var states []string
			for _, s := range agents.Statuses {
				if n := t.states[s]; n > 0 {
					states = append(states, fmt.Sprintf("%d %s", n, s))
				}
			}
			fmt.Fprintf(&b, "- %s · %s: %s\n", orWord(p, "no project"), plural(t.sessions, "session"), strings.Join(states, ", "))
		}
		filled = true
	}

	var days strings.Builder
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		journal := v.JournalRel(day)
		raw, err := os.ReadFile(v.Abs(journal))
		has := err == nil
		if !has && len(perDay[i]) == 0 {
			continue
		}
		days.WriteString("\n### " + day.Format("Monday 2 January") + "\n\n")
		start, _ := v.Cfg.DayStart()
		if log := logOf(vault.SortLog(raw, start), journal, rel); log != "" {
			days.WriteString(log + "\n\n")
		}
		var links []string
		if has {
			links = append(links, "[journal]("+vault.RelLink(rel, journal)+")")
		}
		if n := len(perDay[i]); n > 0 {
			links = append(links, "["+plural(n, "agent session")+"]("+vault.RelLink(rel, v.AgentsRel(day)+"/README.md")+")")
		}
		days.WriteString(strings.Join(links, " · ") + "\n")
		filled = true
	}
	if days.Len() > 0 {
		b.WriteString("\n## Days\n" + days.String())
	}
	return []byte(b.String()), filled
}

// Write compiles the digest of monday's week and writes it under the repository lock when its
// bytes change; a week with nothing in it gets none. It reports whether the file changed.
func Write(v *vault.Vault, monday time.Time) (bool, error) {
	out, filled := Text(v, monday)
	rel := v.WeekRel(monday)
	unlock, err := v.Lock()
	if err != nil {
		return false, err
	}
	defer unlock()
	old, err := os.ReadFile(v.Abs(rel))
	if !filled || err == nil && bytes.Equal(old, out) {
		return false, nil
	}
	return true, vault.WriteFile(v.Abs(rel), out)
}

var weekName = regexp.MustCompile(`^(\d{4})-W(\d{2})\.md$`)

// Refresh writes the digests that may have changed: this week's, last week's, and those of the
// weeks since the latest digest there is, when the machine was off for a while. The machine with
// weekly = true runs it before each sync commits. It returns the weeks it wrote, as GGGG-Www.
func Refresh(v *vault.Vault, now time.Time) ([]string, error) {
	cur := vault.WeekStart(v.Day(now))
	from := cur.AddDate(0, 0, -7)
	if latest, ok := latestWeek(v); ok && latest.Before(from) {
		from = latest
		if limit := cur.AddDate(0, 0, -7*52); from.Before(limit) {
			from = limit
		}
	}
	var wrote []string
	// a summary written late, on a machine that was off, still reaches its week's digest
	for m := cur.AddDate(0, 0, -7*4); m.Before(from); m = m.AddDate(0, 0, 7) {
		if newerSummary(v, m) {
			changed, err := Write(v, m)
			if err != nil {
				return wrote, err
			}
			if changed {
				wrote = append(wrote, strings.TrimSuffix(path.Base(v.WeekRel(m)), ".md"))
			}
		}
	}
	for m := from; !m.After(cur); m = m.AddDate(0, 0, 7) {
		changed, err := Write(v, m)
		if err != nil {
			return wrote, err
		}
		if changed {
			wrote = append(wrote, strings.TrimSuffix(path.Base(v.WeekRel(m)), ".md"))
		}
	}
	return wrote, nil
}

// newerSummary reports a summary of monday's week newer than its digest, or a digest missing.
func newerSummary(v *vault.Vault, monday time.Time) bool {
	digest, err := os.Stat(v.Abs(v.WeekRel(monday)))
	prefix := strings.TrimSuffix(path.Base(v.WeekRel(monday)), ".md") + "-"
	des, _ := os.ReadDir(v.Abs(path.Dir(v.WeekRel(monday))))
	for _, d := range des {
		if !strings.HasPrefix(d.Name(), prefix) {
			continue
		}
		if info, e := d.Info(); e == nil && (err != nil || info.ModTime().After(digest.ModTime())) {
			return true
		}
	}
	return false
}

// Latest is the repository path of the newest weekly digest.
func Latest(v *vault.Vault) (string, bool) {
	monday, ok := latestWeek(v)
	if !ok {
		return "", false
	}
	return v.WeekRel(monday), true
}

// latestWeek is the Monday of the newest weekly digest in the repository.
func latestWeek(v *vault.Vault) (time.Time, bool) {
	var best time.Time
	years, _ := os.ReadDir(v.Abs(v.Cfg.Folders.Weekly))
	for _, y := range years {
		files, _ := os.ReadDir(v.Abs(path.Join(v.Cfg.Folders.Weekly, y.Name())))
		for _, f := range files {
			m := weekName.FindStringSubmatch(f.Name())
			if m == nil {
				continue
			}
			var year, week int
			fmt.Sscan(m[1], &year)
			fmt.Sscan(m[2], &week)
			if d := isoMonday(year, week); d.After(best) {
				best = d
			}
		}
	}
	return best, !best.IsZero()
}

// isoMonday is the Monday of ISO week w of year y.
func isoMonday(y, w int) time.Time {
	jan4 := time.Date(y, 1, 4, 0, 0, 0, 0, time.Local) // always in week 1
	return vault.WeekStart(jan4).AddDate(0, 0, 7*(w-1))
}

// notesOf lists the topic notes made in the week, by the date in their names, then the ones
// changed in it, by git's history: a line each, linked from the digest at rel.
func notesOf(v *vault.Vault, monday, sunday time.Time, rel string) []string {
	first, last := monday.Format("2006-01-02"), sunday.Format("2006-01-02")
	var made []string
	seen := map[string]bool{}
	_ = filepath.WalkDir(v.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		r := v.RelPath(p)
		if d.IsDir() {
			if p != v.Root && (strings.HasPrefix(d.Name(), ".") || reserved(v, r)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !note(d.Name()) {
			return nil
		}
		if date := datePrefix.FindString(d.Name()); date >= first && date <= last {
			made = append(made, r)
			seen[r] = true
		}
		return nil
	})
	sort.Slice(made, func(i, j int) bool {
		if a, b := path.Base(made[i])[:10], path.Base(made[j])[:10]; a != b {
			return a < b
		}
		return made[i] < made[j]
	})
	// a commit that touched many notes was an import or a move, not writing: its notes are
	// counted, not listed
	var changed []string
	bulk := map[string]bool{}
	for _, commit := range commitsIn(v, monday, monday.AddDate(0, 0, 7)) {
		var notes []string
		for _, r := range commit {
			if !seen[r] && !reserved(v, r) && note(path.Base(r)) && !hidden(r) && isFile(v.Abs(r)) {
				notes = append(notes, r)
			}
		}
		for _, r := range notes {
			if len(notes) > bulkNotes {
				bulk[r] = true
			} else if !seen[r] {
				seen[r] = true
				changed = append(changed, r)
			}
		}
	}
	for _, r := range changed {
		delete(bulk, r)
	}
	sort.Strings(changed)
	var out []string
	line := func(r, how string) string {
		dir := path.Dir(r)
		if dir == "." {
			dir = "/"
		}
		head, _ := readHead(v.Abs(r))
		return "- [" + escape(render.Title(head, r)) + "](" + vault.RelLink(rel, r) + ") · " + dir + " · " + how
	}
	for _, r := range made {
		out = append(out, line(r, "new"))
	}
	for i, r := range changed {
		if i == listed {
			out = append(out, fmt.Sprintf("- and %d more changed", len(changed)-listed))
			break
		}
		out = append(out, line(r, "changed"))
	}
	if n := len(bulk); n > 0 {
		out = append(out, fmt.Sprintf("- %s changed together, by an import or a move", plural(n, "more note")))
	}
	return out
}

const (
	bulkNotes = 20 // a commit that changed more notes than this was an import or a move
	listed    = 20 // the changed notes listed by name; the rest are counted
)

// commitsIn is the files each commit between from and to touched, oldest commit first, by
// git's log; nothing when the repository has no history or git is missing. day_starts moves both
// ends, as it moves days.
func commitsIn(v *vault.Vault, from, to time.Time) [][]string {
	start, _ := v.Cfg.DayStart()
	cmd := exec.Command("git", "-C", v.Root, "log", "--reverse", "--no-renames", "--name-only", "--format=%x00",
		"--since="+from.Add(start).Format(time.RFC3339), "--until="+to.Add(start).Format(time.RFC3339))
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var commits [][]string
	for _, c := range strings.Split(string(out), "\x00") {
		var files []string
		for _, l := range strings.Split(c, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				files = append(files, l)
			}
		}
		if len(files) > 0 {
			commits = append(commits, files)
		}
	}
	return commits
}

// logOf is the journal's ## Log section with its links pointing at the same files from the
// digest at rel.
func logOf(journal []byte, from, rel string) string {
	var lines []string
	in := false
	for _, l := range strings.Split(string(journal), "\n") {
		switch {
		case strings.TrimRight(l, " \t\r") == "## Log":
			in = true
		case in && (strings.HasPrefix(l, "## ") || strings.HasPrefix(l, "# ")):
			in = false
		case in && strings.TrimSpace(l) != "":
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	out, _ := mdparse.Rewrite([]byte(strings.Join(lines, "\n")), func(l *mdparse.Link) bool {
		if l.Wiki || l.Dest == "" || mdparse.External(l.Dest) || strings.HasPrefix(l.Dest, "#") {
			return false
		}
		to, anchor, ok := mdparse.Resolve(from, l.Dest)
		if !ok {
			return false
		}
		dest := vault.RelLink(rel, to)
		if anchor != "" {
			dest += "#" + anchor
		}
		changed := dest != l.Dest
		l.Dest = dest
		return changed
	})
	return string(out)
}

var datePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// reserved is a path in the journals, the agent reports or the weekly digests.
func reserved(v *vault.Vault, rel string) bool {
	c := v.Cfg.Folders
	for _, f := range []string{c.Daily, c.Agents, c.Weekly} {
		if f != "" && (rel == f || strings.HasPrefix(rel, f+"/")) {
			return true
		}
	}
	return false
}

// note is a topic note's file name: markdown, not hidden, not a conflict copy, not the todo
// list or the agents' guide.
func note(name string) bool {
	return strings.HasSuffix(name, ".md") && !strings.HasPrefix(name, ".") && !strings.Contains(name, ".conflict-") &&
		name != todo.File && name != "AGENTS.md"
}

func isFile(p string) bool { st, err := os.Stat(p); return err == nil && st.Mode().IsRegular() }

func hidden(rel string) bool {
	for _, p := range strings.Split(rel, "/") {
		if strings.HasPrefix(p, ".") {
			return true
		}
	}
	return false
}

func readHead(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := f.Read(head)
	return head[:n], nil
}

// span is the week's dates: 26 October – 1 November 2026, or 29 December 2025 – 4 January 2026.
func span(a, b time.Time) string {
	if a.Year() != b.Year() {
		return a.Format("2 January 2006") + " – " + b.Format("2 January 2006")
	}
	return a.Format("2 January") + " – " + b.Format("2 January 2006")
}

func weekday(date string) string {
	d, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return date
	}
	return d.Format("Mon")
}

func shortDate(date string) string {
	d, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return date
	}
	return d.Format("Mon 2 Jan")
}

func escape(s string) string { return strings.NewReplacer("[", `\[`, "]", `\]`).Replace(s) }

func orWord(s, word string) string {
	if s == "" {
		return word
	}
	return s
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
