// Package editor opens notes in the person's editor: $VISUAL, then $EDITOR, then nvim. The
// command line is split on spaces and run directly, never through a shell.
package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Argv is the editor command for path, at line when line > 0 and the editor takes +N.
func Argv(path string, line int) []string {
	argv := strings.Fields(firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "nvim"))
	if line > 0 && takesLine(argv[0]) {
		argv = append(argv, fmt.Sprintf("+%d", line))
	}
	return append(argv, path)
}

// Command is Argv as a command attached to the terminal.
func Command(path string, line int) *exec.Cmd {
	argv := Argv(path, line)
	c := exec.Command(argv[0], argv[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c
}

// takesLine reports an editor that opens a file at line N with +N.
func takesLine(bin string) bool {
	switch filepath.Base(bin) {
	case "nvim", "vim", "vi", "hx", "nano", "micro", "kak", "emacs", "emacsclient":
		return true
	}
	return false
}

// LastLine is the 1-based number of the last non-blank line of a file, for opening it there.
func LastLine(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n \t"), "\n")
	return len(lines)
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
