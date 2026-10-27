package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/move"
	"github.com/w4jnl/nt/internal/vault"
)

// nt mv <note|folder> <to> [--dry-run] [--json]: move a note or a folder; the links to it and in
// it keep working (docs/format.md §13).
func runMv(args []string) int {
	args, asJSON := takeBool(args, "--json")
	args, dry := takeBool(args, "--dry-run")
	pos, err := positional("mv", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) != 2 {
		return fail(errors.New("mv: use nt mv <note|folder> <to>; a to ending in / is a folder to move it into"))
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	p, err := move.Move(v, source(v, pos[0]), pos[1], dry)
	if err != nil {
		return fail(fmt.Errorf("mv: %w", err))
	}
	return printPlan(m, p, asJSON)
}

// nt rename <note|folder> <title|name> [--dry-run] [--json]: a note's new title, with the file
// name that goes with it; a folder's new name.
func runRename(args []string) int {
	args, asJSON := takeBool(args, "--json")
	args, dry := takeBool(args, "--dry-run")
	pos, err := positional("rename", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) < 2 {
		return fail(errors.New(`rename: use nt rename <note> "new title", or nt rename <folder> <new name>`))
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	p, err := move.Rename(v, source(v, pos[0]), strings.Join(pos[1:], " "), dry)
	if err != nil {
		return fail(fmt.Errorf("rename: %w", err))
	}
	return printPlan(m, p, asJSON)
}

// nt mkdir <folder> [--json]: a new folder, which git keeps while it is empty.
func runMkdir(args []string) int {
	args, asJSON := takeBool(args, "--json")
	pos, err := positional("mkdir", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) != 1 {
		return fail(errors.New("mkdir: use nt mkdir <folder>, a path from the root of the notes"))
	}
	v, _, err := openVault()
	if err != nil {
		return fail(err)
	}
	rel, err := move.Mkdir(v, pos[0])
	if err != nil {
		return fail(fmt.Errorf("mkdir: %w", err))
	}
	if asJSON {
		return printJSON(map[string]any{"path": v.Abs(rel), "rel": rel})
	}
	fmt.Println(rel + "/")
	return 0
}

// source is the repository path of the note or folder named on the command line: a path from
// the root of the notes, else from the working directory.
func source(v *vault.Vault, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if _, err := os.Stat(v.Abs(p)); err == nil {
		return p
	}
	if abs, err := filepath.Abs(p); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return p
}

// printPlan says what a move or a rename did, or would do: the files that moved and the notes
// whose links changed.
func printPlan(m config.Machine, p *move.Plan, asJSON bool) int {
	if asJSON {
		return printJSON(p)
	}
	st := cliStyles(m)
	from, to := p.From, p.To
	if p.Folder {
		from, to = from+"/", to+"/"
	}
	verb := "moved"
	if p.Title != "" {
		verb = "renamed"
	}
	if p.DryRun {
		verb = "would " + strings.TrimSuffix(verb, "d")
	}
	line := st.Muted.Render(verb+" ") + st.Text.Render(from)
	if to != from {
		line += st.Muted.Render(" → ") + st.Text.Render(to)
	}
	if p.Title != "" {
		line += st.Muted.Render(", title ") + st.Text.Render(fmt.Sprintf("%q", p.Title))
	}
	if p.Folder {
		line += st.Muted.Render(", " + plural(len(p.Files), "file"))
	}
	lipgloss.Println(line)
	if !p.Folder {
		for _, s := range p.Files[min(1, len(p.Files)):] {
			lipgloss.Println(st.Muted.Render("  with ") + st.Text.Render(s.From) + st.Muted.Render(" → ") + st.Text.Render(s.To))
		}
	}
	if len(p.Links) > 0 {
		var notes []string
		for _, e := range p.Links {
			notes = append(notes, e.Path)
		}
		lipgloss.Println(st.Muted.Render("  links fixed in ") + st.Faint.Render(strings.Join(notes, ", ")))
	}
	if p.Config {
		lipgloss.Println(st.Muted.Render("  .nt/config.toml: pins and default_folder follow it"))
	}
	if p.DryRun {
		lipgloss.Println(st.Muted.Render("dry run: nothing written"))
	}
	return 0
}
