# nt — design document

The architecture and the build plan. [format.md](format.md) is the repository contract,
[design-system/README.md](design-system/README.md) the brand and terminal UI rules, and
[research.md](research.md) the survey this design came from. This document is the source of
truth for how nt works; change it before changing the code when the two would disagree.

Decisions taken with the user (2026-10-26):

- The name is `nt`. Notes live in a private repository (`<you>/notes`); this repository is the tool.
- Links are standard markdown links; legacy `[[wikilinks]]` are read, never written.
- One journal per day, plus dated topic notes linked from that day's journal.
- Sync is automatic on a configurable timer, and manual with `s` or `nt sync`.
- Images are drawn in the preview where the terminal can (Ghostty and kitty, also inside tmux), and
  open full size outside it: Quick Look, `xdg-open`, or `chafa` in a tmux popup. Inline drawing
  moved from "later" to the TUI phase after the phase 0 image spike.
- The design language is the w4jnl family's, from the first commit.


## 1. Repo contract (`docs/format.md`, which the Mac and iOS apps must follow)
```
~/notes/      (default; after archiving ~/Notes → ~/Notes.obsidian, see Cutover)
  AGENTS.md                         layout + frontmatter rules for any agent in the repo
  .nt/config.toml                   committed: pins (1-9), default_folder, day_starts, templates
  .nt/templates/{daily,note}.md   Go text/template: .Title .Date .Time .Weekday .Yesterday .Tomorrow
  .gitattributes                    daily/** merge=union
  .gitignore                        .nt-tmp-*  *~  4913  .*.sw?  .DS_Store  .obsidian/
  daily/2026/2026-10-26.md          journal (deterministic template → identical creations merge cleanly)
  <topic>/2026-10-26-project-report.md    dated topic note: slug filename, real title in frontmatter/H1
  <topic>/assets/2026-10-26-x.png   attachments next to notes → ![](assets/x.png)
  agents/2026/2026-10-26/README.md  daily digest (generated; GitHub renders it when you open the folder)
  agents/2026/2026-10-26/<host>--<agent>--<project>--<sid8>.md   one report per agent session
```
**Format rules:**
- Only GitHub-flavoured markdown is written. No wikilinks, embeds or callouts.
- Filenames are slugs with no spaces, so links work in the shell, on GitHub and on iOS. Any legacy name with spaces is linked with `%20`.
- Frontmatter is flat YAML: `title, date, tags`.
- Reports also carry: `type: agent-report, schema, date, created, updated, ended, agent, host, session, project, cwd, git_branch, git_head, status, summary, tags`.
- Creating a topic note appends `- [Title](../../topic/x.md)` under `## Notes` in that day's journal.
- `day_starts = "04:00"` makes late-night entries count toward the previous day.

## 2. Sync: the only git writer, merge only, never blocks

**One lock.** `.git/nt.lock`, taken with `syscall.Flock` (LOCK_EX):
- Every `nt` write takes it for milliseconds. Writes go to a temp file `.nt-tmp-*` and are then renamed into place, as flok's `writeJSON` does.
- Sync takes it around add+commit and around merge+resolve. It is not held during fetch or push.

**`nt sync` is one-shot.** One code path, triggered by:
- a launchd agent with `StartInterval` set from `sync.interval` (default `2m`), installed by `nt install --sync`; on Linux, a systemd user timer;
- the TUI starting;
- nvim exiting after a note was written (the nvim plugin, below);
- the `s` key.

**Each cycle:**
1. Recover first. If `MERGE_HEAD` exists or there are unmerged files, resolve them before anything else. If a rebase is in progress, HEAD is detached, or the branch is wrong, stop and record the error in `status.json`.
2. `git add -A`.
   - Files over 20 MB are skipped, with a warning.
   - Commit message: `nt: <host> · 3 files (daily, agents)`.
3. `git fetch`, then `git merge --no-edit origin/main`. This is explicit, so the global `pull.rebase` doesn't apply, and it fast-forwards when possible.
   - If `origin/main` doesn't exist yet, push with `-u`.
4. Resolve conflicts per file from the index stages:

   | Case | What happens |
   |---|---|
   | Both sides edited | The remote version (`:3`) stays in place. The local version (`:2`) is saved as `<stem>.conflict-<host>-<ts><ext>`. Images work the same way. |
   | One side deleted, the other edited | The edit wins. |
   | Generated digest | Rebuilt. |
   | Journals | Merged with union, so they don't conflict. |

   There are never conflict markers in the tree.
5. Push. A non-fast-forward rejection is retried up to 5 times with jitter. A network or auth failure sets `offline` and waits for the next cycle.

**Environment.** The launchd/systemd unit sets `GIT_SSH_COMMAND="ssh -oBatchMode=yes -oConnectTimeout=10"`, `GIT_TERMINAL_PROMPT=0`, and an explicit PATH.

**Status** is written to `$XDG_STATE_HOME/nt/status.json`: synced, ↑N ahead, conflicts, offline, error. The TUI and flok read it. A new conflict also sends an OS notification (`osascript`, or `notify-send` on Linux).

**Editing safety.** While `nt` has nvim open, it holds `.git/nt-edit`. Sync still commits but skips the merge until nvim exits. This avoids nvim's W12 prompt silently reverting the incoming change. In an nvim started any other way, the nvim plugin holds it only while a note has unsaved changes, the one time a merge underneath can conflict; a long-lived nvim does not block merges for days, and the notes a merge changed reload.

**Manual mode.** `sync.auto = false` installs no timer; the `s` key and `nt sync` still work.

## 3. Agents (the contract for flok)

**`nt agent start|turn|end`** (`internal/agents`). Reads a hook payload on stdin, in either the Claude (snake_case) or Copilot (camelCase) shape.
- Fast and deterministic; always exits 0 and prints nothing. Failures go to `agent.log` in the state folder.
- Claude Code: `SessionStart` → start, `Stop` → turn, `SessionEnd` → end; a subagent's events (`agent_id`) are skipped. Copilot CLI: `sessionStart`, `agentStop`, `sessionEnd`, with `--agent copilot-cli` because its payloads do not name the agent.
- The hooks run in order, not in the background (`async`): each takes milliseconds, and a session's events then reach its report in the order they happened. Every write is under the repository lock.
- The report path is fixed by the date of the session's **first** event. A `sid → path` map lives in the state dir, so sessions that cross midnight stay in one file.
- `turn` updates `updated` (the max) and adds one `## Log` line of at most 200 characters, taken from `last_assistant_message`.
- `end` sets `ended`, which is final, the final `git_head`, and a status from the outcome (docs/format.md §6). A late Stop can't flip the status back. It also starts a sync, so the report goes out at once.

**`nt agent write [--status s] [--summary text] [--session ID]`.** The agent's own rich report (Done / Decisions / Open), read from stdin. Without `--session` it fills the latest report whose session started in the working folder or one above it; with none, it starts one, so an agent without hooks can report too.

**`nt digest [date]`.**
- Builds the day's `README.md` from the reports' frontmatter, grouped by project: status, summary, link.
- Output is deterministic and the file is written only when the bytes differ, so it causes no commit churn.
- Sync runs it before each commit for every day whose folder or reports are newer than its digest. Only the machine with `digest = true` does this, so it has one writer. When both sides changed a digest, sync takes the remote one without a conflict copy and the next run rebuilds it.
- `nt digest [date] [--json]` writes it by hand and lists the day's reports.

**`skills/nt/SKILL.md`.**
- Carried in the binary; `nt install --skill` writes it to `~/.local/share/nt/skills/nt` and symlinks that from `~/.claude/skills/nt` and `~/.agents/skills/nt`, so it works in Claude Code and Copilot CLI alike and always matches the installed `nt`.
- Covers search, read, `nt a`, and `nt agent write`.
- Every command takes `--json`.
- The notes repo also gets an `AGENTS.md`, written by `nt init` when missing.

**flok later.** `flok hook` calls `nt agent …`. Until then, `nt install --hooks` adds hooks next to flok's in Claude Code's `settings.json` (a backup first, other hooks kept) and writes `~/.copilot/hooks/nt.json`, as `flok/internal/install` does. `nt uninstall --hooks` takes them out; `nt doctor` checks them and the skill.

## 4. CLI and TUI (simplicity first)

**CLI.** Every command takes `--json`.

| Command | What it does |
|---|---|
| `nt t` | Open today in `$EDITOR` |
| `nt a "text"` / `pbpaste \| nt a` | Append a timestamped line to today |
| `nt n "title" [folder]` | New dated note in the folder, or in `default_folder` |
| `nt f` | `rg --files \| fzf --preview 'nt _preview {}'` (nt's own style), then open the pick in `$EDITOR` |
| `nt s <query>` | Search with `rg` |
| `nt ls <folder>` | List notes in a folder |
| `nt paste <note> [--name words]` | Save the clipboard's image as `assets/YYYY-MM-DD-<name>.png` beside the note and print its `![name](assets/…)` line; nvim maps a key to it |
| `nt sync`, `nt doctor` | Sync now; check the setup |
| `nt init [--clone url]` | Set up a repo, or clone one on a second machine |
| `nt install --sync`, `nt uninstall --sync` | Start or stop the background sync job (launchd `nl.w4j.nt.sync`, systemd `nt-sync.timer`) |
| `nt install --nvim`, `nt uninstall --nvim` | Write or remove nt's nvim plugin (below) |
| `nt install --hooks`, `--skill` | Agent hooks for Claude Code and Copilot CLI; the agent skill (§3) |
| `nt agent start\|turn\|end`, `nt agent write` | Agent session reports (§3) |
| `nt digest [date]` | Write and list a day's agent digest (§3) |
| `nt completion bash\|zsh` | Print a completion script: commands, flags and their values, and the repository's folders and notes (from the hidden `nt _complete folders\|notes`), as flok's |
| `nt import obsidian <vault> [--dry-run]` | One-shot import of an Obsidian vault (phase 1) |
| `nt` / `nt <folder>` | Start the TUI, optionally in that folder |

**TUI: one screen, two panes.**
```
┌ folder browser (dated names sort themselves) ┐┌ preview: glamour v2, frontmatter stripped ┐
│ daily/ agents/ tech/ …  ▸ notes, newest first ││ … rendered note …                          │
│                                               ││ Linked from: [3 notes]  Images: [2]        │
└───────────────────────────────────────────────┘└────────────────────────────────────────────┘
 ✓ synced · ↑2 · ⚠ 1 conflict        t today  [ ] day  a append  n new  / search  ? help
```

| Key | Action |
|---|---|
| `t` | Today |
| `[` `]` | Previous / next day |
| `A` | Agents day folder, which opens on its digest |
| `1`–`9` | Pinned folders, so preselected subfolders are one key away |
| `a` | One-line append (text input) |
| `n` | Title prompt, then a note in the current folder, opened in nvim |
| `/` | Live `rg --json` search |
| `ctrl-p` | Fuzzy title search |
| `e` | Open in nvim at the current line via `tea.ExecProcess`; reload on return. fsnotify also reloads live. |
| `l` | Link picker; then follow the link |
| `o` | Outline picker |
| `i` | Image picker |
| `v` | Open in `glow` |
| `y` | Copy the text selected in the note, else the note's path: `pbcopy`, `wl-copy` or `xclip`, else OSC 52 |
| `D` | Delete the note (with the images no other note uses, and its line in the journal of its day) or an empty folder, after a y/n question on the footer line |
| `s` | Sync now |
| `<` `>` | Narrower / wider list; dragging the divider with the mouse does the same |

- `l`, `o` and `i` come from `internal/mdparse`, a line scan that skips code and keeps source line numbers. Glamour has no way to map rendered lines back to the source.
- Backlinks ("Linked from") come from an in-memory scan (`internal/scan`), with no persisted index. Titles for `ctrl-p` come from the same scan, matched by nt's own subsequence scorer.
- The browser polls once a second instead of using fsnotify: the note on screen, the folder and `status.json`. It catches edits, merges from sync and agents' writes alike, without a dependency.
- While a picker is open the preview follows its highlighted row; Escape gives it back to the list.
- `i` opens an image with `image_viewer`, else Quick Look, else `xdg-open` on a Linux desktop, else `nt _image` (chafa, full screen) in the terminal.
- The mouse: dragging over the note selects its text, on `accent-soft`, within the note pane (the terminal's or tmux's own selection runs across both panes); `y` copies it without the margin, Escape clears it. Dragging the `│` divider resizes the list, the wheel scrolls the pane under the pointer, a click selects a row and a second click opens it, a click on the note gives it the focus. The width is kept in `browser.json` in the state folder, not in the notes repository. With the mouse captured, Shift-drag still gives the terminal's own selection.
- Deleting is plain: the files go, sync commits it, git keeps them. Other notes' links to a deleted note stay (the question says how many there are).
- Relative links in the preview are shown as repository paths (`tech/2026-10-26-nas.md`), the form `nt view` and `nt ls` take.

- `nt paste` reads the clipboard with the OS: AppKit through `osascript` on macOS (a file copied in Finder, else PNG, else TIFF made PNG), `wl-paste` or `xclip` on Linux. A copied image file keeps its type and name; image data becomes PNG named `image` unless `--name` says otherwise. It writes under the repository lock and prints only the markdown line, so an editor can insert it.

**nvim** (`internal/nvim`). A small Lua plugin carried inside the binary, so it always matches the `nt` that runs it. `nt install --nvim` writes it to `$XDG_DATA_HOME/nt/nvim` (`~/.local/share/nt/nvim`); a lazy.nvim config loads it with `{ dir = "~/.local/share/nt/nvim", name = "nt", lazy = false }`, and `require("nt").setup(opts)` changes `cmd`, `keys.paste` or `reload`. `nt doctor` warns when it is missing or older than `nt`. In a markdown note inside the repository it:
- maps `<leader>ip` to `nt paste` with a name prompt, the line going below the cursor;
- holds `.git/nt-edit` while a note has unsaved changes (`nt _editor hold`, until its input ends);
- reloads notes that changed on disk and have no unsaved changes, every 3 s in normal mode;
- starts a background sync when nvim exits after a note was written (`nt _editor kick`).

It finds the repository with `nt _editor repo`, never a fixed path, and does nothing outside it. Every action is the `nt` command line, so another editor's plugin needs only the same three `_editor` calls and `nt paste`.

**Deferred:** back stack and multiple views.

## 5. Images and platforms
**Image opener.** Configurable as `image_viewer`. The default depends on the OS:

| Environment | Default |
|---|---|
| macOS | `qlmanage -p` (Quick Look), falling back to `open` |
| Linux with a desktop (`$DISPLAY` or `$WAYLAND_DISPLAY` set) | `xdg-open` |
| Linux, headless, or over ssh | `tmux display-popup -E 'chafa … ; read'`. chafa draws with text symbols in any terminal, or uses kitty/sixel where it can. Without tmux, the TUI is suspended and chafa runs full screen. |

**Inline images** (`internal/termimg`, proven in the phase 0 spike). An image alone on a line of a
top-level paragraph is drawn in the preview, and so is an HTML block holding only one `<img src>`
(wrapper tags allowed, as in GitHub READMEs); markdown is the form nt writes. An image inside a
sentence, a list or a table keeps its alt text, and a remote image stays a link because nt fetches
nothing.

- **How.** The kitty graphics protocol with Unicode placeholders. The image is sent once under an
  id; the preview carries rows of placeholder characters (U+10EEEE with row, column and id marks)
  whose 256-colour foreground carries the id. tmux stores the placeholders as ordinary text, so
  scrolling, splits and redraws work, and only the one-time transfer needs passthrough.
- **Where.** `inline_images = "auto"` (`nt view --images auto|on|off`) draws them in Ghostty and
  kitty: `TERM`/`TERM_PROGRAM`/`KITTY_WINDOW_ID` outside tmux, and tmux's
  `#{client_termname}`/`#{client_termtype}` inside it. Inside tmux it also needs
  `set -g allow-passthrough on`; when that is the only thing missing, the footer says so.
  Elsewhere (Terminal.app, a Linux console, tmux in an unknown tmux) images keep their alt text
  and `i`.
- **Inside flok.** nt's tmux is a client in a pane of flok's outer tmux. flok publishes the outer
  socket as `FLOK_OUTER_SOCKET` in the inner server's global environment; when nt's tmux reports
  another tmux as its terminal, nt reads it (`tmux show-environment -g`), asks the outer server
  about its terminal and passthrough, and wraps image data once per server so each unpacks one
  layer. Without the variable (a flok release before it) images keep their alt text.
- **Size.** Its own size in cells, at most the preview width and one row less than the pane, shape
  kept. Cell pixels come from tmux or `TIOCGWINSZ`, else 10×20. PNG is sent as is; JPEG, GIF and
  anything over 1600 px is scaled and re-encoded as PNG. Files over 20 MB stay links.
- **Lifetime.** Data goes out one frame after the alternate screen is entered (the terminal keeps
  each screen's images apart), is sent again after the editor had the screen, gets a new
  placement (not new data) on resize, and is deleted when nt exits.
- **tmux drops output.** When its terminal falls behind (a burst of full frames is enough in a
  small window), tmux discards pending output, image data included, and later redraws only the
  text. 250 ms after sending, nt compares each server's `#{client_discarded}` with the value
  before; if it grew, the images go again, up to three times. Each process picks its own id
  high byte, so nt in another pane keeps its images.

**Builds** are cgo-free for darwin and linux, amd64 and arm64, the same matrix as flok.

**Background sync and notifications:**

| | macOS | Linux |
|---|---|---|
| Sync timer | launchd | systemd user timer |
| Notifications | `osascript` | `notify-send` |

## 6. Design language: the w4jnl family, set up from day one
Sources: `vink/docs/design-system/{README.md,tokens.json,fonts/}`, `vink/assets/brand/{README.md,gen.py}`,
`flok/assets/brand/README.md` (incl. `hero.html`, `render-hero.sh`), and flok's `internal/ui/{theme,brand}.go`,
`internal/cli/brand.go` and `internal/termtheme`. The rule from vink: *keep the bracket geometry
identical across tools; change only the glyph.*

**Mark.** Reuse the bracket path exactly as it is in gen.py: 24 grid, frame 1.6, cursor 4.8 × 2.2 at y 16.2.
- The glyph is two heavy text lines (stroke 2.9, a long one over a short one): a note being written at the cursor.
- The ASCII forms are `[≡▁]` and `⌈≡⌉`.
- `nt-mark-alert.svg` is the solid-tile state variant (conflict or sync error), to be used later by the menu bar.
- The final glyph is reviewed in Phase 0. A quill (the family's bird feather) is the alternative.

**Name and voice.**
- `nt` is always lowercase, and `[nt]` is the wordmark.
- Tagline: "plain markdown notes, synced by git".
- Voice follows vink's rules: short, technical sentences; no emoji, no exclamation marks.
- States are lowercase words. Keys and buttons are verbs. Errors say what is wrong and how to fix it. Times are shown relative first.

**Tokens.** `docs/design-system/tokens.json` uses vink's schema and family values (dark first, light complete):

| Tokens | Dark | Light |
|---|---|---|
| `ground` | `#14181A` | `#F6F8F8` |
| `surface`, `surface-sunk`, `line`, `line-strong` | vink's values | vink's values |
| `ink` / `ink-muted` / `ink-faint` | vink's values | vink's values |
| `accent` | `#3FD0D4` | `#0B7A7E` |
| `brand` | `#12999D` | `#12999D` |
| `mark` | as in vink | as in vink |

**nt's own state tokens.** Each state has its own glyph shape, so colour is never the only signal:

| Kind | State | Glyph | Colour |
|---|---|---|---|
| Sync | synced | `●` | `ok` `#5FC47A` |
| Sync | syncing | `◐◓◑◒` | `accent` |
| Sync | ↑N unpushed | — | `ink-muted` |
| Sync | offline | `◌` | `warn` `#E8963C` |
| Sync | conflict / error | `◆` | `alert` `#F0645A` |
| Agent report | in-progress | spinner | cyan |
| Agent report | done | `✓` | green |
| Agent report | blocked | `●` | orange |
| Agent report | failed | `◆` | red |
| Agent report | abandoned | `○` | muted |

The agent report glyphs are the same as flok's, so a report looks the same in flok's sidebar, in `nt` and in the digest. On GitHub the digest shows glyph + word.

**Theme in code.**
- `internal/theme` maps the tokens to Lip Gloss v2 colours.
- Dark or light is detected with OSC 11 (port of flok's `termtheme`; tmux handling included). A `[theme] mode = auto|dark|light` setting overrides it.
- `internal/render` builds a **Glamour style from the same tokens**: accent headings, links and code on `surface-sunk`, quotes in `ink-muted` with a `line` rule.
- `nt theme --glamour` writes `~/.config/nt/glamour-{dark,light}.json`, so `glow -s` and the `v` key render notes in the same style.

**TUI rules** (the simplicity budget):
- **Screen:** one brand line `[nt]` at the top left, two panes, one status/footer line.
- **Selection:** a `▎` accent bar plus a `surface` background. This is the terminal version of the family's 6px accent bar.
- **Interaction:** no modals. Prompts are a single line at the bottom, and only one picker is open at a time.
- **Footer hints:** the key in `accent`, the verb in `ink-muted`.
- **Colour:** off when `NO_COLOR` is set or output is not a TTY.

**CLI.**
- `nt version` and the `nt doctor` header print the ASCII lockup. Example:
  ```
  [≡▁] nt 0.1.0 · plain markdown notes, synced by git
  ```
  The status line looks like `[≡▁] nt · 214 notes · ● synced · ✓ 3 agents today`.
- Tables in the CLI show state words with their glyph.

**Repo files.**
- `docs/design-system/README.md`: voice, mark, family, colour, type, TUI rules, CLI and assets, in vink's structure.
- `docs/design-system/tokens.json`.
- `docs/design-system/fonts/`: JetBrains Mono 400/700/800 and OFL, copied from vink.
- `assets/brand/`:
  - `README.md` and `gen.py`, adapted from vink's: one geometry, text outlined from JetBrains Mono.
  - Marks in four inks (`currentColor`, teal, cyan, alert) and lockups, with and without the tagline.
  - `[nt]` wordmark, favicon, avatar (460) and app icon (512, for the later Mac/iOS apps).
  - Hero 1280×320 and social card 1280×640, rendered by `hero.html` and `render-hero.sh` (flok's).
- README hero banner and `CLAUDE.md` (conventions, voice, and pointers to the design system).

## 7. Code layout
Go 1.27. Charm v2: `charm.land/bubbletea/v2`, `lipgloss/v2`, `bubbles/v2`, `glamour/v2`.
TOML parsing via BurntSushi. Markdown via goldmark v1 with GFM and `go.abhg.dev/goldmark/{frontmatter,wikilink}`. Fuzzy matching via `sahilm/fuzzy`.
```
cmd/nt/main.go
internal/cli        dispatch like flok/internal/cli/root.go; --json
internal/config     repo .nt/config.toml + machine ~/.config/nt/config.toml (repo path, sync.interval, image_viewer)
internal/vault      paths, slugs, day_starts, templates, atomic write under the repo lock, journal append/link
internal/mdparse    title, links, images, headings (goldmark); wikilink read support
internal/scan       in-memory backlinks/titles (no persisted index)
internal/search     rg --json wrapper, fuzzy titles
internal/gitsync    cycle above, conflict resolver, status.json, launchd/systemd unit install, notify
internal/agents     hook adapters (claude/copilot), report upsert, sid map, digest, hook installers
skills/             skills/nt/SKILL.md, embedded; nt install --skill
internal/theme      tokens → lipgloss v2 styles, state glyphs; termtheme (OSC 11) ported from flok
internal/render     glamour: style from tokens, strip frontmatter, rewrite [[x]] → links, width
internal/termimg    kitty graphics: detect, size, send, placeholder rows (inline images)
internal/tui        two panes, keys, pickers, ExecProcess
internal/importer   hidden one-shot: Obsidian → nt
internal/editor     $VISUAL / $EDITOR hand-off with +line, shared by the CLI and the TUI
internal/clip       the clipboard's image for nt paste: AppKit via osascript, wl-paste, xclip
internal/nvim       the nvim plugin (Lua, embedded) and nt install --nvim
assets/brand/ · docs/design-system/ · docs/{research,format}.md · skills/nt/SKILL.md
scripts/e2e/ · CLAUDE.md · README.md (hero) · Makefile · .github/workflows/ci.yml
scripts/release.sh · CHANGELOG.md · .github/workflows/release.yml   tag, tarballs, Homebrew tap bump
```

---

## Phases (each ends in something usable)

**0. Distill, design language and spike.**
- Write `docs/research.md` and `docs/format.md`.
- Set up the design system from §6: tokens, the design-system README, fonts, `gen.py` with the brand SVGs and PNGs, the hero, `CLAUDE.md` and the README.
- Scaffold `go.mod`, the Makefile, CI and `.gitignore`.
- Build a one-day Charm v2 spike: viewport + glamour with the token-built style + `tea.ExecProcess` into nvim, run inside tmux in Ghostty, plus `internal/theme`. This proves key handling, background detection and editor hand-off on v2. flok is still on v1, so we have no proven v2 code yet.

**1. Core and sync, then cut over.**
- Commands: `init`, `t`, `a`, `n`, `f`, `s`, `ls`, `sync` (with launchd), `doctor`.
- Write the conflict e2e test.
- Then `nt import obsidian`:
  - converts wikilinks and `![[…]]` to md links;
  - moves `img/` into a root `assets/` folder;
  - turns `notes/` into topic notes with slug filenames, keeping the original title in frontmatter;
  - skips `.trash/`, `go.mod`, `.base`/`.canvas` and empty `Untitled*` files;
  - keeps `Untitled 1.md`, which has content;
  - prints anything it could not map.
- Cut over (checklist below).

**2. Local agents.**
- `nt agent …`, digest, skill, `AGENTS.md`, `install --hooks`.
- This puts real concurrent load on sync early.
- Reports are readable in nvim, glow and GitHub before the TUI exists.

**3. TUI** as described in §4, with inline images (§5) in the preview from the start.

**4. Remote hosts.**
- Each host keeps its own reports. It is their only writer and never deletes them mid-session; it prunes ended ones after 14 days.
- The Mac pulls copies with `ssh host nt agent export --since <cursor>`, which streams a tar that `nt` unpacks under the repo lock. This follows the `flok serve --stdio` style.
- Servers need no GitHub credentials. Don't use `rsync --remove-source-files`: it loses updates to reports that are rewritten during a session, and macOS ships openrsync.

**5. Later:**
- `nt mcp` (official go-sdk).
- `nt digest --summarize` (`claude -p` with `disableAllHooks`).
- Weekly notes (ISO week-year).
- Mac app: SwiftUI over the same folder; it can shell out to `nt` and `git`.
- iOS: swift-libgit2 or the GitHub API; sync on open, background and push; App Intent "append to today".

## Cutover checklist (end of phase 1)
1. Turn off obsidian-git, or quit Obsidian. Do a last push of `<you>/obsidian`.
2. `mv ~/Notes ~/Notes.obsidian`, because of case-insensitive APFS.
3. `make install`, so the background job runs a stable `~/.local/bin/nt`.
4. `nt init ~/notes`, create the remote with `gh repo create <you>/notes --private`, and set the SSH remote.
5. `nt import obsidian ~/Notes.obsidian --dry-run`, review the plan, then run it without `--dry-run` and `nt sync`.
6. In nvim, remove `obsidian.lua` and `telekasten.lua`, and enable the Marksman LSP.
7. `nt install --sync` and `nt install --nvim`, then `nt doctor`.

## Verification
- **Unit tests:**
  - slugs and `day_starts`;
  - templates;
  - link and image extraction, and wikilink rewriting;
  - hook adapters, with fixtures for both payload shapes;
  - deterministic digest;
  - importer, against a fixture copied from `~/Notes`.
- **e2e: sync** (`scripts/e2e`, in flok's style). A bare repo stands in for GitHub, with two clones.
  - 5 sessions × parallel `agent start/turn/end`, with both clones looping `nt sync`.
  - Assert: every report is on both sides, the frontmatter is valid, and nothing is lost.
- **e2e: conflict.**
  - The same topic note edited in both clones produces exactly one `.conflict-` copy and no markers.
  - The same journal edited in both clones is merged by union.
  - Delete vs edit: the edit survives.
  - A crash between merge and commit is recovered on the next run.
- **e2e: TUI.** Run in an isolated tmux server and check `capture-pane` with `expect_soon`, covering `t`, `a`, `/`, and the status badge.
- **Design checks:**
  - A unit test asserts that every text/background token pair used in the TUI meets 4.5:1, in both themes.
  - `NO_COLOR` and non-TTY output contain no escape codes.
  - Running `gen.py` again leaves `assets/brand` unchanged (no git diff).
  - The glamour style JSON loads in `glow -s`.
- **Manual.** In Ghostty + tmux, go through `nt` → `t a n e / i`, once with a dark terminal background and once with a light one. Then check on github.com that links, images, the README hero and the agents `README.md` digest render.
