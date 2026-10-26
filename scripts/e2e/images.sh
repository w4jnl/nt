#!/usr/bin/env bash
# nt view draws a note's image as kitty Unicode placeholders inside tmux, keeps the layout
# aligned, and keeps the alt text when the terminal cannot draw images. The second half puts a
# stand-in terminal (faketerm.py, answering as Ghostty) behind one tmux and behind two, as flok
# runs nt, and checks the image command reaches it unwrapped. What Ghostty then paints needs a
# person.
source "$(dirname "$0")/lib.sh"

mkdir -p "$T/notes/assets"
cp "$R/assets/brand/nt-512.png" "$T/notes/assets/mark%.png"
cat > "$T/notes/note.md" <<'MD'
# Images

Above the picture.

![The nt mark](assets/mark%25.png)

Below the picture, and a remote one stays a link:

![Remote](https://example.com/x.png)
MD

# placeholders on stdin: each image row is COLS cells of U+10EEEE, each followed by three
# combining marks (row, column, the id's high byte); tmux must keep all three on every cell
rowstat() { python3 -c '
import sys
rows=[l for l in sys.stdin.read().split("\n") if "\U0010EEEE" in l]
cells=[[c for c in l.split("\U0010EEEE")[1:]] for l in rows]
marks_ok=all(len(c.rstrip())==3 for r in cells for c in r)
row_marks=[{c[0] for c in r} for r in cells]
col_marks={tuple(c[1] for c in r) for r in cells}
high={c[2] for r in cells for c in r}
cols=sorted({len(r) for r in cells})
print("rows=%d cols=%s shape=%s marks=%s rowmark=%s colmark=%s high=%d" % (len(rows), ",".join(map(str,cols)),
  "1:2" if cols and cols[0]==2*len(rows) else "other",
  "ok" if marks_ok else "lost", "ok" if all(len(m)==1 for m in row_marks) and len({next(iter(m)) for m in row_marks})==len(rows) else "bad",
  "ok" if len(col_marks)==1 and len(set(next(iter(col_marks))))==cols[0] else "bad", len(high)))'; }
rows() { capture | rowstat; }
outer_rows() { O capture-pane -p -t o | rowstat; }
# what reached the stand-in terminal: kitty commands outside any tmux passthrough, passthroughs
# no tmux unpacked, and placeholder cells
reached() { python3 -c '
import re, sys
b=open(sys.argv[1],"rb").read()
print("apc=%d wrapped=%d placeholders=%d" % (len(re.findall(rb"(?<!\x1b)\x1b_Ga=T,f=100", b)), b.count(b"\x1bPtmux;"), b.count("\U0010EEEE".encode())))' "$1"; }
# term LOG SOCKET SESSION: a stand-in Ghostty attaches to that tmux server as its client
term() { python3 "$R/scripts/e2e/faketerm.py" "$1" 100 40 "ghostty 1.3.1" env TERM=xterm-256color tmux -L "$2" -f /dev/null attach -t "$3" >/dev/null 2>&1 & }
termtype() { tmux -L "$1" -f /dev/null list-clients -F '#{client_termtype}' 2>/dev/null; }

S new-session -d -s e2e -x 100 -y 40 -c "$T/notes" "$BIN view --theme dark --images on note.md"
S set -g allow-passthrough on
expect_soon "the note renders" 'Above the picture' capture
# a 512x512 px image in cells twice as tall as wide (tmux without a client reports 16x32 px)
# takes twice as many columns as rows, well under the 37-row cap
expect_soon "the square image keeps its shape" '^rows=[1-3][0-9] cols=[0-9]+ shape=1:2 ' rows
expect_soon "every cell keeps its row, column and id marks" 'marks=ok rowmark=ok colmark=ok high=1$' rows
alt_lines() { echo "alt=$(capture | grep -c 'The nt mark')"; }
footer() { capture | tail -1; }
session() { if S has-session -t e2e 2>/dev/null; then echo running; else echo gone; fi; }
expect_soon "the alt text gives way to the image" '^alt=0$' alt_lines
expect_soon "the remote image stays a link" 'Remote' capture
# a placeholder measured wider or narrower than one cell would shift the right-aligned scroll %
expect_soon "the footer stays on the last line, right-aligned" '^ e edit .* q quit {20,}[ 0-9]{3}%$' footer
S send-keys -t e2e G
expect_soon "the text below the image follows it" 'Below the picture' capture
S send-keys -t e2e q
expect_soon "q quits" '^gone$' session

# an <img> in an HTML block, as GitHub READMEs centre banners, is drawn the same way
cat > "$T/notes/html.md" <<'MD'
# HTML image

<p align="center"><img src="assets/mark%.png" alt="The mark in HTML" width="200"></p>

After the HTML image.
MD
S new-session -d -s e2e -x 100 -y 40 -c "$T/notes" "$BIN view --theme dark --images on html.md"
expect_soon "an HTML <img> is drawn" '^rows=[1-9][0-9]* cols=[0-9]+ shape=1:2 marks=ok' rows
expect_soon "the text after it stays its own paragraph" 'After the HTML image' capture
S send-keys -t e2e q
expect_soon "html: quits" '^gone$' session

# auto: no terminal behind this tmux, so no images and no hint
S new-session -d -s e2e -x 100 -y 40 -c "$T/notes" "$BIN view --theme dark note.md"
expect_soon "auto keeps the alt text where images cannot be drawn" 'The nt mark' capture
expect_soon "auto draws no placeholders" '^rows=0 ' rows
S send-keys -t e2e q
expect_soon "auto: quits" '^gone$' session

# One tmux in Ghostty: the stand-in terminal attaches first, so tmux knows it as Ghostty when
# nt starts; nt wraps the image once and tmux unpacks it.
S new-session -d -s e2e -x 100 -y 40 -c "$T/notes" sh
S set -g allow-passthrough on; S set -g status off
term "$T/one.log" nt-e2e e2e
expect_soon "one tmux: tmux knows its terminal as Ghostty" '^ghostty' termtype nt-e2e
S respawn-pane -k -t e2e -c "$T/notes" "$BIN view --theme dark note.md"
expect_soon "one tmux: the image reaches the terminal unwrapped" '^apc=[1-9][0-9]* wrapped=0 placeholders=[1-9]' reached "$T/one.log"
S kill-server

# flok: nt's tmux (S) is a client inside a pane of an outer tmux (O), which runs in Ghostty.
# flok publishes the outer socket as FLOK_OUTER_SOCKET on the inner server.
S new-session -d -s e2e -x 100 -y 40 -c "$T/notes" sh
S set -g allow-passthrough on; S set -g status off
O new-session -d -s o -x 100 -y 40 "env -u TMUX tmux -L nt-e2e -f /dev/null attach -t e2e"
O set -g allow-passthrough on; O set -g status off
S set-environment -g FLOK_OUTER_SOCKET "$(O display -p '#{socket_path}')"
term "$T/flok.log" nt-e2e-outer o
expect_soon "flok: the outer tmux knows its terminal as Ghostty" '^ghostty' termtype nt-e2e-outer
expect_soon "flok: the inner tmux sees a tmux as its terminal" '^tmux ' termtype nt-e2e
S respawn-pane -k -t e2e -c "$T/notes" "$BIN view --theme dark note.md"
expect_soon "flok: the image reaches the terminal through both servers, unwrapped" '^apc=[1-9][0-9]* wrapped=0 placeholders=[1-9]' reached "$T/flok.log"
expect_soon "flok: the outer pane keeps every placeholder mark" 'marks=ok rowmark=ok colmark=ok high=1$' outer_rows

# flok without FLOK_OUTER_SOCKET: nt cannot see Ghostty, so it keeps the alt text and sends nothing
S set-environment -gu FLOK_OUTER_SOCKET
: > "$T/flok.log"
S respawn-pane -k -t e2e -c "$T/notes" "$BIN view --theme dark note.md"
expect_soon "flok, outer unknown: the alt text stays" 'The nt mark' capture
S send-keys -t e2e G
expect_soon "flok, outer unknown: the note still scrolls" 'Below the picture' capture
expect_soon "flok, outer unknown: no image is sent" '^apc=0 wrapped=0 placeholders=0$' reached "$T/flok.log"

# flok with the inner server's passthrough off: the footer says what to set
S set-environment -g FLOK_OUTER_SOCKET "$(O display -p '#{socket_path}')"
S set -g allow-passthrough off
S respawn-pane -k -t e2e -c "$T/notes" "$BIN view --theme dark note.md"
expect_soon "flok, passthrough off: the footer says how to fix it" 'allow-passthrough on' capture
finish
