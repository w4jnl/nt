#!/usr/bin/env bash
# Tag an nt release and bump the Homebrew formula in w4jnl/homebrew-tap. The tag starts the
# release workflow, which builds the Linux and macOS tarballs and writes the GitHub release from
# CHANGELOG.md.
#   scripts/release.sh 0.1.1          (macOS: uses BSD sed -i '')
# Requires: a clean tree on main pushed to origin, and ssh access to both repositories.
set -euo pipefail
ver=${1:?usage: scripts/release.sh <version>   e.g. 0.1.1}
# GitHub's SSH endpoint resets a connection now and then; a push or pull gets three tries.
retry() { local n; for n in 1 2 3; do "$@" && return 0; [ "$n" -lt 3 ] && { echo "release.sh: $* failed, retrying in 10 s" >&2; sleep 10; }; done; return 1; }
ver=${ver#v}
R=$(cd "$(dirname "$0")/.." && pwd)
cd "$R"
[ "$(git branch --show-current)" = main ] || { echo "release.sh: not on main" >&2; exit 1; }
[ -z "$(git status --porcelain)" ] || { echo "release.sh: working tree not clean" >&2; exit 1; }
git fetch -q origin
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || { echo "release.sh: main is not pushed" >&2; exit 1; }
# release notes come from CHANGELOG.md: the "## <ver> (date)" section must exist before tagging
notes=$(awk -v v="$ver" '/^## /{p = ($2 == v)} p && !/^## /' CHANGELOG.md | sed '/./,$!d')
[ -n "$notes" ] || { echo "release.sh: CHANGELOG.md has no \"## $ver (YYYY-MM-DD)\" section; write the release notes first (move the Unreleased items)" >&2; exit 1; }
grep -qE "^## $ver \([0-9]{4}-[0-9]{2}-[0-9]{2}\)" CHANGELOG.md || { echo "release.sh: the CHANGELOG.md heading must be \"## $ver (YYYY-MM-DD)\"" >&2; exit 1; }
[ -z "$(gofmt -l ./cmd ./internal ./skills)" ] || { echo "release.sh: gofmt -l ./cmd ./internal ./skills is not empty" >&2; exit 1; }
go vet ./... && go test -timeout 120s ./... >/dev/null && echo "tests ok"
# the tap is checked before the tag is pushed, so a release never stops half done
TAP=${NT_TAP_DIR:-$R/../homebrew-tap}
[ -d "$TAP/.git" ] || git clone -q git@github.com:w4jnl/homebrew-tap.git "$TAP"
[ -z "$(git -C "$TAP" status --porcelain)" ] || { echo "release.sh: $TAP has uncommitted changes; commit or stash them first" >&2; exit 1; }
f=$TAP/Formula/nt.rb
[ -f "$f" ] || { echo "release.sh: no Formula/nt.rb in $TAP" >&2; exit 1; }
retry git -C "$TAP" pull -q --ff-only

git tag -a "v$ver" -m "nt v$ver"
retry git push -q origin "v$ver"
echo "tag v$ver pushed; the release workflow builds the tarballs and the release notes"

url="https://github.com/w4jnl/nt/archive/refs/tags/v$ver.tar.gz"
sha=""
for _ in $(seq 1 30); do
  if sha=$(curl -sfL "$url" | shasum -a 256 | cut -d' ' -f1) && [ ${#sha} -eq 64 ]; then break; fi
  sha=""; sleep 2
done
[ -n "$sha" ] || { echo "release.sh: $url not available yet; rerun the tap bump by hand" >&2; exit 1; }
echo "sha256 $sha"

retry git -C "$TAP" pull -q --ff-only
sed -i '' -e "s#^  url \".*\"#  url \"$url\"#" -e "s#^  sha256 \".*\"#  sha256 \"$sha\"#" "$f"
git -C "$TAP" add Formula/nt.rb
git -C "$TAP" commit -q -m "nt $ver"
retry git -C "$TAP" push -q
echo "tap bumped: $f"

echo "release: https://github.com/w4jnl/nt/releases/tag/v$ver"
echo "CI:      https://github.com/w4jnl/nt/actions  and  https://github.com/w4jnl/homebrew-tap/actions"
echo "users:   brew update && brew upgrade nt"
