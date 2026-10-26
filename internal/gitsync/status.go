package gitsync

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/w4jnl/nt/internal/vault"
)

// StatusPath is status.json in the state directory.
func StatusPath(stateDir string) string { return filepath.Join(stateDir, "status.json") }

// ReadStatus reads the last run's status; ok is false when there is none.
func ReadStatus(stateDir string) (Status, bool) {
	var st Status
	b, err := os.ReadFile(StatusPath(stateDir))
	if err != nil || json.Unmarshal(b, &st) != nil {
		return st, false
	}
	return st, true
}

// WriteStatus records st, keeping the last successful sync time of the previous run when this
// one did not reach the remote. A "busy" run (another sync was running) records nothing.
func WriteStatus(stateDir string, st Status) error {
	if st.State == "busy" {
		return nil
	}
	if prev, ok := ReadStatus(stateDir); ok && st.LastSynced.IsZero() && prev.Repo == st.Repo {
		st.LastSynced = prev.LastSynced
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return vault.WriteFile(StatusPath(stateDir), append(b, '\n'))
}

// Notify shows an OS notification: osascript on macOS, notify-send on Linux; nothing when
// neither is there.
func Notify(msg string) {
	switch runtime.GOOS {
	case "darwin":
		q := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(msg)
		_ = exec.Command("osascript", "-e", `display notification "`+q+`" with title "nt"`).Run()
	default:
		if p, err := exec.LookPath("notify-send"); err == nil {
			_ = exec.Command(p, "nt", msg).Run()
		}
	}
}
