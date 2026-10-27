package tui

import (
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/w4jnl/nt/internal/config"
	"github.com/w4jnl/nt/internal/move"
	"github.com/w4jnl/nt/internal/scan"
	"github.com/w4jnl/nt/internal/vault"
)

// Moving, renaming and new folders (docs/design.md §4, Folders): m moves the selected note or
// folder, R renames it, N makes a folder. internal/move does the work and keeps the links to
// and in what moves working.

// movedMsg is a move or a rename done, or refused.
type movedMsg struct {
	plan *move.Plan
	err  error
}

// target is what m and R act on: the selected row of the list, else the note in the preview.
func (b *browser) target() (rel string, dir, ok bool) {
	if b.focus == onList {
		e, ok := b.selected()
		return e.rel, e.dir, ok
	}
	return b.showing, false, b.showing != ""
}

// startMove opens the folder picker for m: every folder the note or folder can go to, filtered
// by what is typed, and a new folder when what is typed names none.
func (b *browser) startMove() tea.Cmd {
	rel, dir, ok := b.target()
	if !ok {
		return nil
	}
	if err := move.Movable(b.v, rel); err != nil {
		b.say(err.Error(), true)
		return nil
	}
	name := rel
	if dir {
		name += "/"
	}
	b.moving = rel
	b.pick = &picker{title: "move " + name + " to", folders: moveFolders(b.v, rel, dir)}
	b.prompt("move", "move to: ")
	return b.moveTargets()
}

// moveFolders is every folder rel can move into: not its own folder, not itself or inside it,
// not the journals, the agent reports, assets or hidden folders. "" is the root.
func moveFolders(v *vault.Vault, rel string, dir bool) []string {
	here := path.Dir(rel)
	if here == "." {
		here = ""
	}
	c := v.Cfg.Folders
	var out []string
	if here != "" {
		out = append(out, "")
	}
	_ = filepath.WalkDir(v.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == v.Root {
			return nil
		}
		f := v.RelPath(p)
		if strings.HasPrefix(d.Name(), ".") || d.Name() == "assets" || f == c.Daily || f == c.Agents || dir && f == rel {
			return filepath.SkipDir
		}
		if f != here {
			out = append(out, f)
		}
		return nil
	})
	return out
}

// moveTargets fills the move picker with the folders that match what is typed, best first, and
// a new folder at the end when what is typed names no folder yet.
func (b *browser) moveTargets() tea.Cmd {
	p := b.pick
	if p == nil {
		return nil
	}
	q := strings.Trim(strings.TrimSpace(b.input.Value()), "/")
	type hit struct {
		f     string
		score int
	}
	var hits []hit
	for _, f := range p.folders {
		if s, ok := scan.Match(q, f+"/"); ok {
			hits = append(hits, hit{f, s})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return q != "" && hits[i].score > hits[j].score })
	var items []pickItem
	for _, h := range hits {
		it := pickItem{label: h.f + "/", kind: "folder", target: h.f}
		if h.f == "" {
			it.hint = "the root"
		}
		items = append(items, it)
	}
	if q != "" {
		if f, err := move.Folder(b.v, q); err == nil && !isDir(b.v.Abs(f)) {
			items = append(items, pickItem{label: f + "/", hint: "a new folder", kind: "folder", target: f})
		}
	}
	p.items, p.sel, p.top = items, 0, 0
	return nil
}

// moveInto moves what m was pressed on into the folder of the picked row.
func (b *browser) moveInto(it pickItem) tea.Cmd {
	b.pick = nil
	from, to := b.moving, it.target+"/"
	b.moving = ""
	if from == "" {
		return b.loadNow()
	}
	v := b.v
	return func() tea.Msg {
		p, err := move.Move(v, from, to, false)
		return movedMsg{p, err}
	}
}

// moveNowhere closes the move picker when no row is left to choose, saying why.
func (b *browser) moveNowhere(typed string) tea.Cmd {
	b.moving = ""
	if _, err := move.Folder(b.v, typed); err != nil {
		b.say(err.Error(), true)
	} else {
		b.say("no folder to move it to: type one", true)
	}
	return b.closePick()
}

// startRename opens the prompt for R with the note's title, or the folder's name, to change.
func (b *browser) startRename() tea.Cmd {
	rel, dir, ok := b.target()
	if !ok {
		return nil
	}
	if err := move.Movable(b.v, rel); err != nil {
		b.say(err.Error(), true)
		return nil
	}
	b.moving = rel
	cur, label := path.Base(rel), "rename "+path.Base(rel)+"/ to: "
	if !dir {
		cur, label = titleOf(b.v.Abs(rel)), "new title: "
	}
	b.prompt("rename", label)
	b.input.value, b.input.pos = []rune(cur), len([]rune(cur))
	return nil
}

// rename gives what R was pressed on the name typed.
func (b *browser) rename(name string) tea.Cmd {
	rel := b.moving
	b.moving = ""
	if rel == "" || name == "" {
		return nil
	}
	v := b.v
	return func() tea.Msg {
		p, err := move.Rename(v, rel, name, false)
		return movedMsg{p, err}
	}
}

// moved shows where a note or folder went, selected in its folder, and says what changed.
func (b *browser) moved(msg movedMsg) tea.Cmd {
	if msg.err != nil {
		b.say(msg.err.Error(), true)
		return b.loadNow()
	}
	p := msg.plan
	if p.Config {
		if cfg, err := config.LoadRepo(b.v.Root); err == nil {
			b.v.Cfg = cfg
		}
	}
	text := "moved to " + p.To
	if p.Title != "" {
		text = "renamed: " + p.Title
	}
	if p.Folder {
		text += "/"
	}
	if n := len(p.Links); n > 0 {
		text += " · links fixed in " + plural(n, "note")
	}
	b.say(text, false)
	b.p.forget()
	return tea.Batch(b.reveal(p.To), b.reindex())
}

// startMkdir opens the prompt for N: a new folder in the one the browser is in, or from the
// root when the name starts with "/".
func (b *browser) startMkdir() tea.Cmd {
	return b.prompt("mkdir", "new folder in "+firstNonEmpty(b.plainFolder(), "/")+": ")
}

func (b *browser) mkdir(name string) tea.Cmd {
	if name == "" {
		return nil
	}
	dir := path.Join(b.plainFolder(), name)
	if strings.HasPrefix(name, "/") {
		dir = strings.TrimLeft(name, "/")
	}
	rel, err := move.Mkdir(b.v, dir)
	if err != nil {
		b.say(err.Error(), true)
		return nil
	}
	b.say("made "+rel+"/", false)
	return b.reveal(rel)
}
