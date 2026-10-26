package cli

import (
	"errors"
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/w4jnl/nt/internal/importer"
)

// nt import obsidian <vault> [--dry-run]: bring an Obsidian vault into the notes repository
// once. The vault is only read. --dry-run prints the plan and writes nothing.
func runImport(args []string) int {
	args, asJSON := takeBool(args, "--json")
	args, dry := takeBool(args, "--dry-run")
	pos, err := positional("import", args)
	if err != nil {
		return fail(err)
	}
	if len(pos) != 2 || pos[0] != "obsidian" {
		return fail(errors.New("import: use nt import obsidian <vault> [--dry-run]"))
	}
	v, m, err := openVault()
	if err != nil {
		return fail(err)
	}
	rep, err := importer.Obsidian(pos[1], v, importer.Options{DryRun: dry})
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return printJSON(rep)
	}
	st := cliStyles(m)
	section := func(title string, n int) {
		lipgloss.Println()
		lipgloss.Println(st.Key.Render(fmt.Sprintf("%s (%d)", title, n)))
	}
	moved := func(list []importer.Moved) {
		for _, mv := range list {
			lipgloss.Println(st.Muted.Render("  "+mv.From+" → ") + st.Text.Render(mv.To))
		}
	}
	verb := "imported"
	if dry {
		verb = "would import (dry run, nothing written)"
	}
	lipgloss.Println(st.Text.Render(fmt.Sprintf("%s %s into %s: %d notes, %d journals, %d attachments, %d links converted",
		verb, tilde(rep.Source), tilde(v.Root), len(rep.Notes), len(rep.Journals), len(rep.Assets), rep.Converted)))
	section("notes", len(rep.Notes))
	moved(rep.Notes)
	section("journals", len(rep.Journals))
	moved(rep.Journals)
	section("attachments", len(rep.Assets))
	moved(rep.Assets)
	section("left behind", len(rep.Skipped))
	for _, s := range rep.Skipped {
		lipgloss.Println(st.Muted.Render("  " + s.Path + ": " + s.Reason))
	}
	section("links not resolved, left as written", len(rep.Unresolved))
	for _, u := range rep.Unresolved {
		lipgloss.Println(st.Muted.Render("  "+u.Note+": ") + st.Text.Render(u.Text))
	}
	if !dry {
		lipgloss.Println()
		lipgloss.Println(st.Key.Render("next") + st.Verb.Render("  review the notes, then nt sync"))
	}
	return 0
}
