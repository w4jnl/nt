# nt research: note tools, sync and agents

Date: 2026-10-26. This report condenses three surveys and one design review that were done
before nt was designed:

1. Terminal note tools and markdown viewers.
2. GUI, Mac and iOS apps that work on plain markdown.
3. Agent integration, concurrent writes and rendering technology.

Facts were taken from GitHub API data, READMEs, official docs and release notes. Star counts
and dates are as of 2026-10-26. Two markers are kept from the source reports:

- **(unverified)** or **(U)**: the claim could not be confirmed.
- **(2nd)**: the only source is secondary (an aggregator, a vendor blog, a forum post).

The file format nt writes is specified in [format.md](format.md).

## Summary and verdict

Requirements nt has to meet:

- plain markdown in folders, with full GFM (images, tables, code);
- daily notes, plus a daily digest where agents post updates;
- many agents writing at the same time, from this Mac and from servers reached over ssh;
- git and GitHub as the primary sync, not an add-on;
- reuse of OS tools (git, ssh, rg, fzf, nvim, glow) instead of reimplementing them;
- TUI first, with a Mac app and maybe an iOS app later;
- simplicity above everything else.

**No existing tool meets all of these.** The closest candidates each miss at least one hard
requirement:

| Tool | What it gets right | What it misses |
|---|---|---|
| zk | Files are canonical and the SQLite index is a cache. Folder groups. Idempotent "open or create today". Best query syntax. LSP. | No git, no viewer, opaque ids. |
| nb | Commits every change and syncs in the background. Never blocks on a conflict. | Syncs on every command. Commits conflict markers. One very large bash script. |
| glow / glamour | The reader the user likes. Embeds in Bubble Tea. | No images, no TOC, no following of local links. |
| treemd, md-tui, mdfried | Outline, link picker, back stack, inline images. | Viewers only. |
| FSNotes, GitJournal, Tolaria | Plain files with built-in git. | Each misses the TUI, the agent story, or both. |
| NotePlan | Best daily-note UX (calendar first). | Sandboxed files, CloudKit sync, no git. |
| Logseq | — | Moved to SQLite in April 2026; the file-based app is in maintenance only. |

**Verdict: build nt**, a small Go tool on the Charm v2 stack, in the style of flok. It
delegates every job that an existing tool already does well:

| Job | Delegated to |
|---|---|
| Editing | nvim (`$EDITOR`) |
| Sync | `git` (merge only) |
| Search | `rg`, `fzf` |
| Reading | glamour inside the TUI, and glow itself |
| Remote hosts | `ssh` |
| Images | Quick Look, `xdg-open`, `chafa` |
| Editor intelligence | Marksman LSP (link completion, backlinks, rename, dead links) |

**Borrowed ideas:**

- **zk:** the index is a cache that can be thrown away; folder groups; idempotent today; one
  query syntax for CLI, TUI and agents.
- **nb:** git that stays out of the way; sync that never blocks.
- **glow:** hand-off to `$EDITOR` at the current line and reload on return; live reload with
  fsnotify.
- **treemd, md-tui:** outline and link pickers; `--json` on every command; shipped agent
  skills.
- **NotePlan:** open on today.
- **Tolaria, FSNotes:** commit automatically on idle.
- **GitSync.md:** pull triggered by a push notification (for iOS later).
- **basic-memory:** files are the source of truth; FTS5 table design if an index is ever
  needed.
- **git-auto-sync:** stop and notify instead of writing conflict markers.

**Pitfalls to avoid:**

- a database as the source of truth (dnote, Logseq DB, NotesTUI);
- committing the index, UI state or conflict markers;
- syncing on the input path (nb syncs on every command);
- writing an editor (Basalt's editor is still experimental after 13 releases);
- opaque filenames (zk ids, Notesium hex, org-roam UUIDs);
- waiting on glamour upstream for images, TOC or local link following;
- many processes running git in one working tree (`index.lock` fails fast).

---

## 1. Terminal note tools and viewers

### 1.1 Charm stack status

This decides the Go build. All versions verified through the GitHub API and the `go.mod`
files.

| Library | Module | v2/v3 GA | Latest |
|---|---|---|---|
| Bubble Tea | `charm.land/bubbletea/v2` | 2026-02-24 | v2.1.0 (2026-10-08) |
| Lip Gloss | `charm.land/lipgloss/v2` | 2026-02-24 | v2.0.6 (2026-08-11) |
| Bubbles | `charm.land/bubbles/v2` | 2026 (exact date unverified) | v2.2.1 (2026-08-24) |
| Glamour | `charm.land/glamour/v2` | 2026-03-09 | v2.0.1 (2026-06-12) |
| Glow | `charm.land/glow/v3` | v3 = move to the v2 stack | v3.0.0 (2026-08-11) |

- **Bubble Tea v2:**
  - `View()` returns a `tea.View` struct; alt screen, mouse and cursor are set declaratively.
  - Kitty keyboard protocol, so shift+enter and key release work.
  - Separate paste and mouse messages, OSC 52 clipboard, synchronized output.
- **Lip Gloss v2:** styles are pure; background detection is explicit
  (`tea.RequestBackgroundColor`).
- Everything is actively maintained:
  - glow: 27.8k stars, pushed 2026-10-05.
  - glamour: 3.7k stars, pushed 2026-10-18.
- Glamour embeds cleanly in Bubble Tea:
  - The Bubble Tea repo ships `examples/glamour/main.go` on v2 imports.
  - The pattern: `glamour.NewTermRenderer(...)`, then `Render`, then `viewport.SetContent`.
    Render again on every resize.
  - Glow's own pager does exactly this.

### 1.2 Note managers

| Tool | Stack | Storage | Daily notes | Search | Links | Git | Images | Status |
|---|---|---|---|---|---|---|---|---|
| zk | Go, LSP, fzf | `.md` + YAML; `.zk/notebook.db` SQLite cache | via groups, template, alias | SQLite FTS, exact, regex; fzf | md, wiki or custom; backlinks, orphans, broken | none | none | v0.15.7 (2026-10-15), 2.85k★ |
| nb | Bash, git | `.md`; each notebook a git repo + `.index` | `daily` plugin | `git grep`; rg optional | wiki and `#tags` in `browse` | built in: commit every change, sync per command | chafa, viu, timg, imgcat | 7.25.5 (2026-08-12), 8.46k★ |
| jrnl | Python | one text file (optional AES) or `YYYY/MM/DD.txt` | it is a journal | dates, `@tags`, contains | none | none | none | v4.6 (2026-08-07), 7.35k★ |
| dnote | Go | SQLite | no | full text | none | own server | none | cli-v0.17.0 (2026-10-17), 3.09k★ |
| Notesium | Go + web app (Go unverified) | flat dir, hex epoch names | deterministic names | fzf-style, lines | md links; in, out, broken | none | web only | v0.6.5 (2025-07-20), 166★ |
| Basalt | Rust, ratatui | Obsidian vault in place | not documented | fuzzy full text (v0.13) | parses wikilinks; no backlinks | none | none | v0.13.0 (2026-10-04), 1.38k★, GPL-3 + CLA |
| kb | Python | `~/.kb` + SQLite (unverified) | no | list filters, grep | none | "experimental" | stores binaries | dormant, v0.1.8 (2025-06) |
| nap | Go, Bubble Tea | `~/.nap` (format unverified) | no | fuzzy | none | none | none | dormant since 2022 |
| Denote | Emacs Lisp | `ID==SIG--title__kw.md` | `denote-journal` | standard tools | ID links, backlinks | none | n/a | 4.2.0 (May 2026) |
| org-roam | Emacs Lisp, SQLite | `.org` + UUID ids | dailies | DB queries | ID links, backlinks | none | n/a | slow, 6.05k★ |
| rucola | Rust, ratatui | plain `.md` | diary (v0.10) | tags, links, text | wikilinks; rename rewrites | none | external HTML preview | v0.10.0 (2026-06-24), 547★ |
| Kimün | Rust, ratatui | `.md` vault + SQLite index | `journal append` | full text, fuzzy, `--json` | wiki and md; backlinks | not mentioned | not mentioned | new, 63★; built-in MCP server |
| notesmd-cli | Go | Obsidian vault, headless | yes, templates | fuzzy, grep, JSON | move rewrites links | none | none | about 1.6k★ |
| NotesTUI, Toney, notes-tui | Go, Bubble Tea, Glamour | NotesTUI: SQLite FTS5 is the store | notes-tui: `d`/`D` | fuzzy, tags | minimal | none | none | hobby projects |

### 1.3 Editor plugins and language servers

| Tool | Notes | Status |
|---|---|---|
| obsidian.nvim (obsidian-nvim fork) | Neovim 0.11+, in-process LSP. today/yesterday/dailies, backlinks, rename updates backlinks, image paste, properties browser, ripgrep optional. | v3.16.9 (2026-10-13). The original epwalsh repo is unmaintained. |
| zk-nvim | Wraps `zk lsp`; pickers for Telescope, fzf-lua, snacks, mini.pick. | Active, 870★ |
| telekasten.nvim | Daily to yearly periodic notes, calendar. Filename formats hard-coded. | Slowing; last push 2026-03 |
| markdown-oxide | Rust PKM LSP: `:Today`, relative dailies, wiki and md completion, backlinks, rename. | v0.25.12 (2026-06-29) |
| Marksman | F# LSP for any md folder with git or `.marksman.toml`: wiki, md and reference links; refs, rename, TOC code action, diagnostics. | Last release 2026-02-08, 3.4k★ |

If nt's file conventions match theirs, nvim gets completion, backlinks, rename and dead-link
diagnostics for free. nt writes standard md links, which Marksman understands.

### 1.4 Markdown TUI viewers

| Tool | Stack | Images | Tables | Code | Link navigation | TOC | Status |
|---|---|---|---|---|---|---|---|
| glow | Go, Glamour | no (text `Image: alt → url`) | yes, width issues | Chroma | no local following (PR #883 open); OSC 8 links | no (#287, #835, #928 open) | v3.0.0, 27.8k★ |
| Frogmouth | Python, Textual | not documented | not documented | not documented | history, bookmarks | yes | dormant since 2023; Textualize wound down (2nd) |
| md-tui | Rust, ratatui | optional, max 20 rows | yes | about 18 languages | `s` select mode, `K` preview, `b` back | not mentioned | v0.11.1 (2026-10-25) |
| mdcat | Rust | iTerm2, kitty, WezTerm, Ghostty | partial | syntect | OSC 8 | no | archived 2026-06-19; fork BIRSAX2/mdcat active |
| treemd | Rust | kitty, iTerm2, Sixel, halfblocks, GIF, Mermaid | cell navigation | 50+ languages | numbered links, back/forward | core feature | v0.10.0 (2026-10-18) |
| mdfried | Rust | Sixel, Kitty, iTerm2, chafa; sliced per row so it scrolls | not documented | yes | local `.md` and anchors | not documented | v0.22.8 (2026-10-13) |
| markdown-reader | Rust | Kitty, Sixel, iTerm2 (off in tmux) | fit + full-screen pan | yes | link picker, broken-link checker | outline popup | 93★ |

### 1.5 zk in detail

- **Model.** A notebook is a directory with `.zk/` holding `config.toml`, `templates/` and
  `notebook.db`. It is found by walking up like git does, or through `ZK_NOTEBOOK_DIR`.
- **Indexing.** Every command except `zk index` runs a quiet incremental index first. It diffs
  the filesystem against the DB (inferred from `main.go`). The DB is a disposable cache. The
  docs do not say whether to gitignore it; we would.
- **Groups.** Per-folder rules for filename, template and id. A daily group plus an alias
  `zk new --no-input journal/daily` gives an idempotent "open or create today".
- **Templates.** Handlebars with `format-date`, `slug`, `sh` and more.
- **`zk list` filters.** The best query syntax in this survey:
  - full text (FTS by default, exact, regex);
  - tags with AND, OR, NOT and globs;
  - natural dates;
  - link graph: `--linked-by`, `--link-to`, `--orphan`, `--missing-backlink`, `--related`,
    `--mention`;
  - sorting, limits, fzf `-i`;
  - saved filters and aliases.
- **LSP.** Link completion, tag completion, hover, go to definition, diagnostics.
- **Gaps.** Not a viewer, no git, no MCP, opaque default ids, thin docs.

### 1.6 nb's git design

- Every notebook is a git repo. Every add, edit, move or delete is committed in the
  background.
- With a remote set, nb syncs on every command. `NB_AUTO_SYNC_SECONDS` (default 60) is
  reportedly the minimum gap.
- Several notebooks can share one remote through orphan branches.
- **Conflicts never block:**
  - text files get both versions in the file, separated by git conflict markers;
  - binary or encrypted files keep both copies (`--conflicted-copy`).
- **Weaknesses:**
  - network latency and failures on the interactive path;
  - conflict markers committed and spread to other devices;
  - one commit per operation;
  - numeric ids as an extra layer;
  - one very large bash script.

### 1.7 glow and glamour in detail

- **TUI mode.** Lists markdown files below the current directory, or the whole git repo:
  - `/` fuzzy filter (`sahilm/fuzzy`);
  - `e` opens `$EDITOR` at an approximate line and reloads on return;
  - `c` copies;
  - fsnotify reloads live.
- **History.** v2.0.0 (2024-08) removed the stash and Charm Cloud. v3.0.0 (2026-08) was only
  the stack upgrade.
- **Limitations as of 2026-10:**
  - **No images.** Issue #211 has been open since 2020. PRs glow #856, glow #1028 and
    glamour #598 are open. Maintainers raised concerns:
    - no size checks before decoding;
    - caches that are never evicted;
    - kitty file transfer breaks over ssh;
    - remote images can be used for tracking;
    - `tea.RawMsg` handling.
  - **No TOC.** Issues #287, #835 and #928 are open.
  - **No following of local `.md` links.** PR #883 is open.
  - **Glamour tables:**
    - they fill the wrap width;
    - indentation is ignored in width calculation;
    - links inside tables move to a numbered footer unless `WithInlineTableLinks(true)` is
      set.
- **Lesson.** Feature PRs sit for months. Parse the markdown AST ourselves with goldmark (which
  glamour uses) so nt owns headings, links and image positions. Use glamour only to style.

### 1.8 Per-tool lessons

| Tool | Worth copying | Weakness |
|---|---|---|
| jrnl | Natural-language timestamps (`yesterday: …`); `YYYY/MM/DD.txt` layout. | The single-file journal merges badly across devices. |
| dnote | Fast capture: `dnote add book -c "…"`. | SQLite is the truth; not greppable. |
| Notesium | Deterministic daily and weekly filenames. | Opaque names, flat directory only. |
| Basalt | Read-only by default; `exec:` binding opens an external editor. | Its own editor is still experimental. |
| Denote | The filename is the metadata: sortable, greppable, stable id. | Long names; Emacs only. |
| rucola | Statistics view; rename rewrites links. | Preview is external HTML. |
| Kimün | `claude mcp add kimun -- kimun mcp`; shipped skills; `--json`; stdin piping. | Very young; vim emulation is a large surface. |
| obsidian.nvim | Delete warning when a note has backlinks; `:checkhealth`. | Neovim only; chases Obsidian semantics. |
| treemd | Outline as the main UI; section extraction with line ranges; JSON output; shipped skill; link-following stays inside the working directory. | — |
| md-tui | Link-select mode, preview of the link target, saved reading position. | Single-character keys only. |
| mdfried | Images sliced into rows so they scroll. | Font detection setup; large binaries. |
| markdown-reader | Full-screen pan for wide tables; broken-link checker. | Small project. |
| NotesTUI | Same Go stack as nt; MCP tools. | The DB is the store: the anti-pattern. |

Stalled or archived projects:

- nap, kb and Frogmouth are dormant.
- mdcat was archived on 2026-06-19.
- epwalsh/obsidian.nvim is unmaintained.
- Dendron is in maintenance only.

Lesson: keep the on-disk format standard, so the tool is replaceable.

---

## 2. GUI, Mac and iOS apps

### 2.1 Comparison

| App | Storage | Sync | Git | Daily notes | Mac | iOS | Status |
|---|---|---|---|---|---|---|---|
| Obsidian | `.md` + `.obsidian/` config | Obsidian Sync ($4–5/mo), iCloud, others | community plugin; mobile uses isomorphic-git, no SSH | core plugin | Electron | yes (fixed vault locations) | active |
| Logseq OG | `.md` outline in `journals/`, `pages/` | any file sync | desktop auto-commit (U) | `journals/YYYY_MM_DD.md` | Electron | yes | maintenance only |
| Logseq DB | SQLite is the truth; Markdown Mirror one-way, not shipped | own sync | none | yes | yes | TestFlight | beta |
| NotePlan 3 | `.txt`/`.md` inside the app sandbox | CloudKit | none (manual Working Copy recipe) | `Calendar/YYYYMMDD`, weekly on | native | native | active, $99.99/yr |
| SilverBullet v2 | `.md` folder | self-hosted server | plug broke in v2 (users) | `Journal/YYYY-MM-DD` | browser | browser | active |
| Zettlr | `.md` workspaces | bring your own | none (U) | none (U) | Electron | no | active |
| The Archive | flat plain-text folder | iCloud, Dropbox | none | no | native | no | active, slow |
| iA Writer | `.md`/`.txt` folders | iCloud, Dropbox, Drive | none | none (U) | native | native | active |
| Bear 2 | SQLite | CloudKit | none | none (U) | native | native | not plain files |
| Joplin | SQLite | many targets | none | plugin (U) | Electron | yes | active |
| Notesnook | encrypted DB | own or self-hosted | none | none (U) | Electron (U) | yes | active |
| Foam | `.md` in VS Code | your git | via VS Code | template | VS Code | no | active |
| Dendron | `.md`, dot hierarchy | git | via VS Code | `daily.journal.YYYY.MM.DD` | VS Code | no | maintenance only |
| MarkText | edits any `.md` | none | none | none | Electron | no | revived Sep–Oct 2026 |
| FSNotes | `.md`/`.txt`, TextBundle | iCloud on iOS | built in (Mac: scheduled commits, push/pull) | none (U) | native | native (no subfolders, 2nd) | 7.2.1 (June 2026), MIT |
| GitJournal | `.md` + YAML in git | git | built in (dart-git + go-git) | journal mode (U) | Flutter | yes | active (commits Mar–May 2026) |
| Tolaria | `.md` + YAML; every vault a git repo | any remote | AutoGit on idle or focus loss | not covered | desktop | no | new (Apr 2026), AGPL, MCP server |
| GitSync.md | files in a cloned repo, in Files app | GitHub OAuth/PAT, SSH | libgit2 | n/a | no | yes | v2.8.1 (Oct 2026) |
| NotesHub | `.md` | GitHub, git, iCloud | built in, auto conflict resolution | not mentioned | yes | yes | v3.13 |
| Cache | `.md` | any git remote | built in | date-based journaling | yes | yes | v0.10 |
| mdnb | `.md` | GitHub | built in | not mentioned | companion | yes | v1.0 (Mar 2026) |
| Drafts | CloudKit DB | CloudKit | via actions | capture inbox | yes | yes | active |
| Craft, Capacities, Anytype, Apple Notes | proprietary | own cloud | none | varies | yes | yes | contrast only |

Notes on key apps:

- **Logseq.** Since 2026-04-24 the file-based app is "OG": security and Electron fixes only,
  with no forced migration. Plain-text users are the ones left behind. Lesson: never make a
  database the source of truth.
- **NotePlan.** Calendar and daily notes are the main navigation; tasks move between days.
  - Files live under `~/Library/Containers/co.noteplan.NotePlan3/…`, so outside tools trigger
    macOS access prompts.
  - Attachments get UUID names in `<note>_attachments/`.
  - `YYYYMMDD` filenames do not match `[[YYYY-MM-DD]]` links.
  - Good x-callback-url API.
- **FSNotes.** The closest existing "plain files + git + native Mac and iOS". Single
  developer; iCloud-only sync on iOS.
- **Tolaria.** Principles: "plain files, markdown with YAML frontmatter, Git, and open
  source". No trash: git history is the undo.
- **Bear, Joplin, Drafts.** Databases. Joplin is sometimes described as plain markdown; it is
  not.

### 2.2 Why people leave Obsidian

| Theme | Evidence |
|---|---|
| Complexity, tinkering as busywork | "I tended my 'knowledge garden' obsessively, thinking I was working" (bearblog, Aug 2025); plugin overload (Android Authority, Jul 2025). |
| Plugin fragility and security | Updates broke other plugins; a plugin update plus a sync conflict wiped a note (dev.to, May 2025). Plugins are not sandboxed; Obsidian's "Less is safer" post (Sep 2025) concedes supply-chain risk. |
| Sync cost | $4–5/month; do-it-yourself sync means merge conflicts. |
| Git on mobile | obsidian-git on mobile has no SSH and limited repo size; the maintainer does not recommend it. |
| `.obsidian` clutter | `workspace.json` churn causes conflicts; users ask to move config out of the vault. |
| iOS sandbox | Vaults must be under `iCloud Drive/Obsidian` or `On My iPhone/Obsidian`. |
| Large vaults on mobile | Android startups of 5+ minutes on 10k+ notes. |
| Electron | "Not a good platform citizen on Mac or iOS" (disputed by others). |
| Format lock-in | Wikilinks, embeds, callouts, comments and Dataview do not render on GitHub. |

Counterpoint: many people who "leave" keep the folder and only change the app (XDA, June
2026). The files are the product; the app is replaceable.

### 2.3 Git on iOS

- **GitJournal.**
  - Active, but store updates lag (2nd).
  - It moved off libgit2: cross-compiling was painful and libgit2 gives no control over the
    filesystem layer. It now uses dart-git plus go-git (`go_git_dart`).
- **Working Copy.**
  - Repos in the Files app; a "synced directory" mirrors another app's folder.
  - x-callback-url (`write`, `commit`, `push`, `pull`, `chain`); a pull conflict counts as an
    error.
  - Push needs the Pro unlock (about $35.99, 2nd).
- **Obsidian + Working Copy.**
  - Shortcuts pulls when the app opens and pushes when it closes.
  - The triggers fire on every app switch; detached HEAD happens.
- **a-Shell (`lg2`).** Free, but SSH key paths break when the container id changes.
- **Newer apps.**
  - GitSync.md: GitHub push → APNs → pull.
  - GitSync: background tasks and widgets.
  - NotesHub: automatic merge.
  - Cache, mdnb.
- **Swift libraries:**

| Option | Notes |
|---|---|
| swift-libgit2 | libgit2 1.9.1, libssh2, OpenSSL; iOS 15+, macOS 13+; 1.0.1 (Jan 2026). Push support not stated. Best current candidate. |
| SwiftGit2 and forks | Higher-level; a fork reportedly lacks push. |
| MiniGit | Adds push through C/C++. |
| go-git via gomobile; Rust via UniFFI | GitJournal's route; the Rust route is (U). |
| GitHub REST/GraphQL ("gitless") | No local `.git`; ties you to one host. |

**Practical pitfalls on iOS:**

- **Background work is unreliable.** BGAppRefreshTask gets about 30 s, and some developers
  report 2–5 s. Nothing runs after a force-quit. Realistic triggers are: on foreground, on
  background (best effort), on a push notification, and from Shortcuts or widgets.
- **SSH keys.** The Secure Enclave only does P-256 (`ecdsa-sha2-nistp256`); Ed25519 keys sit
  in the Keychain. GitHub OAuth device flow or a fine-grained PAT is simpler.
- **Never put `.git` inside iCloud Drive.** Reported failures: `.git` renamed to "git 2",
  evicted files read as 0 bytes, file operations reverted.
- **Memory limits** on large clones.

### 2.4 Attachment conventions

| Convention | Who | Link | On GitHub |
|---|---|---|---|
| Global `assets/` | Logseq, Obsidian (root or a fixed folder) | `../assets/x.png` | Breaks when notes move between depths. |
| Per-note sibling folder | NotePlan (UUIDs), Typora `./name.assets/` | `note_attachments/x.png` | Works; spaces need `%20` or `<…>`. |
| TextBundle | FSNotes, Bear export, Ulysses | `note.textbundle/assets/` | Awkward to browse in a repo. |
| Same folder | SilverBullet, iA Writer | `![](x.png)` | Works; iA's bare-path content blocks do not render. |

On GitHub:

- relative links resolve from the current file, and a leading `/` means the repo root;
- images in a private repo show only to readers with access;
- spaces must be `%20` or `![](<a b.png>)` (CommonMark; not tested on GitHub).

### 2.5 Daily-note conventions

| App | Path |
|---|---|
| Obsidian | `YYYY-MM-DD`; slashes in the format create folders |
| Logseq | `journals/YYYY_MM_DD.md` |
| NotePlan | `Calendar/YYYYMMDD.ext` |
| SilverBullet | `Journal/YYYY-MM-DD` |
| Dendron | `daily.journal.YYYY.MM.DD` |
| Foam CLI | `journals/YYYY-MM-DD.md` |

- **Rollups.** Weekly, monthly, quarterly and yearly notes (Periodic Notes, telekasten,
  NotePlan).
- **Year-boundary trap.** Weekly names must use the ISO week-year (`GGGG-[W]WW`), not
  `YYYY`.
- **Journal versus topic notes.** Dendron and Cache separate the daily journal from topic
  notes. Failure mode: the daily note becomes a dumping ground.
- **Suggested shape.** `daily/YYYY/YYYY-MM-DD.md`, with year folders to keep listings short.

### 2.6 Ideas for the Mac and iOS layer

1. Open on today; a date strip is the main navigation (NotePlan).
2. Capture straight into today: share sheet, widget, Control Center, an App Intent "append
   to today" (Drafts, Apple Notes).
3. A scriptable surface instead of plugins: App Intents, Shortcuts and x-callback-url that
   mirror the CLI verbs (Working Copy, NotePlan, GitSync.md).
4. Push-triggered pull: GitHub webhook → APNs → pull (GitSync.md).
5. Auto-commit on idle or backgrounding; git history as undo and trash (Tolaria, FSNotes).
6. Expose the repo in the Files app so other editors can open it (Working Copy).
7. One field that searches and creates (FSNotes, The Archive).
8. Strict GFM output; everything renders on GitHub (Zettlr's stance).
9. Config is one small committed file; UI state stays outside the repo; any index is a
   gitignored cache.
10. Readable attachment names next to the note, not UUIDs.
11. AI through an MCP server over the folder (Tolaria, Logseq DB); no plugins.
12. Conflicts users can understand: keep both versions as a named conflict copy, not a bare
    error.

---

## 3. Agents and concurrency

### 3.1 File layout for agent reports

| Pattern | Pros | Cons |
|---|---|---|
| One file per session (`agents/YYYY/YYYY-MM-DD/<host>--<agent>--<project>--<sid8>.md`) | No locks between writers; paths never overlap, so git never conflicts; a crash damages only its own file; easy to grep. | Many small files (a few dozen a day is fine); the digest must aggregate. |
| Shared append file per day | One place to read. | Two machines appending is a guaranteed conflict without `merge=union`; one bad write corrupts everyone's log. |
| Rolling file per agent | Few files. | Concurrent sessions of the same agent collide again. |

Decision: one file per session. The date comes from the session's first event, so a session
that crosses midnight stays in one file.

### 3.2 Atomic writes and locks on macOS

- **Atomic writes.** Write a temp file in the same directory, `fsync`, then `rename(2)`.
  Readers see the old file or the new one, never half of either.
- **O_APPEND.** The seek and write are atomic, but a record written in several `write()`
  calls can interleave. The PIPE_BUF guarantee applies to pipes, not regular files. NFS
  appends are not atomic.
- **Lock tools:**

| Option | Notes |
|---|---|
| `flock(1)` | Not on macOS (verified). Homebrew has a version; Linux has util-linux. |
| `lockf(1)` | macOS built-in, uses flock(2): `lockf -k -t 10 file cmd`. No Linux equivalent CLI (unverified). |
| `shlock` | PID file; stale locks possible. |
| `mkdir` lock | Portable; needs a trap and stale-lock checks. |
| Lock inside the Go binary | `syscall.Flock`. Released by the kernel when the process dies. Same on macOS and Linux. Used by flok already. |

### 3.3 Git concurrency

- **`index.lock` fails immediately; it does not wait.** Parallel agents sharing one `.git` hit
  exactly this (claude-code issue #55724). Rule: one git writer per working tree.
- **Single-writer sync.** The process loops on non-fast-forward rejections with jitter. One
  attempt before pushing does not close the race.
- **Plumbing with a private `GIT_INDEX_FILE` and `update-ref` compare-and-swap.** Safe with
  many writers, but unnecessary once there is a single writer.
- **`merge=union`.** It takes lines from both sides "in random order". Deleted lines can
  return, duplicates appear silently, and GitHub web merges ignore it. Only fit for a log
  treated as an unordered bag of lines.
- **Per-machine branches.** No push contention, but they need merges and fragment history.
- **Worktrees.** Separate index and HEAD, but they cannot share a branch.

### 3.4 Existing auto-sync tools

| Tool | Mechanism | On conflict | Status |
|---|---|---|---|
| gitwatch | inotify/fswatch, commit every change, optional push | not handled | v0.6 (Jun 2026) |
| git-sync (simonthum) | add, commit, fetch, rebase or fast-forward, push; per-branch opt-in | exits with a clear message; finish by hand | last push Sep 2025 |
| git-auto-sync (GitJournal) | Go daemon, file watch + 10 min poll, rebase only | aborts, stops syncing that repo, system notification | v1.5 (2022), unmaintained |
| obsidian-git | commit-and-sync on a timer | conflict view with ours/theirs/both | active |
| nb | commit every change, sync per command, merge | markers in text, copies for binaries | active |

### 3.5 Remote hosts

| Option | Pros | Cons |
|---|---|---|
| A. Clone + sync on each server | Same design everywhere; works with the Mac offline. | GitHub credential on the server (deploy key); private notes on the server unless sparse-checked out. |
| B. Spool on the server, Mac collects over ssh | No credentials on servers; reuses the ssh config. | Reports arrive when the Mac collects. |
| C. flok relays the report | Reuses an existing pipe. | Depends on flok's transport (unverified). |

The first spool design used `rsync --remove-source-files`. The review showed that it loses
data:

- Once rsync moves a report away, the next hook update on the host writes a fresh file. That
  file has no earlier log and a reset `created`, and it then overwrites the full copy on the
  Mac.
- macOS ships openrsync ("2.6.9 compatible"), which lacks the rsync 3.x behaviour this design
  relies on.
- rsync's temp files land in the repo.

Decision:

- the host keeps its own reports and is their only writer;
- the Mac pulls copies with `ssh host nt agent export --since <cursor>` (a tar stream, in the
  style of `flok serve --stdio`);
- the host prunes ended reports after 14 days.

### 3.6 basic-memory, a design to learn from

- Status: v0.24.1 (2026-10-23), Python, AGPL-3.0. Borrow ideas, not code.
- **Files are the truth.** The SQLite database is a derived, rebuildable index, kept current
  by a file watcher.
- **Syntax:**
  - frontmatter (`title`, `type`, `tags`, `permalink`);
  - observations: `- [category] text #tag`;
  - relations: `- relation_type [[Target]]`.
- **Caution.** It writes a `permalink` into notes that lack one, which causes diff churn in
  git.
- **FTS5 design worth copying:**
  - `tokenize='unicode61 tokenchars 0x2F'` keeps paths whole;
  - `prefix='1,2,3,4'`;
  - unindexed metadata columns;
  - hybrid search with vectors.

### 3.7 MCP versus CLI plus skill

| | CLI with `--json` + skill | MCP server |
|---|---|---|
| From hooks, scripts, humans, cron | yes | Claude Code hooks only (`mcp_tool`); Copilot hooks are commands |
| Context cost | Skill description always loaded, body on use | Tool schemas every session (vendor claims of 4–32× more tokens are disputed) |
| Claude Desktop, claude.ai | no | yes (MCP's main advantage) |
| Typed inputs | by convention | schema-enforced |

Practice in 2025–2026 converged on a hybrid: a skill that teaches the agent a CLI.

**MCP options:**

- the official filesystem server (generic; roots are deprecated in spec 2026-07-28);
- many Obsidian MCP servers of mixed quality;
- Go SDKs: `modelcontextprotocol/go-sdk` v1.8.0 (2026-09-14) and `mark3labs/mcp-go` v1.3.0
  (2026-10-14);
- no mature Go server that understands notes.

**Skills (verified):**

- Claude Code reads `~/.claude/skills/<name>/SKILL.md`.
- Copilot CLI reads `~/.copilot/skills` and `~/.agents/skills`, and in repos `.github/skills`,
  `.claude/skills` and `.agents/skills`.
- One skill directory symlinked into both personal locations serves both agents.
- Skills must be personal, because agents run in project directories, not in the notes repo.

**AGENTS.md:**

- Claude Code reads it when there is no CLAUDE.md.
- Copilot CLI reads AGENTS.md, CLAUDE.md and GEMINI.md together.
- The notes repo gets an AGENTS.md describing its layout and frontmatter.

### 3.8 Hook payloads

**Claude Code** (verified, code.claude.com/docs/en/hooks):

- Every event carries `session_id`, `transcript_path`, `cwd`, `permission_mode`, `effort` and
  `hook_event_name`.
- **`Stop`** fires at the end of every turn, but not on an interrupt (API errors fire
  `StopFailure`).
  - It adds `last_assistant_message`, `background_tasks[]`, `session_crons[]` and
    `stop_hook_active`.
  - Use `last_assistant_message`: the transcript may not yet hold the final message.
- **`SubagentStop`** adds `agent_id`, `agent_type` and `agent_transcript_path`.
- **`SessionEnd`** adds `reason`.
  - Its default timeout is 1.5 s in total; raise it per hook (max 60 s) or with
    `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS`.
  - Command hooks may set `"async": true`.

**Copilot CLI** (verified, docs.github.com):

- 14 events, with camelCase payloads by default.
- `sessionEnd`: `{sessionId, timestamp, cwd, reason}`. It has **no transcript path**.
- `agentStop`: `{sessionId, timestamp, cwd, transcriptPath, stopReason}`.
- Config in `~/.copilot/hooks/*.json`; it also reads a repo's `.claude/settings.json`.

**Design implications:**

- One `nt agent` command accepts both payload shapes.
- Hooks capture metadata deterministically. The skill lets the agent write the rich report.
- An optional LLM summary runs `claude -p --settings '{"disableAllHooks":true}'` so the child
  session does not fire hooks and recurse.
- The review's corrections:
  - `ended` is final, so a late async `Stop` cannot flip the status back;
  - `updated` takes the maximum;
  - each log line is one line of at most 200 characters, which also keeps long answers and
    secrets off GitHub.

### 3.9 Frontmatter for machine-written notes

There is no standard. This schema is a synthesis: flat keys plus a `type` discriminator, so
`rg '^status: blocked'` works.

```yaml
---
type: agent-report
schema: 1
date: 2026-10-26
created: 2026-10-26T09:52:51+01:00
updated: 2026-10-26T11:04:10+01:00
ended: 2026-10-26T11:04:10+01:00
agent: claude-code          # claude-code | copilot-cli
host: mbp
session: 00893aaf-19fa-41d2-8238-13269b9b3ca0
project: noteTaking
cwd: /Volumes/data/work/claude/noteTaking
git_branch: main
git_head: a1b2c3d
status: done                # in-progress | done | blocked | failed | abandoned
summary: "One-line outcome"
tags: [agent/claude-code, project/noteTaking]
---
```

---

## 4. Rendering and terminal tech

### 4.1 Glamour v2 specifics

- **Dependencies.** Lip Gloss v2, chroma v2.14.0, goldmark v1.8.5.
- **Removed options.** `WithAutoStyle` and `WithColorProfile` are gone. The default style is
  dark: detect the background yourself and downsample through Lip Gloss.
- **Options** (verified in `glamour.go`): `WithWordWrap`, `WithTableWrap`,
  `WithInlineTableLinks`, `WithHyperlinkMode`, `WithChromaFormatter`, `WithStyles`,
  `WithStylesFromJSONFile`, `WithPreservedNewLines`, `WithEmoji`, `WithBaseURL`.
- **Links.** OSC 8 hyperlinks. tmux supports OSC 8 since 3.4; the user's client reports
  `hyperlinks`.
- **Images.** None: alt text plus an OSC 8 link.
- **Gotchas:**
  - **Frontmatter is not stripped.** By CommonMark rules, `---` / `key: v` / `---` renders as a
    rule plus a level-2 heading. Strip it before rendering.
  - **`[[wikilinks]]` render literally.** Rewrite them before rendering.
  - Set the wrap width explicitly and re-render on resize.
  - Tables expand to the wrap width.
- **No source map.** Glamour gives no mapping from rendered lines back to source lines. Link,
  outline and image pickers must come from the goldmark parse.
- **Images in Bubble Tea.** Issue #163 is still open. v2 has `tea.Raw(...)` for raw escape
  sequences.

### 4.2 Images in terminals and tmux

| Terminal | Kitty graphics | Kitty Unicode placeholders | iTerm2 OSC 1337 | Sixel |
|---|---|---|---|---|
| Ghostty | yes | yes | no (unverified) | no |
| kitty | yes | yes | no | no |
| WezTerm | buggy | ? | yes | yes |
| iTerm2 | partial (unverified) | ? | yes | yes |
| Terminal.app | no | no | no | no |

**tmux:**

- `allow-passthrough` arrived in 3.3a (with `all` in 3.4). It is off on this machine.
- Homebrew's tmux 3.8 is built with sixel. tmux has no native kitty graphics support.
- Inside tmux, Unicode placeholders are the robust choice. The image becomes text cells, so
  tmux scrolls, splits and clears it correctly. They also fit Bubble Tea's cell renderer:
  upload once with `tea.Raw`, then place the placeholder characters in `View()`.
- The attached client may itself be nested tmux. Then passthrough must be enabled at every
  layer (unverified).

**Prior art:**

- yazi chooses the protocol from `$TERM` and `$TERM_PROGRAM`, and falls back to chafa.
- mdfried and md-tui use ratatui-image.
- Go: `blacktop/go-termimg` (Kitty, iTerm2, Sixel, halfblocks, automatic tmux passthrough;
  placeholders "under construction", 68★). `rasterm` is an encoder only.

**Fallback ladder** (nt v1 uses steps 1 and 2):

1. **Open in an OS viewer.**
   - macOS: `qlmanage -p` (stderr to `/dev/null`), then `open`.
   - Linux with a display: `xdg-open`.
2. **Text-symbol rendering.** A tmux popup with `chafa -f symbols`, which works anywhere,
   including over ssh.
3. **Later.** Kitty Unicode placeholders through `tea.Raw` when `TERM_PROGRAM` is ghostty or
   kitty and passthrough is on.

### 4.3 Search

- **No index is needed.** `rg` searched 7,403 `.md` files (about 17 MB) in about 0.15 s on this
  Mac. Point it at the notes directory only: walking a large unrelated tree took about 5 s.
- **Integration patterns:**
  - parse `rg --json`;
  - hand off to fzf via `tea.ExecProcess` (`--disabled` with `change:reload:rg …`);
  - `sahilm/fuzzy` for in-TUI title filtering.
- **If structured queries are needed later:**
  - first, an in-memory scan of frontmatter and links;
  - then modernc.org/sqlite (pure Go, FTS5 compiled in, verified; about v1.61.0) with
    basic-memory's table design.
  - bleve is heavier and overkill.

### 4.4 Markdown parsing in Go

- **goldmark v1.8.6** (2026-09-03). A v2 module exists (v2.0.0 on 2026-08-27, now v2.1.9).
  Stay on v1, because glamour v2 and the abhinav extensions are on v1.
- **Extensions:**
  - `extension.GFM`: tables, strikethrough, linkify, task lists;
  - `go.abhg.dev/goldmark/frontmatter` v0.3.0;
  - `go.abhg.dev/goldmark/wikilink` v0.6.0 (`[[t]]`, `[[t|label]]`, `[[t#frag]]`,
    `![[embed]]`);
  - `go.abhg.dev/goldmark/hashtag`;
  - `parser.WithAutoHeadingID()`.
- **Pattern.** Parse once to extract the title, links, images, headings and tags. Render
  separately with glamour, after stripping frontmatter and rewriting wikilinks.

---

## 5. Decisions taken for nt

The repo contract is in [format.md](format.md). Key choices, and where each came from:

| Decision | Source |
|---|---|
| Plain GFM only; standard md links `[Title](rel/path.md)`; legacy `[[wikilinks]]` are read but never written. | Obsidian lock-in complaints; GitHub rendering; Marksman support. |
| Slug filenames without spaces (`2026-10-26-project-report.md`); the real title lives in frontmatter or the first H1. | GitHub and CommonMark space rules; shell and iOS friendliness; Denote. |
| Daily journal `daily/YYYY/YYYY-MM-DD.md` with a deterministic template, plus dated topic notes linked from the day's journal. | zk groups, Notesium deterministic names, Dendron's journal versus topic split. |
| Attachments in `assets/` next to the notes. | Attachment survey (2.4). |
| Files are the truth; no persisted index; in-memory scan and `rg`. | Logseq, dnote, NotesTUI anti-patterns; rg timing. |
| nt is the only git writer. One repo lock, `.git/nt.lock`, via `syscall.Flock`, taken briefly by every write and by sync around add+commit and merge+resolve. Writes are temp file plus rename (`.nt-tmp-*`). | `index.lock` behaviour; flok's lock pattern; the review's single-lock correction. |
| **Merge only, never rebase.** `git fetch` then `git merge --no-edit origin/main`, called explicitly because the user's global config sets `pull.rebase=true`. | Review: rebase plus fallback meant two code paths with swapped ours/theirs; obsidian-git already used merge. |
| **Conflict copies, never markers.** The remote version stays in place; the local version becomes `<stem>.conflict-<host>-<ts><ext>`; an edit beats a deletion; a crashed merge is resolved at the start of the next cycle. | nb's committed markers; Syncthing-style copies; the review's crash-recovery note. |
| `daily/** merge=union` in `.gitattributes`. | Review: the Mac and another device each create today's journal before syncing. |
| One-shot `nt sync` on a launchd (or systemd) timer, plus sync when the TUI starts, when nvim exits and on `s`. No fsnotify daemon. | Review: one code path, no singleton contention, batched commits. |
| Non-interactive git under launchd: `GIT_SSH_COMMAND="ssh -oBatchMode=yes -oConnectTimeout=10"`, `GIT_TERMINAL_PROMPT=0`, explicit PATH; files over 20 MB are not staged. | Review. |
| Sync skips the merge while nt has nvim open. | Review: nvim's W12 prompt can silently revert an incoming change. |
| One report file per agent session; path fixed by the first event's date; `ended` is final; log lines are capped at 200 characters. | Section 3. |
| The digest is `agents/YYYY/YYYY-MM-DD/README.md`. It is deterministic, written only when the bytes change, and produced only on the Mac. | GitHub renders README.md in a folder; the review's churn warning. |
| Remote hosts keep their own reports; the Mac pulls copies over `ssh … nt agent export`; no `rsync --remove-source-files`. | Review: the spool design lost updates. |
| CLI with `--json`, plus a personal skill for Claude Code and Copilot CLI, plus optional hooks. MCP comes later. | Section 3.7. |
| Charm v2 stack; glamour styled from the nt design tokens; frontmatter stripped before rendering; pickers built from goldmark. | Section 4.1. |
| Images open in Quick Look, `xdg-open` or a chafa popup in v1; inline images come later. | Section 4.2. |

Cutover lessons from this machine:

- **APFS is case-insensitive.** `~/notes` and `~/Notes` are the same path, so the old vault
  is moved to `~/Notes.obsidian` first.
- **Two syncers must not run at once.** obsidian-git has to be turned off.
- **nvim plugins.** obsidian.nvim currently writes wikilinks into `~/Notes`, so it is removed
  in favour of Marksman.

---

## Sources

### Terminal tools and viewers

- Charm: https://github.com/charmbracelet/glow · https://api.github.com/repos/charmbracelet/glow/releases · https://github.com/charmbracelet/glow/pull/1028 · https://api.github.com/repos/charmbracelet/glow/pulls/883 · https://raw.githubusercontent.com/charmbracelet/glow/master/ui/pager.go · https://github.com/charmbracelet/glamour · https://github.com/charmbracelet/glamour/pull/598 · https://charm.land/blog/glamour-tables/ · https://charm.land/blog/v2/ · https://api.github.com/repos/charmbracelet/bubbletea/releases/tags/v2.0.0 · https://api.github.com/repos/charmbracelet/lipgloss/releases/tags/v2.0.0 · https://raw.githubusercontent.com/charmbracelet/bubbletea/main/examples/glamour/main.go
- zk: https://github.com/zk-org/zk · https://zk-org.github.io/zk/config/config.html · https://zk-org.github.io/zk/notes/note-filtering.html · https://zk-org.github.io/zk/tips/daily-journal.html · https://zk-org.github.io/zk/notes/template.html · https://zk-org.github.io/zk/config/config-lsp.html · https://github.com/zk-org/zk-nvim
- nb: https://xwmx.github.io/nb/ · https://github.com/xwmx/nb
- jrnl: https://github.com/jrnl-org/jrnl · https://jrnl.sh/en/stable/usage/
- Others: https://github.com/dnote/dnote · https://github.com/alonswartz/notesium · https://github.com/erikjuhani/basalt · https://github.com/gnebbia/kb · https://github.com/maaslalani/nap · https://protesilaos.com/emacs/denote · https://github.com/Linus-Mussmaecher/rucola · https://github.com/nico2sh/kimun · https://github.com/Yakitrak/notesmd-cli · https://github.com/nunocoracao/Vibe30-day08-notestui
- Editor plugins: https://github.com/obsidian-nvim/obsidian.nvim · https://github.com/epwalsh/obsidian.nvim · https://github.com/nvim-telekasten/telekasten.nvim · https://github.com/Feel-ix-343/markdown-oxide · https://github.com/artempyanykh/marksman
- Viewers: https://github.com/Textualize/frogmouth · https://github.com/henriklovhaug/md-tui · https://github.com/swsnr/mdcat · https://github.com/Epistates/treemd · https://github.com/benjajaja/mdfried · https://github.com/leboiko/markdown-reader
- Obsidian CLI: https://obsidian.md/changelog/2026-02-27-desktop-v1.12.4/ · https://help.obsidian.md/headless

### GUI, Mac and iOS

- Obsidian: https://obsidian.md/pricing · https://obsidian.md/help/attachments · https://obsidian.md/help/plugins/daily-notes · https://github.com/vinzent03/obsidian-git · https://obsidian.md/blog/less-is-safer/
- Logseq: https://logseq.io/p/e3YDyX5AYr · https://discuss.logseq.com/t/whats-new-with-logseq-db-may-16th-2026/35020
- NotePlan: https://help.noteplan.co/article/31-where-are-my-notes-saved · https://help.noteplan.co/article/102-sync-with-git · https://help.noteplan.co/article/49-x-callback-url-scheme
- SilverBullet: https://community.silverbullet.md/t/silverbullet-v2-released/3100 · https://docs.silverbullet.md/Journal
- Others: https://docs.zettlr.com/en/core/workspaces · https://ia.net/writer/support/library/content-blocks/content-blocks-windows · https://lwn.net/Articles/1016400 · https://github.com/dendronhq/dendron · https://support.typora.io/Images · https://github.com/marktext/marktext/releases · https://fsnot.es/ · https://github.com/glushchenko/fsnotes · https://refactoring.fm/p/introducing-tolaria · https://forums.getdrafts.com/t/using-dropbox-with-drafts/20/2
- Leaving Obsidian: https://ma314.bearblog.dev/obsidian-is-starting-to-stress-me-out/ · https://www.androidauthority.com/i-was-wrong-about-obsidian-3574597 · https://dev.to/dev_tips/why-i-switched-from-obsidian-a-real-developers-story-and-what-im-using-now-ndn · https://forum.obsidian.md/t/refactor-workspace-file-to-be-git-friendly/31717 · https://www.xda-developers.com/notion-obsidian-apple-notes-lost-to-plain-folder-of-markdown-files/
- Git on iOS: https://github.com/GitJournal/GitJournal · https://github.com/GitJournal/dart-git · https://workingcopyapp.com/x-callback-url.html · https://apps.apple.com/app/id6758960270 · https://apps.apple.com/app/noteshub-take-notes/id1621210753 · https://swiftpackageregistry.com/swift-developer-tools/swift-libgit2 · https://github.com/mssun/MiniGit · https://developer.apple.com/forums/thread/780956 · https://developer.apple.com/forums/thread/729824 · https://developer.apple.com/forums/thread/694855 · https://developer.apple.com/forums/thread/123145
- Conventions: https://docs.github.com/en/get-started/writing-on-github/getting-started-with-writing-and-formatting-on-github/basic-writing-and-formatting-syntax · https://talk.commonmark.org/t/whitespace-in-image-paths/1121 · https://community.obsidian.md/plugins/periodic-notes

### Agents and concurrency

- Sync tools: https://github.com/simonthum/git-sync · https://github.com/gitwatch/gitwatch · https://github.com/GitJournal/git-auto-sync · https://publish.obsidian.md/git-doc/Start+here
- Git and locks: https://git-scm.com/docs/gitattributes · https://phabricator.wikimedia.org/T201514 · https://nullprogram.com/blog/2016/08/03/ · https://mywiki.wooledge.org/BashFAQ/045 · https://pkg.go.dev/github.com/gofrs/flock · https://claudeissues.com/issue/55724-agent-isolation-worktree-parallel-agents-lose-work-due-to-git-lock-contention-au
- basic-memory and MCP: https://github.com/basicmachines-co/basic-memory · https://docs.basicmemory.com/concepts/knowledge-format · https://github.com/modelcontextprotocol/servers/tree/main/src/filesystem · https://github.com/modelcontextprotocol/go-sdk · https://pkg.go.dev/github.com/mark3labs/mcp-go
- Hooks, skills, AGENTS.md: https://code.claude.com/docs/en/hooks · https://code.claude.com/docs/en/skills · https://code.claude.com/docs/en/memory · https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-hooks-reference · https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills · https://agents.md/ · https://pkg.go.dev/github.com/vulcanshen/clerk
- MCP versus CLI: https://www.checklyhq.com/blog/mcp-vs-cli-token-efficiency/ · https://firecrawl.dev/blog/mcp-vs-cli

### Rendering and terminal tech

- Images: https://yazi-rs.github.io/docs/image-preview · https://github.com/benjajaja/ratatui-image · https://pkg.go.dev/github.com/blacktop/go-termimg · https://github.com/kovidgoyal/kitty/discussions/4021 · https://man.archlinux.org/man/chafa.1.en · https://ss64.com/mac/qlmanage.html
- Search and parsing: https://pkg.go.dev/modernc.org/sqlite · https://pkg.go.dev/go.abhg.dev/goldmark/wikilink · https://pkg.go.dev/go.abhg.dev/goldmark/frontmatter · https://pkg.go.dev/github.com/junegunn/fzf@v0.73.1
- Local checks on this machine: `man lockf`, `brew cat tmux`, tmux CHANGES, the modernc.org/sqlite v1.60.1 source, `rg` timing over 7.4k files.
