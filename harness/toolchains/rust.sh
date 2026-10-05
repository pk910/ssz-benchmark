#!/bin/bash
# Rust: rustup with one pinned stable toolchain (it brings rust-lld, which
# links x86_64 Linux targets by default since 1.90 and takes the section
# shuffle of the layout seeds). CARGO_HOME holds the registry and git
# caches of every build.
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
RUST_VERSION="${RUST_VERSION:-1.99.0}"
export RUSTUP_HOME="$BENCH_TOOLCHAINS/rustup"
export CARGO_HOME="$BENCH_TOOLCHAINS/cargo"
export PATH="$CARGO_HOME/bin:$PATH"
if [ ! -x "$CARGO_HOME/bin/rustup" ]; then
  fetch https://sh.rustup.rs "$BENCH_TOOLCHAINS/rustup-init.sh"
  sh "$BENCH_TOOLCHAINS/rustup-init.sh" -y --no-modify-path --profile minimal --default-toolchain none >/dev/null
fi
if ! rustup toolchain list | grep -q "^$RUST_VERSION-"; then
  echo "toolchain: installing rust $RUST_VERSION"
  rustup toolchain install "$RUST_VERSION" --profile minimal >/dev/null
fi
rustup default "$RUST_VERSION" >/dev/null 2>&1 || true
export CARGO_BUILD_JOBS="$BENCH_JOBS"
export CARGO_NET_RETRY=3
export CARGO_INCREMENTAL=0
