package gitsync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w4jnl/nt/internal/vault"
)

// world is a bare repository standing in for GitHub and clones of it, each a vault on its own
// host, as in docs/design.md Verification.
type world struct {
	t      *testing.T
	remote string
	now    time.Time
}

func newWorld(t *testing.T) *world {
	t.Helper()
	dir := t.TempDir()
	w := &world{t: t, remote: filepath.Join(dir, "remote.git"), now: time.Date(2026, 10, 26, 9, 30, 0, 0, time.Local)}
	w.git(dir, "init", "--quiet", "--bare", "-b", "main", w.remote)
	return w
}

func (w *world) git(dir string, args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		w.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// clone makes a working copy on host. The first one seeds the remote with nt's files.
func (w *world) clone(host string) *vault.Vault {
	w.t.Helper()
	dir := filepath.Join(filepath.Dir(w.remote), host)
	w.git(filepath.Dir(w.remote), "clone", "--quiet", w.remote, dir)
	w.git(dir, "config", "user.name", host)
	w.git(dir, "config", "user.email", host+"@example.com")
	w.git(dir, "config", "commit.gpgsign", "false")
	v, err := vault.Open(dir, host)
	if err != nil {
		w.t.Fatal(err)
	}
	v.Now = func() time.Time { return w.now }
	return v
}

func (w *world) seeded(hosts ...string) []*vault.Vault {
	first := w.clone(hosts[0])
	write(w.t, first, ".gitattributes", "daily/** merge=union\n")
	write(w.t, first, ".gitignore", ".nt-tmp-*\n")
	write(w.t, first, "tech/nas.md", "# NAS\n\nbase\n")
	write(w.t, first, "tech/gone.md", "# Gone\n")
	if st := sync(w.t, first); st.State != "synced" {
		w.t.Fatalf("seed: %+v", st)
	}
	vs := []*vault.Vault{first}
	for _, h := range hosts[1:] {
		vs = append(vs, w.clone(h))
	}
	return vs
}

func write(t *testing.T, v *vault.Vault, rel, s string) {
	t.Helper()
	if err := vault.WriteFile(v.Abs(rel), []byte(s)); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, v *vault.Vault, rel string) string {
	t.Helper()
	b, err := os.ReadFile(v.Abs(rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func sync(t *testing.T, v *vault.Vault) Status {
	t.Helper()
	return Run(v, Options{Wait: true, Retry: time.Millisecond})
}

func noMarkers(t *testing.T, v *vault.Vault) {
	t.Helper()
	_ = filepath.WalkDir(v.Root, func(p string, d os.DirEntry, err error) error {
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "<<<<<<<") || strings.Contains(string(b), ">>>>>>>") {
				t.Errorf("conflict markers in %s", p)
			}
		}
		return nil
	})
}

func TestBothSidesEditKeepsRemoteAndACopy(t *testing.T) {
	w := newWorld(t)
	vs := w.seeded("alpha", "beta")
	a, b := vs[0], vs[1]
	write(t, a, "tech/nas.md", "# NAS\n\nalpha's version\n")
	write(t, b, "tech/nas.md", "# NAS\n\nbeta's version\n")
	if st := sync(t, a); st.State != "synced" {
		t.Fatalf("alpha: %+v", st)
	}
	st := sync(t, b)
	copyName := "tech/nas.conflict-beta-20261026T093000.md"
	if st.State != "conflict" || len(st.NewConflicts) != 1 || st.NewConflicts[0] != copyName {
		t.Fatalf("beta: %+v", st)
	}
	if got := read(t, b, "tech/nas.md"); !strings.Contains(got, "alpha's version") {
		t.Errorf("the remote version should stay in place: %q", got)
	}
	if got := read(t, b, copyName); !strings.Contains(got, "beta's version") {
		t.Errorf("the local version should be in the copy: %q", got)
	}
	noMarkers(t, b)
	if st := sync(t, a); st.State != "conflict" || len(st.Conflicts) != 1 {
		t.Errorf("the copy should reach alpha and show as a conflict there: %+v", st)
	}
	if read(t, a, copyName) != read(t, b, copyName) {
		t.Error("the copy differs between the clones")
	}
	// the person merges the copy into the note and deletes it: back to synced
	_ = os.Remove(a.Abs(copyName))
	if st := sync(t, a); st.State != "synced" || len(st.Conflicts) != 0 {
		t.Errorf("after deleting the copy: %+v", st)
	}
}

func TestDeleteAgainstEditKeepsTheEdit(t *testing.T) {
	w := newWorld(t)
	vs := w.seeded("alpha", "beta")
	a, b := vs[0], vs[1]
	_ = os.Remove(a.Abs("tech/gone.md"))
	write(t, b, "tech/gone.md", "# Gone\n\nbut edited\n")
	sync(t, a)
	if st := sync(t, b); st.State != "synced" {
		t.Fatalf("beta: %+v", st)
	}
	sync(t, a)
	if got := read(t, a, "tech/gone.md"); !strings.Contains(got, "but edited") {
		t.Errorf("the edit should win on alpha too: %q", got)
	}
	// and the other way round: deleted remotely after an edit here
	write(t, a, "tech/nas.md", "# NAS\n\nedited on alpha\n")
	_ = os.Remove(b.Abs("tech/nas.md"))
	sync(t, b)
	if st := sync(t, a); st.State != "synced" {
		t.Fatalf("alpha: %+v", st)
	}
	if got := read(t, a, "tech/nas.md"); !strings.Contains(got, "edited on alpha") {
		t.Errorf("the local edit should win: %q", got)
	}
	noMarkers(t, a)
}

func TestJournalsMergeLineByLine(t *testing.T) {
	w := newWorld(t)
	vs := w.seeded("alpha", "beta")
	a, b := vs[0], vs[1]
	at := time.Date(2026, 10, 26, 9, 0, 0, 0, time.Local)
	// both create the same day and append different lines
	if _, _, err := a.Append("from alpha", at); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Append("from beta", at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	sync(t, a)
	if st := sync(t, b); st.State != "synced" || len(st.NewConflicts) != 0 {
		t.Fatalf("beta: %+v", st)
	}
	sync(t, a)
	for _, v := range []*vault.Vault{a, b} {
		j := read(t, v, "daily/2026/2026-10-26.md")
		if !strings.Contains(j, "- 09:00 from alpha") || !strings.Contains(j, "- 09:01 from beta") || strings.Count(j, "## Log") != 1 {
			t.Errorf("%s journal:\n%s", v.Host, j)
		}
	}
	noMarkers(t, b)
}

func TestInterruptedMergeIsFinishedFirst(t *testing.T) {
	w := newWorld(t)
	vs := w.seeded("alpha", "beta")
	a, b := vs[0], vs[1]
	write(t, a, "tech/nas.md", "# NAS\n\nalpha\n")
	sync(t, a)
	write(t, b, "tech/nas.md", "# NAS\n\nbeta\n")
	w.git(b.Root, "add", "-A")
	w.git(b.Root, "commit", "--quiet", "-m", "local")
	w.git(b.Root, "fetch", "--quiet")
	// a crash in the middle: git merge stopped with markers in the file and MERGE_HEAD set
	cmd := exec.Command("git", "merge", "--no-edit", "origin/main")
	cmd.Dir = b.Root
	_ = cmd.Run()
	if !strings.Contains(read(t, b, "tech/nas.md"), "<<<<<<<") {
		t.Fatal("the setup did not leave a conflicted merge")
	}
	st := sync(t, b)
	if st.State != "conflict" || len(st.NewConflicts) != 1 || !st.Merged {
		t.Fatalf("beta: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(b.Root, ".git", "MERGE_HEAD")); err == nil {
		t.Error("the merge is still open")
	}
	noMarkers(t, b)
	if got := read(t, b, st.NewConflicts[0]); !strings.Contains(got, "beta") || strings.Contains(got, "<<<") {
		t.Errorf("the copy should hold beta's version, not the marked file: %q", got)
	}
}

func TestLargeFilesStayOut(t *testing.T) {
	w := newWorld(t)
	a := w.seeded("alpha")[0]
	big := make([]byte, MaxFile+1)
	_ = os.MkdirAll(filepath.Dir(a.Abs("tech/assets/video.mov")), 0o755)
	if err := os.WriteFile(a.Abs("tech/assets/video.mov"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, a, "tech/small.md", "x\n")
	st := sync(t, a)
	if st.State != "synced" || len(st.Skipped) != 1 || st.Skipped[0] != "tech/assets/video.mov" || st.Committed != 1 {
		t.Fatalf("%+v", st)
	}
	if out := w.git(a.Root, "ls-files", "tech"); strings.Contains(out, "video.mov") || !strings.Contains(out, "small.md") {
		t.Errorf("tracked: %s", out)
	}
}

func TestStopsOnStatesItMustNotRepair(t *testing.T) {
	w := newWorld(t)
	a := w.seeded("alpha")[0]
	head := w.git(a.Root, "rev-parse", "HEAD")
	w.git(a.Root, "checkout", "--quiet", head)
	if st := sync(t, a); st.State != "error" || !strings.Contains(st.Message, "detached") {
		t.Errorf("detached HEAD: %+v", st)
	}
	w.git(a.Root, "checkout", "--quiet", "main")
	w.git(a.Root, "remote", "set-head", "origin", "main")
	w.git(a.Root, "switch", "--quiet", "-c", "other")
	if st := sync(t, a); st.State != "error" || !strings.Contains(st.Message, "git switch main") {
		t.Errorf("wrong branch: %+v", st)
	}
	w.git(a.Root, "switch", "--quiet", "main")
	_ = os.MkdirAll(filepath.Join(a.Root, ".git", "rebase-merge"), 0o755)
	if st := sync(t, a); st.State != "error" || !strings.Contains(st.Message, "rebase") {
		t.Errorf("rebase in progress: %+v", st)
	}
}

func TestOfflineKeepsCommitting(t *testing.T) {
	w := newWorld(t)
	a := w.seeded("alpha")[0]
	w.git(a.Root, "remote", "set-url", "origin", "ssh://nt-test.invalid/notes.git")
	write(t, a, "tech/offline.md", "x\n")
	st := sync(t, a)
	if st.State != "offline" || st.Committed != 1 || st.Ahead != 1 {
		t.Fatalf("%+v", st)
	}
}

func TestNewRemoteAndNoRemote(t *testing.T) {
	w := newWorld(t)
	dir := filepath.Join(filepath.Dir(w.remote), "fresh")
	w.git(filepath.Dir(w.remote), "init", "--quiet", "-b", "main", dir)
	w.git(dir, "config", "user.name", "t")
	w.git(dir, "config", "user.email", "t@example.com")
	v, _ := vault.Open(dir, "fresh")
	write(t, v, "inbox/a.md", "a\n")
	if st := sync(t, v); st.State != "local" || st.Committed != 1 {
		t.Fatalf("no remote: %+v", st)
	}
	w.git(dir, "remote", "add", "origin", w.remote)
	if st := sync(t, v); st.State != "synced" || st.Ahead != 0 {
		t.Fatalf("first push to an empty remote: %+v", st)
	}
	if got := w.git(w.remote, "log", "--oneline", "main"); !strings.Contains(got, "nt: fresh · 1 file (inbox)") {
		t.Errorf("remote log: %s", got)
	}
}

func TestPushRaceIsRetried(t *testing.T) {
	w := newWorld(t)
	vs := w.seeded("alpha", "beta")
	a, b := vs[0], vs[1]
	// beta's push hook makes alpha push first, once: the remote moves on between beta's fetch
	// and its push, so the push is rejected and must be retried after a new merge
	mark := filepath.Join(t.TempDir(), "raced")
	hook := "#!/bin/sh\n[ -e " + mark + " ] && exit 0\ntouch " + mark + "\n" +
		"unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE\n" +
		"cd " + a.Root + " && echo a > inbox/a.md && git add -A && git commit -qm race && git push -q origin main\n"
	if err := os.WriteFile(filepath.Join(b.Root, ".git", "hooks", "pre-push"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(a.Abs("inbox"), 0o755)
	write(t, b, "inbox/b.md", "b\n")
	var log strings.Builder
	st := Run(b, Options{Wait: true, Retry: time.Millisecond, Log: &log})
	if st.State != "synced" || !st.Merged {
		t.Fatalf("beta after the race: %+v\n%s", st, log.String())
	}
	if !strings.Contains(log.String(), "push rejected") {
		t.Errorf("the push was never rejected, so nothing was retried:\n%s", log.String())
	}
	if got := w.git(w.remote, "ls-tree", "-r", "--name-only", "main"); !strings.Contains(got, "inbox/a.md") || !strings.Contains(got, "inbox/b.md") {
		t.Errorf("remote tree: %s", got)
	}
}

func TestCommitMessageAndConflictName(t *testing.T) {
	if got := commitMessage("mbp", []string{"daily/2026/x.md", "agents/a.md", "agents/b.md", "README.md", "tech/x.md"}); got != "nt: mbp · 5 files (README.md, agents, daily, …)" {
		t.Errorf("message %q", got)
	}
	at := time.Date(2026, 10, 26, 9, 3, 7, 0, time.Local)
	for _, tc := range []struct{ in, want string }{
		{"tech/nas.md", "tech/nas.conflict-mbp-20261026T090307.md"},
		{"img/a.b.png", "img/a.b.conflict-mbp-20261026T090307.png"},
		{"Makefile", "Makefile.conflict-mbp-20261026T090307"},
		{".gitignore", ".gitignore.conflict-mbp-20261026T090307"},
	} {
		if got := ConflictName(tc.in, "mbp", at); got != tc.want {
			t.Errorf("ConflictName(%s) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestPastJournalsInTimeOrder(t *testing.T) {
	w := newWorld(t)
	v := w.seeded("a")[0]
	yesterday := v.JournalRel(v.Day(w.now).AddDate(0, 0, -1))
	write(t, v, yesterday, "# D\n\n## Log\n\n- 10:05 from b\n- 10:01 from a\n")
	sync(t, v)
	if got := w.git(v.Root, "show", "HEAD:"+yesterday); got != "# D\n\n## Log\n\n- 10:01 from a\n- 10:05 from b" {
		t.Errorf("sync committed yesterday's journal as\n%s", got)
	}
}
