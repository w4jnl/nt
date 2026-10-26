<p align="center"><img src="assets/brand/nt-hero.png" alt="nt: plain markdown notes, synced by git" width="800"></p>

Plain markdown notes in a folder, synced by git, read and written from the terminal. A journal per
day, dated topic notes, and a folder where coding agents report what they did, with a daily
digest. Your editor edits, git syncs, `rg` searches; nt keeps them together and stays out of the
way.

nt is in its first phase: the design, the repository format and the design language are written,
and `nt view` shows a note in nt's style. The commands below the line are the plan.

## Why

Obsidian does a lot, and most of it gets in the way: views, plugins, canvases, a config folder
that conflicts in git, and sync that is either paid or fragile. nt does less:

- **Files are the truth.** GitHub-flavoured markdown in folders you name. Every note renders on
  github.com as it is, and opens in any editor.
- **git is the sync**, not an add-on. One process per machine commits and merges; agents and the
  TUI only write files. Conflicts never leave markers: the other version is kept beside the note.
- **Agents report in.** Each agent session writes its own report into `agents/<day>/`, so many
  agents on many machines never conflict, and the day's digest is one file.
- **The terminal first.** A two-pane TUI and a plain CLI, both with `--json`; a Mac app and an
  iOS app can follow the same [format](docs/format.md).

## Try it

```sh
make build
./bin/nt view README.md                # e edits in $EDITOR, r reloads, q quits
./bin/nt view assets/brand/README.md   # images are drawn in Ghostty and kitty, also in tmux
./bin/nt theme --glamour               # nt's preview style for glow: glow -s ~/.config/nt/glamour-dark.json
```

---

## Planned

| Command | What it does |
|---|---|
| `nt` | the TUI: folders and notes on the left, the rendered note on the right |
| `nt t` | open today's journal in `$EDITOR` |
| `nt a "text"` | add a timestamped line to today's journal (`pbpaste \| nt a` works too) |
| `nt n "title" [folder]` | a new dated note, linked from today's journal |
| `nt f` / `nt s <query>` | find a note with fzf; search with rg |
| `nt sync` | commit, merge, push; also on a timer |
| `nt agent …` | write agent session reports (used by hooks and by flok) |

## Docs

- [design.md](docs/design.md): how nt works, and the phases it is built in.
- [format.md](docs/format.md): the repository format.
- [design-system](docs/design-system/README.md): voice, colour, the mark and the terminal UI rules.
- [research.md](docs/research.md): the tools and apps surveyed before nt was designed.

## Licence

MIT. JetBrains Mono (in `docs/design-system/fonts/`) is under the SIL Open Font License.
