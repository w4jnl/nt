package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// The settings nt config and the browser's settings list show and change (docs/format.md §9):
// each key, the file it lives in, what kind of value it takes, and what it does. A change edits
// that one line of the file, keeping its comment and the rest of the file as it is.

// Setting is one key of a config file.
type Setting struct {
	Key    string   // as in the file, with its table: "summary.model"
	File   string   // "machine" (~/.config/nt/config.toml) or "repo" (.nt/config.toml)
	Kind   string   // bool, enum, string, list, time (HH:MM), duration
	Values []string // an enum's values; "" is none
	Help   string
}

// Settings is every setting, the machine's first.
var Settings = []Setting{
	{Key: "repo", File: "machine", Kind: "string", Help: "the notes repository this machine works on"},
	{Key: "host", File: "machine", Kind: "string", Help: "this machine's name in reports, commits and summaries; empty: the short host name"},
	{Key: "digest", File: "machine", Kind: "bool", Help: "this machine writes the agents' daily digests (one machine only)"},
	{Key: "weekly", File: "machine", Kind: "bool", Help: "this machine compiles the weekly digest (one machine only)"},
	{Key: "summary.tool", File: "machine", Kind: "enum", Values: []string{"", "claude", "copilot"}, Help: "the tool that writes this machine's weekly summaries; empty: none"},
	{Key: "summary.model", File: "machine", Kind: "string", Help: "the model it uses, e.g. haiku or sonnet; empty: the tool's default"},
	{Key: "sync.auto", File: "machine", Kind: "bool", Help: "a timer runs nt sync; apply with nt install --sync"},
	{Key: "sync.interval", File: "machine", Kind: "duration", Help: "how often the timer syncs, e.g. 2m; apply with nt install --sync"},
	{Key: "theme.mode", File: "machine", Kind: "enum", Values: []string{"auto", "dark", "light"}, Help: "the browser's colours: auto follows the terminal"},
	{Key: "inline_images", File: "machine", Kind: "enum", Values: []string{"auto", "on", "off"}, Help: "draw images in the note (Ghostty, kitty); from the next start"},
	{Key: "image_viewer", File: "machine", Kind: "string", Help: "the command that opens an image full size; empty: Quick Look, xdg-open or chafa"},
	{Key: "default_folder", File: "repo", Kind: "string", Help: "where nt n puts a note when no folder is given"},
	{Key: "day_starts", File: "repo", Kind: "time", Help: "before this time an entry counts toward the day before"},
	{Key: "pins", File: "repo", Kind: "list", Help: "folders on the browser's keys 1-9, e.g. tech, work"},
	{Key: "folders.daily", File: "repo", Kind: "string", Help: "the journals' folder; the files there stay where they are"},
	{Key: "folders.agents", File: "repo", Kind: "string", Help: "the agent reports' folder; the files there stay where they are"},
	{Key: "folders.weekly", File: "repo", Kind: "string", Help: "the weekly digests' folder; the files there stay where they are"},
}

// FindSetting is the setting key names.
func FindSetting(key string) (Setting, bool) {
	for _, s := range Settings {
		if s.Key == key {
			return s, true
		}
	}
	return Setting{}, false
}

// SettingKeys are the keys, in order.
func SettingKeys() []string {
	var out []string
	for _, s := range Settings {
		out = append(out, s.Key)
	}
	return out
}

// Value is the setting's value in m or r, as nt config shows it.
func (s Setting) Value(m Machine, r Repo) string {
	switch s.Key {
	case "repo":
		return m.Repo
	case "host":
		return m.Host
	case "digest":
		return fmt.Sprint(m.Digest)
	case "weekly":
		return fmt.Sprint(m.Weekly)
	case "summary.tool":
		return m.Summary.Tool
	case "summary.model":
		return m.Summary.Model
	case "sync.auto":
		return fmt.Sprint(m.Sync.Auto)
	case "sync.interval":
		return m.Sync.Interval
	case "theme.mode":
		return m.Theme.Mode
	case "inline_images":
		return m.InlineImages
	case "image_viewer":
		return m.ImageViewer
	case "default_folder":
		return r.DefaultFolder
	case "day_starts":
		return r.DayStarts
	case "pins":
		return strings.Join(r.Pins, ", ")
	case "folders.daily":
		return r.Folders.Daily
	case "folders.agents":
		return r.Folders.Agents
	case "folders.weekly":
		return r.Folders.Weekly
	}
	return ""
}

// Choices is what an enum takes, or true and false: what a prompt offers.
func (s Setting) Choices() []string {
	if s.Kind == "bool" {
		return []string{"true", "false"}
	}
	return s.Values
}

// TOML is the value typed as raw, written as the file holds it, or why it is not a value.
func (s Setting) TOML(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	switch s.Kind {
	case "bool":
		switch strings.ToLower(raw) {
		case "true", "yes", "on", "1":
			return "true", nil
		case "false", "no", "off", "0":
			return "false", nil
		}
		return "", fmt.Errorf("%s is true or false, not %q", s.Key, raw)
	case "enum":
		if raw == "none" {
			raw = ""
		}
		if !slices.Contains(s.Values, raw) {
			return "", fmt.Errorf("%s is one of %s, not %q", s.Key, choices(s.Values), raw)
		}
		return quote(raw), nil
	case "time":
		if _, err := time.Parse("15:04", raw); err != nil {
			return "", fmt.Errorf("%s is a time like 04:00, not %q", s.Key, raw)
		}
		return quote(raw), nil
	case "duration":
		if d, err := time.ParseDuration(raw); err != nil || d <= 0 {
			return "", fmt.Errorf("%s is a duration like 2m or 30s, not %q", s.Key, raw)
		}
		return quote(raw), nil
	case "list":
		var items []string
		for _, f := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' }) {
			items = append(items, quote(strings.Trim(f, "/")))
		}
		return "[" + strings.Join(items, ", ") + "]", nil
	}
	if strings.ContainsAny(raw, "\n\r") {
		return "", fmt.Errorf("%s is one line", s.Key)
	}
	return quote(raw), nil
}

func choices(vs []string) string {
	var out []string
	for _, v := range vs {
		if v == "" {
			v = "none"
		}
		out = append(out, v)
	}
	return strings.Join(out, ", ")
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\t", `\t`).Replace(s) + `"`
}

// Edit sets key to value (TOML text) in the file data: the line that has it, a commented-out
// line that names it (as nt init writes # host = …), or a new line at the end of its table,
// which is added when missing. A comment after the value is kept, in its column.
func Edit(data []byte, key, value string) []byte {
	table, name := "", key
	if i := strings.LastIndexByte(key, '.'); i > 0 {
		table, name = key[:i], key[i+1:]
	}
	lines := strings.SplitAfter(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	cur, last, header, firstTable, commented := "", -1, -1, -1, -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			if firstTable < 0 {
				firstTable = i
			}
			cur = strings.TrimSpace(strings.Trim(strings.TrimSpace(strings.SplitN(t, "#", 2)[0]), "[]"))
			if cur == table {
				header, last = i, i
			}
			continue
		}
		if cur != table {
			continue
		}
		if c, ok := strings.CutPrefix(t, "#"); ok {
			if k, _, ok := strings.Cut(c, "="); ok && strings.TrimSpace(k) == name && commented < 0 {
				commented = i
			}
			continue
		}
		k, _, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(k) == name {
			end := i
			if v := l[strings.Index(l, "=")+1:]; strings.Contains(stripComment(v), "[") && !strings.Contains(stripComment(v), "]") {
				for end+1 < len(lines) && !strings.Contains(stripComment(lines[end]), "]") {
					end++ // a list over several lines goes as a whole, up to its ]
				}
			}
			repl := withComment(l, l, name, value)
			lines = append(lines[:i], append([]string{repl}, lines[end+1:]...)...)
			return []byte(strings.Join(lines, ""))
		}
		last = i
	}
	switch {
	case commented >= 0:
		l := lines[commented]
		lines[commented] = withComment(strings.Replace(l, "# ", "", 1), strings.Replace(l, "#", " ", 1), name, value)
	case table == "" && firstTable >= 0:
		at := last + 1
		if last < 0 {
			at = firstTable
		}
		lines = append(lines[:at], append([]string{name + " = " + value + "\n"}, lines[at:]...)...)
	case header >= 0 || table == "":
		at := last + 1
		lines = append(lines[:at], append([]string{name + " = " + value + "\n"}, lines[at:]...)...)
	default:
		if len(lines) > 0 && !strings.HasSuffix(lines[len(lines)-1], "\n") {
			lines[len(lines)-1] += "\n"
		}
		lines = append(lines, "\n["+table+"]\n", name+" = "+value+"\n")
	}
	return []byte(strings.Join(lines, ""))
}

// withComment is the line l set to name = value, keeping its indent and, in its column, the
// comment of orig (l itself, or the commented-out line it came from, its # blanked).
func withComment(l, orig, name, value string) string {
	indent := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
	body := strings.TrimRight(orig, "\r\n")
	out := indent + name + " = " + value
	if i := commentAt(body); i >= 0 {
		out += strings.Repeat(" ", max(1, i-len(out))) + body[i:]
	}
	return out + "\n"
}

func stripComment(l string) string {
	if i := commentAt(l); i >= 0 {
		return l[:i]
	}
	return l
}

// commentAt is where a comment starts on the line, outside quotes, or -1.
func commentAt(l string) int {
	in := false
	for i := 0; i < len(l); i++ {
		switch {
		case l[i] == '\\' && in:
			i++
		case l[i] == '"':
			in = !in
		case l[i] == '#' && !in:
			return i
		}
	}
	return -1
}

// CheckMachine reports why data is not a machine config nt can run with, or nil.
func CheckMachine(data []byte) error {
	m := DefaultMachine()
	if _, err := toml.Decode(string(data), &m); err != nil {
		return err
	}
	if _, err := m.SyncInterval(); err != nil {
		return fmt.Errorf("sync.interval: %w", err)
	}
	return nil
}

// CheckRepo reports why data is not a repository config nt can run with, or nil.
func CheckRepo(data []byte) error {
	r := DefaultRepo()
	if _, err := toml.Decode(string(data), &r); err != nil {
		return err
	}
	if r.Format > Format {
		return fmt.Errorf("format %d is newer than this nt knows (%d)", r.Format, Format)
	}
	if _, err := r.DayStart(); err != nil {
		return fmt.Errorf("day_starts: %w", err)
	}
	return nil
}

// SetMachine sets key in this machine's config file, which it writes from the template when
// there is none, and returns the new contents. Nothing is written when the result would not
// load.
func SetMachine(key, raw string) ([]byte, error) {
	s, ok := FindSetting(key)
	if !ok || s.File != "machine" {
		return nil, fmt.Errorf("%q is not a setting of this machine", key)
	}
	value, err := s.TOML(raw)
	if err != nil {
		return nil, err
	}
	p := MachinePath()
	old, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		old = []byte(fmt.Sprintf(MachineTemplate, "~/notes"))
	} else if err != nil {
		return nil, err
	}
	out := Edit(old, key, value)
	if err := CheckMachine(out); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	if bytes.Equal(out, old) {
		return out, nil
	}
	return out, writeAtomic(p, out)
}

// writeAtomic writes data to path through a temp file renamed over it.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".nt-tmp-*")
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}
