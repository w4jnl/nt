package gitsync

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/w4jnl/nt/internal/vault"
)

// Label names the background sync job: the launchd label on macOS, the systemd unit on Linux.
const Label = "nl.w4j.nt.sync"

// Timer is the background job that runs `nt sync --background` every interval.
type Timer struct {
	Bin      string        // the nt binary, absolute
	Interval time.Duration // sync.interval
	StateDir string        // where the job writes sync.log
	Home     string
}

// Path is the job's file: the launchd plist, or the systemd timer unit.
func (t Timer) Path() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(t.Home, "Library", "LaunchAgents", Label+".plist")
	}
	return filepath.Join(t.Home, ".config", "systemd", "user", "nt-sync.timer")
}

// Install writes the job and starts it, replacing an older one.
func (t Timer) Install() error {
	if runtime.GOOS == "darwin" {
		return t.installLaunchd()
	}
	return t.installSystemd()
}

// Remove stops the job and deletes its files; removing a job that is not there is no error.
func (t Timer) Remove() error {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid())+"/"+Label).Run()
		return removeIfThere(t.Path())
	}
	_ = exec.Command("systemctl", "--user", "disable", "--now", "nt-sync.timer").Run()
	err := errors.Join(removeIfThere(t.Path()), removeIfThere(strings.TrimSuffix(t.Path(), ".timer")+".service"))
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return err
}

// Loaded reports whether the job is known to launchd or enabled in systemd.
func (t Timer) Loaded() bool {
	if runtime.GOOS == "darwin" {
		return exec.Command("launchctl", "print", "gui/"+strconv.Itoa(os.Getuid())+"/"+Label).Run() == nil
	}
	return exec.Command("systemctl", "--user", "is-enabled", "--quiet", "nt-sync.timer").Run() == nil
}

// InstalledBin is the binary the installed job runs, read from its file; "" when there is none.
func (t Timer) InstalledBin() string {
	if runtime.GOOS == "darwin" {
		b, _ := os.ReadFile(t.Path())
		return plistBin(string(b))
	}
	b, _ := os.ReadFile(strings.TrimSuffix(t.Path(), ".timer") + ".service")
	return serviceBin(string(b))
}

var (
	plistArgs  = regexp.MustCompile(`<key>ProgramArguments</key>\s*<array>\s*<string>([^<]*)</string>`)
	execStart  = regexp.MustCompile(`(?m)^ExecStart=(?:"([^"]*)"|(\S+))`)
	xmlUnquote = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&amp;", "&")
)

// plistBin is the program of a launchd job: the first of its ProgramArguments.
func plistBin(plist string) string {
	if m := plistArgs.FindStringSubmatch(plist); m != nil {
		return xmlUnquote.Replace(m[1])
	}
	return ""
}

// serviceBin is the program a systemd service's ExecStart runs.
func serviceBin(service string) string {
	if m := execStart.FindStringSubmatch(service); m != nil {
		return m[1] + m[2]
	}
	return ""
}

// env is what the job runs with: a PATH that finds git, rg and the nt binary, and git that
// never waits for a person.
func (t Timer) env() [][2]string {
	path := []string{filepath.Dir(t.Bin)}
	if g, err := exec.LookPath("git"); err == nil {
		path = append(path, filepath.Dir(g))
	}
	path = append(path, "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin")
	return [][2]string{
		{"PATH", strings.Join(dedupe(path), ":")},
		{"GIT_TERMINAL_PROMPT", "0"},
	}
}

func (t Timer) installLaunchd() error {
	if err := os.MkdirAll(t.StateDir, 0o755); err != nil {
		return err
	}
	if err := vault.WriteFile(t.Path(), []byte(t.plist())); err != nil {
		return err
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain+"/"+Label).Run() // an older job, if any
	if out, err := exec.Command("launchctl", "bootstrap", domain, t.Path()).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap %s: %s", t.Path(), strings.TrimSpace(string(out)))
	}
	return nil
}

// plist is the launchd job: nt sync --background every Interval and at login.
func (t Timer) plist() string {
	var env strings.Builder
	for _, kv := range t.env() {
		fmt.Fprintf(&env, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", kv[0], xmlEscape(kv[1]))
	}
	log := xmlEscape(filepath.Join(t.StateDir, "sync.log"))
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>sync</string>
		<string>--background</string>
	</array>
	<key>StartInterval</key>
	<integer>%d</integer>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>EnvironmentVariables</key>
	<dict>
%s	</dict>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, Label, xmlEscape(t.Bin), int(t.Interval.Seconds()), env.String(), log, log)
}

func (t Timer) installSystemd() error {
	service, timer := t.units()
	if err := os.MkdirAll(t.StateDir, 0o755); err != nil {
		return err
	}
	if err := vault.WriteFile(strings.TrimSuffix(t.Path(), ".timer")+".service", []byte(service)); err != nil {
		return err
	}
	if err := vault.WriteFile(t.Path(), []byte(timer)); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", "nt-sync.timer"}, {"restart", "nt-sync.timer"}} {
		if out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// units are the systemd user service and timer: a one-shot nt sync --background every Interval.
func (t Timer) units() (service, timer string) {
	var env strings.Builder
	for _, kv := range t.env() {
		fmt.Fprintf(&env, "Environment=\"%s=%s\"\n", kv[0], kv[1])
	}
	service = fmt.Sprintf(`[Unit]
Description=nt sync: commit, merge and push the notes repository

[Service]
Type=oneshot
ExecStart="%s" sync --background
%sStandardOutput=append:%s
StandardError=append:%s
`, t.Bin, env.String(), filepath.Join(t.StateDir, "sync.log"), filepath.Join(t.StateDir, "sync.log"))
	timer = fmt.Sprintf(`[Unit]
Description=nt sync every %s

[Timer]
OnBootSec=1min
OnUnitActiveSec=%ds
Unit=nt-sync.service

[Install]
WantedBy=timers.target
`, t.Interval, int(t.Interval.Seconds()))
	return service, timer
}

func removeIfThere(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
