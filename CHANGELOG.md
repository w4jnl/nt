# Changelog

User-facing changes per release, newest first. `scripts/release.sh` refuses to tag a version
that has no section here and uses the section as the GitHub release notes, so every release
updates this file first. Dates are the tag dates.

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
