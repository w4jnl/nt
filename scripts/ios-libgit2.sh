#!/usr/bin/env bash
# Build libgit2 for the iOS app (docs/design.md §9): libgit2 with ssh through libssh2 on mbedTLS,
# from pinned source tarballs whose SHA-256 is checked, for the iPhone and the simulator (arm64),
# into app/ios/build/Git2.xcframework: one static library and its headers, with a module map so
# Swift can import Clibgit2. Nothing is committed; the sources are kept in app/ios/build/src.
#   scripts/ios-libgit2.sh          (make ios-libgit2)
# Requires: Xcode, cmake (brew install cmake), curl.
set -euo pipefail
R=$(cd "$(dirname "$0")/.." && pwd)
OUT=$R/app/ios/build
SRC=$OUT/src
WORK=$OUT/git2-work
MIN=26.0

# name version url sha256: the versions nt is tested with; a new one is a reviewed change here.
SOURCES=(
  "mbedtls 3.6.7 https://github.com/Mbed-TLS/mbedtls/releases/download/mbedtls-3.6.7/mbedtls-3.6.7.tar.bz2 a7e8bcbec0e6f761b4af24f25677626b35f762f68eef79c08677a363212d11f6"
  "libssh2 1.11.1 https://github.com/libssh2/libssh2/releases/download/libssh2-1.11.1/libssh2-1.11.1.tar.gz d9ec76cbe34db98eec3539fe2c899d26b0c837cb3eb466a56b0f109cabf658f7"
  "libgit2 1.9.7 https://github.com/libgit2/libgit2/archive/refs/tags/v1.9.7.tar.gz 1a4fbe7589e814777ae76b64734ad80f4ecad22cd33a22682a2aaea4ae5375e7"
)
command -v cmake >/dev/null || { echo "ios-libgit2.sh: cmake is missing: brew install cmake" >&2; exit 1; }
mkdir -p "$SRC"
rm -rf "$WORK" "$OUT/Git2.xcframework"
mkdir -p "$WORK"

for s in "${SOURCES[@]}"; do
  read -r name ver url sum <<<"$s"
  file=$SRC/$name-$ver.${url##*.tar.}
  file=${file%.*}.tar.${url##*.tar.}
  [ -f "$file" ] || curl -sfL -o "$file" "$url"
  got=$(shasum -a 256 "$file" | cut -d' ' -f1)
  [ "$got" = "$sum" ] || { echo "ios-libgit2.sh: $file has SHA-256 $got, not $sum" >&2; exit 1; }
  mkdir -p "$WORK/$name"
  tar -xf "$file" -C "$WORK/$name" --strip-components 1
done

# build one library for one platform (iphoneos or iphonesimulator) into $WORK/<platform>
build() {
  local plat=$1 lib=$2
  shift 2
  cmake -S "$WORK/$lib" -B "$WORK/build/$plat/$lib" -G "Unix Makefiles" \
    -DCMAKE_SYSTEM_NAME=iOS -DCMAKE_OSX_SYSROOT="$plat" -DCMAKE_OSX_ARCHITECTURES=arm64 \
    -DCMAKE_OSX_DEPLOYMENT_TARGET=$MIN -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$WORK/$plat" \
    -DCMAKE_PREFIX_PATH="$WORK/$plat" -DCMAKE_FIND_ROOT_PATH_MODE_LIBRARY=BOTH -DCMAKE_FIND_ROOT_PATH_MODE_INCLUDE=BOTH \
    -DCMAKE_FIND_ROOT_PATH_MODE_PACKAGE=BOTH -DBUILD_SHARED_LIBS=OFF "$@" >"$WORK/build-$plat-$lib.log" 2>&1 ||
    { tail -30 "$WORK/build-$plat-$lib.log" >&2; exit 1; }
  cmake --build "$WORK/build/$plat/$lib" --target install -j 8 >>"$WORK/build-$plat-$lib.log" 2>&1 ||
    { tail -30 "$WORK/build-$plat-$lib.log" >&2; exit 1; }
}

for plat in iphoneos iphonesimulator; do
  echo "$plat: mbedtls, libssh2, libgit2"
  P=$WORK/$plat
  build $plat mbedtls -DENABLE_PROGRAMS=OFF -DENABLE_TESTING=OFF -DMBEDTLS_FATAL_WARNINGS=OFF
  build $plat libssh2 -DCRYPTO_BACKEND=mbedTLS -DMBEDTLS_INCLUDE_DIR="$P/include" \
    -DMBEDCRYPTO_LIBRARY="$P/lib/libmbedcrypto.a" -DMBEDTLS_LIBRARY="$P/lib/libmbedtls.a" -DMBEDX509_LIBRARY="$P/lib/libmbedx509.a" \
    -DBUILD_STATIC_LIBS=ON -DBUILD_EXAMPLES=OFF -DBUILD_TESTING=OFF -DENABLE_ZLIB_COMPRESSION=OFF
  build $plat libgit2 -DBUILD_TESTS=OFF -DBUILD_CLI=OFF -DBUILD_EXAMPLES=OFF -DBUILD_FUZZERS=OFF \
    -DUSE_SSH=libssh2 -DLIBSSH2_INCLUDE_DIR="$P/include" -DLIBSSH2_LIBRARY="$P/lib/libssh2.a" \
    -DUSE_HTTPS=OFF -DUSE_NTLMCLIENT=OFF -DUSE_GSSAPI=OFF -DREGEX_BACKEND=builtin -DUSE_BUNDLED_ZLIB=ON
  # one library with all of them, and the headers Swift reads through a module map
  mkdir -p "$WORK/out/$plat/include"
  libtool -static -o "$WORK/out/$plat/libgit2.a" "$P/lib/libgit2.a" "$P/lib/libssh2.a" \
    "$P/lib/libmbedtls.a" "$P/lib/libmbedx509.a" "$P/lib/libmbedcrypto.a" 2>/dev/null
  cp -R "$P/include/git2" "$P/include/git2.h" "$WORK/out/$plat/include/"
  cat >"$WORK/out/$plat/include/module.modulemap" <<'EOF'
module Clibgit2 {
  header "git2.h"
  link "iconv"
  export *
}
EOF
done

xcodebuild -create-xcframework \
  -library "$WORK/out/iphoneos/libgit2.a" -headers "$WORK/out/iphoneos/include" \
  -library "$WORK/out/iphonesimulator/libgit2.a" -headers "$WORK/out/iphonesimulator/include" \
  -output "$OUT/Git2.xcframework" >/dev/null
rm -rf "$WORK"
echo "built $OUT/Git2.xcframework"
