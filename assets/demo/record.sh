#!/usr/bin/env bash
# Records the README's command-line GIF, assets/readme/cli.gif, or the tapes named on the command
# line: scene.sh builds a notes repository, vhs (brew install vhs) plays each tape into a frames
# folder, and ffmpeg assembles the GIF. The README's browser.gif comes from ghostty.sh, since vhs
# cannot draw the note's image; `record.sh browser` makes one without it. As in flok, vhs runs in a temp folder on the
# system volume, where it can write, and ffmpeg makes the GIF, since vhs 0.12's own encoder
# writes an empty one with ffmpeg 9.
set -euo pipefail
R=$(cd "$(dirname "$0")/../.." && pwd)
cd "$R"
ver=$(sed -n 's/^## \([0-9][0-9.]*\) (.*/\1/p' CHANGELOG.md | head -1) # the footer shows the release's version
make build VERSION="v$ver" >/dev/null
FPS=20
D=$(BIN="$R/bin/nt" assets/demo/scene.sh)
W=$(mktemp -d "${TMPDIR:-/tmp}/nt-vhs.XXXX")
trap 'rm -rf "$D" "$W"' EXIT
mkdir -p "$W/bin" assets/readme
cp bin/nt "$W/bin/nt"
tapes=("$@"); [ ${#tapes[@]} -gt 0 ] || tapes=(cli)
for tape in "${tapes[@]}"; do
  sed -e "s#@D@#$D#g" -e "s#@BIN@#$W/bin#g" "assets/demo/$tape.tape" > "$W/$tape.tape"
  rm -rf "$W/frames"
  (cd "$W" && vhs "$tape.tape" >/dev/null)
  F=$W/frames
  echo "$tape: $(ls "$F"/frame-text-*.png | wc -l | tr -d ' ') frames"
  ffmpeg -y -loglevel error -framerate $FPS -i "$F/frame-text-%05d.png" -framerate $FPS -i "$F/frame-cursor-%05d.png" \
    -filter_complex "[0][1]overlay,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    "$W/$tape.gif"
  cp "$W/$tape.gif" "assets/readme/$tape.gif"
  ls -la "assets/readme/$tape.gif"
done
