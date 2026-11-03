package gitsync

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Git is the plumbing sync needs from a repository; everything else sync does (the commit
// message, conflict copies, the digest rule, journals in time order) is policy, in sync.go. The
// command line's is git itself (execGit); the iOS app's is libgit2 (docs/design.md §9).
type Git interface {
	Branch() (string, error)                      // the branch checked out; an error when HEAD is detached
	RemoteBranch() string                         // the branch origin/HEAD names, "" when it names none
	HasRemote() bool                              // origin is set
	Upstream(branch string) bool                  // refs/remotes/origin/<branch> exists
	Status() ([]string, error)                    // the paths changed against HEAD, untracked ones too
	Stage(exclude []string) error                 // stage every change but temp files and the paths in exclude
	Staged() ([]string, error)                    // the paths staged for the next commit
	Commit(msg string) error                      // commit what is staged; "" finishes a merge with its own message
	Fetch() error                                 // fetch origin, pruning; ErrOffline when it cannot reach it
	Push(branch string, first bool) error         // push HEAD to origin/<branch> (first: set the upstream); ErrOffline, ErrRejected
	Merge(branch string) error                    // merge origin/<branch>, fast-forward when it can; conflicts stay in the index
	Unmerged() (map[string]map[int]string, error) // conflicted path → index stage (1 base, 2 ours, 3 theirs) → blob id
	Blob(id string) ([]byte, error)               // a blob's bytes
	Take(path string, theirs bool) error          // the worktree's path from the index's theirs (3) or ours (2)
	Add(paths ...string) error                    // stage these paths
	Forget(path string) error                     // unstage and untrack path, when it is there
	Count(rng string) int                         // commits in rng: "A..B", or "HEAD" for all of them
	Files() ([]string, error)                     // the tracked paths
	History() ([]Entry, error)                    // every commit but the merges, newest first: when, its subject, the paths it changed
}

// Entry is one commit of the history: when it was made, its subject and the paths it changed.
// On the wire between the apps and nt it is "<unix seconds>\t<subject>" and then one path per
// line, entries separated by NUL (ParseHistory), the shape git log --name-only prints.
type Entry struct {
	At      time.Time
	Subject string
	Files   []string
}

// Change is the last change git's history knows for a path: when, and the host whose nt
// committed it ("" for a commit nt did not make).
type Change struct {
	At time.Time
	By string
}

// Changes is the last change of every path in the history: the newest commit that touched it.
// A note written since the last sync keeps its earlier change until sync commits it.
func Changes(g Git) (map[string]Change, error) {
	entries, err := g.History()
	if err != nil {
		return nil, err
	}
	out := map[string]Change{}
	for _, e := range entries {
		for _, f := range e.Files {
			if _, seen := out[f]; !seen {
				out[f] = Change{At: e.At, By: HostOf(e.Subject)}
			}
		}
	}
	return out, nil
}

// HostOf is the host in a subject sync wrote, "nt: <host> · 3 files (daily)"; "" for any other.
func HostOf(subject string) string {
	rest, ok := strings.CutPrefix(subject, "nt: ")
	if !ok {
		return ""
	}
	host, _, _ := strings.Cut(rest, " · ")
	return strings.TrimSpace(host)
}

// ParseHistory reads entries off the wire, newest first as they come.
func ParseHistory(s string) []Entry {
	var out []Entry
	for _, chunk := range strings.Split(s, "\x00") {
		lines := strings.Split(strings.Trim(chunk, "\n"), "\n")
		head, subject, _ := strings.Cut(lines[0], "\t")
		secs, err := strconv.ParseInt(strings.TrimSpace(head), 10, 64)
		if err != nil {
			continue
		}
		e := Entry{At: time.Unix(secs, 0), Subject: subject}
		for _, l := range lines[1:] {
			if l = strings.TrimSpace(l); l != "" {
				e.Files = append(e.Files, l)
			}
		}
		out = append(out, e)
	}
	return out
}

// ErrOffline is a fetch or push that could not reach the remote, for the network or for
// credentials: the next cycle tries again, nothing is wrong with the repository.
var ErrOffline = errors.New("offline")

// ErrRejected is a push that lost a race: the remote moved on since the last fetch.
var ErrRejected = errors.New("push rejected")

// execGit runs git in one repository, never interactively: no terminal prompts, no editor, ssh
// in batch mode unless the person set their own ssh command, and English messages for parsing.
type execGit struct {
	dir string
	env []string
}

// Exec is git itself in the repository at dir, as nt sync runs it.
func Exec(dir string) Git { return newGit(dir) }

func newGit(dir string) execGit {
	env := append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_MERGE_AUTOEDIT=no")
	g := execGit{dir: dir, env: env}
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		if _, err := g.run("config", "--get", "core.sshCommand"); err != nil {
			g.env = append(g.env, "GIT_SSH_COMMAND=ssh -oBatchMode=yes -oConnectTimeout=10")
		}
	}
	return g
}

func (g execGit) Branch() (string, error) { return g.run("symbolic-ref", "--quiet", "--short", "HEAD") }

func (g execGit) RemoteBranch() string {
	b, err := g.run("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(b, "origin/")
}

func (g execGit) HasRemote() bool { return g.ok("remote", "get-url", "origin") }

func (g execGit) Upstream(branch string) bool {
	return g.ok("rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch)
}

func (g execGit) Status() ([]string, error) {
	out, err := g.bytes("status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var paths []string
	fields := splitZ(out)
	for i := 0; i < len(fields); i++ {
		e := fields[i]
		if len(e) < 4 {
			continue
		}
		if e[0] == 'R' || e[0] == 'C' {
			i++ // the source path follows
		}
		paths = append(paths, e[3:])
	}
	return paths, nil
}

func (g execGit) Stage(exclude []string) error {
	args := []string{"add", "-A", "--", ".", ":(exclude,glob)**/.nt-tmp-*"}
	for _, p := range exclude {
		args = append(args, ":(exclude,literal)"+p)
	}
	_, err := g.run(args...)
	return err
}

func (g execGit) Staged() ([]string, error) {
	out, err := g.bytes("diff", "--cached", "--name-only", "-z")
	return splitZ(out), err
}

func (g execGit) Commit(msg string) error {
	if msg == "" {
		return g.quiet("commit", "--quiet", "--no-edit")
	}
	return g.quiet("commit", "--quiet", "-m", msg)
}

func (g execGit) Fetch() error { return classify(g.quiet("fetch", "--quiet", "--prune", "origin")) }

func (g execGit) Push(branch string, first bool) error {
	if first {
		return classify(g.quiet("push", "--quiet", "-u", "origin", branch))
	}
	return classify(g.quiet("push", "--quiet", "origin", "HEAD:refs/heads/"+branch))
}

func (g execGit) Merge(branch string) error {
	return g.quiet("merge", "--quiet", "--no-edit", "--ff", "origin/"+branch)
}

func (g execGit) Unmerged() (map[string]map[int]string, error) {
	out, err := g.bytes("ls-files", "-u", "-z")
	if err != nil {
		return nil, err
	}
	m := map[string]map[int]string{}
	for _, e := range splitZ(out) {
		meta, p, ok := strings.Cut(e, "\t")
		if !ok {
			continue
		}
		f := strings.Fields(meta) // mode sha stage
		if len(f) != 3 {
			continue
		}
		stage, _ := strconv.Atoi(f[2])
		if m[p] == nil {
			m[p] = map[int]string{}
		}
		m[p][stage] = f[1]
	}
	return m, nil
}

func (g execGit) Blob(id string) ([]byte, error) { return g.bytes("cat-file", "blob", id) }

func (g execGit) Take(path string, theirs bool) error {
	side := "--ours"
	if theirs {
		side = "--theirs"
	}
	return g.quiet("checkout", side, "--", path)
}

func (g execGit) Add(paths ...string) error {
	return g.quiet(append([]string{"add", "--"}, paths...)...)
}

func (g execGit) Forget(path string) error {
	return g.quiet("rm", "--quiet", "--cached", "--ignore-unmatch", "--", path)
}

func (g execGit) Count(rng string) int {
	out, err := g.run("rev-list", "--count", rng)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(out)
	return n
}

func (g execGit) Files() ([]string, error) {
	out, err := g.bytes("ls-files", "-z")
	return splitZ(out), err
}

// History is git log --name-only without the merges, in the wire shape; nothing before the
// first commit.
func (g execGit) History() ([]Entry, error) {
	if !g.ok("rev-parse", "--verify", "--quiet", "HEAD") {
		return nil, nil
	}
	out, err := g.bytes("log", "--no-merges", "--no-renames", "--name-only", "--format=%x00%ct%x09%s")
	if err != nil {
		return nil, err
	}
	return ParseHistory(string(out)), nil
}

// classify marks a failed fetch or push as offline or rejected, keeping git's message.
func classify(err error) error {
	switch {
	case err == nil:
		return nil
	case offline(err):
		return fmt.Errorf("%w: %w", ErrOffline, err)
	case rejected(err):
		return fmt.Errorf("%w: %w", ErrRejected, err)
	}
	return err
}

// gitError is a failed git command with what it printed on stderr.
type gitError struct {
	args   []string
	stderr string
	err    error
}

func (e *gitError) Error() string {
	msg := strings.TrimSpace(e.stderr)
	if msg == "" {
		msg = e.err.Error()
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.args, " "), firstLines(msg, 3))
}

func (e *gitError) Unwrap() error { return e.err }

// run returns stdout with the trailing newline removed.
func (g execGit) run(args ...string) (string, error) {
	out, err := g.bytes(args...)
	return strings.TrimRight(string(out), "\n"), err
}

// quiet runs a command whose stdout nobody needs.
func (g execGit) quiet(args ...string) error {
	_, err := g.bytes(args...)
	return err
}

func (g execGit) bytes(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = g.dir, g.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), &gitError{args: args, stderr: stderr.String(), err: err}
	}
	return stdout.Bytes(), nil
}

// ok runs a command whose exit status is the answer (git remote get-url, rev-parse --verify).
func (g execGit) ok(args ...string) bool {
	_, err := g.bytes(args...)
	return err == nil
}

// stderrOf is the stderr of a failed git command, lowercased, for classifying it and for the
// status line; another Git's error is its message.
func stderrOf(err error) string {
	var ge *gitError
	if errors.As(err, &ge) {
		return strings.ToLower(ge.stderr)
	}
	if err == nil {
		return ""
	}
	return strings.ToLower(err.Error())
}

// offline reports a fetch or push that failed for the network or for credentials.
func offline(err error) bool {
	s := stderrOf(err)
	for _, m := range []string{
		"could not resolve host", "connection refused", "connection timed out", "operation timed out",
		"network is unreachable", "no route to host", "could not read from remote repository",
		"permission denied", "host key verification failed", "connection closed", "connection reset",
		"unable to access", "the remote end hung up", "temporary failure in name resolution",
	} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// rejected reports a push that lost a race.
func rejected(err error) bool {
	s := stderrOf(err)
	return strings.Contains(s, "non-fast-forward") || strings.Contains(s, "fetch first") ||
		strings.Contains(s, "[rejected]") || strings.Contains(s, "failed to push some refs")
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, " / ")
}

func splitZ(b []byte) []string {
	s := strings.TrimRight(string(b), "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}
