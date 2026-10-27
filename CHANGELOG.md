# Changelog

User-facing changes per release, newest first. `scripts/release.sh` refuses to tag a version
that has no section here and uses the section as the GitHub release notes, so every release
updates this file first. Dates are the tag dates.

## Unreleased

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
