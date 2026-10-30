// Package agents keeps agent session reports (docs/format.md §6) and the day's digest (§7).
// flok's records of Claude Code and Copilot CLI sessions (nt agent import) become one report per
// session, written under the repository lock; the agent adds its own Done, Decisions and Open
// with nt agent write.
package agents

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/w4jnl/nt/internal/vault"
)

// keys is the frontmatter in the order nt writes it (docs/format.md §6); other keys follow in
// the order they came.
var keys = []string{"type", "schema", "date", "created", "updated", "ended", "agent", "agent_version",
	"model", "host", "session", "project", "cwd", "git_branch", "git_head", "status", "summary", "tags", "continues",
	"turns", "tools", "blocked_minutes", "waited_minutes"}

// raw are the keys written as they are: dates, times, numbers and a flow list.
var raw = map[string]bool{"schema": true, "date": true, "created": true, "updated": true, "ended": true, "tags": true,
	"turns": true, "tools": true, "blocked_minutes": true, "waited_minutes": true}

// sections are the body's sections in order; Done, Decisions and Open are the agent's.
var sections = []string{"Done", "Decisions", "Open", "Log"}

type field struct{ key, value string }

// Report is one session's report: flat frontmatter and the sections under its title.
type Report struct {
	front []field
	text  map[string]string // section → its text, without the heading
	extra []string          // sections nt does not know, kept in order after Open
}

func newReport() *Report { return &Report{text: map[string]string{}} }

// Get is the value of key, "" when it is not set.
func (r *Report) Get(key string) string {
	for _, f := range r.front {
		if f.key == key {
			return f.value
		}
	}
	return ""
}

// Set sets key; an empty value removes it.
func (r *Report) Set(key, value string) {
	for i, f := range r.front {
		if f.key == key {
			if value == "" {
				r.front = append(r.front[:i], r.front[i+1:]...)
			} else {
				r.front[i].value = value
			}
			return
		}
	}
	if value != "" {
		r.front = append(r.front, field{key, value})
	}
}

// Section is the text of a section, "" when empty.
func (r *Report) Section(name string) string { return r.text[name] }

// SetSection replaces the text of a section.
func (r *Report) SetSection(name, text string) { r.text[name] = strings.Trim(text, "\n") }

// Title is the report's heading: "<project>: <summary>", or the project alone.
func (r *Report) Title() string {
	t := r.Get("project")
	if s := r.Get("summary"); s != "" {
		t += ": " + s
	}
	return t
}

// ParseReport reads a report. What it does not understand it keeps: unknown keys and sections.
func ParseReport(b []byte) *Report {
	r := newReport()
	s := string(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")))
	if rest, ok := strings.CutPrefix(s, "---\n"); ok {
		if fm, body, ok := strings.Cut(rest, "\n---\n"); ok {
			for _, l := range strings.Split(fm, "\n") {
				k, v, ok := strings.Cut(l, ":")
				if !ok || strings.HasPrefix(l, " ") || strings.HasPrefix(l, "#") {
					continue
				}
				r.front = append(r.front, field{strings.TrimSpace(k), decode(strings.TrimSpace(v))})
			}
			s = body
		}
	}
	name := ""
	var cur []string
	flush := func() {
		if name == "" {
			return
		}
		r.text[name] = strings.Trim(strings.Join(cur, "\n"), "\n")
		known := false
		for _, k := range sections {
			known = known || k == name
		}
		if !known {
			r.extra = append(r.extra, name)
		}
	}
	for _, l := range strings.Split(s, "\n") {
		if h, ok := strings.CutPrefix(l, "## "); ok {
			flush()
			name, cur = strings.TrimSpace(h), nil
			continue
		}
		if name != "" {
			cur = append(cur, l)
		}
	}
	flush()
	return r
}

// Bytes is the report as written: frontmatter in nt's key order, the title, then every section
// with a blank line after its heading.
func (r *Report) Bytes() []byte {
	var b strings.Builder
	b.WriteString("---\n")
	done := map[string]bool{}
	write := func(f field) {
		v := f.value
		if !raw[f.key] {
			v = vault.YAMLString(v)
		}
		b.WriteString(f.key + ": " + v + "\n")
		done[f.key] = true
	}
	for _, k := range keys {
		if v := r.Get(k); v != "" {
			write(field{k, v})
		}
	}
	for _, f := range r.front {
		if !done[f.key] {
			write(f)
		}
	}
	b.WriteString("---\n\n# " + r.Title() + "\n")
	names := append(append(append([]string{}, sections[:3]...), r.extra...), "Log")
	for _, n := range names {
		b.WriteString("\n## " + n + "\n")
		if t := r.text[n]; t != "" {
			b.WriteString("\n" + t + "\n")
		}
	}
	return []byte(b.String())
}

// decode reads a YAML scalar as nt and people write them: plain, double- or single-quoted.
func decode(v string) string {
	switch {
	case len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"':
		if s, err := strconv.Unquote(v); err == nil {
			return s
		}
		return v[1 : len(v)-1]
	case len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'':
		return strings.ReplaceAll(v[1:len(v)-1], "''", "'")
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}
