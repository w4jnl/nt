package gitsync

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// git runs git in one repository, never interactively: no terminal prompts, no editor, ssh in
// batch mode unless the person set their own ssh command, and English messages for parsing.
type git struct {
	dir string
	env []string
}

func newGit(dir string) git {
	env := append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_MERGE_AUTOEDIT=no")
	g := git{dir: dir, env: env}
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		if _, err := g.run("config", "--get", "core.sshCommand"); err != nil {
			g.env = append(g.env, "GIT_SSH_COMMAND=ssh -oBatchMode=yes -oConnectTimeout=10")
		}
	}
	return g
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
func (g git) run(args ...string) (string, error) {
	out, err := g.bytes(args...)
	return strings.TrimRight(string(out), "\n"), err
}

func (g git) bytes(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = g.dir, g.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), &gitError{args: args, stderr: stderr.String(), err: err}
	}
	return stdout.Bytes(), nil
}

// ok runs a command whose exit status is the answer (git diff --quiet, rev-parse --verify).
func (g git) ok(args ...string) bool {
	_, err := g.bytes(args...)
	return err == nil
}

// stderrOf is the stderr of a failed git command, lowercased, for classifying it.
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

// offline reports a fetch or push that failed for the network or for credentials: the next
// cycle tries again, nothing is wrong with the repository.
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

// rejected reports a push that lost a race: the remote moved on since the last fetch.
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
