package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const machine = `# nt settings for this machine
repo = "~/notes"
# host = "mbp"             # default: the short host name
digest = true              # true on the one machine that writes agent digests

[sync]
auto = true                # a timer runs nt sync
interval = "2m"

[theme]
mode = "auto"              # auto | dark | light
`

func TestEdit(t *testing.T) {
	for _, c := range []struct{ name, in, key, value, want string }{
		{"a value in a table, its comment kept in its column", machine, "theme.mode", `"dark"`,
			strings.Replace(machine, `mode = "auto"              # auto`, `mode = "dark"              # auto`, 1)},
		{"a top-level value", machine, "digest", "false",
			strings.Replace(machine, "digest = true              #", "digest = false             #", 1)},
		{"a commented-out line is the place", machine, "host", `"work-mac"`,
			strings.Replace(machine, `# host = "mbp"             #`, `host = "work-mac"          #`, 1)},
		{"a key new to its table goes at its end", machine, "sync.extra", `"x"`,
			strings.Replace(machine, "interval = \"2m\"\n", "interval = \"2m\"\nextra = \"x\"\n", 1)},
		{"a table that is not there is added", machine, "summary.tool", `"claude"`, machine + "\n[summary]\ntool = \"claude\"\n"},
		{"a new top-level key goes before the tables", machine, "weekly", "true",
			strings.Replace(machine, "agent digests\n", "agent digests\nweekly = true\n", 1)},
		{"a list over several lines is replaced whole", "pins = [\n  \"a\",\n  \"b\",\n]\n\n[folders]\ndaily = \"daily\"\n", "pins", `["tech"]`,
			"pins = [\"tech\"]\n\n[folders]\ndaily = \"daily\"\n"},
		{"a comment with quotes after a list", `pins = []                  # e.g. ["inbox", "tech"]` + "\n", "pins", `["work"]`,
			`pins = ["work"]            # e.g. ["inbox", "tech"]` + "\n"},
	} {
		if got := string(Edit([]byte(c.in), c.key, c.value)); got != c.want {
			t.Errorf("%s:\n%s\nwant:\n%s", c.name, got, c.want)
		}
	}
}

func TestSettingTOML(t *testing.T) {
	for _, c := range []struct{ key, raw, want, err string }{
		{"weekly", "yes", "true", ""},
		{"weekly", "maybe", "", "true or false"},
		{"theme.mode", "dark", `"dark"`, ""},
		{"theme.mode", "blue", "", "one of auto, dark, light"},
		{"summary.tool", "none", `""`, ""},
		{"day_starts", "05:30", `"05:30"`, ""},
		{"day_starts", "5", "", "a time like 04:00"},
		{"sync.interval", "5m", `"5m"`, ""},
		{"sync.interval", "often", "", "a duration"},
		{"pins", "tech, work/", `["tech", "work"]`, ""},
		{"pins", "", "[]", ""},
		{"summary.model", `my "model"`, `"my \"model\""`, ""},
	} {
		s, _ := FindSetting(c.key)
		got, err := s.TOML(c.raw)
		if c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) || c.err == "" && (err != nil || got != c.want) {
			t.Errorf("%s %q: %q, %v; want %q %q", c.key, c.raw, got, err, c.want, c.err)
		}
	}
}

func TestSetMachine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("NT_CONFIG", p)
	if err := os.WriteFile(p, []byte(machine), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SetMachine("summary.model", "haiku"); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMachine()
	if err != nil || m.Summary.Model != "haiku" || !m.Digest {
		t.Fatalf("after set: %+v, %v", m, err)
	}
	if _, err := SetMachine("pins", "tech"); err == nil {
		t.Error("a repository setting was written to the machine's file")
	}
	if _, err := SetMachine("sync.interval", "0s"); err == nil {
		t.Error("an interval of nothing was written")
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "# nt settings for this machine") || !strings.Contains(string(b), "[summary]\nmodel = \"haiku\"") {
		t.Errorf("the file:\n%s", b)
	}
	// no file yet: the template, with the setting
	os.Remove(p)
	if _, err := SetMachine("weekly", "true"); err != nil {
		t.Fatal(err)
	}
	if m, _ := LoadMachine(); !m.Weekly || m.Sync.Interval != "2m" {
		t.Errorf("from the template: %+v", m)
	}
}
