#!/bin/bash
# Builds the Grandine SSZ adapter at a commit of grandinetech/grandine:
#
#   build.sh <commit> <out>
#
# installs or updates the Rust toolchain (toolchains/rust.sh), fetches the
# grandine workspace at the commit into work/ (the ssz crate and the
# sibling crates it needs come from there), generates the type definitions
# for both forks and presets from the harness types, builds the benchmark
# once per layout seed (lld's section shuffle on the final link, the
# dependencies compiled once) and leaves the launchers out/<fork>-<seed>
# and out/<fork>.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
COMMIT="$1"
OUT="$2"
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
checkout https://github.com/grandinetech/grandine work/grandine "$COMMIT"

PAYLOAD="${REAL_DATA:-/srv/benchd/res/real}"
python3 ../convert_foreign.py grandine ../../types/fulu/types.go src/gen_fulu.rs --roots FuluBeaconState,ElectraSignedBeaconBlock \
  --preset mainnet="$PAYLOAD/fulu/spec.json" --preset minimal="$PAYLOAD/fulu/minimal/spec.json"
python3 ../convert_foreign.py grandine ../../types/gloas/types.go src/gen_gloas.rs --roots GloasBeaconState,GloasSignedBeaconBlock,GloasSignedExecutionPayloadEnvelope \
  --preset mainnet="$PAYLOAD/gloas/spec.json" --preset minimal="$PAYLOAD/gloas/minimal/spec.json"

mkdir -p "$OUT"
export CARGO_TARGET_DIR="$PWD/target"
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
