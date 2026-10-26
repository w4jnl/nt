# nt brand assets

![nt: plain markdown notes, synced by git](nt-hero.png)

## The mark
A terminal bracket holding two lines of text and a cursor. The bracket is flok's frame. flok's
chevron is a bird in flight and vink's tick is the bird landed on the line; in nt the glyph is
what the cursor leaves behind: a full line and a short last line, a note being written. Thin
frame (1.6), heavy lines (2.9), cursor 4.8 × 2.2 with round ends: the same weights as flok and
vink on the same 24 grid. Only the glyph is nt's; the bracket and the cursor are unchanged.

The short line ends left of the cursor's centre, so the cursor reads as the prompt on the next
line and not as a third bar. Three centred bars would read as a filter icon.

![The nt app icon: the mark on a dark tile](nt-512.png)

## The two states
| file | meaning |
| --- | --- |
| `nt-mark.svg` | all clear: notes committed and pushed, or a push waiting for the next cycle |
| `nt-mark-alert.svg` | sync needs you: a conflict copy was made, or sync stopped with an error. A solid tile with the lines and the cursor knocked out |

At 16–22px only value reads, not detail, so the alert state fills the whole icon. Offline is
not an alert: nothing is lost and the next cycle retries, so the mark stays as it is. A future
menu bar item follows flok: a template icon in black and alpha, the solid variant while sync
needs you, and optionally the two alternating every ~1150 ms until the window gets focus.

## Files
| file | use |
| --- | --- |
| `nt-mark.svg` | mark, `currentColor`: docs, templates, inline |
| `nt-mark-alert.svg` | alert variant, `currentColor` |
| `nt-mark-teal.svg` | W4J teal, for light grounds |
| `nt-mark-cyan.svg` | cyan (`accent` dark), for dark grounds |
| `nt-lockup.svg` | mark + wordmark, horizontal, `currentColor` |
| `nt-lockup-tagline.svg` | lockup + "plain markdown notes, synced by git" |
| `nt-wordmark.svg` | `[nt]`, the wordmark for places without the mark |
| `favicon.svg` / `favicon-alert.svg` | 32px tiles: teal with the white mark / alert red with the white alert mark |
| `nt-tile.svg` | the teal tile at `radius-tile` (22%) with the white mark; source of the avatar and the 512 |
| `nt-appicon.svg` | the Mac and phone app icon: the cyan mark on a dark gradient tile, 1024px, flok's recipe with a heavier frame and lines |
| `nt-hero.png` | README banner, 1280×320 |
| `nt-social.png` | GitHub social preview, 1280×640 |
| `nt-avatar.png` / `nt-512.png` | avatar 460px / app icon 512px, from `nt-tile.svg` |
| `gen.py` | writes every SVG and the two render pages from one geometry; text is outlined from JetBrains Mono 700 and 400 |
| `hero.html` / `social.html` | render pages written by `gen.py` |
| `render-hero.sh` | renders the four PNGs: headless Chrome for the pages, `rsvg-convert` for the tiles |

All wordmark text is converted to outlines, so no font is needed to render the SVGs. `gen.py`
is deterministic: running it again writes the same bytes. It needs `fontTools` and `brotli`
(the fonts are woff2). The render pages load JetBrains Mono from `docs/design-system/fonts`, so
rendering needs no network.

```sh
python3 assets/brand/gen.py
assets/brand/render-hero.sh
```

## Palette
Dark is the brand's home; light is complete too. `docs/design-system/tokens.json` is the source
of truth; `gen.py` mirrors the values it needs.

| token | dark | light | role |
| --- | --- | --- | --- |
| W4J teal (`brand`) | `#12999D` | `#12999D` | the tie to w4jnl: tiles, favicon, avatar |
| `accent` | `#3FD0D4` | `#0B7A7E` | interactive colour, the selection bar, links, mark on dark |
| `ok` | `#5FC47A` | `#1F7A3A` | synced, a finished agent report |
| `warn` | `#E8963C` | `#9A5A0E` | offline, a blocked agent |
| `alert` | `#F0645A` | `#C0342B` | conflict or sync error, a failed agent |
| `ink-muted` | `#8A999C` | `#5A686B` | unpushed count, abandoned reports, secondary text |
| `ground` | `#14181A` | `#F6F8F8` | the page, and flok's terminal background |

`ok`, `warn` and `alert` carry the same values as vink's `up`, `late` and `down`. Every state
also has a glyph shape (disc, spinner, dashed ring, diamond, tick, ring), so colour is never the
only signal.

## Type
JetBrains Mono 700 for the wordmark and headlines and 400 for the tagline, as in flok and vink.
In the terminal nt is set in the person's own terminal font. Embed the fonts; never load them
from a CDN.

## ASCII lockup
For `nt version`, `nt doctor`, prompts and footers:

```
  ╭─────────╮
  │   ≡     │   nt 0.1.0
  │   ▁     │   plain markdown notes, synced by git
  ╰─────────╯   w4j.nl · MIT

[≡▁] nt  ·  214 notes  ·  ● synced  ·  ✓ 3 agents today
```

The one-line form `[≡▁] nt` is the mark: `≡` for the lines, `▁` for the cursor. The TUI's brand
line uses the wordmark `[nt]`; a narrow column uses `⌈≡⌉`.

## Clear space
One bracket width (3.9 units on the 24 grid) on all sides. Never place the mark on a background
between `#4A5A5D` and `#A8B4B6`, where the frame loses contrast.

The full system (voice, colour, the terminal UI rules and the CLI) is in
[`docs/design-system/README.md`](../../docs/design-system/README.md).
