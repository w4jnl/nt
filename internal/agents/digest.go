package agents

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/w4jnl/nt/internal/theme"
	"github.com/w4jnl/nt/internal/vault"
)

// Line is one report as the digest and nt agent list show it: its frontmatter, and where it is.
type Line struct {
	Rel     string `json:"path"`
	Project string `json:"project"`
	Agent   string `json:"agent"`
	Host    string `json:"host"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Created string `json:"created"`
	Updated string `json:"updated"`
	Ended   string `json:"ended,omitempty"`
}

// Day reads the reports of a day, ordered by project name and then by created.
func Day(v *vault.Vault, day time.Time) []Line {
	dir := v.AgentsRel(day)
	des, _ := os.ReadDir(v.Abs(dir))
	var out []Line
	for _, d := range des {
		n := d.Name()
		if d.IsDir() || n == "README.md" || !strings.HasSuffix(n, ".md") || strings.HasPrefix(n, ".") {
			continue
		}
		b, err := os.ReadFile(v.Abs(path.Join(dir, n)))
		if err != nil {
			continue
		}
		r := ParseReport(b)
		if r.Get("type") != "agent-report" {
			continue
		}
		out = append(out, Line{Rel: path.Join(dir, n), Project: r.Get("project"), Agent: r.Get("agent"), Host: r.Get("host"),
			Status: r.Get("status"), Summary: r.Get("summary"), Created: r.Get("created"), Updated: r.Get("updated"), Ended: r.Get("ended")})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if pa, pb := strings.ToLower(a.Project), strings.ToLower(b.Project); pa != pb {
			return pa < pb
		}
		if a.Created != b.Created {
			return a.Created < b.Created
		}
		return a.Rel < b.Rel
	})
	return out
}

// DigestText is the day's digest (docs/format.md §7): no generation time, so the same reports
// always give the same bytes.
func DigestText(v *vault.Vault, day time.Time, lines []Line) []byte {
	rel := v.AgentsRel(day) + "/README.md"
	var b strings.Builder
	b.WriteString("# Agents · " + day.Format("Monday 2 January 2006") + "\n\n")
	b.WriteString("[← journal](" + vault.RelLink(rel, v.JournalRel(day)) + ")\n")
	project := "\x00"
	for _, l := range lines {
		if l.Project != project {
			project = l.Project
			b.WriteString("\n## " + orWord(project, "no project") + "\n")
		}
		parts := []string{theme.Glyph(theme.State(l.Status), 0) + " " + l.Status, l.Agent, l.Host}
		if l.Summary != "" {
			parts = append(parts, l.Summary)
		}
		parts = append(parts, "[report]("+vault.RelLink(rel, l.Rel)+")")
		b.WriteString("- " + strings.Join(parts, " · ") + "\n")
	}
	return []byte(b.String())
}

// Digest writes the day's README.md from its reports, under the repository lock and only when
// the bytes change; a day without reports loses it. It reports whether the file changed.
func Digest(v *vault.Vault, day time.Time) (bool, error) {
	lines := Day(v, day)
	rel := v.AgentsRel(day) + "/README.md"
	unlock, err := v.Lock()
	if err != nil {
		return false, err
	}
	defer unlock()
	old, err := os.ReadFile(v.Abs(rel))
	if len(lines) == 0 {
		if err != nil {
			return false, nil
		}
		return true, os.Remove(v.Abs(rel))
	}
	out := DigestText(v, day, lines)
	if err == nil && bytes.Equal(old, out) {
		return false, touch(v.Abs(rel))
	}
	if err := vault.WriteFile(v.Abs(rel), out); err != nil {
		return false, err
	}
	return true, touch(v.Abs(rel))
}

// touch dates the digest now, after its folder's own change, so Refresh sees it as current.
func touch(p string) error {
	now := time.Now()
	return os.Chtimes(p, now, now)
}

var dayDir = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Refresh writes the digest of every day whose folder or reports changed after it: the
// machine with digest = true runs it before each sync commits. It returns the days it wrote.
func Refresh(v *vault.Vault) ([]string, error) {
	var days []string
	root := v.Abs(v.Cfg.Folders.Agents)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return nil
		}
		if !d.IsDir() || !dayDir.MatchString(d.Name()) {
			return nil
		}
		if stale(p) {
			day, err := time.ParseInLocation("2006-01-02", d.Name(), time.Local)
			if err != nil {
				return filepath.SkipDir
			}
			changed, err := Digest(v, day)
			if err != nil {
				return err
			}
			if changed {
				days = append(days, d.Name())
			}
		}
		return filepath.SkipDir
	})
	return days, err
}

// stale reports a day folder whose digest is missing or older than the folder or a report.
func stale(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "README.md"))
	if err != nil {
		return true
	}
	at := st.ModTime()
	if d, err := os.Stat(dir); err == nil && d.ModTime().After(at) {
		return true
	}
	des, _ := os.ReadDir(dir)
	for _, e := range des {
		if i, err := e.Info(); err == nil && e.Name() != "README.md" && i.ModTime().After(at) {
			return true
		}
	}
	return false
}
