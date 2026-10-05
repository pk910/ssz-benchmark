#!/bin/bash
# C++: the system compiler (gcc 14 on the box, C++23), plus CMake from the
# Kitware release tarball, since the box has none.
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
CMAKE_VERSION="${CMAKE_VERSION:-3.31.8}"
CMAKE_DIR="$BENCH_TOOLCHAINS/cmake-$CMAKE_VERSION"
if [ ! -f "$CMAKE_DIR/.complete" ]; then
  echo "toolchain: installing cmake $CMAKE_VERSION"
  fetch "https://github.com/Kitware/CMake/releases/download/v$CMAKE_VERSION/cmake-$CMAKE_VERSION-linux-x86_64.tar.gz" "$BENCH_TOOLCHAINS/cmake-$CMAKE_VERSION.tar.gz"
  unpack "$BENCH_TOOLCHAINS/cmake-$CMAKE_VERSION.tar.gz" "$CMAKE_DIR"
fi
export PATH="$CMAKE_DIR/bin:$PATH"
export CMAKE_BUILD_PARALLEL_LEVEL="$BENCH_JOBS"
