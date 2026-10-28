// Package mobile is nt inside the iOS app (docs/design.md §9), built with gomobile into
// NtCore.xcframework. The app asks it the command lines the Mac app runs nt with (Core.Run) and
// reads the same JSON; git is the app's own, libgit2, through Git. Lists cross into Swift as
// strings of NUL-separated items, since gomobile passes no slices of strings.
package mobile

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/w4jnl/nt/internal/gitsync"
	"github.com/w4jnl/nt/internal/ops"
)

// Git is the repository's plumbing as the app implements it on libgit2: gitsync.Git with lists
// as NUL-separated strings. A fetch or push that cannot reach the remote fails with an error
// whose message starts "offline: "; a push the remote refuses as behind it, "rejected: ".
type Git interface {
	Branch() (string, error)
	RemoteBranch() string
	HasRemote() bool
	Upstream(branch string) bool
	Status() (string, error)
	Stage(exclude string) error
	Staged() (string, error)
	Commit(msg string) error
	Fetch() error
	Push(branch string, first bool) error
	Merge(branch string) error
	// Unmerged is one item per conflicted index entry: "<stage> <blob id> <path>".
	Unmerged() (string, error)
	Blob(id string) ([]byte, error)
	Take(path string, theirs bool) error
	Add(paths string) error
	Forget(path string) error
	Count(rng string) int
	Files() (string, error)
}

// Result is what nt printed and how it ended: the command's standard output, its error message
// (what nt prints after "nt: "), and its exit status. (Not Stdout and Stderr: C keeps those names.)
type Result struct {
	Output  []byte
	Message string
	Status  int
}

// Core is nt for one repository on the phone.
type Core struct {
	runner ops.Runner
	mu     sync.Mutex
	hold   func()
}

// NewCore points nt at the app's own config file and state folder and at the clone, and takes
// git from the app. Call it once: nt reads them from the process's environment.
func NewCore(config, state, repo string, git Git) *Core {
	_ = os.Setenv("NT_CONFIG", config)
	_ = os.Setenv("NT_STATE", state)
	_ = os.Setenv("NT_REPO", repo)
	return &Core{runner: ops.Runner{Git: adapter{git}}}
}

// Run runs a command line given as a JSON array of strings, with input as its standard input.
func (c *Core) Run(argsJSON string, input []byte) *Result {
	var args []string
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return &Result{Message: "the command line is no JSON array of strings: " + err.Error(), Status: 2}
	}
	out, code, err := c.runner.Run(args, input)
	r := &Result{Output: out, Status: code}
	if err != nil {
		r.Message = err.Error()
	}
	return r
}

// Hold holds the repository's edit lock (.git/nt-edit) until Unhold, so sync commits but does
// not merge under a note with unsaved changes, as nt _editor hold does for the Mac app.
func (c *Core) Hold() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hold != nil {
		return nil
	}
	v, _, err := ops.Open()
	if err != nil {
		return err
	}
	release, err := v.LockEdit()
	if err != nil {
		return err
	}
	c.hold = release
	return nil
}

// Unhold lets the edit lock go. (Not Release: Objective-C keeps that name for memory.)
func (c *Core) Unhold() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hold != nil {
		c.hold()
		c.hold = nil
	}
}

// adapter is the app's Git as gitsync.Git.
type adapter struct{ g Git }

func (a adapter) Branch() (string, error)     { return a.g.Branch() }
func (a adapter) RemoteBranch() string        { return a.g.RemoteBranch() }
func (a adapter) HasRemote() bool             { return a.g.HasRemote() }
func (a adapter) Upstream(branch string) bool { return a.g.Upstream(branch) }
func (a adapter) Commit(msg string) error     { return a.g.Commit(msg) }
func (a adapter) Merge(branch string) error   { return a.g.Merge(branch) }
func (a adapter) Forget(path string) error    { return a.g.Forget(path) }
func (a adapter) Count(rng string) int        { return a.g.Count(rng) }

func (a adapter) Blob(id string) ([]byte, error) { return a.g.Blob(id) }

func (a adapter) Take(path string, theirs bool) error { return a.g.Take(path, theirs) }

func (a adapter) Status() ([]string, error) { return list(a.g.Status()) }
func (a adapter) Staged() ([]string, error) { return list(a.g.Staged()) }
func (a adapter) Files() ([]string, error)  { return list(a.g.Files()) }

func (a adapter) Stage(exclude []string) error { return a.g.Stage(Join(exclude)) }
func (a adapter) Add(paths ...string) error    { return a.g.Add(Join(paths)) }

func (a adapter) Fetch() error { return typed(a.g.Fetch()) }

func (a adapter) Push(branch string, first bool) error { return typed(a.g.Push(branch, first)) }

func (a adapter) Unmerged() (map[string]map[int]string, error) {
	items, err := list(a.g.Unmerged())
	if err != nil {
		return nil, err
	}
	m := map[string]map[int]string{}
	for _, it := range items {
		f := strings.SplitN(it, " ", 3)
		if len(f) != 3 {
			continue
		}
		stage, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		if m[f[2]] == nil {
			m[f[2]] = map[int]string{}
		}
		m[f[2]][stage] = f[1]
	}
	return m, nil
}

// Join makes a list a NUL-separated string, as Git passes lists.
func Join(items []string) string { return strings.Join(items, "\x00") }

func list(s string, err error) ([]string, error) {
	if err != nil || s == "" {
		return nil, err
	}
	items := strings.Split(strings.TrimRight(s, "\x00"), "\x00")
	sort.Strings(items)
	return items, nil
}

// typed makes the app's "offline: " and "rejected: " errors the ones sync acts on.
func typed(err error) error {
	if err == nil {
		return nil
	}
	for prefix, kind := range map[string]error{"offline: ": gitsync.ErrOffline, "rejected: ": gitsync.ErrRejected} {
		if msg, ok := strings.CutPrefix(err.Error(), prefix); ok {
			return kindError{kind, msg}
		}
	}
	return err
}

// kindError is an error of kind (offline, rejected) that says only msg: sync's status line
// names the kind itself.
type kindError struct {
	kind error
	msg  string
}

func (e kindError) Error() string { return e.msg }
func (e kindError) Unwrap() error { return e.kind }
