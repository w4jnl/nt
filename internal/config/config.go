// Package config reads nt's two configuration files (docs/format.md §9): the repository's
// .nt/config.toml, committed and shared by every device, and the machine's
// ~/.config/nt/config.toml, which stays on one machine.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Format is the newest repository format this nt reads and writes.
const Format = 1

// Repo is .nt/config.toml.
type Repo struct {
	Format        int      `toml:"format"`
	DefaultFolder string   `toml:"default_folder"`
	DayStarts     string   `toml:"day_starts"`
	Pins          []string `toml:"pins"`
	Folders       struct {
		Daily  string `toml:"daily"`
		Agents string `toml:"agents"`
		Weekly string `toml:"weekly"`
	} `toml:"folders"`
}

// DefaultRepo is the configuration a repository without .nt/config.toml gets.
func DefaultRepo() Repo {
	r := Repo{Format: Format, DefaultFolder: "inbox", DayStarts: "04:00"}
	r.Folders.Daily, r.Folders.Agents, r.Folders.Weekly = "daily", "agents", "weekly"
	return r
}

// RepoTemplate is the .nt/config.toml that nt init writes.
const RepoTemplate = `# nt repository settings, shared by every device (docs/format.md §9)
format = 1
default_folder = "inbox"   # where nt n puts a note when no folder is given
day_starts = "04:00"       # local time before which entries count toward the previous day
pins = []                  # folders on keys 1-9 in the TUI, e.g. ["inbox", "tech", "work"]

[folders]
daily = "daily"
agents = "agents"
weekly = "weekly"
`

// LoadRepo reads <root>/.nt/config.toml over the defaults. A missing file is the defaults.
func LoadRepo(root string) (Repo, error) {
	r := DefaultRepo()
	p := filepath.Join(root, ".nt", "config.toml")
	if _, err := toml.DecodeFile(p, &r); err != nil && !errors.Is(err, os.ErrNotExist) {
		return r, fmt.Errorf("%s: %w", p, err)
	}
	if r.Format > Format {
		return r, fmt.Errorf("%s uses format %d; this nt knows format %d: update nt", root, r.Format, Format)
	}
	if _, err := r.DayStart(); err != nil {
		return r, fmt.Errorf("%s: day_starts: %w", p, err)
	}
	for _, f := range []*string{&r.Folders.Daily, &r.Folders.Agents, &r.Folders.Weekly, &r.DefaultFolder} {
		*f = strings.Trim(filepath.ToSlash(*f), "/")
	}
	return r, nil
}

// DayStart is day_starts as a duration after midnight.
func (r Repo) DayStart() (time.Duration, error) {
	if r.DayStarts == "" {
		return 0, nil
	}
	t, err := time.Parse("15:04", r.DayStarts)
	if err != nil {
		return 0, fmt.Errorf("%q is not HH:MM", r.DayStarts)
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}

// Machine is ~/.config/nt/config.toml.
type Machine struct {
	Repo         string `toml:"repo"`
	Host         string `toml:"host"`
	Digest       bool   `toml:"digest"`
	Weekly       bool   `toml:"weekly"`
	ImageViewer  string `toml:"image_viewer"`
	InlineImages string `toml:"inline_images"`
	Sync         struct {
		Auto     bool   `toml:"auto"`
		Interval string `toml:"interval"`
	} `toml:"sync"`
	Theme struct {
		Mode string `toml:"mode"`
	} `toml:"theme"`
}

// DefaultMachine is the configuration a machine without a config file gets.
func DefaultMachine() Machine {
	var m Machine
	m.Repo = "~/notes"
	m.InlineImages = "auto"
	m.Sync.Auto, m.Sync.Interval = true, "2m"
	m.Theme.Mode = "auto"
	return m
}

// MachineTemplate is the file nt init writes when the machine has none; %s is the repository.
const MachineTemplate = `# nt settings for this machine, not committed (docs/format.md §9)
repo = %q
# host = "mbp"             # default: the short host name
digest = false             # true on the one machine that writes agent digests
weekly = false             # true on the one machine that compiles the weekly digest
image_viewer = ""          # default per OS: qlmanage -p / xdg-open / chafa in a tmux popup
inline_images = "auto"     # auto | on | off: draw images in the preview (Ghostty, kitty)

[sync]
auto = true                # a timer runs nt sync (nt install --sync); false: only nt sync
interval = "2m"

[theme]
mode = "auto"              # auto | dark | light
`

// MachinePath is the machine config file: $NT_CONFIG, else ~/.config/nt/config.toml (on macOS
// too, as flok).
func MachinePath() string {
	if p := os.Getenv("NT_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(home(), ".config", "nt", "config.toml")
}

// LoadMachine reads the machine config over the defaults. A missing file is the defaults.
func LoadMachine() (Machine, error) {
	m := DefaultMachine()
	p := MachinePath()
	if _, err := toml.DecodeFile(p, &m); err != nil && !errors.Is(err, os.ErrNotExist) {
		return m, fmt.Errorf("%s: %w", p, err)
	}
	if m.Host == "" {
		m.Host = ShortHost()
	}
	if _, err := m.SyncInterval(); err != nil {
		return m, fmt.Errorf("%s: sync.interval: %w", p, err)
	}
	m.Repo = Expand(m.Repo)
	return m, nil
}

// SyncInterval is sync.interval, at least 30 s.
func (m Machine) SyncInterval() (time.Duration, error) {
	d, err := time.ParseDuration(m.Sync.Interval)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration like 2m", m.Sync.Interval)
	}
	return max(d, 30*time.Second), nil
}

// StateDir holds status.json and the sync log: $NT_STATE, else $XDG_STATE_HOME/nt, else
// ~/.local/state/nt.
func StateDir() string {
	if p := os.Getenv("NT_STATE"); p != "" {
		return p
	}
	if p := os.Getenv("XDG_STATE_HOME"); p != "" {
		return filepath.Join(p, "nt")
	}
	return filepath.Join(home(), ".local", "state", "nt")
}

// ShortHost is the host name up to its first dot, lowercased: the default host in reports and
// conflict copies.
func ShortHost() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "host"
	}
	h, _, _ = strings.Cut(strings.ToLower(h), ".")
	return h
}

// Expand replaces a leading ~ with the home directory.
func Expand(p string) string {
	if p == "~" {
		return home()
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home(), rest)
	}
	return p
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}
