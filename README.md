<p align="center"><img src="assets/brand/nt-hero.png" alt="nt: plain markdown notes, synced by git" width="800"></p>

Plain markdown notes in a folder, synced by git, read and written from the terminal. A journal per
day, dated topic notes, and a folder where coding agents report what they did, with a daily
digest. Your editor edits, git syncs, `rg` searches; nt keeps them together and stays out of the
way.

nt is in its second phase: the everyday commands and git sync work; the two-pane TUI and agent
reports come next ([phases](docs/design.md#phases-each-ends-in-something-usable)).

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

## Use it

```sh
make install                       # nt into ~/.local/bin
nt init ~/notes                    # a notes repository; --clone <url> on a second machine
git -C ~/notes remote add origin git@github.com:<you>/notes.git
nt install --sync                  # nt sync in the background every 2 minutes
nt doctor                          # check the setup
```

| Command | What it does |
|---|---|
| `nt t` | open today's journal in `$EDITOR` |
| `nt a "text"` | add a timestamped line to today's journal (`pbpaste \| nt a` works too) |
| `nt n "title" [folder]` | a new dated note, linked from today's journal |
| `nt f` / `nt s <query>` | find a note with fzf; search with rg |
| `nt ls [folder]` | the folders and notes in a folder, newest first |
| `nt view <file>` | one note rendered in nt's style, images included; `e` edits it |
| `nt sync` | commit, merge, push; conflicts never leave markers, the other version is kept beside the note |
| `nt import obsidian <vault>` | bring an Obsidian vault over once; `--dry-run` shows the plan |
| `nt theme --glamour` | nt's preview style for glow: `glow -s ~/.config/nt/glamour-dark.json` |

Commands that print data take `--json`.

### Planned

| Command | What it does |
|---|---|
| `nt` | the TUI: folders and notes on the left, the rendered note on the right |
| `nt agent …` | write agent session reports (used by hooks and by flok), with a daily digest |

## Docs

- [design.md](docs/design.md): how nt works, and the phases it is built in.
- [format.md](docs/format.md): the repository format.
- [design-system](docs/design-system/README.md): voice, colour, the mark and the terminal UI rules.
- [research.md](docs/research.md): the tools and apps surveyed before nt was designed.

## Licence

MIT. JetBrains Mono (in `docs/design-system/fonts/`) is under the SIL Open Font License.
