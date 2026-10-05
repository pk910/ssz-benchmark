#!/bin/bash
# Shared by the toolchain recipes: a toolchain is installed under
# $BENCH_TOOLCHAINS/<name>-<version> by the build that needs it and kept
# for the next ones; a new version pinned in a recipe installs next to
# the old one, which the daemon's retention leaves alone (it is small
# next to the build caches). Every recipe is sourced by an adapter's
# build.sh, which then has the tools on PATH and the caches under the
# same directory.
set -euo pipefail
: "${BENCH_TOOLCHAINS:?set BENCH_TOOLCHAINS (the daemon does)}"
: "${BENCH_JOBS:=2}"
mkdir -p "$BENCH_TOOLCHAINS"

# fetch <url> <file>: downloads once (a complete download is kept).
fetch() {
  local url="$1" out="$2"
  if [ -s "$out" ]; then return 0; fi
  echo "toolchain: downloading $url"
  curl -fsSL --retry 3 --retry-delay 5 -o "$out.part" "$url"
  mv "$out.part" "$out"
}

# unpack <archive> <dir> [strip]: extracts into dir once; a marker says
# the extraction completed.
unpack() {
  local archive="$1" dir="$2" strip="${3:-1}"
  if [ -f "$dir/.complete" ]; then return 0; fi
  rm -rf "$dir"
  mkdir -p "$dir"
  tar -xf "$archive" -C "$dir" --strip-components="$strip"
  touch "$dir/.complete"
}
