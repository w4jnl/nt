package mobile

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/w4jnl/nt/internal/gitsync"
)

// viaExec is the app's side of Git played by git itself: lists joined with NULs, conflicts as
// "<stage> <id> <path>", offline and rejected as message prefixes, as the Swift one does.
type viaExec struct{ g gitsync.Git }

func (x viaExec) Branch() (string, error)     { return x.g.Branch() }
func (x viaExec) RemoteBranch() string        { return x.g.RemoteBranch() }
func (x viaExec) HasRemote() bool             { return x.g.HasRemote() }
func (x viaExec) Upstream(branch string) bool { return x.g.Upstream(branch) }
func (x viaExec) Commit(msg string) error     { return x.g.Commit(msg) }
func (x viaExec) Merge(branch string) error   { return x.g.Merge(branch) }
func (x viaExec) Forget(path string) error    { return x.g.Forget(path) }
func (x viaExec) Count(rng string) int        { return x.g.Count(rng) }

func (x viaExec) Blob(id string) ([]byte, error)      { return x.g.Blob(id) }
func (x viaExec) Take(path string, theirs bool) error { return x.g.Take(path, theirs) }
func (x viaExec) Stage(exclude string) error          { return x.g.Stage(split(exclude)) }
func (x viaExec) Add(paths string) error              { return x.g.Add(split(paths)...) }

func (x viaExec) Status() (string, error) { l, err := x.g.Status(); return Join(l), err }
func (x viaExec) Staged() (string, error) { l, err := x.g.Staged(); return Join(l), err }
func (x viaExec) Files() (string, error)  { l, err := x.g.Files(); return Join(l), err }

func (x viaExec) Fetch() error { return prefixed(x.g.Fetch()) }

func (x viaExec) Push(branch string, first bool) error { return prefixed(x.g.Push(branch, first)) }

func (x viaExec) Unmerged() (string, error) {
	m, err := x.g.Unmerged()
	var items []string
	for p, stages := range m {
		for stage, id := range stages {
			items = append(items, strconv.Itoa(stage)+" "+id+" "+p)
		}
	}
	return Join(items), err
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

func prefixed(err error) error {
	switch {
	case errors.Is(err, gitsync.ErrOffline):
		return errors.New("offline: " + err.Error())
	case errors.Is(err, gitsync.ErrRejected):
		return errors.New("rejected: " + err.Error())
	}
	return err
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// TestTheCoreSyncsThroughTheAppsGit: the phone's core, its git the app's (here git itself behind
// the same strings Swift passes), syncs as nt sync does: a note changed on both sides keeps the
// remote version and a conflict copy, and the journals merge line by line.
func TestTheCoreSyncsThroughTheAppsGit(t *testing.T) {
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	git(t, base, "init", "--quiet", "--bare", "-b", "main", remote)
	clone := func(host string) string {
		dir := filepath.Join(base, host)
		git(t, base, "clone", "--quiet", remote, dir)
		for k, v := range map[string]string{"user.name": host, "user.email": host + "@example.com", "commit.gpgsign": "false"} {
			git(t, dir, "config", k, v)
		}
		return dir
	}
	mac := clone("mac")
	for p, body := range map[string]string{".gitattributes": "daily/** merge=union\n", "tech/nas.md": "# NAS\n\nbase\n", "daily/2026/2026-10-28.md": "# Day\n\n## Log\n\n- 09:00 start\n"} {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(mac, p)), 0o755)
		if err := os.WriteFile(filepath.Join(mac, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, mac, "add", "-A")
	git(t, mac, "commit", "--quiet", "-m", "seed")
	git(t, mac, "push", "--quiet", "-u", "origin", "main")
	phone := clone("iphone")

	write := func(dir, p, body string) {
		if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(mac, "tech/nas.md", "# NAS\n\nfrom the mac\n")
	write(mac, "daily/2026/2026-10-28.md", "# Day\n\n## Log\n\n- 09:00 start\n- 10:00 from the mac\n")
	git(t, mac, "commit", "--quiet", "-am", "mac")
	git(t, mac, "push", "--quiet")
	write(phone, "tech/nas.md", "# NAS\n\nfrom the phone\n")
	write(phone, "daily/2026/2026-10-28.md", "# Day\n\n## Log\n\n- 09:00 start\n- 10:05 from the phone\n")

	config := filepath.Join(base, "config.toml")
	write(base, "config.toml", "repo = \""+phone+"\"\nhost = \"iphone\"\n[sync]\nauto = false\n")
	core := NewCore(config, filepath.Join(base, "state"), phone, viaExec{gitsync.Exec(phone)})
	r := core.Run(`["sync","--json"]`, nil)
	if r.Status != 0 && r.Stderr != "" {
		t.Fatalf("sync: %d %s", r.Status, r.Stderr)
	}
	var st gitsync.Status
	if err := json.Unmarshal(r.Stdout, &st); err != nil {
		t.Fatalf("sync printed %q: %v", r.Stdout, err)
	}
	if st.State != "conflict" || len(st.NewConflicts) != 1 || !strings.HasPrefix(st.NewConflicts[0], "tech/nas.conflict-iphone-") {
		t.Fatalf("status %+v", st)
	}
	read := func(p string) string { b, _ := os.ReadFile(filepath.Join(phone, p)); return string(b) }
	if read("tech/nas.md") != "# NAS\n\nfrom the mac\n" || read(st.NewConflicts[0]) != "# NAS\n\nfrom the phone\n" {
		t.Errorf("the remote version does not stay, or the phone's is not kept beside it")
	}
	if j := read("daily/2026/2026-10-28.md"); !strings.Contains(j, "from the mac") || !strings.Contains(j, "from the phone") || strings.Contains(j, "<<<<<<<") {
		t.Errorf("the journal does not merge line by line:\n%s", j)
	}

	// the other commands answer as nt does, and a broken command line says so
	if r := core.Run(`["todo","add","--json","--","Renew the domain"]`, nil); r.Status != 0 || !strings.Contains(string(r.Stdout), `"text": "Renew the domain"`) {
		t.Errorf("todo add: %d %s %s", r.Status, r.Stderr, r.Stdout)
	}
	if r := core.Run(`not json`, nil); r.Status != 2 {
		t.Errorf("a broken command line: %+v", r)
	}
	if err := core.Hold(); err != nil {
		t.Fatal(err)
	}
	core.Unhold()
}

func TestOfflineAndRejectedAreTyped(t *testing.T) {
	if err := typed(errors.New("offline: could not resolve host github.com")); !errors.Is(err, gitsync.ErrOffline) || err.Error() != "could not resolve host github.com" {
		t.Errorf("offline: %v", err)
	}
	if err := typed(errors.New("rejected: the remote moved on")); !errors.Is(err, gitsync.ErrRejected) {
		t.Errorf("rejected: %v", err)
	}
	if err := typed(errors.New("disk full")); errors.Is(err, gitsync.ErrOffline) || errors.Is(err, gitsync.ErrRejected) {
		t.Errorf("another error: %v", err)
	}
}
