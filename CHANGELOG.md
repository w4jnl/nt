# Changelog

User-facing changes per release, newest first. `scripts/release.sh` refuses to tag a version
that has no section here and uses the section as the GitHub release notes, so every release
updates this file first. Dates are the tag dates.

## Unreleased

- **Settings from nt.** `nt config` lists every setting of this machine and of the repository,
  with its value and what it does; `nt config set <key> <value>` changes one, in the right file,
  keeping its comments and refusing a value that would not load; `nt config get` and
  `nt config edit [--repo]` too. In the browser `,` shows the same list: enter flips a setting or
  asks for its value, `e` opens its file. The browser's own sync (`s`) now also compiles the
  weekly digest and writes the summaries, as the timer's does.
- **Weekly summaries.** A machine whose config names a tool (`[summary] tool = "claude"` or
  `"copilot"`) has it write, once a week is over, what was written on that machine in three
  short lists, Done, Decided and Open: its journal lines, todos, notes and agent sessions, read from the commits sync made
  there. Each machine writes its own file, `weekly/GGGG/GGGG-Www-<host>.md`, so a week written at
  work goes only to the work machine's tool. The weekly digest shows them all;
  `nt week --summarize` writes one by hand, and `nt doctor` checks the tool.

## 0.3.0 (2026-10-27)

Follow links in the note, the weekly digest, journals in time order.

- **Links that point at nothing look broken.** In the browser's preview a link to a file that is
  not there is grey and struck through; a click still says what it pointed to.
- **Journals in time order.** When two machines add lines to the same day, the merge puts one
  machine's after the other's. Once the day is over, sync puts its Log back in time order (lines
  after midnight but before `day_starts` last) and drops a line the merge left twice. Today's
  journal is left alone: moving lines another machine is still adding to would duplicate them.
  The weekly digest shows every day's lines in time order, today's too. Like `nt doctor`, the
  preview does not count a journal's links to the days around it, or to a digest not written
  yet, as broken.
- **Follow links in the note.** In the browser a click on a link in the note follows it, and `f`
  puts letters on the links on screen: type them to follow one. `b` goes back to the note before,
  scrolled as it was. A note opens at the heading the link names; folders, images, other files
  and URLs open as they do elsewhere. The "Linked from" lines under a note follow too.
- **Weekly digest.** The machine with `weekly = true` compiles `weekly/GGGG/GGGG-Www.md` (the ISO
  week) on every sync: the notes made and changed that week, the todos done and due, the agent
  sessions per project, and each day's journal lines. It is written only when it changes and
  catches up on weeks it missed; `nt week [date|last]` compiles one by hand. Weekly digests,
  like journals, do not move. Turn it on in one machine's `~/.config/nt/config.toml`.
- `nt doctor` lists the links in the notes that point at nothing: a note deleted or renamed by
  hand, or a link to a note's old place written on a machine that did not have its move yet.
  The journals' links to days without a journal are left out; `--json` lists every link.
- The README shows the browser and the command line in two recordings, and says how nt is
  designed; `CONTRIBUTING.md`, `SECURITY.md` and an issue form say how to report a bug or a
  vulnerability.
- Copilot CLI sessions no longer end as `abandoned`. Its `agentStop` hook carries no message
  text, so its turns left no line in the report's log, and a session with no turns counts as
  abandoned. Every turn now adds a line, `turn ended` when there is no text.

## 0.2.0 (2026-10-27)

Todos, folders and the calendar.

- **Todos.** A todo is a task line in any note, `- [ ] text due:YYYY-MM-DD`; new ones go to
  `todo.md`, the root's or a folder's (docs/format.md §12). `nt todo [folder]` lists the open
  ones, overdue first; `nt todo add [--in folder]`, `done`, `undo`, `due` and `rm` change them,
  by `path:line` or words from their text. In the browser `x` opens the todo list, with the note
  each todo is in beside it: `x` checks one, `d` sets its due date, `a` adds one to the `todo.md`
  of the folder you are in, `X` shows the done ones. In nvim `<leader>tx` checks the todo
  under the cursor. The skill and `AGENTS.md` teach agents the same. `todo.md` merges line by
  line like the journals: run `nt init <repo>` once in an existing repository to add the
  `.gitattributes` line, which `nt doctor` checks.
- **Folders.** `nt mv` moves a note or a folder and `nt rename` gives a note a new title (the
  file name follows, its date kept) or a folder a new name. Every link to what moves and in it
  is fixed in the same pass, the images only that note uses go along, `[[wikilinks]]` to it
  become markdown links, and `pins` and `default_folder` follow a folder (docs/format.md §13).
  `--dry-run` shows what would change. `nt mkdir` makes a folder that git keeps while it is
  empty. In the browser `m` moves the selected note or folder, `R` renames it and `N` makes a
  folder.
- **Calendar.** `c` in the browser shows the month: the days with a journal, `•` on days with
  todos due (in `warn` once the day has passed), and under it the selected day's todos and agent
  sessions, with its journal in the preview. `h` `l` `j` `k` move by day and week, `[` `]` by
  month, `e` edits the day's journal (starting it), `x` lists that day's todos.
- The browser's footer shows nt's version at its right end when there is room and no message.

## 0.1.0 (2026-10-27)

The first release: phases 0 to 3 of [the design](docs/design.md#phases-each-ends-in-something-usable).

- **Notes from the terminal.** `nt t` opens today's journal in your editor, `nt a` adds a
  timestamped line to it, `nt n` makes a dated note and links it from the day's journal, `nt f`
  and `nt s` find notes with fzf and rg, `nt ls` lists a folder. Every command that prints data
  takes `--json`.
- **git is the sync.** `nt sync` commits, merges and pushes; `nt install --sync` runs it every
  2 minutes (launchd, or a systemd user timer). It never rebases and never leaves conflict
  markers: when both sides changed a note, the other version is kept beside it.
- **The browser.** `nt` opens folders and notes on the left and the rendered note on the right,
  images included in kitty-protocol terminals. Resize the list with the mouse or `<` `>`, select
  text with the mouse and copy it with `y`, delete a note with `D`.
- **The editor stays yours.** `nt install --nvim` adds nt's nvim plugin: `<leader>ip` pastes the
  clipboard's image beside the note, and sync waits while a note has unsaved changes.
- **Agents report in.** `nt install --hooks` gives every Claude Code and Copilot CLI session a
  report in `agents/YYYY/YYYY-MM-DD/`; `nt install --skill` teaches the agents nt, so they add
  their own Done, Decisions and Open with `nt agent write`. The machine with `digest = true`
  keeps each day's digest.
- **Coming from Obsidian.** `nt import obsidian <vault>` brings a vault over once: wikilinks and
  embeds become markdown links, names become slugs.
- Shell completions for bash and zsh: `nt completion bash|zsh`.
