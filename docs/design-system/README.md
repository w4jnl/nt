nt is a terminal-first note tool: plain markdown files in folders, synced by git, read and
browsed in the terminal and written in your own editor. It is one Go binary with a TUI and a
CLI, and agents write their daily reports through the same CLI. It lives beside flok and vink
and shares their bracket, weights, ground and type. Someone who knows flok should recognise nt
as a sibling at a glance. The name is short for note: two letters you type many times a day.

## Voice

- Write like flok's README: technical, direct, honest about scope. Short sentences, no
  exclamation marks, no emoji.
- The product name is always lowercase: `nt`, never "NT" or "Nt", even at the start of a
  sentence. Start the sentence another way if it looks wrong.
- States are lowercase words: synced, syncing, unpushed, offline, conflict; in-progress, done,
  blocked, failed, abandoned. Say "offline since 10:42", not "Error".
- Name things by what people recognise: *note*, *today*, *folder*, *link*, *report*, *digest*.
  Not "entity", "vault", "document object" or "workspace".
- Keys are verbs that say what happens: `t` today, `a` append, `n` new, `s` sync. Footer hints
  show the key and the verb, nothing else.
- Errors say what is wrong and how to fix it, on one line: "No notes repo at ~/notes. Run nt
  init ~/notes." No apologies, no stack traces unless `-d` is set.
- Times are relative first ("2 min ago", "yesterday 18:04"), with the absolute time where
  there is room: in `--json`, frontmatter and the digest.

## The mark

A terminal bracket holding two lines of text and a cursor. The bracket is the same frame flok
and vink use. flok's chevron is a bird in flight and vink's tick is the bird landed on the line;
nt's glyph is what the cursor leaves behind, a full line and a short last line: a note being
written.

- Construction is on a 24-unit grid with flok's exact weights: frame stroke 1.6, lines stroke
  2.9 with round caps, cursor 4.8 × 2.2 with round ends. Thin frame, heavy lines: the frame is
  a container, not a subject.
- The lines are left-aligned and the short one ends left of the cursor's centre, so the cursor
  reads as the next line's prompt and not as a third bar.
- Ink: `mark` (cyan `#3FD0D4` on dark grounds, W4J teal `#12999D` on light). On a `brand` tile
  the mark is white.
- Sync needs you: `nt-mark-alert.svg` fills the frame solid and knocks out the lines and the
  cursor. At 16–22px the eye catches value, not detail, so the state lives in the whole icon.
- Clear space: one bracket width (3.9 units on the 24 grid) on all sides. Minimum size 16px;
  below that use the favicon tile.
- Never place the mark on a ground between `#4A5A5D` and `#A8B4B6` (the frame loses contrast).
  Never outline, rotate, recolour per state, or put the lines outside the bracket.
- Lockups: `nt-lockup` sets the mark beside the wordmark at flok's lockup spacing;
  `nt-lockup-tagline` adds "plain markdown notes, synced by git"; `nt-wordmark` is `[nt]` for
  places without the mark. All text is outlined; no font needed.
- In text the mark is `[≡▁]` (lines, cursor), and `⌈≡⌉` in a narrow column.

## The w4jnl family

Every w4jnl tool uses the same system: the bracket frame, one glyph inside it, JetBrains Mono,
the `ground` colour, W4J teal (`brand`) as the tie, and an `accent` bar on the left edge of
banners (6px in images, `▎` in the terminal). flok's glyph is the chevron and cursor, vink's is
the tick and cursor, nt's is two lines and the cursor. Keep the bracket geometry identical
across tools and change only the glyph. nt's state colours carry the same values as vink's
(`ok` = `up`, `warn` = `late`, `alert` = `down`), and agent report states use flok's glyphs, so
a report reads the same in flok's sidebar, in nt and in the digest.

## Colour

- `tokens.json` is the source of truth. Dark is the brand's home and the first theme; light is
  complete too.
- `ground` is the background of the whole screen. `surface` raises the selected row, an open
  picker and the one-line prompt. `surface-sunk` recesses code blocks in the preview and the
  text you type into a prompt. `line` draws pane borders and the rule under the brand line;
  it is decorative only.
- Set text in `ink`, secondary text (dates, paths, footer verbs) in `ink-muted`, placeholders
  only in `ink-faint`.
- `accent` is the only interactive colour: the selection bar, footer keys, links and headings
  in the preview, the syncing spinner. A highlighted search match, and text selected in the note (in `ink`), sit on `accent-soft`.
- State colours mean state and nothing else: `ok`, `warn`, `alert`, with `-soft` fills where a
  fill is wanted (Mac app badges later). `alert` also marks destructive confirmations.
- Every state has its own glyph shape and word, so the UI is readable in greyscale, to
  colour-blind people and with `NO_COLOR`. Never show a state by colour alone.

| kind | state | glyph | token | colour |
| --- | --- | --- | --- | --- |
| sync | synced | `●` | `synced` → `ok` | green |
| sync | syncing | `◐◓◑◒` (spinner) | `syncing` → `accent` | cyan |
| sync | unpushed | `↑N` | `unpushed` → `ink-muted` | grey; waiting for the next push is normal, never a warning |
| sync | offline | `◌` (dashed ring) | `offline` → `warn` | orange; nothing is lost, the next cycle retries |
| sync | conflict | `◆` | `conflict` → `alert` | red; a conflict copy was made, or sync stopped with an error |
| report | in-progress | `◐◓◑◒` (flok's working spinner; `◐` in static output such as the digest) | `in-progress` → `accent` | cyan |
| report | done | `✓` | `done` → `ok` | green |
| report | blocked | `●` | `blocked` → `warn` | orange |
| report | failed | `◆` | `failed` → `alert` | red |
| report | abandoned | `○` | `abandoned` → `ink-muted` | grey |
| todo | open | `○` | `ink-muted` | grey; its due date in words at the right |
| todo | late | `○` and "2d late" | `warn` on the words | orange; the word says it, the colour repeats it |
| todo | done | `✓` | `done` → `ok` | green; when it was done in words |
| link | points at nothing | its text struck through | `ink-faint` | grey; the strike says it, a click says where it pointed |
| calendar day | todos due | `•` after the number | `ink-muted`; `warn` once the day has passed | grey, orange when overdue; the dot says it, the colour repeats it |
| calendar day | today | the number, bold | `accent` | cyan; "today" beside the date under the month says it |

- `◐` is only ever a spinner frame, so a still `◐` means in-progress and never offline.
- All text pairs named in the token notes are 4.5:1 or better in both themes; `ink-faint` is
  below that by design and never carries text a person must read. A unit test in
  `internal/theme` checks every text and background pair the TUI uses.

## Type

- JetBrains Mono (700 for the wordmark and headlines, 400 for the tagline and data) is the
  brand face, as in flok and vink. The three weights ship in `fonts/` (SIL Open Font License);
  embed them and never load them from a CDN.
- In the terminal nt is set in the person's own terminal font. The TUI uses bold for titles
  and headings and nothing else; no italics for UI words, no underline except links.
- Sentence case everywhere; no uppercase labels.

## Terminal UI

The simplicity budget: one screen, two panes, one line at the top and one at the bottom.

```
[nt]  daily/2026                                                       ● synced · ↑2
──────────────────────────────────────────────────────────────────────────────────────
▎ 2026-10-26.md   today     │  # Monday 26 October
  2026-10-25.md   yesterday │
  2026-10-24.md   Sat       │  - 09:12 sync design reviewed
  ...                       │  ...
──────────────────────────────────────────────────────────────────────────────────────
t today  [ ] day  a append  n new  / search  ? help
```

- The brand line: the wordmark `[nt]` at the top left (brackets in `mark`, name in `ink` bold),
  then the folder you are in, then the sync state at the right edge as glyph and word.
- The list pane: at the root, the home list first (the views, then the pinned folders, each with
  its key at the right in `accent`, a rule in `line` after each group), then a folder browser.
  Dated names sort themselves, newest first. It is
  `list-width` columns wide, at least `list-min`. Dragging the `│` divider with the mouse, or
  `<` `>` (`list-step` columns a press), changes the width; nt remembers it per machine. The
  divider turns `accent` while it is held.
- The preview pane: the selected note rendered (see Preview styling), wrapped at no more than
  `preview-max` columns. Under `list-min` + `preview-min` columns the preview hides and Enter
  shows the note full width.
- Selection: a `▎` bar in `accent` at the left edge of the row on a `surface` background. It is
  the terminal form of the family's accent bar. There is no zebra striping and no boxes around
  rows.
- Link labels (`f`): one or two letters in `on-accent`, bold, on `accent`, over the first cells
  of each link on screen; the footer says what to type.
- The footer: key hints, the key in `accent` and the verb in `ink-muted`, separated by two
  spaces; at the right end the last message, else nt's version in `ink-faint` when the hints
  leave room. With a prompt open, the footer is the prompt.
- No modals. A prompt (append, new note title, search) is one line at the bottom on `surface`,
  with its text on `surface-sunk`. Enter accepts, Escape cancels.
- One picker at a time (links, outline, images, fuzzy title). A picker replaces the list pane;
  Escape brings the list back.
- Editing is nvim's job: `e` hands the terminal over and nt redraws when nvim exits.
- Colour is off when `NO_COLOR` is set or stdout is not a terminal. Glyphs and words stay.
- Light or dark comes from the terminal's background, asked with OSC 11 (flok's `termtheme`,
  including its tmux handling). `[theme] mode = "auto" | "dark" | "light"` in the config
  overrides it.

## Preview styling

- The preview is rendered with Glamour, using a style built from the same tokens. nt does not
  ship a third-party theme.
- Headings in `accent`, bold. Body text in `ink`.
- Links in `accent`, underlined, as OSC 8 hyperlinks where the terminal supports them.
- Inline code and code blocks on `surface-sunk`, with Chroma's highlighting limited to a
  palette drawn from the tokens.
- Block quotes in `ink-muted` with a `line` rule on the left.
- Tables use `line` for borders and `ink` for cells; links in tables stay inline.
- Frontmatter is never rendered. An image alone on its line (`![alt](path)`, or one HTML
  `<img src>` alone in its block) is drawn in place where the terminal can (Ghostty, kitty, also
  through tmux), indented with the text and never wider than it; elsewhere it shows its alt text. `i` opens images full size in the OS viewer.
- `nt theme --glamour` writes the same style to `~/.config/nt/glamour-dark.json` and
  `glamour-light.json`, so `glow -s` and the `v` key render notes exactly as the preview does.

## CLI and logs

- `nt version` and the `nt doctor` header print the lockup:

```
  ╭─────────╮
  │   ≡     │   nt 0.1.0
  │   ▁     │   plain markdown notes, synced by git
  ╰─────────╯   w4j.nl · MIT

[≡▁] nt  ·  214 notes  ·  ● synced  ·  ✓ 3 agents today
```

- Where one line is enough (a pipe, `nt version` without a terminal): `[≡▁] nt 0.1.0 · plain
  markdown notes, synced by git`.
- State words in CLI output get their state glyph and colour, never colour alone: `● synced`,
  `◌ offline since 10:42`, `◆ 1 conflict`.
- `--json` output has no colour, no glyphs and absolute times.
- `-d` logs in colour, mapped to the palette: DEBUG `ink-muted`, INFO `accent`, WARN `warn`,
  ERROR `alert`. Colour is off when stdout is not a terminal or `NO_COLOR` is set.

## Mac app

- **Type.** The system face for the text: SF Pro Text 16/26 for a note's body, SF Pro Display
  semibold for its headings (28, 21, 17). JetBrains Mono, bundled from `fonts/`, for code, times,
  dates, paths and keys. Sentence case, no uppercase labels, menus included.
- **Colour.** The tokens in both themes, following the Mac's appearance: `ground` behind the
  note, the system's sidebar beside it, `accent` for links, keys and the selection. Headings are
  `ink`, not `accent` as in the terminal: a proportional face sets them apart by size, and accent
  stays for what can be clicked.
- **The text column.** 680 px wide and centred, 40 px inside its edges; the rest is ground.
- **Markdown while editing.** Only on the line with the cursor, in `ink-faint`. A heading's `#`
  hangs in the left margin so the text does not move.
- **States.** Glyph and word, as everywhere. The sidebar's sync state is the terminal browser's
  badge, and a click on it syncs.
- **Selection.** The system's, tinted with `accent` (the asset catalogue's AccentColor). In Open,
  the selected row is `accent-soft` with the family's accent bar at its left edge.
- **No modals.** Open, Search and New note are one floating field over the window; Escape closes
  it.
- **Focus mode.** The sidebar goes and every paragraph but the one being written is at 30%
  opacity; nothing else changes, no colour, no size.
- **Read-only.** A digest says so in the bottom line, in mono `ink-muted` beside the word count,
  with why in its tooltip.
- **Nothing remote.** A note's remote image shows its alt text; the app loads only its own files
  and the repository's.

## Favicon, menu bar and app icon

- `favicon.svg` is the teal tile with the white mark. `favicon-alert.svg` is an alert tile with
  the white alert mark, for a future web view while sync needs you.
- A future menu bar item follows flok: a template icon in black and alpha, with the solid alert
  variant while sync needs you. Optionally alternate the two every ~1150 ms and stop when the
  window gets focus. The mark holds a cursor, so blinking is the one animation it is allowed.
  Offline is not an alert.
- `nt-appicon.svg` is the Mac and phone app icon: flok's recipe, the cyan mark on a dark
  gradient tile with a heavier frame and lines. The App Store wants it flattened without an
  alpha channel.

## Assets

- Logos: marks (`currentColor`, teal, cyan, alert), the lockup, the tagline lockup and the
  wordmark, in `assets/brand/`. They use `currentColor` so docs can tint them.
- Tiles: favicon, favicon-alert, `nt-tile.svg` (avatar 460px and the 512px icon) and the app
  icon, on the `brand` tile at `radius-tile`.
- Banners: the README hero (1280×320) and the social card (1280×640), in flok's and vink's
  layouts on `ground` with the 6px `accent` bar.
- `assets/brand/gen.py` writes every SVG from one geometry and `render-hero.sh` renders the
  PNGs. See `assets/brand/README.md`.

## Consuming this system

- Go: `internal/theme` mirrors `tokens.json` as Lip Gloss v2 colours for both themes, plus the
  state glyphs. A test parses `docs/design-system/tokens.json` and fails when the two drift.
  `internal/render` builds the Glamour style from the same values.
- The terminal tokens (`list-width`, `list-step`, `list-min`, `preview-min`, `preview-max`,
  `gutter`, `selection-bar`) are the layout rules above in numbers.
- The Mac app holds the colours twice, in `app/macos/Nt/Brand.swift` for its views and in
  `app/macos/editor/src/editor.css` for the note; a Swift test and a node test compare each with
  `tokens.json`. It bundles `fonts/` and draws its icon from `nt-appicon.svg`. Its UI words use the
  system face, as vink's web UI does. The iOS app will do the same.
