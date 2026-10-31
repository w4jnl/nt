#!/usr/bin/env bash
# Captures assets/readme/phone-screens.jpg, the README's iPhone screenshots, from the demo scene
# (scene.sh): the app from `make ios-build` in the iPhone simulator, the scene's notes cloned
# into its folder as the phone would clone them, launched once per screen with the screenshot
# arguments (-NTOpen, -NTView), and `simctl io screenshot`. phones.html frames the shots and
# headless Chrome renders them. SHOTS=<dir> keeps the shots there too (for a look at each).
# Needs Xcode, xcodegen, gomobile and cmake (make ios-build), and Google Chrome.
set -euo pipefail
R=$(cd "$(dirname "$0")/../.." && pwd)
cd "$R"
W=$(mktemp -d "${TMPDIR:-/tmp}/nt-phone.XXXX")
mkdir -p "$W/shots"
SIM=${PHONE_SIM:-iPhone 17}
U=$(xcrun simctl list devices available | sed -n "s/^ *$SIM (\([0-9A-F-]*\)).*/\1/p" | tail -1)
[ -n "$U" ] || { echo "phone.sh: no '$SIM' simulator" >&2; exit 1; }
APP=app/ios/build/DerivedData/Build/Products/Debug-iphonesimulator/nt.app
[ -d "$APP" ] || make ios-build
cleanup() {
  xcrun simctl terminate "$U" nl.w4j.nt.ios 2>/dev/null || true
  xcrun simctl status_bar "$U" clear 2>/dev/null || true
  rm -rf "$W" "${D:-}"
}
trap cleanup EXIT

make build >/dev/null
D=$(assets/demo/scene.sh)
nas=$(cd "$D/notes" && ls homelab/*-nas-setup.md)

xcrun simctl boot "$U" 2>/dev/null || true
xcrun simctl bootstatus "$U" -b >/dev/null
xcrun simctl ui "$U" appearance dark
xcrun simctl status_bar "$U" override --time 9:41 --batteryState charged --batteryLevel 100 --cellularMode active --cellularBars 4 --wifiBars 3 --dataNetwork wifi --operatorName ''
xcrun simctl uninstall "$U" nl.w4j.nt.ios 2>/dev/null || true
xcrun simctl install "$U" "$APP"
# the phone's clone: the scene's repository is its remote, so the sync state reads synced
C=$(xcrun simctl get_app_container "$U" nl.w4j.nt.ios data)
git clone -q --bare "$D/notes" "$W/remote.git"
mkdir -p "$C/Library/Application Support/nt"
git clone -q "$W/remote.git" "$C/Library/Application Support/nt/notes"
git -C "$C/Library/Application Support/nt/notes" config user.name mbp
git -C "$C/Library/Application Support/nt/notes" config user.email mbp@example.com

shot() { # name, launch arguments
  local name=$1; shift
  xcrun simctl terminate "$U" nl.w4j.nt.ios 2>/dev/null || true
  xcrun simctl launch "$U" nl.w4j.nt.ios "$@" >/dev/null
  sleep 5   # the index, the note's web view
  xcrun simctl io "$U" screenshot "$W/shots/$name.png" >/dev/null 2>&1
}
shot home
shot note -NTOpen "$nas"
shot todos -NTView todos
[ -z "${SHOTS:-}" ] || { mkdir -p "$SHOTS" && cp "$W"/shots/*.png "$SHOTS/"; }

cp assets/demo/phones.html "$W/phones.html"
ln -s "$R/docs/design-system/fonts" "$W/fonts"
CHROME=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
"$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=2 \
  --window-size=920,712 --virtual-time-budget=10000 --screenshot="$W/phones.png" "file://$W/phones.html" 2>/dev/null
mkdir -p assets/readme
sips -s format jpeg -s formatOptions 82 "$W/phones.png" --out assets/readme/phone-screens.jpg >/dev/null
sips -g pixelWidth -g pixelHeight assets/readme/phone-screens.jpg | tail -2 | tr '\n' ' '; ls -l assets/readme/phone-screens.jpg | awk '{print $5" bytes"}'
