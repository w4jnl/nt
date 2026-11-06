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
- `nt history <note> --json`: when the note changed and from which machine, newest first, from
  git's history; a change not yet synced is not in it.
- `weekly/GGGG/GGGG-Www.md` (ISO week) is the week's digest: its notes, todos, agent sessions
  and journal lines. Read it to see what a week held; never edit it, nt compiles it.
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

## How to write

Write notes, journal lines and reports in Simplified Technical English (ASD-STE100) unless the
person asks for another style. Code, commands, paths, quoted text and the person's own words
stay as they are.

- One topic per sentence, one instruction per sentence. At most 20 words in an instruction,
  25 in a description; at most six sentences in a paragraph.
- Active voice, present tense. Instructions in the imperative: "Run the tests", not "The tests
  should be run".
- Use "must" for a requirement, "can" for a possibility; not "should", "may" or "might".
- One word for one meaning, the same word every time: "start", not "initiate", "launch" or
  "kick off" in turn. Plain words over long ones: "use", "show", "do", "get".
- No noun strings of more than three words: "the config of the sync timer", not "sync timer
  config value".
- Write the articles: "the file", "a note". No gerund as a verb: "when you save", not "when
  saving".
- A warning or a caution comes before the step it belongs to, as its own sentence.
- Steps in a numbered list, in the order they are done; facts that line up in a table.
- Keep the person's names for things (projects, hosts, tools) exactly as they write them.

## Todos

A todo is a task line, `- [ ] text due:YYYY-MM-DD`, and once done `- [x] text done:YYYY-MM-DD`,
in a `todo.md` or in a note whose frontmatter says `todos: true`; any other note's task lines
are a checklist, not todos (`nt todo on <note>` makes them todos, `off` a checklist). New ones
go to `todo.md`, the root's or a folder's.

- `nt todo [folder] --json`: the open todos, overdue first, in every note or under folder
  (`--done`: the done ones, `--all`: both); each has `rel`, `line`, `text`, `due`.
- `nt todo add "text" --due fri`: add one when the person asks you to remember something for
  them; `--in work` puts it in `work/todo.md`. `--due` takes YYYY-MM-DD, today, tomorrow,
  mon…sun, +3d, +2w.
- `nt todo done <todo>`, `nt todo due <todo> <when>`, `nt todo undo <todo>`, `nt todo rm <todo>`:
  `<todo>` is `path:line` from the list, or words from its text.

## This session's report

flok logs this session into nt (start, turns, blocks, end) in
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
- Keep it short and factual, in Simplified Technical English as above. No secrets, tokens,
  passwords or transcripts: the notes are pushed to GitHub.
- The day's `README.md` in the same folder is generated from the reports. Do not edit it.
