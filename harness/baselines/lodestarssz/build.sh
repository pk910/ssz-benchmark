#!/bin/bash
# Builds the @chainsafe/ssz adapter at a commit of ChainSafe/ssz:
#
#   build.sh <commit> <out>
#
# installs or updates Node (toolchains/node.sh), fetches the monorepo at
# the commit into work/ssz, builds packages/ssz and the workspace packages
# it depends on (as-sha256, persistent-merkle-tree) with the repository's
# own package manager and build scripts, generates the type modules for
# both forks and presets from the harness types, and leaves the launchers
# out/<fork>. The driver is plain JavaScript (src/bench.mjs), nothing of
# it is compiled: a layout seed changes nothing, so there is one launcher
# per fork.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
COMMIT="$1"
OUT="$2"
source ../../toolchains/node.sh
# corepack runs the package manager the repository pins (pnpm today, yarn
# in older commits); its downloads and pnpm's store live with the
# toolchains, shared by every build.
export COREPACK_HOME="$BENCH_TOOLCHAINS/corepack"
export COREPACK_ENABLE_DOWNLOAD_PROMPT=0
export npm_config_store_dir="$BENCH_TOOLCHAINS/pnpm-store"
export YARN_CACHE_FOLDER="$BENCH_TOOLCHAINS/yarn-cache"
# The repository's own package scripts call pnpm (and yarn) by name:
# corepack's shims for them live with the toolchains and lead the PATH.
mkdir -p "$COREPACK_HOME/bin"
corepack enable --install-directory "$COREPACK_HOME/bin" pnpm yarn
export PATH="$COREPACK_HOME/bin:$PATH"

# checkout <repo> <dir> <rev>: a shallow fetch of one revision.
checkout() {
  local repo="$1" dir="$2" rev="$3"
  if [ ! -d "$dir/.git" ]; then
    git init -q "$dir"
    git -C "$dir" remote add origin "$repo"
  fi
  git -C "$dir" fetch -q --depth 1 origin "$rev"
  git -C "$dir" checkout -q FETCH_HEAD
}
mkdir -p work
checkout https://github.com/ChainSafe/ssz work/ssz "$COMMIT"

PAYLOAD="${REAL_DATA:-/srv/benchd/res/real}"
python3 -B ../convert_foreign.py ts ../../types/fulu/types.go src/gen_fulu.mjs --roots FuluBeaconState,ElectraSignedBeaconBlock \
  --preset mainnet="$PAYLOAD/fulu/spec.json" --preset minimal="$PAYLOAD/fulu/minimal/spec.json"
python3 -B ../convert_foreign.py ts ../../types/gloas/types.go src/gen_gloas.mjs --roots GloasBeaconState,GloasSignedBeaconBlock,GloasSignedExecutionPayloadEnvelope \
  --preset mainnet="$PAYLOAD/gloas/spec.json" --preset minimal="$PAYLOAD/gloas/minimal/spec.json"

# The monorepo's own build: ssz and the workspace packages it depends on,
# in dependency order, each with its build script.
(
  cd work/ssz
  if [ -f pnpm-lock.yaml ]; then
    corepack pnpm install --frozen-lockfile
    corepack pnpm -r --filter "@chainsafe/ssz..." --workspace-concurrency="$BENCH_JOBS" run build
  else
    corepack yarn install --frozen-lockfile
    corepack yarn workspaces foreach -Rt --from @chainsafe/ssz run build 2>/dev/null || corepack yarn build
  fi
)

NODE="$(command -v node)"
for fork in fulu gloas; do
  printf '#!/bin/sh\nexec "%s" --expose-gc --max-old-space-size=12000 "%s/src/bench.mjs" --fork %s "$@"\n' "$NODE" "$PWD" "$fork" > "$OUT/$fork"
  chmod +x "$OUT/$fork"
done
