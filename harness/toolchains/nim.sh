#!/bin/bash
# Nim: one pinned release from nim-lang.org (prebuilt for Linux x64), with
# nimble's package directory under the toolchain directory.
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
NIM_VERSION="${NIM_VERSION:-2.2.12}"
NIM_DIR="$BENCH_TOOLCHAINS/nim-$NIM_VERSION"
if [ ! -f "$NIM_DIR/.complete" ]; then
  echo "toolchain: installing nim $NIM_VERSION"
  fetch "https://nim-lang.org/download/nim-$NIM_VERSION-linux_x64.tar.xz" "$BENCH_TOOLCHAINS/nim-$NIM_VERSION.tar.xz"
  unpack "$BENCH_TOOLCHAINS/nim-$NIM_VERSION.tar.xz" "$NIM_DIR"
fi
export PATH="$NIM_DIR/bin:$PATH"
export NIMBLE_DIR="$BENCH_TOOLCHAINS/nimble"
