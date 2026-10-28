package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/render"
	"github.com/w4jnl/nt/internal/search"
)

// nt s <query>: search the notes with rg (smart case, markdown files). Exits 1 when nothing
// matches, as rg and grep do.
func runSearch(args []string) int {
	args, asJSON := takeBool(args, "--json")
	words, err := positional("s", args)
	if err != nil {
		return fail(err)
	}
	if len(words) == 0 {
		return fail(errors.New("s: give a query, e.g. nt s podman"))
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	matches, err := search.Search(context.Background(), v.Root, strings.Join(words, " "))
	if err != nil {
		return fail(fmt.Errorf("s: %w", err))
	}
	if asJSON {
		if matches == nil {
			matches = []search.Match{}
		}
		code := printJSON(matches)
		if code == 0 && len(matches) == 0 {
			return 1
		}
		return code
	}
	if len(matches) == 0 {
		fmt.Fprintln(os.Stderr, "nt: no notes match", strconvQuote(strings.Join(words, " ")))
		return 1
	}
	st := cliStyles(m)
	prev := ""
	for _, mt := range matches {
		if mt.Path != prev {
			if prev != "" {
				lipgloss.Println()
			}
			lipgloss.Println(st.Key.Render(mt.Path))
			prev = mt.Path
		}
		lipgloss.Println(st.Muted.Render(fmt.Sprintf("%5d  ", mt.Line)) + highlight(mt, st.Text, st.Name))
	}
	return 0
}

func strconvQuote(s string) string { return strconv.Quote(s) }

// highlight draws the line with its matches in the hit style, cut to 200 characters.
func highlight(mt search.Match, text, hit lipgloss.Style) string {
	line := mt.Text
	var b strings.Builder
	at := 0
	for _, sp := range mt.Spans {
		if sp[0] < at || sp[1] > len(line) {
			continue
		}
		b.WriteString(text.Render(line[at:sp[0]]))
		b.WriteString(hit.Render(line[sp[0]:sp[1]]))
		at = sp[1]
	}
	b.WriteString(text.Render(line[at:]))
	out := b.String()
	if lipgloss.Width(out) > 200 {
		out = text.Render(truncate(line, 200))
	}
	return out
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// nt f: pick a note with fzf, newest first, previewed in nt's style; open it in the editor.
func runFind(args []string) int {
	if _, err := positional("f", args); err != nil {
		return fail(err)
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	for _, tool := range []string{"rg", "fzf"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fail(fmt.Errorf("f: %s is not installed: brew install %s", tool, map[string]string{"rg": "ripgrep", "fzf": "fzf"}[tool]))
		}
	}
	list := exec.Command("rg", "--files", "--glob", "*.md", "--glob", "!.nt-tmp-*", "--sortr", "modified")
	list.Dir = v.Root
	files, err := list.Output()
	if err != nil {
		return fail(fmt.Errorf("f: rg --files: %w", err))
	}
	bin, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	pick := exec.Command("fzf", "--prompt", "note> ", "--scheme", "path",
		"--preview", shellQuote(bin)+" _preview {}", "--preview-window", "right,60%,border-left")
	pick.Dir = v.Root
	pick.Stdin, pick.Stderr = bytes.NewReader(files), os.Stderr
	out, err := pick.Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) && (ee.ExitCode() == 130 || ee.ExitCode() == 1) {
		return 0 // nothing picked
	}
	if err != nil {
		return fail(fmt.Errorf("f: fzf: %w", err))
	}
	rel := strings.TrimSpace(string(out))
	if rel == "" {
		return 0
	}
	return edit(v, m, v.Abs(rel), 0)
}

// shellQuote quotes for the shell fzf runs its preview command in.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// nt _preview <file>: the note rendered for fzf's preview pane ($FZF_PREVIEW_COLUMNS wide).
func runPreview(args []string) int {
	if len(args) != 1 {
		return 2
	}
	note, err := os.ReadFile(args[0])
	if err != nil {
		return fail(err)
	}
	width, _ := strconv.Atoi(os.Getenv("FZF_PREVIEW_COLUMNS"))
	if width <= 0 {
		width = 80
	}
	m, _ := config.LoadMachine()
	r, err := render.New(min(width-2, 100), m.Theme.Mode != "light")
	if err != nil {
		return fail(err)
	}
	out, err := r.Render(note)
	if err != nil {
		return fail(err)
	}
	fmt.Print(out)
	return 0
}
