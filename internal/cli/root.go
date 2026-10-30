// Package cli dispatches nt subcommands.
package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/w4jnl/nt/internal/config"

	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/tui"
)

// Version is stamped at build time: -ldflags "-X github.com/w4jnl/nt/internal/cli.Version=v0.1.0".
var Version = "dev"

// Tagline is nt's one-line description, as in the brand lockup.
const Tagline = "plain markdown notes, synced by git"

const usageText = `nt — plain markdown notes, synced by git

usage: nt <command>; commands that print data take --json

notes
  (none) [folder]      the browser: today, the todos, the calendar, the weekly digest and the agents,
                       then the folders, on the left; the note on the right; ? lists its keys
  <file>               show a note, or any text file, on the whole screen, as view does
  t                    open today's journal in $EDITOR
  a <text>             add a timestamped line to today's journal (or pipe the text in)
  n <title> [folder]   a new dated note in folder, linked from today's journal
  f                    find a note with fzf and open it
  s <query>            search the notes; '#tag' keeps to the notes with that tag
  ls [folder]          the folders and notes in a folder, newest first
  view <file>          show one note rendered, any other text as written; f follows a link, b goes
                       back [--theme auto|dark|light] [--images auto|on|off]
  paste <note>         save the clipboard's image beside the note and print its markdown line
                       [--name words]

folders
  mv <from> <to>       move a note or a folder; to ending in / is a folder to move it into. The
                       links to it and in it are fixed, its images go along [--dry-run]
  rename <from> <new>  a note's new title and file name, or a folder's new name [--dry-run]
  mkdir <folder>       a new folder, kept by git while it is empty
  rm <note|folder>     delete a note, with the images only it uses and its line in its day's
                       journal, or an empty folder; git keeps them [--dry-run]

todos
  todo [folder]        the open todos in every note, or under folder, overdue first [--done] [--all]
  todo add <text>      a new todo at the end of Inbox in todo.md [--due fri|+3d|YYYY-MM-DD]
                       [--in folder]: in that folder's todo.md
  todo done <todo>     check it; undo, rm, and due <todo> <when> too. <todo>: words or path:line

agents
  agent import            flok's record of a session (JSON on stdin) into its report [--probe]
  agent write             the agent's own Done, Decisions and Open on stdin [--status s] [--summary text]
  digest [date]           write the day's agents/…/README.md from its reports and list them
  week [date|last]        compile the week's weekly/GGGG/GGGG-Www.md now; the machine with
                          weekly = true does it on every sync

repository
  init [folder]        make folder a notes repository; --clone <url> on a second machine
  sync                 commit, merge and push now
  install --sync       run nt sync in the background every sync.interval; uninstall --sync stops it
  install --nvim       nt's nvim plugin: paste images, no merge under unsaved notes, sync on exit
  install --skill      teach Claude Code and Copilot CLI nt; uninstall --hooks: nt's old hooks
  doctor               check the repository, git, the sync timer and the tools nt uses
  config               every setting, its value and what it does; config set <key> <value>,
                       config get <key>, config edit [--repo] (the whole file in $EDITOR)

other
  theme --glamour [dir]   write nt's preview style for glow (glamour-dark.json, glamour-light.json)
  completion bash|zsh     print a shell completion script; nt completion shows how to install it
  version                 print the version
`

func usage() { fmt.Fprint(os.Stderr, usageText) }

// Main runs the CLI and returns the exit code.
func Main(args []string) int {
	if len(args) == 0 {
		return runBrowser("")
	}
	switch args[0] {
	case "t", "today":
		return runToday(args[1:])
	case "a", "add":
		return runAppend(args[1:])
	case "n", "new":
		return runNew(args[1:])
	case "f", "find":
		return runFind(args[1:])
	case "s", "search":
		return runSearch(args[1:])
	case "ls":
		return runList(args[1:])
	case "init":
		return runInit(args[1:])
	case "sync":
		return runSync(args[1:])
	case "install":
		return runInstall(args[1:], false)
	case "uninstall":
		return runInstall(args[1:], true)
	case "doctor":
		return runDoctor(args[1:])
	case "import":
		return runImport(args[1:])
	case "_preview":
		return runPreview(args[1:])
	case "_image":
		return runImage(args[1:])
	case "_editor":
		return runEditorHelper(args[1:])
	case "_complete":
		return runComplete(args[1:])
	case "completion":
		return runCompletion(args[1:])
	case "view":
		return runView(args[1:])
	case "paste":
		return runPaste(args[1:])
	case "todo":
		return runTodo(args[1:])
	case "mv", "move":
		return runMv(args[1:])
	case "rename":
		return runRename(args[1:])
	case "rm":
		return runRm(args[1:])
	case "mkdir":
		return runMkdir(args[1:])
	case "theme":
		return runTheme(args[1:])
	case "agent":
		return runAgent(args[1:])
	case "digest":
		return runDigest(args[1:])
	case "week":
		return runWeek(args[1:])
	case "config":
		return runConfig(args[1:])
	case "version", "--version", "-V":
		if stdoutIsTerminal() { // the lockup for people; "nt X.Y.Z" for scripts and the brew test
			fmt.Println(Lockup(Version))
		} else {
			fmt.Println("nt", strings.TrimPrefix(Version, "v"))
		}
		return 0
	case "help", "-h", "--help":
		usage()
		return 0
	}
	// Not a command: a file is shown, a folder of the notes browsed. No command has a . or a / in
	// its name, so nt notes.md and nt ./sync always mean the file.
	if !strings.HasPrefix(args[0], "-") {
		p, perr := notePath(args[0])
		if perr == nil && isFile(p) {
			return runView(args) // nt <file> [--theme …] [--images …]
		}
		if v, _, err := openVault(); err == nil && len(args) == 1 && isDir(v.Abs(strings.Trim(args[0], "/"))) {
			return runBrowser(args[0]) // nt <folder>
		}
		if perr != nil && strings.ContainsAny(args[0], "./") {
			return fail(perr) // a path, so no command: say where nt looked
		}
	}
	fmt.Fprintf(os.Stderr, "nt: %q is not a command, a file or a folder of the notes\n\n", args[0])
	usage()
	return 2
}

// Lockup is the one-line ASCII brand lockup from assets/brand/README.md: the mark as "[≡▁]"
// (bracket, two text lines, cursor), then the name, version and tagline.
func Lockup(version string) string {
	return "[≡▁] nt " + strings.TrimPrefix(version, "v") + " · " + Tagline
}

func runView(args []string) int {
	o := tui.ViewOptions{Mode: "auto", Images: "auto"}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--theme" && i+1 < len(args):
			i++
			o.Mode = args[i]
		case strings.HasPrefix(a, "--theme="):
			o.Mode = strings.TrimPrefix(a, "--theme=")
		case a == "--images" && i+1 < len(args):
			i++
			o.Images = args[i]
		case strings.HasPrefix(a, "--images="):
			o.Images = strings.TrimPrefix(a, "--images=")
		case strings.HasPrefix(a, "-"):
			return fail(fmt.Errorf("view: unknown flag %s", a))
		default:
			o.Path = a
		}
	}
	if o.Mode != "auto" && o.Mode != "dark" && o.Mode != "light" {
		return fail(fmt.Errorf("view: --theme is auto, dark or light, not %q", o.Mode))
	}
	if o.Images != "auto" && o.Images != "on" && o.Images != "off" {
		return fail(fmt.Errorf("view: --images is auto, on or off, not %q", o.Images))
	}
	if o.Path == "" {
		return fail(errors.New("view: name a note, e.g. nt view daily/2026/2026-10-26.md"))
	}
	path, err := notePath(o.Path)
	if err != nil {
		return fail(fmt.Errorf("view: %w", err))
	}
	if !textFile(path) {
		return fail(fmt.Errorf("view: %s is not a text file; open it with the OS (open, xdg-open)", o.Path))
	}
	if !isTerminal(os.Stdout) {
		return fail(errors.New("view: it needs a terminal; cat prints the file"))
	}
	o.Path, o.Root = path, ""
	o.Bin, _ = os.Executable()
	v, m, verr := openVault()
	if verr == nil {
		o.ImageViewer = m.ImageViewer
	}
	if verr == nil && strings.HasPrefix(path, v.Root+string(filepath.Separator)) {
		o.Root = v.Root
	} else if wd, err := os.Getwd(); err == nil {
		o.Root = wd
	}
	return report(tui.RunView(o))
}

// textFile reports a file the viewer can show: a regular file with no NUL byte in its first 8 KB,
// the test git and grep use for binary files.
func textFile(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 8192)
	n, _ := f.Read(head)
	st, err := f.Stat()
	return err == nil && st.Mode().IsRegular() && !bytes.Contains(head[:n], []byte{0})
}

// nt, nt <folder>: the browser.
func runBrowser(folder string) int {
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	if !isTerminal(os.Stdout) {
		return fail(errors.New("the browser needs a terminal; nt help lists the commands"))
	}
	bin, _ := os.Executable()
	return report(tui.RunBrowser(tui.BrowserOptions{
		Vault: v, Machine: m, Folder: strings.Trim(folder, "/"), Bin: bin,
		Kick: func() { kickSync(m) }, StateDir: config.StateDir(), Version: Version,
	}))
}

// nt _image <file>: a picture in the terminal with chafa, until Enter; the browser's i key on a
// Linux machine without a desktop.
func runImage(args []string) int {
	if len(args) != 1 {
		return 2
	}
	if _, err := exec.LookPath("chafa"); err != nil {
		fmt.Println("chafa is not installed (apt install chafa); the image is", args[0])
	} else {
		c := exec.Command("chafa", args[0])
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		_ = c.Run()
	}
	fmt.Print("\npress enter to go back ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return 0
}

// notePath finds a note the way the other commands name it: a path from the current directory
// when one exists there, else a path inside the notes repository (as nt ls, s and f print them).
func notePath(p string) (string, error) {
	if _, err := os.Stat(p); err == nil || filepath.IsAbs(p) {
		return p, err
	}
	v, _, err := openVault()
	if err != nil {
		return "", fmt.Errorf("%s: no such file here, and no notes repository to look in", p)
	}
	if abs := v.Abs(p); isFile(abs) {
		return abs, nil
	}
	return "", fmt.Errorf("%s: no such file here or in %s", p, tilde(v.Root))
}

func runTheme(args []string) int {
	if len(args) == 0 || args[0] != "--glamour" {
		return fail(errors.New("theme: use nt theme --glamour [dir]"))
	}
	dir := ""
	if len(args) > 1 {
		dir = args[1]
	} else {
		cfg, err := os.UserConfigDir()
		if home, herr := os.UserHomeDir(); herr == nil {
			cfg, err = filepath.Join(home, ".config"), nil // ~/.config on macOS too, as flok
		}
		if err != nil {
			return fail(err)
		}
		dir = filepath.Join(cfg, "nt")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(err)
	}
	for _, v := range []struct {
		name string
		dark bool
	}{{"glamour-dark.json", true}, {"glamour-light.json", false}} {
		b, err := render.JSON(v.dark)
		if err != nil {
			return fail(err)
		}
		p := filepath.Join(dir, v.name)
		if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
			return fail(err)
		}
		fmt.Println("wrote", p)
	}
	fmt.Printf("use it with: glow -s %s <file>\n", filepath.Join(dir, "glamour-dark.json"))
	return 0
}

// stdoutIsTerminal reports whether a person, not a pipe, is reading stdout.
func stdoutIsTerminal() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "nt:", err)
	return 2
}

func report(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "nt:", err)
		return 1
	}
	return 0
}
