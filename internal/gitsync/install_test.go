package gitsync

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTimerFiles(t *testing.T) {
	tm := Timer{Bin: "/Users/a b/.local/bin/nt", Interval: 2 * time.Minute, StateDir: "/Users/a b/.local/state/nt", Home: "/Users/a b"}
	p := tm.plist()
	for _, want := range []string{
		"<string>" + Label + "</string>",
		"<string>/Users/a b/.local/bin/nt</string>\n\t\t<string>sync</string>\n\t\t<string>--background</string>",
		"<key>StartInterval</key>\n\t<integer>120</integer>",
		"<key>GIT_TERMINAL_PROMPT</key>\n\t\t<string>0</string>",
		"<string>/Users/a b/.local/state/nt/sync.log</string>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist lacks %q:\n%s", want, p)
		}
	}
	if runtime.GOOS == "darwin" {
		f := filepath.Join(t.TempDir(), "job.plist")
		_ = os.WriteFile(f, []byte(p), 0o644)
		if out, err := exec.Command("plutil", "-lint", f).CombinedOutput(); err != nil {
			t.Errorf("plutil: %s", out)
		}
	}
	service, timer := tm.units()
	if !strings.Contains(service, `ExecStart="/Users/a b/.local/bin/nt" sync --background`) || !strings.Contains(service, "Type=oneshot") {
		t.Errorf("service:\n%s", service)
	}
	if !strings.Contains(timer, "OnUnitActiveSec=120s") || !strings.Contains(timer, "WantedBy=timers.target") {
		t.Errorf("timer:\n%s", timer)
	}
	// what nt doctor reads back: the binary the job runs
	if got := plistBin(p); got != tm.Bin {
		t.Errorf("plistBin = %q, want %q", got, tm.Bin)
	}
	if got := plistBin(Timer{Bin: "/a&b/nt", Interval: time.Minute}.plist()); got != "/a&b/nt" {
		t.Errorf("plistBin of an escaped path = %q", got)
	}
	if got := serviceBin(service); got != tm.Bin {
		t.Errorf("serviceBin = %q, want %q", got, tm.Bin)
	}
	if got := serviceBin("[Service]\nExecStart=/usr/bin/nt sync --background\n"); got != "/usr/bin/nt" {
		t.Errorf("serviceBin unquoted = %q", got)
	}
	home := t.TempDir()
	if got := (Timer{Home: home}).InstalledBin(); got != "" {
		t.Errorf("no job installed, yet InstalledBin = %q", got)
	}
}
