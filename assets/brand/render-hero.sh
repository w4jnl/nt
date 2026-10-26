#!/usr/bin/env bash
# Renders the PNGs from what gen.py writes: hero.html to nt-hero.png (1280x320) and social.html to
# nt-social.png (1280x640) with headless Chrome, nt-tile.svg to nt-avatar.png (460) and nt-512.png
# with rsvg-convert. The fonts come from docs/design-system/fonts, so no network is needed.
# Run gen.py first after changing the geometry, the tagline or the claim.
set -euo pipefail
cd "$(dirname "$0")"
CHROME=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
shot() { # page width height out
  "$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
    --allow-file-access-from-files --window-size="$2,$3" --virtual-time-budget=10000 \
    --screenshot="$PWD/$4" "file://$PWD/$1" 2>/dev/null
}
shot hero.html 1280 320 nt-hero.png
shot social.html 1280 640 nt-social.png
rsvg-convert -w 460 -h 460 nt-tile.svg -o nt-avatar.png
rsvg-convert -w 512 -h 512 nt-tile.svg -o nt-512.png
for f in nt-hero.png nt-social.png nt-avatar.png nt-512.png; do
  printf '%s ' "$f"; sips -g pixelWidth -g pixelHeight "$f" | tail -2 | awk '{printf "%s ", $2}'; echo
done
