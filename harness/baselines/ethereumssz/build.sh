#!/bin/bash
# Builds the ethereum_ssz adapter at a commit of sigp/ethereum_ssz:
#
#   build.sh <commit> <out>
#
# installs or updates the Rust toolchain (toolchains/rust.sh), fetches
# ethereum_ssz at the commit and ssz_types and tree_hash at the revisions
# pinned below into work/, generates the type definitions for both forks
# and presets from the harness types, builds the benchmark once per layout
# seed (lld's section shuffle on the final link, the dependencies compiled
# once) and leaves the launchers out/<fork>-<seed> and out/<fork>.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
COMMIT="$1"
OUT="$2"
# The revisions of ssz_types and tree_hash Lighthouse builds with
# (progressive lists for the Gloas types).
SSZ_TYPES_REV=8955d22edc7633f26fe3b5ac933facb5121d717a
TREE_HASH_REV=e5ea1875edddf12438506c3751cbb313f9c52d08
source ../../toolchains/rust.sh

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
checkout https://github.com/sigp/ethereum_ssz work/ethereum_ssz "$COMMIT"
checkout https://github.com/sigp/ssz_types work/ssz_types "$SSZ_TYPES_REV"
checkout https://github.com/sigp/tree_hash work/tree_hash "$TREE_HASH_REV"

PAYLOAD="${REAL_DATA:-/srv/benchd/res/real}"
python3 ../convert_foreign.py rust ../../types/fulu/types.go src/gen_fulu.rs --roots FuluBeaconState,ElectraSignedBeaconBlock \
  --preset mainnet="$PAYLOAD/fulu/spec.json" --preset minimal="$PAYLOAD/fulu/minimal/spec.json"
python3 ../convert_foreign.py rust ../../types/gloas/types.go src/gen_gloas.rs --roots GloasBeaconState,GloasSignedBeaconBlock,GloasSignedExecutionPayloadEnvelope \
  --preset mainnet="$PAYLOAD/gloas/spec.json" --preset minimal="$PAYLOAD/gloas/minimal/spec.json"

export CARGO_TARGET_DIR="$PWD/target"
mkdir -p "$OUT"
IFS=',' read -r -a SEEDS <<< "${BENCH_SEEDS:-101}"
for seed in "${SEEDS[@]}"; do
  cargo rustc --release --quiet --bin bench -- -C link-arg="-Wl,--shuffle-sections=*=$seed"
  cp target/release/bench "$OUT/bench-$seed"
  for fork in fulu gloas; do
    printf '#!/bin/sh\nexec "%s/bench-%s" --fork %s "$@"\n' "$OUT" "$seed" "$fork" > "$OUT/$fork-$seed"
    chmod +x "$OUT/$fork-$seed"
  done
done
for fork in fulu gloas; do
  ln -sf "$fork-${SEEDS[0]}" "$OUT/$fork"
done
