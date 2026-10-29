# nt repository format

Format version 1, 2026-10-26.

This document is the contract for a notes repository. `nt` follows it. So do the agents that write reports, the Mac and iOS apps that come later, and any script a person writes. A repository that follows it is readable without `nt`: in nvim, in glow, on github.com and in any markdown editor.

Rules are marked **must** or **should**. A client that cannot follow a must rule leaves the repository alone.

## 1. Principles

- The files are the truth. Any index or cache is derived, lives outside the repository and can be deleted.
- Everything is GitHub-flavoured markdown that renders on github.com as written.
- One writer per file at a time. A file has one owner, or every writer takes the repository lock (§10).
- git is the sync. Clients commit and merge; they never leave conflict markers in a file.

## 2. Layout

```
<repo>/
  .nt/config.toml                 repository settings (§9)
  .nt/templates/                  daily.md, note.md (§8)
  .gitattributes                  daily/** and todo.md merge=union
  .gitignore                      §11
  AGENTS.md                       layout and rules for agents working in the repository
  README.md                       optional, owned by the person
  todo.md                         the todo list; a folder can have its own, and todos can be in any note (§12)
  daily/YYYY/YYYY-MM-DD.md        daily journal, one per day (§5)
  daily/YYYY/assets/              attachments pasted into journals
  agents/YYYY/YYYY-MM-DD/         agent reports for that day (§6)
  agents/YYYY/YYYY-MM-DD/README.md   the day's digest (§7)
  weekly/GGGG/GGGG-Www.md         the week's digest, named by ISO week (§7)
  <folder>/…/YYYY-MM-DD-slug.md   topic notes, in any folder the person creates
  <folder>/…/assets/              attachments for the notes in that folder
```

- `daily/`, `agents/` and `weekly/` are reserved. Their names can be changed in `.nt/config.toml`.
- Any other folder is the person's. Folders can nest to any depth.
- Names that start with `.` are ignored by clients, except `.nt/`, `.gitattributes` and `.gitignore`.
- A folder that holds no note yet holds an empty `.gitkeep`, so git keeps it. Clients leave it in
  place and do not show it.

## 3. Names

### Slugs

A slug is made from a title as follows:

1. Lowercase it.
2. Replace accented letters with their base letter (`é` → `e`, `ß` → `ss`).
3. Replace every run of characters other than `a-z` and `0-9` with one `-`.
4. Trim `-` from both ends.
5. Cut it to 60 characters at the last `-` that fits.

An empty result is no slug. A slug never contains `--`.

### File names

| What | Name |
|---|---|
| Daily journal | `daily/YYYY/YYYY-MM-DD.md` |
| Topic note | `YYYY-MM-DD-<slug>.md`, in the folder it belongs to |
| Topic note without a title | `YYYY-MM-DD-HHMMSS.md` |
| Attachment | `assets/YYYY-MM-DD-<slug>.<ext>` beside the note, where `<slug>` comes from the original file name or `image`; a name that starts with a date keeps it, and a taken name gets `-2`, `-3` |
| Agent report | `agents/YYYY/YYYY-MM-DD/<host>--<agent>--<project>--<sid8>.md` |
| Digest | `agents/YYYY/YYYY-MM-DD/README.md` |
| Conflict copy | `<stem>.conflict-<host>-<YYYYMMDDTHHMMSS><ext>`, beside the original |

- The date in a name is the local day the file was created, shifted by `day_starts` (§9).
- A name that is already taken gets `-2`, `-3` and so on before the extension.
- Agent report components are slugs. `<sid8>` is the first 8 characters of the session id, lowercased.
- Names are case-sensitive in git, but clients must not create two names that differ only in case. macOS and Windows file systems treat them as the same file.

## 4. Notes

### Markdown

Notes use CommonMark plus the GitHub extensions: tables, task lists, strikethrough, autolinks and footnotes. Code blocks are fenced and name their language.

### Links

- A link to another note is relative and keeps the extension: `[Docker to Podman](../tech/2023-10-16-docker-to-podman.md)`.
- A link to a heading adds GitHub's anchor: `[Setup](2026-10-26-nas.md#setup)`.
- Spaces and other reserved characters in a path are percent-encoded (`%20`). New files never need this.
- External links are plain URLs or `[text](https://…)`.

### Images

`![alt](assets/2026-10-26-iptables.png)`, with the path relative to the note.

Put an image on a line of its own to have it shown as a picture: clients that can draw images
(nt in Ghostty and kitty) draw it there, also when text sits on the lines just above or below it.
An image inside a sentence, a list or a table is shown as its alt text. Always write alt text; it
is what every other client shows.

Clients write markdown images. They should also draw an HTML `<img src="…">` that is the only
thing in its block, wrappers like `<p align="center">` or `<a href="…">` included, because
GitHub READMEs and pasted snippets use that form to centre or size an image.

### Not written by clients

Clients must not write `[[wikilinks]]`, `![[embeds]]`, callouts, `%%comments%%`, or block references. They don't render on GitHub.

### Reading legacy wikilinks

Clients should still read `[[wikilinks]]`. `[[name]]`, `[[name|label]]` and `[[name#heading]]` resolve to the note whose file stem or title equals `name`, ignoring case. If several notes match, the one closest to the linking note in the folder tree wins.

### Frontmatter

Frontmatter is optional. It is YAML between two `---` lines at the very top of the file. Keys are flat: no nesting.

| Key | Type | Meaning |
|---|---|---|
| `title` | string | The note's title |
| `date` | `YYYY-MM-DD` | The day the note belongs to |
| `tags` | list of strings | Lowercase. `/` makes a hierarchy: `work/q3` |
| `type` | string | `note` (the default), `daily`, `agent-report`, `digest` or `weekly` |

- Clients must keep keys they don't know, in their order, when they rewrite frontmatter.
- **Title** is found in this order: the `title` key, then the first `#` heading, then the file stem.
- **Tags** also come from inline `#tag` words in the body, outside code. A tag starts with a letter and may contain letters, digits, `-`, `_` and `/`.

## 5. Daily journal

A journal is created the first time a day is opened or written to. It is never created ahead of time.

The default template is fixed by the date alone, with no creation time. Two devices that create the same day write identical bytes, so git merges them cleanly.

```markdown
---
title: Monday 26 October 2026
date: 2026-10-26
type: daily
---

# Monday 26 October 2026

[← 2026-10-25](2026-10-25.md) · [2026-10-27 →](2026-10-27.md) · [agents](../../agents/2026/2026-10-26/README.md)

## Log

## Notes
```

- Links are relative to the journal, so across a year they read `../2025/2025-12-31.md`.
- **Appending** (`nt a`, and anything else that appends to the journal) adds one line, `- HH:MM text`, at the end of the `## Log` section. If the section is missing, it is created at the end of the file.
- **Creating a topic note** adds `- [Title](relative/path.md)` at the end of `## Notes` in that day's journal.
- Everything else in the journal is the person's to edit.
- `daily/** merge=union` in `.gitattributes` keeps lines added on two devices without a conflict.
- **Order.** The line-by-line merge puts lines two devices added at the same time one device's
  after the other's, not in time order. Once a day is over, a client may put that day's `## Log`
  back in time order: by `HH:MM`, a time before `day_starts` after the others, a line without a
  time staying with the one above it, and a line that is there twice word for word, as the merge
  can leave it, kept once. Nothing else in the journal moves. Today's journal is left as it is,
  because lines moved in a file another device is still adding to come back twice from the merge.

## 6. Agent reports

- **One file per agent session.**
  - nt is the only writer: of flok's records of the session (`nt agent import`), of its own hooks' events until flok imports the session, and of the agent's own account (`nt agent write`).
  - The path is fixed by the date of the session's first event and never changes. A session that runs past midnight stays in its first day's folder.
  - A session flok imports stays in one file: each record is the whole session.
  - A session resumed after it ended, or on a later day, starts a new file, which links to the earlier one (`continues`). If the earlier one never ended, the writer ends it then: `ended` is its `updated`, its status is set as at an end, and its last log line links to the new file.
  - A writer that finds no report for the session (no hooks, or hooks added mid-session) starts one.
  - A session in a temporary folder (`/tmp`, `/var/tmp`, the system's temp folder) gets no report: it is a throwaway, such as a script's `claude -p` in a scratch folder.
- **The host comes first in the name** (§3), so two machines never write the same file, also for the same project, agent and day. Clients show a report's host beside its title, which is the project's, so the reports of one project from two machines read apart.

**Frontmatter.** All keys are flat. Times are RFC 3339 with offset.

| Key | Type | Rule |
|---|---|---|
| `type` | `agent-report` | |
| `schema` | `1` | |
| `date` | `YYYY-MM-DD` | the session's first day |
| `created` | time | set once |
| `updated` | time | never moves backwards |
| `ended` | time | set once, when the session ends; final |
| `agent` | slug | `claude-code`, `copilot-cli`, … |
| `agent_version` | string | optional |
| `model` | string | optional |
| `host` | slug | short host name |
| `session` | string | the agent's full session id |
| `project` | string | the name of the git repository around `cwd`, else of `cwd`; its slug goes into the file name |
| `cwd` | path | |
| `git_branch` | string | optional |
| `git_head` | string | optional, short hash |
| `status` | enum | `in-progress`, `done`, `blocked`, `failed`, `abandoned` |
| `summary` | string | one line, the outcome |
| `tags` | list | optional |
| `continues` | path | optional, the report this one resumes |
| `turns` | number | optional, the turns the session took (from flok) |
| `tools` | number | optional, the tools it ran |
| `blocked_minutes` | number | optional, the minutes it waited on a person: a permission, a question |
| `waited_minutes` | number | optional, the minutes it waited on background work |

- The hooks set `in-progress` at the start. At the end a report still `in-progress` becomes `abandoned` when it never finished a turn, `failed` when the session ended on an error, else `done`; a status the agent wrote stays.
- From flok's records: `blocked` while the session waits on a person, else `in-progress`; at the end as above, `abandoned` when no turn ended.
- Once `ended` is set, nothing sets `in-progress` again; a late turn still adds its log line.

**Body.**

```markdown
# <project>: <summary>

## Done
## Decisions
## Open
## Log
- 09:52 started in /Volumes/data/work/claude/noteTaking (main)
- 10:14 <first line of the agent's last message, at most 200 characters>
```

- `## Done`, `## Decisions` and `## Open` belong to the agent: `nt agent write` replaces them.
- `## Log` holds `started in <cwd> (<branch>)`, one line per turn, and `ended`. From flok's records a turn's line has what happened in it on indented lines under it: the tools it ran (`  - Bash ×12, Read ×3`) and its events with their times (`  - 13:48 blocked: permission: Bash`, `asked: …`, `waited 12 min for …`, `error: …`, `compacted`), every line at most 200 characters. A line is a single line of at most 200 characters: the first line of the agent's last message that has text, or `turn ended` when the hook carries no message (Copilot CLI's `agentStop` does not). A line that would repeat the log's last line, its minute included, is not added. Secrets and full transcripts do not belong in a report.

## 7. Digest

`agents/YYYY/YYYY-MM-DD/README.md` is generated from the frontmatter of that day's reports. People don't edit it.

- Only one machine generates digests: the one with `digest = true` in its machine config (§9).
- Output is deterministic and has no generation timestamp. The file is written only when its bytes change.
- Reports are grouped by project, in name order. Inside a project they are ordered by `created`.
- Each report is one line: glyph and status word, agent, host, summary, and a link to the report.

```markdown
# Agents · Monday 26 October 2026

[← journal](../../../daily/2026/2026-10-26.md)

## noteTaking
- ✓ done · claude-code · mbp · Phase 0 scaffold and design system · [report](mbp--claude-code--notetaking--00893aaf.md)
- ● blocked · copilot-cli · beta · Waiting on the API key · [report](beta--copilot-cli--notetaking--7c1d0e2a.md)
```

Glyphs: `◐` in-progress, `✓` done, `●` blocked, `◆` failed, `○` abandoned. The status word is always written next to the glyph.

### Weekly digest

`weekly/GGGG/GGGG-Www.md` is compiled from the week's notes. People don't edit it: it is written again when they change.

- `GGGG-Www` is the ISO week-year and week, Monday to Sunday: 29 December 2025 is in `2026-W01`, and 3 January 2027 in `2026-W53`. The days are the journals' days, shifted by `day_starts`.
- Only one machine compiles it: the one with `weekly = true` in its machine config (§9). Before each sync commits, it compiles the current week, the week before, and every week since the newest digest there is, so it catches up after being off. A change that a merge brings in reaches the digest on its next sync.
- A week with nothing in it gets no digest. The file is written only when its bytes change; nothing in it depends on when it was made.
- It has `type: weekly` frontmatter, the week as its title, links to the weeks before and after, and these sections, each left out when empty:
  - **Notes:** topic notes made that week, by the date in their names (`new`), then those changed that week, by the repository's commit history (`changed`, at most 20 by name, the rest counted). A commit that changed more than 20 notes was an import or a move: its notes are counted on one line, not listed. Journals, agent reports, digests, `todo.md` files and conflict copies are not listed.
  - **Todos:** those done that week (`✓` and the day), then the open ones due by its Sunday (`○` and the due date).
  - **Agents:** the week's sessions per project, with a count per status.
  - **Days:** for each day with a journal or agent sessions, the journal's `## Log` lines (their links made relative to the digest), then links to the journal and the day's agent digest.

```markdown
---
title: Week 44, 2026
date: 2026-10-26
type: weekly
---

# Week 44 · 26 October – 1 November 2026

[← W43](2026-W43.md) · [W45 →](2026-W45.md)

Compiled by nt from the week's notes; it is written again when they change.

## Notes

- [NAS setup](../../tech/2026-10-27-nas-setup.md) · tech · new

## Todos

- ✓ Pay the invoice · Tue
- ○ Renew passport · due Thu 29 Oct

## Agents

- noteTaking · 9 sessions: 1 blocked, 8 done

## Days

### Monday 26 October

- 09:12 Called the bank

[journal](../../daily/2026/2026-10-26.md) · [3 agent sessions](../../agents/2026/2026-10-26/README.md)
```

### Weekly summaries

`weekly/GGGG/GGGG-Www-<host>.md` is one machine's summary of a week, written by an AI tool on
that machine, about what was written on that machine: three short lists, Done, Decided and Open.

- A machine writes summaries only when its machine config names a tool (`[summary] tool`, §9),
  and writes only its own file, once the week is over, so every summary has one writer.
- "Written on that machine" is read from the repository: the commits sync made there (their
  message starts `nt: <host> ·`, merges left out) and the agent reports with that `host`. The
  journal lines, todos and notes another machine wrote never go to this machine's tool, so a work
  machine's week stays with the work machine's tool, whatever folder it is in.
- What the tool reads: the week's journal lines, the todos added or done, the notes made (their
  text) or changed (the lines added), a commit of more than 20 notes only counted, and each agent
  session's summary, Done, Decisions and Open, at most about 40,000 characters. It is asked for
  the three lists, about outcomes rather than what the notes say, without tests, secrets or
  personal data.
- The body is what is kept of the answer: each list as a bold label (`**Done**`, `**Decided**`,
  `**Open**`) and one to four bullets of plain text, at most 200 characters each. Headings, other
  text and checkboxes are dropped, so no bullet is a todo; a list with nothing is left out. An
  answer without the labels is kept as one paragraph.
- The file has `type: weekly-summary` frontmatter with `host`, `tool` and, when one is set,
  `model`, then the body. It is not rewritten when the week's notes change later.
- The weekly digest shows each machine's summary under `## Summary`, after its title.

```markdown
---
title: "Week 44, 2026 · mbp"
date: 2026-10-26
type: weekly-summary
host: mbp
tool: claude
---

**Done**

- The NAS got four disks in RAID-Z1 and a nightly restic backup.
- infra: the Terraform for the new cron host is finished.

**Decided**

- Keep restic and drop the Synology Hyper Backup job.

**Open**

- website: waits on the DNS change.
```

## 8. Templates

- Templates are in `.nt/templates/` and use Go `text/template`. Missing templates fall back to the built-in defaults.
- Variables: `.Title`, `.Slug`, `.Date` (`2026-10-26`), `.Time` (`14:03`), `.Weekday` (`Monday`), `.Long` (`Monday 26 October 2026`), `.Yesterday`, `.Tomorrow`, `.Folder`, and `.Rel`, a function that returns the relative path from the new note to a repository path.
- Also `.YesterdayJournal`, `.TomorrowJournal` and `.AgentsDigest` (repository paths, for `.Rel`), and the function `yaml`, which quotes a string for frontmatter: `title: {{yaml .Title}}`.
- `daily.md` must use only date-derived variables (§5).

## 9. Configuration

`.nt/config.toml` is committed and shared by every device:

```toml
format = 1
default_folder = "inbox"   # where `nt n` puts a note when no folder is given
day_starts = "04:00"       # local time before which entries count toward the previous day
pins = ["inbox", "tech", "work", "agents"]   # folders on keys 1-9

[folders]
daily = "daily"
agents = "agents"
weekly = "weekly"
```

`nt config` lists both files' settings and changes them one line at a time, keeping comments; the
browser's `,` does the same.

The machine config `~/.config/nt/config.toml` stays on one machine and is not committed:

```toml
repo = "~/notes"
host = "mbp"               # default: the short host name
digest = true              # this machine writes the agent digests (one machine only)
weekly = true              # this machine compiles the weekly digest (one machine only)
image_viewer = ""          # default per OS: qlmanage -p / xdg-open / chafa in a tmux popup
inline_images = "auto"     # auto | on | off: draw images in the preview (Ghostty, kitty)

[sync]
auto = true                # a timer runs `nt sync`; false: only the s key and `nt sync`
interval = "2m"

[theme]
mode = "auto"              # auto | dark | light

[summary]
tool = "claude"            # claude | copilot: write this machine's weekly summaries with it; "" none
model = ""                 # optional: the model the tool uses, e.g. "haiku"
```

## 10. Writing and syncing

These rules apply to every client that writes to the repository.

- **Atomic writes.** A file is written to a temp file `.nt-tmp-*` in the same folder, then renamed over the target.
- **The repository lock** is `.git/nt.lock`, taken with flock(2) LOCK_EX.
  - Every write takes it for as long as the write lasts.
  - Sync takes it around staging and committing, and around merging and resolving. It does not hold it while talking to the remote.
- **One git writer per machine.** Agents, the TUI and scripts write files. Only `nt sync` runs git commands that change the repository.
- **Merge, never rebase.** Sync runs `git fetch`, then `git merge --no-edit origin/<branch>`, and fast-forwards when it can.
- **Conflicts never leave markers.**
  - When both sides changed a file, the remote version stays in place. The local version is saved as a conflict copy (§3).
  - When one side changed a file and the other deleted it, the change wins.
  - Digests, the days' and the weeks', are regenerated: the remote version is taken and the machine that writes them writes them again.
  - Journals don't conflict, because of the union rule.
- **Large files.** Files over 20 MB are not committed. Sync reports them instead.
- **Interrupted merges.** A client that finds an unfinished merge (`MERGE_HEAD` exists, or files are unmerged) finishes the resolution before it does anything else.
- **Stopping.** A client that finds a rebase in progress, a detached HEAD or an unexpected branch stops and reports it. It does not repair the repository.

## 11. Ignore and attribute files

`.gitignore`:

```
.nt-tmp-*
*~
4913
.*.sw?
.DS_Store
.obsidian/
```

`.gitattributes`:

```
daily/** merge=union
todo.md merge=union
```

## 12. Todos

A todo is a task list item in any note. A `todo.md` is where clients add new ones: the one at the root, or the one in a folder the person names.

```markdown
# Todo

## Inbox

- [ ] Renew passport due:2026-11-03
- [x] Pay the invoice done:2026-10-27

## Work

- [ ] Review the Q3 report due:2026-10-30
```

- **A todo** is a list item whose text starts with a checkbox: `- [ ] text` is open, `- [x] text`
  (or `[X]`) is done. The bullet can be `-`, `*`, `+` or a number (`1.`); any indent is allowed.
  A checkbox with no text is not a todo.
- **Not todos:** lines in frontmatter or in fenced code blocks, and task lines in the agents
  folder (§6), in conflict copies (§3) and in hidden folders such as `.nt/templates/`.
- **Due date:** `due:YYYY-MM-DD` anywhere in the text. Clients accept words such as `fri` or
  `+3d` when a person types a date, and always write the date as `YYYY-MM-DD`.
- **Done date:** checking a todo appends `done:YYYY-MM-DD`, the day it was done as `day_starts`
  counts it. Unchecking it takes the date away. A todo checked in an editor, with no `done:`,
  is done all the same.
- **The text** a client shows is the line after the checkbox without `due:` and `done:`.
- **`todo.md`** starts as `# Todo` and `## Inbox`. A client adds a new todo, `- [ ] text` with
  its `due:` when it has one, at the end of `## Inbox` of the root's `todo.md`, or of
  `<folder>/todo.md` when the person names a folder, and adds the file or the section when it is
  missing. The agents folder and hidden folders have no todo list. The rest of the file is the
  person's: other sections, order, notes between items.
- **Changing a todo** rewrites its one line, and only when the line is still as the client read
  it.
- **Merging:** `todo.md merge=union`, which matches a `todo.md` in any folder, keeps todos added
  on two devices without a conflict, as it does for journals. In the rare case that one device changes the last todo while the other adds
  one right after it, both versions of the changed todo are kept, and the person deletes one.

## 13. Moving and renaming

A client that moves or renames a note or a folder keeps every link working. Under the
repository lock, in the same pass as the move, it:

- rewrites every relative link and image to a file that moved, in every note, to point at the
  file's new place;
- rewrites the relative links in a note that moved, to point at the same files from its new place;
- keeps a link's `#anchor`, its trailing `/` and its title;
- turns a legacy `[[wikilink]]` to a note that moved into a markdown link;
- moves the attachments in the note's own `assets/` folder that no other note names into the
  `assets/` folder at its new place. Attachments other notes use stay, and the links to them are
  rewritten.

Renaming a note:

- sets `title` in its frontmatter, adding the key when it is missing, and its first `#` heading
  when the title came from that heading or it showed the old title;
- renames the file after the new title, `YYYY-MM-DD-<slug>.md`, keeping its date;
- gives every link to the note whose text was the old title, such as its line in the journal,
  the new title.

Rules:

- New folder names are slugs (§3). Folders that exist keep their names, whatever case is typed.
- Journals, agent reports and digests are not moved or renamed, and neither are the `daily/`,
  `agents/` and `weekly/` folders. Nothing is moved into them.
- `pins` and `default_folder` in `.nt/config.toml` follow a folder that moves.
- A new name that differs from the old one only in case is refused: git on macOS would not see
  the change.
- Links in code, URLs, paths outside the repository and links to files that do not exist are
  left as they are.

Sync commits a move as files deleted and added, and git's merge finds the rename:

- An edit another device made to the note at its old path is merged into the note at its new
  place.
- A note another device deleted stays, at its new place: a change wins over a deletion (§10).
- A note two devices moved to different places at once is kept in both. Nothing is lost; delete
  the one you do not want.
- A note whose links the move rewrote, changed on another device near those lines at the same
  time, gives a conflict copy, as any note changed on both sides does.
- A link to the old path that another device wrote before it had the move points at nothing.
  `nt doctor` lists such links.

## 14. Changes to this format

A change that older clients would misread raises `format` in `.nt/config.toml`. A client must not write to a repository with a higher `format` than it knows. Adding an optional frontmatter key is not such a change.
