---
name: nt
description: The person's notes and todos, kept by the nt command in a git-synced folder of markdown. Use it to look something up in their notes, add a line to today's journal, make a note, add or check off a todo, or — at the end of a piece of work, or when blocked — write this session's report with Done, Decisions and Open.
---

# nt: the person's notes

`nt` keeps plain markdown notes in a git repository (usually `~/notes`) and syncs it in the
background. Every command that prints data takes `--json`. Never run git in the notes
repository: `nt sync` is its only git writer.

## Find and read

- `nt s "<words>" --json`: search every note (rg); each match has `path`, `line` and `text`.
- `nt ls [folder] --json`: the folders and notes in a folder, newest first.
- Read a note with your file tools at the path `nt` prints (`nt _editor repo` prints the
  repository's folder). Links between notes are relative markdown links.

## Write

- `nt a "text"`: add a timestamped line to today's journal. Use it for a short fact the person
  should find in their day, not for long text.
- `nt n "Title" [folder] --json`: a new dated note, linked from today's journal; it prints the
  path. Then write the body into that file. Default folder: `inbox`.
- `nt paste <note> --name "what it shows"`: save the image on the clipboard beside the note and
  print the `![…](assets/…)` line to put in it.
- `nt mv <note|folder> <to> [--dry-run]`, `nt rename <note> "New title"`, `nt mkdir <folder>`:
  move, rename and make folders. nt fixes every link to and in what moves; never move notes
  with `mv` or `git mv`.
- Notes are GitHub markdown: `[Title](relative/path.md)` links, `![alt](assets/x.png)` images,
  flat YAML frontmatter (`title`, `date`, `tags`). No wikilinks, embeds or callouts.

## Todos

A todo is a task line in any note: `- [ ] text due:YYYY-MM-DD`, and once done
`- [x] text done:YYYY-MM-DD`. New ones go to `todo.md`, the root's or a folder's.

- `nt todo [folder] --json`: the open todos, overdue first, in every note or under folder
  (`--done`: the done ones, `--all`: both); each has `rel`, `line`, `text`, `due`.
- `nt todo add "text" --due fri`: add one when the person asks you to remember something for
  them; `--in work` puts it in `work/todo.md`. `--due` takes YYYY-MM-DD, today, tomorrow,
  mon…sun, +3d, +2w.
- `nt todo done <todo>`, `nt todo due <todo> <when>`, `nt todo undo <todo>`, `nt todo rm <todo>`:
  `<todo>` is `path:line` from the list, or words from its text.

## This session's report

A hook already logs this session (start, a line per turn, end) in
`agents/YYYY/YYYY-MM-DD/<host>--<agent>--<project>--<session>.md`. Add your own account when
a piece of work is finished, when you are blocked, or when the person asks:

```sh
nt agent write --status done --summary "One line: the outcome" <<'EOF'
## Done
- what changed, with paths or commit hashes
## Decisions
- what you chose and why, briefly
## Open
- what is left, and who it waits on
EOF
```

- `--status` is `done`, `blocked` or `failed` (or `in-progress` while the session runs).
- The command finds the report by this session's working folder; `--session <id>` picks one.
  Each write replaces Done, Decisions and Open, so write the whole account each time.
- Keep it short and factual. No secrets, tokens, passwords or transcripts: the notes are
  pushed to GitHub.
- The day's `README.md` in the same folder is generated from the reports. Do not edit it.
