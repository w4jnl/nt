<p align="center"><img src="assets/brand/nt-hero.png" alt="nt: plain markdown notes, synced by git" width="800"></p>

Plain markdown notes in a folder, synced by git, read and written from the terminal. A journal per
day, dated topic notes, and a folder where coding agents report what they did, with a daily
digest. Your editor edits, git syncs, `rg` searches; nt keeps them together and stays out of the
way.

Current version: 0.2.0 · [release notes](CHANGELOG.md). The everyday commands, git sync, the
browser, agent reports, todos, folders and the calendar work; a weekly digest comes next
([phases](docs/design.md#phases-each-ends-in-something-usable)).

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
brew install w4jnl/tap/nt          # macOS or Linux; or make install from a clone, into ~/.local/bin
nt init ~/notes                    # a notes repository; --clone <url> on a second machine
git -C ~/notes remote add origin git@github.com:<you>/notes.git
nt install --sync                  # nt sync in the background every 2 minutes
nt install --nvim                  # the nvim plugin; lazy.nvim: { dir = "~/.local/share/nt/nvim", name = "nt", lazy = false }
nt install --hooks --skill         # agent session reports, and the skill that teaches agents nt
echo 'eval "$(nt completion zsh)"' >> ~/.zshrc   # without Homebrew, after compinit; bash: nt completion bash
nt doctor                          # check the setup
```

| Command | What it does |
|---|---|
| `nt` / `nt <folder>` | the browser: folders and notes on the left, the rendered note on the right, images included; `?` lists the keys |
| `nt t` | open today's journal in `$EDITOR` |
| `nt a "text"` | add a timestamped line to today's journal (`pbpaste \| nt a` works too) |
| `nt n "title" [folder]` | a new dated note, linked from today's journal |
| `nt f` / `nt s <query>` | find a note with fzf; search with rg |
| `nt ls [folder]` | the folders and notes in a folder, newest first |
| `nt view <file>` | one note rendered in nt's style, images included; `e` edits it |
| `nt paste <note>` | save the clipboard's image beside the note and print its markdown line; in nvim, `<leader>ip` |
| `nt sync` | commit, merge, push; conflicts never leave markers, the other version is kept beside the note |
| `nt import obsidian <vault>` | bring an Obsidian vault over once; `--dry-run` shows the plan |
| `nt theme --glamour` | nt's preview style for glow: `glow -s ~/.config/nt/glamour-dark.json` |

Commands that print data take `--json`. Without Homebrew or Go, a static binary for Linux or macOS
is on the [releases page](https://github.com/w4jnl/nt/releases).

### Todos

A todo is a task line in any note, `- [ ] Renew passport due:2026-11-03`; new ones go to
`todo.md`, and a folder can have its own `todo.md`. In the browser, `x` lists them all, overdue
first, with the note each is in beside it: `x` checks one, `d` sets its due date, `a` adds one to
the `todo.md` of the folder you are in, `X` shows the done ones. `c` shows the month: the days
with a journal, `•` on days with todos due, and under it the selected day's todos and agent
sessions, its journal beside it. In nvim, `<leader>tx` checks the todo under the cursor.

| Command | What it does |
|---|---|
| `nt todo [folder] [--done\|--all]` | the open todos in every note, or under folder, overdue first; or the done ones, or all |
| `nt todo add "text" [--due fri] [--in work]` | add one to `todo.md`, or to `work/todo.md`; `--due` takes `YYYY-MM-DD`, `today`, `mon`…`sun`, `+3d`, `+2w` |
| `nt todo done\|undo\|rm <todo>` | check, uncheck or delete one, named by `path:line` or words from its text |
| `nt todo due <todo> <when>` | set its due date; `none` clears it |

### Folders

Move and rename with nt rather than `mv`: it fixes every link to what moves and in it, takes the
images only that note uses along, and keeps the journal's line pointing at the note. `nt doctor`
lists any link that points at nothing. In the
browser, `m` moves the selected note or folder (type to filter the folders, or name a new one),
`R` renames it, `N` makes a folder in the one you are in (`/name` from the root).

| Command | What it does |
|---|---|
| `nt mv <note\|folder> <to>` | move it: into `to` when that ends in `/` or is a folder, else to that path; `--dry-run` shows what changes |
| `nt rename <note> "New title"` | the title in its frontmatter and heading, and the file name, its date kept; links that showed the old title show the new one |
| `nt rename <folder> <name>` | the folder's new name; `pins` and `default_folder` follow it |
| `nt mkdir <folder>` | a new folder, with a `.gitkeep` so git keeps it while it is empty |

### Agents

`nt install --hooks` gives every Claude Code and Copilot CLI session a report in
`agents/YYYY/YYYY-MM-DD/`: when it started and where, a line per turn, when it ended. `nt install --skill`
teaches the agents nt, so they add their own Done, Decisions and Open with `nt agent write`. The machine
with `digest = true` keeps each day's `README.md` digest, which GitHub shows when you open the folder.

| Command | What it does |
|---|---|
| `nt agent start\|turn\|end` | the hooks: a session's start, each turn, its end, from the hook payload on stdin |
| `nt agent write` | the agent's own account on stdin, `--status done\|blocked\|failed`, `--summary "…"` |
| `nt digest [date]` | write and list a day's digest |

## Docs

- [design.md](docs/design.md): how nt works, and the phases it is built in.
- [format.md](docs/format.md): the repository format.
- [design-system](docs/design-system/README.md): voice, colour, the mark and the terminal UI rules.
- [research.md](docs/research.md): the tools and apps surveyed before nt was designed.

## Licence

MIT. JetBrains Mono (in `docs/design-system/fonts/`) is under the SIL Open Font License.
