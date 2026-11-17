#!/bin/sh
# Install nt's prebuilt binary from a GitHub release: Linux and macOS, amd64 and arm64, no Go and
# no root. It checks the tarball against the release's sha256sums.txt before it installs.
#   curl -fsSL https://raw.githubusercontent.com/w4jnl/nt/main/scripts/install.sh | sh
#   ... | sh -s -- 0.9.6          a version instead of the latest
#   ... | sudo PREFIX=/usr/local sh    another place than ~/.local (nt goes to $PREFIX/bin)
# Run it again to upgrade.
set -eu

repo=https://github.com/w4jnl/nt
prefix=${PREFIX:-$HOME/.local}
ver=${1:-${NT_VERSION:-}}

die() { echo "install.sh: $*" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || die "curl is not installed"
command -v tar >/dev/null 2>&1 || die "tar is not installed"

case $(uname -s) in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "no binary for $(uname -s): nt runs on Linux and macOS" ;;
esac
case $(uname -m) in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) die "no binary for $(uname -m): build from source, $repo#more-install-options" ;;
esac

if [ -z "$ver" ]; then
  # the latest release's page redirects to its tag
  url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$repo/releases/latest") ||
    die "cannot reach $repo/releases/latest"
  ver=${url##*/}
fi
ver=${ver#v}
case $ver in
  [0-9]*.[0-9]*.[0-9]*) ;;
  *) die "\"$ver\" is not a version like 0.9.6" ;;
esac

name=nt_${ver}_${os}_${arch}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
echo "nt $ver for $os/$arch"
curl -fsSL -o "$tmp/$name.tar.gz" "$repo/releases/download/v$ver/$name.tar.gz" ||
  die "no $name.tar.gz in release v$ver: $repo/releases"
curl -fsSL -o "$tmp/sha256sums.txt" "$repo/releases/download/v$ver/sha256sums.txt" ||
  die "release v$ver has no sha256sums.txt"

want=$(awk -v f="$name.tar.gz" '$2 == f || $2 == "*" f {print $1}' "$tmp/sha256sums.txt")
[ -n "$want" ] || die "sha256sums.txt does not list $name.tar.gz"
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$name.tar.gz" | cut -d' ' -f1)
elif command -v shasum >/dev/null 2>&1; then
  got=$(shasum -a 256 "$tmp/$name.tar.gz" | cut -d' ' -f1)
else
  die "neither sha256sum nor shasum is installed: the download cannot be checked"
fi
[ "$got" = "$want" ] || die "$name.tar.gz does not match its checksum: nothing installed"

tar -xzf "$tmp/$name.tar.gz" -C "$tmp" "$name/nt"
mkdir -p "$prefix/bin" 2>/dev/null || die "cannot make $prefix/bin: set PREFIX to a folder you can write"
# beside the old one, then renamed: a running nt keeps its file
cp "$tmp/$name/nt" "$prefix/bin/.nt-new-$$" 2>/dev/null || die "cannot write $prefix/bin: set PREFIX, or run with sudo"
chmod 755 "$prefix/bin/.nt-new-$$"
mv -f "$prefix/bin/.nt-new-$$" "$prefix/bin/nt"
echo "installed $prefix/bin/nt ($("$prefix/bin/nt" version 2>/dev/null || echo "nt $ver"))"

case ":$PATH:" in
  *":$prefix/bin:"*) ;;
  *) echo "$prefix/bin is not on PATH: add  export PATH=\"$prefix/bin:\$PATH\"  to your shell's profile" ;;
esac
missing=""
for t in git fzf rg; do
  command -v "$t" >/dev/null 2>&1 || missing="$missing $t"
done
[ -z "$missing" ] || echo "not installed:$missing (git is needed; fzf is for nt f, rg makes search faster)"
echo "next: nt init ~/notes, or nt init --clone git@github.com:<you>/notes.git; then nt doctor"
