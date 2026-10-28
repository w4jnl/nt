#!/usr/bin/env bash
# Build nt for Mac for a release: signed with the team's Developer ID, notarized by Apple and
# stapled, the app and the DMG that carries it, checked as Gatekeeper checks them. The bundled nt
# is stamped with the release's version, as the tarballs are. Nothing is tagged or uploaded:
# scripts/release.sh does that with the DMG this writes.
#   scripts/mac-release.sh 0.5.1     → app/macos/build/release/nt-0.5.1.dmg
# Requires: Xcode 26, XcodeGen, Go; in app/macos/Local.xcconfig the team (NT_TEAM_ID), whose
# Developer ID Application certificate is in the keychain, and the App Store Connect API key that
# notarizes (NT_ASC_KEY_ID, NT_ASC_ISSUER_ID; the .p8 in ~/.appstoreconnect/private_keys/).
set -euo pipefail
ver=${1:?usage: scripts/mac-release.sh <version>   e.g. 0.5.1}
ver=${ver#v}
[[ $ver =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "mac-release.sh: $ver is not a version like 0.5.1" >&2; exit 1; }
R=$(cd "$(dirname "$0")/.." && pwd)
MAC=$R/app/macos
conf() { [ -f "$MAC/Local.xcconfig" ] && sed -n "s/^$1 *= *//p" "$MAC/Local.xcconfig" | tr -d ' \r' || true; }
TEAM=$(conf NT_TEAM_ID)
KEY=$(conf NT_ASC_KEY_ID)
ISSUER=$(conf NT_ASC_ISSUER_ID)
P8=${NT_ASC_KEY_PATH:-$HOME/.appstoreconnect/private_keys/AuthKey_$KEY.p8}
[ -n "$TEAM" ] || { echo "mac-release.sh: no NT_TEAM_ID in app/macos/Local.xcconfig" >&2; exit 1; }
[ -n "$KEY" ] && [ -n "$ISSUER" ] || { echo "mac-release.sh: no NT_ASC_KEY_ID or NT_ASC_ISSUER_ID in app/macos/Local.xcconfig" >&2; exit 1; }
[ -f "$P8" ] || { echo "mac-release.sh: no API key at $P8" >&2; exit 1; }
ID=$(security find-identity -v -p codesigning | sed -n "s/.*\"\(Developer ID Application: .* ($TEAM)\)\".*/\1/p" | head -1)
[ -n "$ID" ] || { echo "mac-release.sh: no Developer ID Application certificate for team $TEAM in the keychain (Xcode › Settings › Accounts › Manage Certificates)" >&2; exit 1; }

OUT=$MAC/build/release
rm -rf "$OUT"
mkdir -p "$OUT"

echo "1/6 nt v$ver for both architectures"
make -s -C "$R" nt-universal VERSION="v$ver"

echo "2/6 the app, Release, for both architectures"
(cd "$MAC" && xcodegen generate --quiet)
build=$(awk -F. '{print $1 * 10000 + $2 * 100 + $3}' <<<"$ver") # 0.5.1 → 501: always rising
(cd "$MAC" && xcodebuild -project Nt.xcodeproj -scheme Nt -configuration Release -destination 'generic/platform=macOS' \
  -derivedDataPath build/ReleaseData MARKETING_VERSION="$ver" CURRENT_PROJECT_VERSION="$build" build -quiet)
ditto "$MAC/build/ReleaseData/Build/Products/Release/nt.app" "$OUT/nt.app"
[ "$("$OUT/nt.app/Contents/Helpers/nt" version)" = "nt $ver" ] || { echo "mac-release.sh: the bundled nt is not v$ver" >&2; exit 1; }

echo "3/6 signing with $ID"
# the bundled nt first, then the app around it; the hardened runtime and a secure timestamp on both
codesign --force --options runtime --timestamp --sign "$ID" "$OUT/nt.app/Contents/Helpers/nt"
codesign --force --options runtime --timestamp --sign "$ID" "$OUT/nt.app"
codesign --verify --deep --strict "$OUT/nt.app"

# notarize submits a file to Apple's notary service and waits for its verdict; on anything but
# Accepted it prints Apple's log, which names each problem, and stops.
notarize() {
  local out id status
  out=$(xcrun notarytool submit "$1" --key "$P8" --key-id "$KEY" --issuer "$ISSUER" --wait --timeout 2h --output-format json)
  id=$(python3 -I -c 'import json, sys; print(json.load(sys.stdin).get("id", ""))' <<<"$out" 2>/dev/null || true)
  status=$(python3 -I -c 'import json, sys; print(json.load(sys.stdin).get("status", ""))' <<<"$out" 2>/dev/null || true)
  echo "    $(basename "$1"): ${status:-no answer} ($id)"
  if [ "$status" != Accepted ]; then
    [ -n "$id" ] && xcrun notarytool log "$id" --key "$P8" --key-id "$KEY" --issuer "$ISSUER" >&2
    echo "mac-release.sh: Apple did not notarize $(basename "$1")" >&2
    exit 1
  fi
}

echo "4/6 notarizing the app (a few minutes)"
# the app on its own first, so the copy people drag to Applications carries its own ticket
ditto -c -k --keepParent "$OUT/nt.app" "$OUT/nt.zip"
notarize "$OUT/nt.zip"
xcrun stapler staple -q "$OUT/nt.app"
rm "$OUT/nt.zip"

echo "5/6 the DMG: the app and a link to Applications, signed and notarized"
stage=$OUT/dmg
mkdir -p "$stage"
ditto "$OUT/nt.app" "$stage/nt.app"
ln -s /Applications "$stage/Applications"
dmg=$OUT/nt-$ver.dmg
hdiutil create -quiet -volname "nt $ver" -srcfolder "$stage" -fs HFS+ -format UDZO -ov "$dmg"
rm -rf "$stage"
codesign --force --timestamp --sign "$ID" "$dmg"
notarize "$dmg"
xcrun stapler staple -q "$dmg"

echo "6/6 checking as Gatekeeper does"
spctl --assess --type execute -vv "$OUT/nt.app" 2>&1 | grep -q "source=Notarized Developer ID" || { spctl --assess --type execute -vv "$OUT/nt.app"; exit 1; }
spctl --assess --type open --context context:primary-signature -vv "$dmg" 2>&1 | grep -q "source=Notarized Developer ID" || { spctl --assess --type open --context context:primary-signature -vv "$dmg"; exit 1; }
xcrun stapler validate -q "$OUT/nt.app"
xcrun stapler validate -q "$dmg"
echo "notarized: $dmg"
shasum -a 256 "$dmg" | cut -d' ' -f1
