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
# The companion crates (the collection types and the hashing) at revisions
# that fit the library: the current ones need the progressive types, an
# ethereum_ssz without them (0.10.x up to the release v0.10.4) builds with
# the last revisions before, ssz_types v0.14.1 and tree_hash 0.12.1.
SSZ_TYPES_REV=8955d22edc7633f26fe3b5ac933facb5121d717a
TREE_HASH_REV=e5ea1875edddf12438506c3751cbb313f9c52d08
SSZ_TYPES_REV_PRE=9d3aef1e4ed9ab5f5daf63b04005c7bfdf6dc8fe
TREE_HASH_REV_PRE=adef12897ee1a35cc15db94f907973b2d1626f19
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
if ! grep -rqs 'Progressive' --include=lib.rs work/ethereum_ssz; then
  echo "ethereum_ssz at this commit has no progressive types: companions before them" >&2
  SSZ_TYPES_REV="$SSZ_TYPES_REV_PRE"
  TREE_HASH_REV="$TREE_HASH_REV_PRE"
fi
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
# The Gloas types need progressive and stable containers, which the
# library has only from some version on: the build tries them, and a
# version without them ships the Fulu fork alone (the runner measures the
# forks that have a launcher).
FEATURES="--features gloas"
FORKS="fulu gloas"
mkdir -p "$CARGO_TARGET_DIR"
if ! cargo build --release --quiet --bin bench --features gloas 2> "$CARGO_TARGET_DIR/gloas-probe.log"; then
  echo "gloas: the library at this commit cannot build the Gloas types ($(grep -m1 '^error' "$CARGO_TARGET_DIR/gloas-probe.log" | cut -c1-120)); Fulu only" >&2
  FEATURES=""
  FORKS="fulu"
fi
for seed in "${SEEDS[@]}"; do
  # shellcheck disable=SC2086
  cargo rustc --release --quiet --bin bench $FEATURES -- -C link-arg="-Wl,--shuffle-sections=*=$seed"
  cp target/release/bench "$OUT/bench-$seed"
  for fork in $FORKS; do
    printf '#!/bin/sh\nexec "%s/bench-%s" --fork %s "$@"\n' "$OUT" "$seed" "$fork" > "$OUT/$fork-$seed"
    chmod +x "$OUT/$fork-$seed"
  done
done
rm -f "$OUT/gloas" "$OUT"/gloas-*
for fork in $FORKS; do
  ln -sf "$fork-${SEEDS[0]}" "$OUT/$fork"
done
