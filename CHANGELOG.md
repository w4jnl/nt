# Changelog

User-facing changes per release, newest first. `scripts/release.sh` refuses to tag a version
that has no section here and uses the section as the GitHub release notes, so every release
updates this file first. Dates are the tag dates.

## 0.9.0 (2026-11-01)

Code coloured, math and diagrams drawn, and tables as tables in nt for Mac and on the phone;
Tab in the capture panel picks a todo's folder.

- **Code, math and diagrams in the apps:** a fenced block in shell, go, python, javascript or
  typescript, json, yaml, swift, sql, css, html, rust, c or c++ is coloured as the terminal
  colours it (keywords and types in accent, strings in ok, numbers in warn, comments muted);
  another language, or none, stays plain. Math as pandoc writes it, `$x$` inline and `$$ … $$`
  on its own lines, is drawn by KaTeX ("$5 and $10" stays money). A `mermaid` fence is drawn
  as a diagram in nt's colours, light and dark. With the cursor on any of them, the source
  shows. The terminal coloured code already; it shows math and diagrams as their source.
- **Tables are drawn as tables** in the apps' live preview, the header set apart and the cells'
  code, emphasis and links kept; the source shows with the cursor on them.
- **A note opened while the editor had the keyboard** showed no line's markdown, so a click on
  a table left it drawn with the cursor at its edge. Fixed.
- **Tab in the capture panel** (⌥⌘Space) opens the folder list for a todo: the root's `todo.md`
  first, then every folder that takes todos with its open count. Typing narrows the list, ↑↓
  choose, Return or Tab take the folder, esc goes back; the field keeps the keyboard
  throughout. The panel also makes itself key again shortly after it shows, so its field takes
  the keys.
- **nt for Mac no longer quits unexpectedly** when a sync finishes while the menu bar mark's
  menu is open (an update loop in the menu's clock). The sidebar's sync state sits under the
  list instead of over its last rows.
- **README and docs:** pictures of the Mac app and the phone, retaken by `assets/demo/mac.sh`
  and `assets/demo/phone.sh`; the setup covers both apps.

## 0.8.1 (2026-10-30)

A session resumed on a later day is reported in that day's folder again.

- **One report per session and day:** `nt agent import` wrote a session resumed on a later day
  into the report of the day it began, so that day's digest left it out; nt's hooks had kept a
  report per day, and the import now does too. A session that goes on on a later day, or runs
  again after it ended, goes on in a new report in that day's folder: `continues` links the
  earlier report, which is ended at its last update and links on from its Log. A session past
  midnight, before `day_starts`, stays in one report. A report's `created` is no longer
  overwritten by flok's.

## 0.8.0 (2026-10-30)

New Todo in nt for Mac's menus, and flok writes every agent session's report.

- **New Todo… in nt for Mac:** a right-click on a folder, at any depth, has New Todo…, which
  asks for the text and adds it under Inbox in that folder's `todo.md`. File › New Todo… (⇧⌘N)
  does the same in the folder ⌘N would use, and the Folders heading's menu in the root's
  `todo.md`. The journals', agents' and weekly folders take no todos.
- **File › Add to Today…** shows; it was missing from the menu since 0.5.1.
- **nt's own agent hooks are gone:** [flok](https://github.com/w4jnl/flok) 0.13 or later, with
  its `[notes]` on, writes the session reports through `nt agent import`. `nt install --hooks`
  says so and installs nothing. On a machine that still has nt's hooks,
  `nt agent start|turn|end` read their input and do nothing, `nt doctor` warns about them,
  and `nt uninstall --hooks` takes them out and leaves flok's; both go in a later release.
  `nt agent write` still starts a report for a session flok does not follow.

## 0.7.0 (2026-10-29)

Tags as a search filter, todos by folder, and agent reports from flok.

- **`#tag` in a search:** `nt s '#docker'`, the browser's `/`, and ⌘P and ⇧⌘F in nt for Mac find
  the notes tagged docker, by their frontmatter's `tags` or a `#docker` in the text; `#work` also
  finds `work/q3`. A tag alone lists each note once, newest first; with words, their lines in
  those notes. In a shell, quote it.
- **Tags in nt for Mac** are in the note's folded frontmatter line in accent; a click searches
  for the tag.
- **Todos by folder:** the browser's todos take `f`, a picker of every note and the top-level
  folders with open todos; in nt for Mac, "from every note" in the Todos summary is the same as
  a menu. A todo added there goes to that folder's `todo.md`.
- **`nt agent import`:** [flok](https://github.com/w4jnl/flok), with its `[notes]` on, sends
  each agent session's record, and nt writes the report from it: a Log line per turn with the
  tools it ran and where it was blocked, asked or waited, and the counts `turns`, `tools`,
  `blocked_minutes` and `waited_minutes` (optional keys; no format change). nt's own hooks
  leave the sessions flok imports alone, and `nt doctor` says when flok last imported one.
  Once flok reports every session on a machine, `nt uninstall --hooks` there; a later release
  removes the hooks.

## 0.6.0 (2026-10-29)

Notes and folders made, renamed, moved and deleted in nt for Mac, `nt rm`, and search without
rg. Underneath, the groundwork for nt on iPhone and iPad, which is in testing and not released
yet.

- **`nt rm <note|folder>`** deletes a note, with the images no other note uses and its line in
  its day's journal, or an empty folder. `--dry-run` says what would go, `--json` also how many
  notes link to it. The browser's `D` does the same; sync commits it, and git keeps the files.
- **Notes and folders in nt for Mac:** a folder in the sidebar selects, as the place ⌘N makes a
  note in. A right-click on a folder or a note has New Note, New Folder, Rename, Move and
  Delete. A delete says what goes with it and which notes link to it; a move shows the folders.
- **Search without rg:** `nt s` and the browser's search use rg when it is there, else nt's own
  search, which finds the same lines (smart case, markdown only, hidden folders left out).
- **Under the hood:** the operations the apps call live in one place (`internal/ops`), so the
  command line, nt for Mac and the phone print the same JSON; sync reaches git through an
  interface, so the phone can sync with libgit2. Nothing changes on the command line.

## 0.5.1 (2026-10-28)

nt for Mac, the first release: `brew install --cask w4jnl/tap/nt-app`, or the DMG below. It
needs macOS 26 and a notes repository nt set up; it ships on its own, with its own nt inside.

- **The note in live preview,** as Typora shows it: a line's markdown only while the cursor is
  on it, images drawn in place, links that follow, tasks you tick with a click. What you write
  is saved through nt as the nvim plugin saves it, never under a sync's merge.
- **The sidebar:** Today, Todos, Calendar, Weekly and Agents, the pinned folders, every folder,
  and the sync state at its foot. ⌘P opens a note by its title, ⇧⌘F searches the text, ⌘N makes
  one, ⇧⌘L is focus mode.
- **The views:** every todo, open then done, checked with a click; the month with its journals
  and the todos due; the weekly digest; the agent sessions of the last two weeks from every
  machine.
- **The menu bar and capture:** nt's mark in the menu bar, and ⌥⌘Space from anywhere opens a
  field that adds a line to today, or with Shift-Return a todo. macOS keeps ⌥⌘Space for the
  Finder search window until it is turned off; Settings › App says so, or records another key.
- **Settings:** nt's own, this Mac's and every machine's, and the app's: the capture shortcut
  and the text size.
- Signed with a Developer ID and notarized by Apple, so it opens without a warning.

## 0.5.0 (2026-10-28)

A home in the browser, and nt as a viewer for any file.

- **The browser starts at home.** The root of the left pane begins with Today, Todos, Calendar,
  Weekly and Agents, each with its key at the right, then the pinned folders with their numbers,
  then the folders, a rule between the groups. Selecting one shows it on the right: Today is
  selected at the start and shows today's journal; Todos the open todos, overdue first, with
  their notes and dates; Calendar this month; Weekly the newest weekly digest; Agents today's
  digest. Enter does what the key does. Inside a folder the list is as before.
- **`w`** goes to the newest weekly digest.
- **Newest first, folders too.** Folders named by a date (the years in `daily/`, `agents/` and
  `weekly/`, the days in `agents/2026/`) list newest first, as the notes do; other folders stay
  A to Z, before them. `nt ls` lists folders the same way.
- **A day's agent digest comes first** in its folder, then its reports by when the session
  began, the latest first, where they were in the order of their file names.
- **The todos beside the folders.** `x`, or enter on the home list's Todos, shows every todo in
  the note's pane, as the Mac app does: the open ones, then the done ones under Done (`X` folds
  them). `j` `k` move, `x` checks, `d` sets the due date, `a` adds one, `e` edits, enter opens
  the note at the todo, esc goes back; the folder list stays as it was.
- **A tick shows before the todo moves.** `x` turns the row's glyph and keeps it in place until
  the ticks settle, 0.7 s after the last, so another `x` takes a slip back; then it moves under
  Done, or back up, and the next one is selected.
- **`~`** goes to the root from any folder, on the top-level folder you were in; at the root,
  to Today.
- **`nt <file>`** shows a note, or any text file, on the whole screen, from anywhere: `nt
  README.md`, `nt ~/Downloads/spec.md`. Commands come first, and no command has a `.` or a `/`
  in its name, so `nt notes.md` or `nt ./sync` always means the file.
- **The viewer reads like the browser.** `f` and a letter, or a click, follow a link: a note or a
  text file opens in the viewer, an image in the image viewer, anything else with the OS. `b`
  goes back, `o` and `i` list the outline and the images, the mouse selects text and `y` copies
  it. A file changed on disk is shown again.
- **Text as written.** A file that is not markdown is shown line for line, not reflowed into
  paragraphs; a binary file is refused.
- **Agent reports end when the session goes on.** A session resumed or compacted on a later day
  starts a new report that continues the earlier one; the earlier one now ends at its last
  update, with a line that links to the next, instead of reading in progress for good.

## 0.4.2 (2026-10-28)

- **`nt doctor` finds hooks that cannot run.** When the nt that the agent hooks (Claude Code's
  or Copilot CLI's) or the background sync job run is gone, as when `~/.local/bin/nt` is removed
  for Homebrew's, the hooks failed without a word and no session got a report. `nt doctor` now
  says so and how to fix it: `nt install --hooks`, `nt install --sync`. It also reports Copilot
  CLI's hooks, not only Claude Code's.

## 0.4.1 (2026-10-28)

Agent reports from two machines read apart, without noise.

- **Which machine.** The browser shows an agent report's host, faint, before its title, as it
  shows a note's date: `work vuln-remediation`, `jaromac flok`. Reports of one project from two
  machines never share a file, since the host is the first part of the name, and now read apart
  too.
- **No throwaway sessions.** A session in a temporary folder (`/tmp`, `/var/tmp`, `$TMPDIR`), such
  as a script's `claude -p` probe in a scratch folder, gets no report.
- **No repeated lines.** Copilot CLI sends several `agentStop`s for one turn; a log line that
  repeats the last one in the same minute is no longer written again.

## 0.4.0 (2026-10-27)

Weekly summaries, and settings from nt.

- **Weekly summaries.** A machine whose config names a tool (`[summary] tool = "claude"` or
  `"copilot"`) has it write, once a week is over, what was written on that machine in three
  short lists, Done, Decided and Open: its journal lines, todos, notes and agent sessions, read
  from the commits sync made there. Each machine writes its own file,
  `weekly/GGGG/GGGG-Www-<host>.md`, so a week written at work goes only to the work machine's
  tool. The weekly digest shows them all; `nt week --summarize` writes one by hand, and
  `nt doctor` checks the tool.
- **Settings from nt.** `nt config` lists every setting of this machine and of the repository,
  with its value and what it does; `nt config set <key> <value>` changes one, in the right file,
  keeping its comments and refusing a value that would not load; `nt config get` and
  `nt config edit [--repo]` too. In the browser `,` shows them in the note's pane, beside the
  folders: enter flips a setting or asks for its value, `e` opens its file.
- The browser's own sync (`s`) now also compiles the weekly digest and writes the summaries, as
  the timer's does.
- The README's browser recording comes from a real Ghostty window, with a note's image drawn
  inline (`assets/demo/ghostty.sh`).
- goldmark 1.8.6 and `golang.org/x/text` 0.43.0.

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
