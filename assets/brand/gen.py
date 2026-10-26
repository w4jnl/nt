"""Generate nt's brand SVGs and render pages from one geometry, on flok's 24-unit grid.

The mark: flok's terminal bracket holding two lines of text and a cursor. flok's chevron is a
bird in flight and vink's tick is the bird landed on the line; nt's glyph is what the cursor
leaves behind: a full line and a short last line, a note being written. Thin frame (1.6),
heavy lines (2.9), cursor 4.8 x 2.2: the same weights as flok and vink. The bracket and the
cursor are copied from vink's gen.py unchanged; only the glyph is nt's.

Wordmark text is outlined from JetBrains Mono, so no file needs the font to render.
Deterministic: running it again writes the same bytes.

    python3 assets/brand/gen.py        # needs fontTools and brotli (woff2)
    assets/brand/render-hero.sh        # then the PNGs
"""
import os
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen

HERE = os.path.dirname(os.path.abspath(__file__))
FONTS = os.environ.get('NT_FONTS', os.path.join(HERE, '..', '..', 'docs', 'design-system', 'fonts'))

# ---- palette (docs/design-system/tokens.json is the source of truth; these mirror it) ----
TEAL = '#12999D'     # brand: W4J teal, the tie to w4jnl
CYAN = '#3FD0D4'     # accent and mark on dark
INK_D = '#F2F5F5'    # ink, dark theme
GROUND = '#14181A'   # ground, dark theme
MUTED = '#8A999C'    # ink-muted, dark theme
OK = '#5FC47A'       # ok, dark theme
WARN = '#E8963C'     # warn, dark theme
ALERT = '#F0645A'    # alert, dark theme
ALERT_TILE = '#C0342B'  # alert, light theme: tile fill under a white mark

TAGLINE = 'plain markdown notes, synced by git'
REPO = 'w4jnl/nt'

# ---- geometry (bracket and cursor: vink's gen.py, unchanged) ----
BRACKET = ('<g stroke="{c}" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" fill="none">'
           '<path d="M8 3.2 H6 a2.2 2.2 0 0 0 -2.2 2.2 V18.6 a2.2 2.2 0 0 0 2.2 2.2 H8"/>'
           '<path d="M16 3.2 H18 a2.2 2.2 0 0 1 2.2 2.2 V18.6 a2.2 2.2 0 0 1 -2.2 2.2 H16"/></g>')
CURSOR = '<rect x="9.6" y="16.2" width="4.8" height="2.2" rx="1.1" fill="{c}"/>'
# nt's glyph: a full line over a short last line, left-aligned. The short line ends left of the
# cursor's centre, so the cursor reads as the next line's prompt, not as a third bar.
LINES_D = 'M7.6 7.8 H16.4 M7.6 11.8 H11.2'
LINES = '<path d="' + LINES_D + '" stroke="{c}" stroke-width="2.9" stroke-linecap="round" fill="none"/>'

def mark_inner(c):
    return BRACKET.format(c=c) + LINES.format(c=c) + CURSOR.format(c=c)

def mark_alert_inner(c, mid='ntAlert'):
    # solid tile, lines and cursor knocked out: at 16-22px value, not detail, carries the state
    return (f'<mask id="{mid}"><rect x="2.6" y="2.2" width="18.8" height="19.6" rx="5.2" fill="#fff"/>'
            f'<path d="{LINES_D}" stroke="#000" stroke-width="3.1" stroke-linecap="round" fill="none"/>'
            f'<rect x="9.6" y="16.4" width="4.8" height="2.3" rx="1.15" fill="#000"/></mask>'
            f'<rect x="2.6" y="2.2" width="18.8" height="19.6" rx="5.2" fill="{c}" mask="url(#{mid})"/>')

def svg(w, h, body, title, vb=None):
    vb = vb or f'0 0 {w} {h}'
    return f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}" width="{w}" height="{h}"><title>{title}</title>{body}</svg>\n'

# ---- outlined text ----
def num(v):
    s = ('%.2f' % v).rstrip('0').rstrip('.')
    return '0' if s in ('-0', '') else s

_fonts = {}
def font(weight):
    if weight not in _fonts:
        _fonts[weight] = TTFont(os.path.join(FONTS, f'JetBrainsMono-{weight}.woff2'))
    return _fonts[weight]

def text_path(txt, size, x, y, weight=700, spacing=0.0):
    f = font(weight)
    gs = f.getGlyphSet(); cmap = f.getBestCmap(); upm = f['head'].unitsPerEm
    s = size / upm
    parts = []
    cx = x
    for ch in txt:
        gname = cmap[ord(ch)]
        pen = SVGPathPen(gs, ntos=num)
        gs[gname].draw(TransformPen(pen, (s, 0, 0, -s, cx, y)))
        d = pen.getCommands()
        if d:
            parts.append(d)
        cx += gs[gname].width * s + spacing
    return ' '.join(parts), cx

def text_el(txt, size, x, y, fill, weight=700, spacing=0.0):
    d, _ = text_path(txt, size, x, y, weight, spacing)
    return f'<path d="{d}" fill="{fill}"/>'

def text_end(txt, size, x, weight=700, spacing=0.0):
    return text_path(txt, size, x, 0, weight, spacing)[1]

# ---- builders ----
def mark(c, title='nt'):
    return svg(24, 24, mark_inner(c), title)

def mark_alert(c):
    return svg(24, 24, mark_alert_inner(c), 'nt - sync needs you')

def lockup_body(mark_c, word_c):
    return f'<g transform="scale(1.83333)">{mark_inner(mark_c)}</g>' + text_el('nt', 34, 56, 32, word_c, 700, -1)

def lockup(mark_c, word_c):
    w = round(text_end('nt', 34, 56, 700, -1) + 4)
    return svg(w, 44, lockup_body(mark_c, word_c), 'nt - horizontal lockup')

def lockup_tagline_body(mark_c, word_c, tag_c, tag_size=11):
    return (f'<g transform="translate(0 4) scale(1.83333)">{mark_inner(mark_c)}</g>'
            + text_el('nt', 28, 56, 30, word_c, 700, -1)
            + text_el(TAGLINE, tag_size, 57, 46, tag_c, 400, 0))

def lockup_tagline(mark_c, word_c, tag_c):
    w = round(max(text_end('nt', 28, 56, 700, -1), text_end(TAGLINE, 11, 57, 400)) + 4)
    return svg(w, 52, lockup_tagline_body(mark_c, word_c, tag_c), 'nt - lockup with tagline')

def wordmark(bracket_c, word_c):
    end = text_end('nt', 40, 26, 700, -1)
    a = num(end - 2)          # the right arm starts where vink's does: 2 units inside the text
    b = num(end + 5)
    w = round(end + 14)
    body = (f'<g stroke="{bracket_c}" stroke-width="4.2" stroke-linecap="round" stroke-linejoin="round" fill="none">'
            '<path d="M16 5 H9 a5 5 0 0 0 -5 5 V42 a5 5 0 0 0 5 5 H16"/>'
            f'<path d="M{a} 5 H{b} a5 5 0 0 1 5 5 V42 a5 5 0 0 1 -5 5 H{a}"/></g>'
            + text_el('nt', 40, 26, 38, word_c, 700, -1))
    return svg(w, 52, body, '[nt]')

def favicon(tile, fg):
    return svg(32, 32, f'<rect width="32" height="32" rx="7" fill="{tile}"/><g transform="translate(4 4)">{mark_inner(fg)}</g>', 'nt')

def favicon_alert(tile, fg):
    body = (f'<rect width="32" height="32" rx="7" fill="{tile}"/>'
            f'<g transform="translate(4 4)">{mark_alert_inner(fg, "ntFavAlert")}</g>')
    return svg(32, 32, body, 'nt - sync needs you')

def tile(fill, fg):
    # W4J's rounded square (radius-tile, 22%) with the mark at 35% of the width, like vink's avatar
    return svg(120, 120, f'<rect width="120" height="120" rx="26.4" fill="{fill}"/>'
               f'<g transform="translate(29.28 29.28) scale(2.56)">{mark_inner(fg)}</g>', 'nt')

def appicon():
    # the Mac and phone app icon, flok's recipe: cyan mark on a dark gradient tile, the frame and
    # the lines a step heavier because the icon is seen small and on busy wallpapers
    body = ('<defs><linearGradient id="g" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#1b2124"/>'
            '<stop offset="1" stop-color="#0f1214"/></linearGradient></defs><rect width="120" height="120" fill="url(#g)"/>'
            f'<g transform="translate(19.2 19.2) scale(3.4)"><g stroke="{CYAN}" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" fill="none">'
            '<path d="M7.6 3.2 H5 a2.4 2.4 0 0 0 -2.4 2.4 V18.4 a2.4 2.4 0 0 0 2.4 2.4 H7.6"/>'
            '<path d="M16.4 3.2 H19 a2.4 2.4 0 0 1 2.4 2.4 V18.4 a2.4 2.4 0 0 1 -2.4 2.4 H16.4"/></g>'
            f'<path d="M7.4 7.8 H16.6 M7.4 11.9 H11.2" stroke="{CYAN}" stroke-width="3.35" stroke-linecap="round" fill="none"/>'
            f'<rect x="9.5" y="16.2" width="5" height="2.4" rx="1.2" fill="{CYAN}"/></g>')
    return svg(1024, 1024, body, 'nt', vb='0 0 120 120')

def write(path, text):
    with open(path, 'w') as fh:
        fh.write(text)

# ---- render pages: headless Chrome turns these into the PNGs (render-hero.sh) ----
FONT_FACE = ''.join(
    f'@font-face{{font-family:"JetBrains Mono";font-weight:{w};src:url("../../docs/design-system/fonts/JetBrainsMono-{w}.woff2") format("woff2")}}'
    for w in (400, 700))

def page(w, h, body, extra_css=''):
    return ('<!doctype html>\n<!-- generated by gen.py; render-hero.sh turns it into a PNG with headless Chrome -->\n'
            '<html><head><meta charset="utf-8"><style>'
            + FONT_FACE
            + f'html,body{{margin:0;width:{w}px;height:{h}px;overflow:hidden;background:{GROUND};color:{INK_D};'
            'font-family:"JetBrains Mono",ui-monospace,monospace}'
            f'.bar{{position:absolute;left:0;top:0;width:6px;height:{h}px;background:{CYAN}}}'
            + extra_css + '</style></head><body><div class="bar"></div>' + body + '</body></html>\n')

def hero_page():
    # flok's banner: the tagline lockup at 3.2x, the tagline set smaller than the small lockup's
    vw = round(text_end(TAGLINE, 6.4, 57, 400) + 4)
    lk = (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {vw} 52" width="{round(vw * 3.2)}" height="166">'
          + lockup_tagline_body(CYAN, INK_D, MUTED, tag_size=6.4) + '</svg>')
    return page(1280, 320, f'<div class="lockup">{lk}</div>', '.lockup{position:absolute;left:72px;top:77px}')

def social_page():
    # vink's social card: lockup, repo line, a two-line claim and the states in their colours
    lk = (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {round(text_end("nt", 34, 56, 700, -1) + 4)} 44" '
          f'height="132">' + lockup_body(CYAN, INK_D) + '</svg>')
    states = ''.join(f'<span style="color:{c}">{g}&nbsp;{w}</span>' for g, w, c in (
        ('●', 'synced', OK), ('↑2', 'unpushed', MUTED), ('◌', 'offline', WARN), ('◆', 'conflict', ALERT)))
    body = (f'<div class="lockup">{lk}</div>'
            f'<div class="repo">{REPO}&nbsp;&nbsp;·&nbsp;&nbsp;Go&nbsp;&nbsp;·&nbsp;&nbsp;MIT</div>'
            '<div class="claim">Your notes are plain files.<br>git keeps them in sync.</div>'
            f'<div class="states">{states}</div>')
    css = ('.lockup{position:absolute;left:96px;top:180px}'
           f'.repo{{position:absolute;left:266px;top:288px;font-size:20px;color:{MUTED}}}'
           '.claim{position:absolute;left:96px;top:396px;font-size:36px;font-weight:700;line-height:52px}'
           '.states{position:absolute;left:96px;top:540px;font-size:18px;display:flex;gap:36px}')
    return page(1280, 640, body, css)

# ---- the repo set: currentColor masters, so docs and templates can tint them ----
R = HERE
write(f'{R}/nt-mark.svg', mark('currentColor'))
write(f'{R}/nt-mark-alert.svg', mark_alert('currentColor'))
write(f'{R}/nt-mark-teal.svg', mark(TEAL))
write(f'{R}/nt-mark-cyan.svg', mark(CYAN))
write(f'{R}/nt-lockup.svg', lockup('currentColor', 'currentColor'))
write(f'{R}/nt-lockup-tagline.svg', lockup_tagline('currentColor', 'currentColor', MUTED))
write(f'{R}/nt-wordmark.svg', wordmark('currentColor', 'currentColor'))
write(f'{R}/favicon.svg', favicon(TEAL, '#ffffff'))
write(f'{R}/favicon-alert.svg', favicon_alert(ALERT_TILE, '#ffffff'))
write(f'{R}/nt-tile.svg', tile(TEAL, '#ffffff'))
write(f'{R}/nt-appicon.svg', appicon())
write(f'{R}/hero.html', hero_page())
write(f'{R}/social.html', social_page())
print('ok')
