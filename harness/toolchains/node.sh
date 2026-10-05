#!/bin/bash
# Node: one pinned release from nodejs.org; npm's cache under the
# toolchain directory.
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
NODE_VERSION="${NODE_VERSION:-v24.21.0}"
NODE_DIR="$BENCH_TOOLCHAINS/node-$NODE_VERSION"
if [ ! -f "$NODE_DIR/.complete" ]; then
  echo "toolchain: installing node $NODE_VERSION"
  fetch "https://nodejs.org/dist/$NODE_VERSION/node-$NODE_VERSION-linux-x64.tar.xz" "$BENCH_TOOLCHAINS/node-$NODE_VERSION.tar.xz"
  unpack "$BENCH_TOOLCHAINS/node-$NODE_VERSION.tar.xz" "$NODE_DIR"
fi
export PATH="$NODE_DIR/bin:$PATH"
export npm_config_cache="$BENCH_TOOLCHAINS/npm-cache"
export npm_config_update_notifier=false
export npm_config_fund=false
export npm_config_audit=false
