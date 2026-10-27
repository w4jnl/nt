#!/usr/bin/env bash
# Records assets/readme/browser.gif from a real Ghostty window, so the notes' images show: the
# headless recorder (vhs, xterm.js) cannot draw kitty graphics. macOS only, at the screen: a
# full-screen Ghostty window opens for about 40 seconds, so nothing else on the desktop is in the
# picture. nt runs in a tmux server of its own that the window attaches to; tmux send-keys plays
# the tour; screencapture takes the screen about 7 times a second and ffmpeg makes the GIF from
# the shots, each shown for as long as it was on screen. The shots are deleted afterwards.
# Do not touch the keyboard or the mouse while it runs: the window has the focus.
set -euo pipefail
R=$(cd "$(dirname "$0")/../.." && pwd)
cd "$R"
if ioreg -n Root -d1 -a | grep -A1 CGSSessionScreenIsLocked | grep -q '<true/>'; then
  echo "ghostty.sh: the screen is locked; run it at the screen" >&2; exit 1
fi
[ -d /Applications/Ghostty.app ] || { echo "ghostty.sh: needs Ghostty in /Applications" >&2; exit 1; }
ver=$(sed -n 's/^## \([0-9][0-9.]*\) (.*/\1/p' CHANGELOG.md | head -1)
make build VERSION="v$ver" >/dev/null
D=$(BIN="$R/bin/nt" assets/demo/scene.sh)
W=$(mktemp -d "${TMPDIR:-/tmp}/nt-ghostty.XXXX")
T="tmux -L nt-rec -f $W/tmux.conf"
cleanup() { $T kill-server 2>/dev/null || true; rm -rf "$D" "$W"; }
trap cleanup EXIT
mkdir -p "$W/bin" "$W/shots"
cp bin/nt "$W/bin/nt"
cat > "$W/tmux.conf" <<CONF
set -g allow-passthrough on
set -g status off
set -sg escape-time 0
set -g default-terminal tmux-256color
set -g window-size latest
CONF
# a shell first: nt starts once Ghostty is attached, so it sees Ghostty and draws images
$T new-session -d -s rec -x 140 -y 40 "env -i HOME=$HOME TERM=tmux-256color PATH=$W/bin:/opt/homebrew/bin:/usr/bin:/bin NT_CONFIG=$D/config.toml NT_STATE=$D/state PS1= bash --norc --noprofile"
open -na Ghostty.app --args --config-default-files=false --fullscreen=true --macos-non-native-fullscreen=true \
  --quit-after-last-window-closed=true --confirm-close-surface=false --font-size=22 \
  --background=#14181A --foreground=#F2F5F5 --cursor-style=block --window-padding-x=16 --window-padding-y=12 \
  -e /opt/homebrew/bin/tmux -L nt-rec attach -t rec
for _ in $(seq 1 100); do [ -n "$($T list-clients 2>/dev/null)" ] && break; sleep 0.1; done
[ -n "$($T list-clients 2>/dev/null)" ] || { echo "ghostty.sh: the Ghostty window did not attach" >&2; exit 1; }
key() { $T send-keys -t rec "$@"; }
key "cd $D/notes && clear && exec nt" Enter
sleep 3 # the window goes full screen and nt draws its first frame

# the shots: the screen as fast as screencapture goes, with the time of each
python3 - "$W/shots" <<'PY' &
import os, subprocess, sys, time
d = sys.argv[1]
i = 0
with open(os.path.join(d, "times"), "w") as log:
    while not os.path.exists(os.path.join(d, "stop")):
        f = os.path.join(d, f"{i:05d}.png")
        subprocess.run(["screencapture", "-x", "-t", "png", f])
        log.write(f"{f} {time.time()}\n"); log.flush()
        i += 1
PY
shots=$!
pause() { sleep "$1"; }
# the tour: the homelab pin, the NAS note with its image, f a to the backups note, b back, the
# todo list, the calendar on yesterday, a search
pause 2; key 1; pause 1.5; key j; pause 0.6; key j; pause 4
key f; pause 1.8; key a; pause 2.8; key b; pause 3
key x; pause 3; key Escape; pause 0.5
key c; pause 1.8; key h; pause 2.6; key Escape; pause 0.5
key /; pause 0.4
for c in r e s t i c; do key -l "$c"; sleep 0.12; done
pause 2.8; key Escape; pause 1.2
touch "$W/shots/stop"; wait "$shots"
$T kill-server 2>/dev/null || true

# a shot is shown until the next one was taken
python3 - "$W/shots" <<'PY'
import os, sys
d = sys.argv[1]
rows = [l.split() for l in open(os.path.join(d, "times"))]
with open(os.path.join(d, "list.txt"), "w") as out:
    for (f, t), (_, nt) in zip(rows, rows[1:] + [rows[-1]]):
        out.write(f"file '{f}'\nduration {max(float(nt) - float(t), 0.05):.3f}\n")
    out.write(f"file '{rows[-1][0]}'\n")
PY
first=$(head -1 "$W/shots/times" | cut -d' ' -f1)
if [ "$(stat -f %z "$first")" -lt 400000 ]; then
  echo "ghostty.sh: the shots are empty (screen asleep or no screen-recording permission)" >&2; exit 1
fi
echo "browser: $(wc -l < "$W/shots/times" | tr -d ' ') shots"
ffmpeg -y -loglevel error -f concat -safe 0 -i "$W/shots/list.txt" \
  -vf "scale=1280:-2:flags=lanczos,split[a][b];[a]palettegen=max_colors=192:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
  "$W/browser.gif"
mkdir -p assets/readme
cp "$W/browser.gif" assets/readme/browser.gif
ls -la assets/readme/browser.gif
