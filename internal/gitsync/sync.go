// Package gitsync is nt sync (docs/design.md §2, docs/format.md §10): the one process per
// machine that runs git commands that change a notes repository. It commits, fetches, merges
// and pushes; it never rebases and never leaves conflict markers: when both sides changed a file
// the remote version stays and the local one is kept beside it as a conflict copy.
package gitsync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/w4jnl/nt/internal/agents"
	"github.com/w4jnl/nt/internal/vault"
	"github.com/w4jnl/nt/internal/weekly"
)

// digestPath is a day's agent digest, inside the agents folder; weekPath a weekly digest, inside
// the weekly folder.
var (
	digestPath = regexp.MustCompile(`^\d{4}/\d{4}-\d{2}-\d{2}/README\.md$`)
	weekPath   = regexp.MustCompile(`^\d{4}/\d{4}-W\d{2}\.md$`)
)

// isDigest reports a file nt compiles: a day's agent digest or a weekly digest.
func (r *run) isDigest(p string) bool {
	if rest, ok := strings.CutPrefix(p, r.v.Cfg.Folders.Agents+"/"); ok && digestPath.MatchString(rest) {
		return true
	}
	rest, ok := strings.CutPrefix(p, r.v.Cfg.Folders.Weekly+"/")
	return ok && weekPath.MatchString(rest)
}

// MaxFile is the largest file sync commits; larger ones are reported instead.
const MaxFile = 20 << 20

// Options are one sync run's settings.
type Options struct {
	Wait     bool             // wait for a sync already running instead of leaving it to finish
	Notify   func(msg string) // tells the person about new conflict copies; nil: nobody
	Log      io.Writer        // progress, one line per step; nil: discard
	Retry    time.Duration    // base pause before retrying a rejected push; 0: 1 s
	Digest   bool             // this machine writes the agent digests (digest = true)
	Weekly   bool             // this machine compiles the weekly digest (weekly = true)
	Summary  weekly.Tool      // the tool that writes this machine's weekly summaries; Name "" none
	StateDir string           // the state folder: when a summary was last tried
	Git      Git              // the repository's plumbing; nil: git itself
}

// Status is the outcome of a run, written to status.json for doctor, the TUI and flok.
type Status struct {
	Repo         string    `json:"repo"`
	State        string    `json:"state"` // synced, unpushed, offline, conflict, error, local
	Branch       string    `json:"branch,omitempty"`
	Ahead        int       `json:"ahead"`
	Committed    int       `json:"committed"`               // files committed in this run
	Merged       bool      `json:"merged"`                  // commits came in from the remote
	NewConflicts []string  `json:"new_conflicts,omitempty"` // conflict copies this run made
	Conflicts    []string  `json:"conflicts,omitempty"`     // conflict copies in the repository
	Skipped      []string  `json:"skipped,omitempty"`       // files over 20 MB, not committed
	Message      string    `json:"message,omitempty"`       // what is wrong or waiting, and what to do
	At           time.Time `json:"at"`
	LastSynced   time.Time `json:"last_synced,omitzero"`
}

// errStop is a repository state sync must not repair (docs/format.md §10, Stopping).
type errStop struct{ msg string }

func (e errStop) Error() string { return e.msg }

type run struct {
	v      *vault.Vault
	g      Git
	o      Options
	st     *Status
	branch string
	remote bool
}

// Run syncs v once and returns what happened. It does not write status.json; the caller does.
func Run(v *vault.Vault, o Options) Status {
	st := Status{Repo: v.Root, At: v.Now()}
	if o.Log == nil {
		o.Log = io.Discard
	}
	if o.Retry == 0 {
		o.Retry = time.Second
	}
	release, ok, err := runnerLock(v, o.Wait)
	if err != nil {
		return fail(st, err)
	}
	if !ok {
		st.State, st.Message = "busy", "another nt sync is running"
		return st
	}
	defer release()
	g := o.Git
	if g == nil {
		g = newGit(v.Root)
	}
	r := &run{v: v, g: g, o: o, st: &st}
	if err := r.cycle(); err != nil {
		return fail(st, err)
	}
	return st
}

func fail(st Status, err error) Status {
	st.State, st.Message = "error", err.Error()
	var stop errStop
	if errors.As(err, &stop) {
		st.Message = stop.msg
	}
	return st
}

// runnerLock keeps sync runs on one repository one at a time (.git/nt-sync.lock).
func runnerLock(v *vault.Vault, wait bool) (func(), bool, error) {
	p := filepath.Join(v.Root, ".git", "nt-sync.lock")
	if !wait {
		return vault.TryLock(p)
	}
	for {
		rel, ok, err := vault.TryLock(p)
		if err != nil || ok {
			return rel, ok, err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (r *run) logf(format string, a ...any) { fmt.Fprintf(r.o.Log, format+"\n", a...) }

func (r *run) cycle() error {
	if err := r.preflight(); err != nil {
		return err
	}
	if err := r.locked(r.recover); err != nil {
		return err
	}
	if r.o.Digest { // before the commit, so a day's digest goes out with its reports
		days, err := agents.Refresh(r.v)
		if err != nil {
			return err
		}
		for _, d := range days {
			r.logf("digest: %s", d)
		}
	}
	if r.o.Summary.Name != "" { // before the digest, which shows it; a failure does not stop the sync
		wrote, err := weekly.RefreshSummaries(context.Background(), r.v, r.o.Summary, r.o.StateDir, r.v.Now())
		for _, w := range wrote {
			r.logf("summary: %s", w)
		}
		if err != nil {
			r.logf("%v; tried again in an hour", err)
		}
	}
	if r.o.Weekly { // likewise: a week's digest goes out with the notes it is made from
		weeks, err := weekly.Refresh(r.v, r.v.Now())
		if err != nil {
			return err
		}
		for _, w := range weeks {
			r.logf("weekly: %s", w)
		}
	}
	sorted, err := r.v.SortPastLogs(r.v.Now()) // merges can leave a day's lines out of time order
	if err != nil {
		return err
	}
	for _, rel := range sorted {
		r.logf("journal in time order: %s", rel)
	}
	if err := r.locked(r.commit); err != nil {
		return err
	}
	if !r.remote {
		r.st.State, r.st.Message = "local", "no remote: git remote add origin git@github.com:<you>/notes.git"
		return r.finish()
	}
	for attempt := 1; ; attempt++ {
		if err := r.g.Fetch(); err != nil {
			if errors.Is(err, ErrOffline) {
				return r.offline(err)
			}
			return err
		}
		upstream := r.g.Upstream(r.branch)
		if upstream {
			if r.v.Editing() && r.behind() > 0 {
				r.st.Message = "waiting for the editor to close before merging"
				return r.finish()
			}
			if err := r.locked(r.merge); err != nil {
				return err
			}
			if r.behind() == 0 && r.ahead() == 0 {
				break
			}
		}
		err := r.g.Push(r.branch, !upstream)
		if err == nil {
			r.logf("pushed %s", r.branch)
			break
		}
		if errors.Is(err, ErrOffline) {
			return r.offline(err)
		}
		if !errors.Is(err, ErrRejected) || attempt >= 5 {
			return err
		}
		pause := r.o.Retry + time.Duration(rand.Int64N(int64(r.o.Retry)))
		r.logf("push rejected, the remote moved on; trying again in %s", pause.Round(time.Millisecond))
		time.Sleep(pause)
	}
	return r.finish()
}

// preflight checks the repository is one sync may touch: on a branch, no rebase running, the
// branch the remote expects.
func (r *run) preflight() error {
	for _, d := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(r.v.Root, ".git", d)); err == nil {
			return errStop{"a rebase is in progress: finish it, or run git rebase --abort, then sync again"}
		}
	}
	b, err := r.g.Branch()
	if err != nil {
		return errStop{"HEAD is detached: check out the notes branch (git switch main), then sync again"}
	}
	r.branch, r.st.Branch = b, b
	r.remote = r.g.HasRemote()
	if want := r.g.RemoteBranch(); want != "" && want != b {
		return errStop{fmt.Sprintf("on branch %s, but the remote's branch is %s: git switch %s, then sync again", b, want, want)}
	}
	return nil
}

func (r *run) locked(fn func() error) error {
	unlock, err := r.v.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

// recover finishes a merge an earlier run (or a crash) left behind, before anything else.
func (r *run) recover() error {
	_, err := os.Stat(filepath.Join(r.v.Root, ".git", "MERGE_HEAD"))
	merging := err == nil
	unmerged, err := r.unmerged()
	if err != nil {
		return err
	}
	if !merging && len(unmerged) == 0 {
		return nil
	}
	r.logf("finishing an interrupted merge")
	if err := r.resolve(unmerged); err != nil {
		return err
	}
	if merging {
		if err := r.stageAll(); err != nil {
			return err
		}
		if err := r.g.Commit(""); err != nil {
			return err
		}
		r.st.Merged = true
	}
	return nil
}

// commit stages every change but files over MaxFile and commits them, under the lock.
func (r *run) commit() error {
	if err := r.stageAll(); err != nil {
		return err
	}
	files, err := r.g.Staged()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	msg := commitMessage(r.v.Host, files)
	if err := r.g.Commit(msg); err != nil {
		return err
	}
	r.st.Committed += len(files)
	r.logf("%s", msg)
	return nil
}

// stageAll is git add -A without temp files and without files over MaxFile.
func (r *run) stageAll() error {
	paths, err := r.g.Status()
	if err != nil {
		return err
	}
	var skipped []string
	for _, p := range paths {
		if st, err := os.Stat(filepath.Join(r.v.Root, p)); err == nil && st.Mode().IsRegular() && st.Size() > MaxFile {
			skipped = append(skipped, p)
		}
	}
	r.st.Skipped = skipped
	return r.g.Stage(skipped)
}

// commitMessage is "nt: <host> · 3 files (daily, agents)": the count and the top-level folders.
func commitMessage(host string, files []string) string {
	seen := map[string]bool{}
	var areas []string
	for _, f := range files {
		a, _, _ := strings.Cut(f, "/")
		if !seen[a] {
			seen[a] = true
			areas = append(areas, a)
		}
	}
	sort.Strings(areas)
	if len(areas) > 3 {
		areas = append(areas[:3], "…")
	}
	n := "1 file"
	if len(files) != 1 {
		n = strconv.Itoa(len(files)) + " files"
	}
	return fmt.Sprintf("nt: %s · %s (%s)", host, n, strings.Join(areas, ", "))
}

// merge brings in origin/<branch>, under the lock. Changes written since the commit are
// committed first, so the merge never meets a dirty file it would have to refuse.
func (r *run) merge() error {
	if err := r.commit(); err != nil {
		return err
	}
	if r.behind() == 0 {
		return nil
	}
	err := r.g.Merge(r.branch)
	r.st.Merged = true
	if err == nil {
		r.logf("merged origin/%s", r.branch)
		return nil
	}
	unmerged, uerr := r.unmerged()
	if uerr != nil || len(unmerged) == 0 {
		return err // a failure that is not a conflict
	}
	if err := r.resolve(unmerged); err != nil {
		return err
	}
	if err := r.stageAll(); err != nil {
		return err
	}
	if err := r.g.Commit(""); err != nil {
		return err
	}
	r.logf("merged origin/%s with %d conflicting file(s)", r.branch, len(unmerged))
	return nil
}

// unmerged lists the conflicted paths and the index stages each one has: 1 base, 2 ours
// (local), 3 theirs (remote).
func (r *run) unmerged() (map[string]map[int]string, error) { return r.g.Unmerged() }

// resolve settles every conflicted path without conflict markers (docs/format.md §10):
// changed on both sides, the remote version stays and the local one becomes a conflict copy;
// changed on one side and deleted on the other, the change wins.
func (r *run) resolve(unmerged map[string]map[int]string) error {
	paths := make([]string, 0, len(unmerged))
	for p := range unmerged {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		st := unmerged[p]
		ours, theirs := st[2], st[3]
		switch {
		case ours != "" && theirs != "" && r.isDigest(p):
			// a digest is made from its reports: take either, the digest machine writes it anew
			if err := r.g.Take(p, true); err != nil {
				return err
			}
			if err := r.g.Add(p); err != nil {
				return err
			}
		case ours != "" && theirs != "":
			local, err := r.g.Blob(ours)
			if err != nil {
				return err
			}
			copyRel := r.v.Unique(ConflictName(p, r.v.Host, r.v.Now()))
			if err := vault.WriteFile(r.v.Abs(copyRel), local); err != nil {
				return err
			}
			if err := r.g.Take(p, true); err != nil {
				return err
			}
			if err := r.g.Add(p, copyRel); err != nil {
				return err
			}
			r.st.NewConflicts = append(r.st.NewConflicts, copyRel)
			r.logf("conflict: %s changed here and on the remote; kept the remote version, yours is %s", p, copyRel)
		case theirs != "":
			if err := r.g.Take(p, true); err != nil {
				return err
			}
			if err := r.g.Add(p); err != nil {
				return err
			}
			r.logf("kept %s: changed on the remote, deleted here", p)
		case ours != "":
			if err := r.g.Take(p, false); err != nil {
				return err
			}
			if err := r.g.Add(p); err != nil {
				return err
			}
			r.logf("kept %s: changed here, deleted on the remote", p)
		default:
			_ = os.Remove(r.v.Abs(p))
			if err := r.g.Forget(p); err != nil {
				return err
			}
		}
	}
	if len(r.st.NewConflicts) > 0 && r.o.Notify != nil {
		r.o.Notify(fmt.Sprintf("%d conflict copy(s) in your notes, e.g. %s", len(r.st.NewConflicts), r.st.NewConflicts[0]))
	}
	return nil
}

// ConflictName is the conflict copy of rel made on host at t (docs/format.md §3):
// <stem>.conflict-<host>-<YYYYMMDDTHHMMSS><ext>, beside the original.
func ConflictName(rel, host string, t time.Time) string {
	dir, base := path.Split(rel)
	ext := path.Ext(base)
	if ext == base { // a dot file: no extension
		ext = ""
	}
	return dir + strings.TrimSuffix(base, ext) + ".conflict-" + host + "-" + t.Format("20060102T150405") + ext
}

func (r *run) behind() int { return r.count("HEAD..origin/" + r.branch) }
func (r *run) ahead() int  { return r.count("origin/" + r.branch + "..HEAD") }

func (r *run) count(rng string) int { return r.g.Count(rng) }

func (r *run) offline(err error) error {
	r.st.State, r.st.Message = "offline", "offline: "+firstLines(strings.TrimSpace(stderrOf(err)), 1)
	r.st.Ahead = r.ahead()
	r.st.Conflicts = r.conflictCopies()
	return nil
}

// finish sets the state from where the repository stands now.
func (r *run) finish() error {
	r.st.Conflicts = r.conflictCopies()
	if r.remote && r.g.Upstream(r.branch) {
		r.st.Ahead = r.ahead()
	} else if r.remote {
		r.st.Ahead = r.count("HEAD")
	}
	switch {
	case r.st.State == "local":
	case len(r.st.Conflicts) > 0:
		r.st.State = "conflict"
		if r.st.Message == "" {
			r.st.Message = "merge each conflict copy into its note, then delete the copy"
		}
	case r.st.Ahead > 0:
		r.st.State = "unpushed"
	default:
		r.st.State = "synced"
	}
	if r.st.State == "synced" || r.st.State == "conflict" && r.st.Ahead == 0 {
		r.st.LastSynced = r.st.At
	}
	return nil
}

// conflictCopies lists the conflict copies committed in the repository.
func (r *run) conflictCopies() []string {
	files, err := r.g.Files()
	if err != nil {
		return nil
	}
	var c []string
	for _, f := range files {
		if strings.Contains(path.Base(f), ".conflict-") {
			c = append(c, f)
		}
	}
	return c
}
