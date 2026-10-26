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
  .nt/templates/                  daily.md, note.md, report.md (§8)
  .gitattributes                  daily/** merge=union
  .gitignore                      §11
  AGENTS.md                       layout and rules for agents working in the repository
  README.md                       optional, owned by the person
  daily/YYYY/YYYY-MM-DD.md        daily journal, one per day (§5)
  daily/YYYY/assets/              attachments pasted into journals
  agents/YYYY/YYYY-MM-DD/         agent reports for that day (§6)
  agents/YYYY/YYYY-MM-DD/README.md   the day's digest (§7)
  <folder>/…/YYYY-MM-DD-slug.md   topic notes, in any folder the person creates
  <folder>/…/assets/              attachments for the notes in that folder
```

- `daily/` and `agents/` are reserved. Their names can be changed in `.nt/config.toml`.
- Any other folder is the person's. Folders can nest to any depth.
- Names that start with `.` are ignored by clients, except `.nt/`, `.gitattributes` and `.gitignore`.

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
| `type` | string | `note` (the default), `daily`, `agent-report` or `digest` |

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

## 6. Agent reports

- **One file per agent session.**
  - The session's process is the only writer: the hook receiver, or the agent through `nt agent`.
  - The path is fixed by the date of the session's first event and never changes. A session that runs past midnight stays in its first day's folder.
  - A session resumed on a later day starts a new file, which links to the earlier one.

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
| `project` | slug | the repository or directory name of `cwd` |
| `cwd` | path | |
| `git_branch` | string | optional |
| `git_head` | string | optional, short hash |
| `status` | enum | `in-progress`, `done`, `blocked`, `failed`, `abandoned` |
| `summary` | string | one line, the outcome |
| `tags` | list | optional |
| `continues` | path | optional, the report this one resumes |

`status` changes only to a later state. Once `ended` is set, a late event cannot set `in-progress` again.

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
- `## Log` is appended to, one line per turn. A line is a single line of at most 200 characters. Secrets and full transcripts do not belong in a report.

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
```

The machine config `~/.config/nt/config.toml` stays on one machine and is not committed:

```toml
repo = "~/notes"
host = "mbp"               # default: the short host name
digest = true              # this machine writes digests (one machine only)
image_viewer = ""          # default per OS: qlmanage -p / xdg-open / chafa in a tmux popup
inline_images = "auto"     # auto | on | off: draw images in the preview (Ghostty, kitty)

[sync]
auto = true                # a timer runs `nt sync`; false: only the s key and `nt sync`
interval = "2m"

[theme]
mode = "auto"              # auto | dark | light
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
  - Digests are regenerated.
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
```

## 12. Changes to this format

A change that older clients would misread raises `format` in `.nt/config.toml`. A client must not write to a repository with a higher `format` than it knows. Adding an optional frontmatter key is not such a change.
