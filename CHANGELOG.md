# Changelog

User-facing changes per release, newest first. `scripts/release.sh` refuses to tag a version
that has no section here and uses the section as the GitHub release notes, so every release
updates this file first. Dates are the tag dates.

## Unreleased

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
