package ops

import (
	"errors"
	"fmt"
	"strings"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/gitsync"
)

// Runner answers the command lines an app runs nt with, in the process: what nt prints, and its
// exit status. It takes the Mac app's command lines (docs/design.md §8, Reading and writing),
// so an app that cannot start a process, as on iOS, asks the same questions and reads the same
// JSON. Git is the repository's plumbing for sync; nil: git itself.
type Runner struct {
	Git gitsync.Git
}

// errUsage is a command line the runner does not take.
var errUsage = errors.New("not a command nt runs for an app")

// Run runs args, with stdin as the command's input. A command that fails returns its error,
// which nt would print after "nt: "; code is its exit status.
func (r Runner) Run(args []string, stdin []byte) (out []byte, code int, err error) {
	if len(args) == 0 {
		return nil, 2, errUsage
	}
	out, code, err = r.run(args[0], args[1:], stdin)
	if err != nil && code == 0 {
		code = 1
	}
	return out, code, err
}

func (r Runner) run(cmd string, args []string, stdin []byte) ([]byte, int, error) {
	args, _ = Flag(args, "--json") // every answer here is JSON where nt has one
	switch cmd {
	case "t":
		if _, err := Positional("t", args); err != nil {
			return nil, 2, err
		}
		v, _, err := Open()
		if err != nil {
			return nil, 1, err
		}
		j, err := Today(v)
		return answer(j, err)
	case "a":
		words, err := Positional("a", args)
		if err != nil {
			return nil, 2, err
		}
		text := strings.Join(words, " ")
		if text == "" {
			text = string(stdin)
		}
		v, _, err := Open()
		if err != nil {
			return nil, 1, err
		}
		l, err := Append(v, text)
		return answer(l, err)
	case "n":
		pos, err := Positional("n", args)
		if err != nil {
			return nil, 2, err
		}
		if len(pos) > 2 {
			return nil, 2, errors.New(`n: use nt n "title" [folder]; quote a title with spaces`)
		}
		title, folder := "", ""
		if len(pos) > 0 {
			title = pos[0]
		}
		if len(pos) > 1 {
			folder = pos[1]
		}
		v, _, err := Open()
		if err != nil {
			return nil, 1, err
		}
		n, err := New(v, title, folder)
		return answer(n, err)
	case "todo":
		return r.todo(args, stdin)
	case "_editor":
		return r.editor(args, stdin)
	case "s":
		words, err := Positional("s", args)
		if err != nil {
			return nil, 2, err
		}
		if len(words) == 0 {
			return nil, 2, errors.New("s: give a query, e.g. nt s podman")
		}
		v, _, err := Open()
		if err != nil {
			return nil, 1, err
		}
		m, err := Search(v, strings.Join(words, " "))
		out, code, err := answer(m, err)
		if code == 0 && len(m) == 0 {
			code = 1 // as rg and grep do
		}
		return out, code, err
	case "config":
		if len(args) > 0 && args[0] == "set" {
			pos, err := Positional("config set", args[1:])
			if err != nil {
				return nil, 2, err
			}
			if len(pos) < 1 {
				return nil, 2, errors.New("config set: use nt config set <key> <value>; nt config lists the keys")
			}
			s, ok := config.FindSetting(pos[0])
			if !ok {
				return nil, 2, fmt.Errorf("config set: %q is not a setting; nt config lists them", pos[0])
			}
			where, err := SetSetting(s, strings.Join(pos[1:], " "))
			if err != nil {
				return nil, 1, fmt.Errorf("config set: %w", err)
			}
			return []byte(s.Key + " = " + strings.Join(pos[1:], " ") + "  " + where + "\n"), 0, nil
		}
		if pos, err := Positional("config", args); err != nil {
			return nil, 2, err
		} else if len(pos) > 0 {
			return nil, 2, fmt.Errorf("config: %q is not get, set or edit", pos[0])
		}
		s, err := Settings()
		return answer(SettingList{Settings: s}, err)
	case "paste":
		args, name, _, err := Value(args, "--name")
		if err != nil {
			return nil, 2, fmt.Errorf("paste: %w", err)
		}
		args, from, _, err := Value(args, "--from")
		if err != nil {
			return nil, 2, fmt.Errorf("paste: %w", err)
		}
		pos, err := Positional("paste", args)
		if err != nil {
			return nil, 2, err
		}
		if len(pos) != 1 || from == "" {
			return nil, 2, errors.New("paste: name the note and the image: nt paste <note> --from <file>")
		}
		v, _, err := Open()
		if err != nil {
			return nil, 1, err
		}
		note, err := InRepo(v, pos[0])
		if err != nil {
			return nil, 1, fmt.Errorf("paste: %w", err)
		}
		p, err := PasteFile(v, note, name, from)
		return answer(p, err)
	case "sync":
		if _, err := Positional("sync", args); err != nil {
			return nil, 2, err
		}
		v, m, err := Open()
		if err != nil {
			return nil, 1, err
		}
		st := gitsync.Run(v, gitsync.Options{Wait: true, Git: r.Git, StateDir: config.StateDir(), Digest: m.Digest, Weekly: m.Weekly})
		if err := gitsync.WriteStatus(config.StateDir(), st); err != nil {
			return nil, 1, fmt.Errorf("status: %w", err)
		}
		out, code, err := answer(st, nil)
		if code == 0 && st.State == "error" {
			code = 1
		}
		return out, code, err
	}
	return nil, 2, fmt.Errorf("%q is %w", cmd, errUsage)
}

func (r Runner) todo(args []string, stdin []byte) ([]byte, int, error) {
	kind := ""
	if len(args) > 0 {
		switch args[0] {
		case "add", "done", "undo", "due", "rm":
			kind, args = args[0], args[1:]
		}
	}
	v, _, err := Open()
	if err != nil {
		return nil, 1, err
	}
	switch kind {
	case "":
		args, done := Flag(args, "--done")
		args, all := Flag(args, "--all")
		pos, err := Positional("todo", args)
		if err != nil {
			return nil, 2, err
		}
		if len(pos) > 1 {
			return nil, 2, errors.New("todo: name one folder, or use add, done, undo, due or rm")
		}
		folder := ""
		if len(pos) == 1 {
			folder = pos[0]
		}
		items, err := Todos(v, folder, done, all)
		if err != nil {
			return nil, 1, err
		}
		list := TodoList{Todos: []Todo{}}
		for _, it := range items {
			list.Todos = append(list.Todos, TodoOf(v, it))
		}
		return answer(list, nil)
	case "add":
		args, due, _, err := Value(args, "--due")
		if err != nil {
			return nil, 2, fmt.Errorf("todo add: %w", err)
		}
		args, folder, _, err := Value(args, "--in")
		if err != nil {
			return nil, 2, fmt.Errorf("todo add: %w", err)
		}
		words, err := Positional("todo add", args)
		if err != nil {
			return nil, 2, err
		}
		text := strings.Join(words, " ")
		if text == "" {
			text = string(stdin)
		}
		it, err := AddTodo(v, folder, text, due)
		if err != nil {
			return nil, 1, err
		}
		return answer(TodoOf(v, it), nil)
	}
	words, err := Positional("todo "+kind, args)
	if err != nil {
		return nil, 2, err
	}
	when := ""
	if kind == "due" {
		if len(words) < 2 {
			return nil, 2, errors.New(`todo due: give the todo and when it is due: nt todo due "passport" fri (none clears it)`)
		}
		when, words = words[len(words)-1], words[:len(words)-1]
	}
	it, err := ChangeTodo(v, kind, strings.Join(words, " "), when)
	if err != nil {
		return nil, 1, err
	}
	t := TodoOf(v, it)
	deleted := kind == "rm"
	t.Deleted = &deleted
	return answer(t, nil)
}

func (r Runner) editor(args []string, stdin []byte) ([]byte, int, error) {
	if len(args) == 0 {
		return nil, 2, errors.New("_editor: use repo, kick, day, index or save")
	}
	v, _, err := Open()
	if err != nil {
		return nil, 1, err
	}
	switch args[0] {
	case "repo":
		return []byte(v.Root + "\n"), 0, nil
	case "day":
		return []byte(Day(v) + "\n"), 0, nil
	case "index":
		return answer(IndexOf(v, config.StateDir()), nil)
	case "kick":
		return nil, 0, nil // the app syncs when it sees fit; there is no process to start
	case "save":
		rest, keep := Flag(args[1:], "--keep")
		rest, base, _, err := Value(rest, "--base")
		if err != nil {
			return nil, 2, fmt.Errorf("_editor save: %w", err)
		}
		pos, err := Positional("_editor save", rest)
		if err != nil {
			return nil, 2, err
		}
		if len(pos) != 1 {
			return nil, 2, errors.New("_editor save: name one note")
		}
		rel, err := InRepo(v, pos[0])
		if err != nil {
			return nil, 1, fmt.Errorf("_editor save: %w", err)
		}
		s, err := Save(v, rel, stdin, base, keep)
		return answer(s, err)
	}
	return nil, 2, fmt.Errorf("_editor: unknown %q: use repo, kick, day, index or save", args[0])
}

// answer is v as JSON, or the error.
func answer(v any, err error) ([]byte, int, error) {
	if err != nil {
		return nil, 1, err
	}
	b, err := JSON(v)
	if err != nil {
		return nil, 1, err
	}
	return b, 0, nil
}
