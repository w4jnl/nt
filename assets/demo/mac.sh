#!/usr/bin/env bash
# Captures the README's Mac pictures from the demo scene (scene.sh): assets/readme/mac.png, the
# window with a note in live preview, and assets/readme/mac-capture.png, nt's mark in the menu
# bar above its open menu, beside the capture panel with its folder list open. A copy of the app
# is built with its own bundle id (nl.w4j.nt.shot), so the nt for Mac you run is left alone,
# and opened in the background with the screenshot arguments; the panel stays off screen and
# draws itself to a file (-NTShot), so it never takes the keys. The menu opens on screen for
# the moment its picture takes. Needs Xcode and xcodegen, ffmpeg, the Screen Recording permission for the terminal,
# and an unlocked screen; the pictures are dark, so a light Mac is switched to dark for the
# few seconds and put back (MAC_DARK=0 keeps the appearance).
set -euo pipefail
R=$(cd "$(dirname "$0")/../.." && pwd)
cd "$R"
W=$(mktemp -d "${TMPDIR:-/tmp}/nt-mac.XXXX")
ID=nl.w4j.nt.shot
dark() { osascript -e "tell application \"System Events\" to tell appearance preferences to set dark mode to $1" >/dev/null 2>&1; }
RESTORE_LIGHT=
cleanup() {
  osascript -e "tell application id \"$ID\" to quit" >/dev/null 2>&1 || true
  defaults delete "$ID" >/dev/null 2>&1 || true
  [ -z "$RESTORE_LIGHT" ] || dark false
  rm -rf "$W" "${D:-}"
}
trap cleanup EXIT

if [ -z "${MAC_APP:-}" ]; then   # MAC_APP=<nt.app built with the bundle id> skips the build
  make mac-project >/dev/null
  (cd app/macos && xcodebuild -project Nt.xcodeproj -scheme Nt -configuration Debug -destination 'platform=macOS,arch=arm64' \
    -derivedDataPath build/ShotDD PRODUCT_BUNDLE_IDENTIFIER=$ID CODE_SIGN_IDENTITY=- build -quiet)
  MAC_APP=$R/app/macos/build/ShotDD/Build/Products/Debug/nt.app   # open wants a full path
fi
APP=$MAC_APP
swiftc -O -o "$W/winid" assets/demo/winid.swift 2>/dev/null
D=$(assets/demo/scene.sh)
nas=$(cd "$D/notes" && ls homelab/*-nas-setup.md)
if [ "${MAC_DARK:-1}" = 1 ] && [ "$(osascript -e 'tell application "System Events" to tell appearance preferences to get dark mode' 2>/dev/null)" = false ]; then
  dark true; RESTORE_LIGHT=1; sleep 2
fi

open -g -n -a "$APP" --env NT_CONFIG="$D/config.toml" --env NT_STATE="$D/state" --args \
  -NTAppearance dark -NTOpen "$nas" -NTCapture todo -NTQuery "Replace the fan in bay 2 due:fri" -NTPlaces 1 -NTShot "$W"
# the panel's picture lands in $W/capture.png a second after it opens
PID=
for _ in $(seq 1 300); do   # a fresh build's first launch can take a while
  PID=$(pgrep -f "$APP/Contents/MacOS/nt" | head -1 || true)
  [ -n "$PID" ] && [ -f "$W/capture.png" ] && "$W/winid" "$PID" > "$W/window.json" 2>/dev/null && break
  sleep 0.2
done
[ -s "$W/window.json" ] || { echo "mac.sh: the app did not open its window (pid '$PID'; $(ls "$W" | tr '\n' ' '))" >&2; exit 1; }
ax() { osascript -e "tell application \"System Events\" to tell (first process whose bundle identifier is \"$ID\") to $1"; }
if command -v aerospace >/dev/null; then   # a tiling window manager: let this window float at its own size
  aerospace layout floating --window-id "$(aerospace list-windows --monitor all --app-bundle-id $ID --format '%{window-id}' | head -1)" || true
  sleep 0.5
fi
ax "set position of (first window whose subrole is \"AXStandardWindow\") to {40, 60}" >/dev/null
ax "set size of (first window whose subrole is \"AXStandardWindow\") to {1180, 820}" >/dev/null
sleep 4   # the note's web view, the image in it
"$W/winid" "$PID" > "$W/window.json"
j() { python3 -c "import json;print(int(round(json.load(open('$1'))['$2'])))"; }
mkdir -p assets/readme
screencapture -x -o -l "$(j "$W/window.json" window)" assets/readme/mac.png

# the menu: opened by its menu bar item and pictured (a menu is a window at layer 101); a
# second click closes it, before the app quits. The item itself is not pictured: the menu bar
# is translucent, so its strip would show the wallpaper.
ax "click menu bar item 1 of menu bar 2" >/dev/null
for _ in $(seq 1 50); do "$W/winid" "$PID" 101 > "$W/menu.json" 2>/dev/null && break; sleep 0.1; done
[ -s "$W/menu.json" ] || { echo "mac.sh: the menu did not open" >&2; exit 1; }
sleep 0.5
screencapture -x -o -l "$(j "$W/menu.json" window)" "$W/menu.png"
ax "click menu bar item 1 of menu bar 2" >/dev/null
sleep 0.5
osascript -e "tell application id \"$ID\" to quit" >/dev/null 2>&1 || true

# to 1x (a Retina display captures at 2x)
norm() { # in, out, width in points
  local px; px=$(sips -g pixelWidth "$1" | awk '/pixelWidth/{print $2}')
  local scale=$(( px / $3 )); [ "$scale" -lt 1 ] && scale=1
  ffmpeg -y -loglevel error -i "$1" -vf "scale=iw/$scale:ih/$scale:flags=lanczos" "$2"
}
norm "$W/menu.png" "$W/menu1x.png" "$(j "$W/menu.json" w)"
norm "$W/capture.png" "$W/capture1x.png" 620
px() { sips -g "pixel$2" "$1" | awk '{print $2}' | tail -1; }
mw=$(px "$W/menu1x.png" Width); mh=$(px "$W/menu1x.png" Height)
cw_=$(px "$W/capture1x.png" Width); ch_=$(px "$W/capture1x.png" Height)
# the menu at the left, the panel at the right, their tops level, on a transparent canvas
gap=40; cw=$(( mw + gap + cw_ + 4 )); ch=$(( mh > ch_ ? mh : ch_ ))
ffmpeg -y -loglevel error -f lavfi -i "color=c=black@0.0:s=${cw}x${ch},format=rgba" -i "$W/menu1x.png" -i "$W/capture1x.png" \
  -filter_complex "[0][1]overlay=2:0[a];[a][2]overlay=$((mw + gap + 2)):0" -frames:v 1 assets/readme/mac-capture.png
ls -l assets/readme/mac.png assets/readme/mac-capture.png | awk '{print $5" bytes "$9}'
