#!/bin/bash
# Builds the nim-ssz-serialization (Nimbus) adapter at a commit of
# status-im/nim-ssz-serialization:
#
#   build.sh <commit> <out>
#
# installs or updates the Nim toolchain (toolchains/nim.sh), fetches the
# library at the commit and its dependencies at the revisions pinned below
# into work/ (shallow checkouts; the SHA-256 backends blst and hashtree
# are compiled from their sources as Nimbus does), generates the type
# definitions for both forks and presets from the harness types, builds
# the one driver binary (both forks, both presets: the sizes are
# compile-time, so each preset is its own set of types) once per layout
# seed when lld is available (its section shuffle on the final link, the
# C objects compiled once), and leaves the launchers out/<fork>-<seed> and
# out/<fork>.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
COMMIT="$1"
OUT="$2"
# The revisions of the dependencies (nim-ssz-serialization's nimble file
# names them unpinned; these build with the library's master).
STEW_REV=0e601977e5ac8766dbf39fa53b94f908b9cb4753
STINT_REV=3a6ab344351840dc713f8a4be26a9827f5fd5c3c
INTOPS_REV=6fcecf8662881bd74d5292f6b8aec676338252e5
NIMCRYPTO_REV=48f4079cf84640e85970298ca7fdf963c6966283
RESULTS_REV=a3f6185d4c2bae01967da5b04ad9e98e057ddaed
SERIALIZATION_REV=04c08d3095df8da4b3e91d912d75dae3facd21ef
JSON_SERIALIZATION_REV=326b48feb222a7198f42ee4a86fe7070249e0cdf
FASTSTREAMS_REV=5bdef9939b483435a61aa91a6422c1a9852127b4
BLSCURVE_REV=87504d0e58cb1203243c661275a5d5ac06cc6fc5
TASKPOOLS_REV=906059014ae082a7ef5b23cb7ceb95aa9b85fa0a
HASHTREE_REV=76c54d6dcf3596c03788ea5bc0ef030e9f1b43f4
source ../../toolchains/nim.sh

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
checkout https://github.com/status-im/nim-ssz-serialization work/ssz "$COMMIT"
checkout https://github.com/status-im/nim-stew work/stew "$STEW_REV"
checkout https://github.com/status-im/nim-stint work/stint "$STINT_REV"
checkout https://github.com/iftech/nim-intops work/intops "$INTOPS_REV"
checkout https://github.com/cheatfate/nimcrypto work/nimcrypto "$NIMCRYPTO_REV"
checkout https://github.com/arnetheduck/nim-results work/results "$RESULTS_REV"
checkout https://github.com/status-im/nim-serialization work/serialization "$SERIALIZATION_REV"
checkout https://github.com/status-im/nim-json-serialization work/json_serialization "$JSON_SERIALIZATION_REV"
checkout https://github.com/status-im/nim-faststreams work/faststreams "$FASTSTREAMS_REV"
checkout https://github.com/status-im/nim-blscurve work/blscurve "$BLSCURVE_REV"
checkout https://github.com/status-im/nim-taskpools work/taskpools "$TASKPOOLS_REV"
checkout https://github.com/OffchainLabs/hashtree work/hashtree "$HASHTREE_REV"
# blst is a submodule of blscurve, at the revision blscurve pins.
checkout https://github.com/supranational/blst work/blscurve/vendor/blst "$(git -C work/blscurve ls-tree HEAD vendor/blst | awk '{print $3}')"

PAYLOAD="${REAL_DATA:-/srv/benchd/res/real}"
python3 ../convert_foreign.py nim ../../types/fulu/types.go src/gen_fulu.nim --roots FuluBeaconState,ElectraSignedBeaconBlock \
  --preset mainnet="$PAYLOAD/fulu/spec.json" --preset minimal="$PAYLOAD/fulu/minimal/spec.json"
python3 ../convert_foreign.py nim ../../types/gloas/types.go src/gen_gloas.nim --roots GloasBeaconState,GloasSignedBeaconBlock,GloasSignedExecutionPayloadEnvelope \
  --preset mainnet="$PAYLOAD/gloas/spec.json" --preset minimal="$PAYLOAD/gloas/minimal/spec.json"

# The library and its dependencies by path; the SHA-256 backends are the
# library's defaults (blst for whole messages, hashtree for 64-byte
# chunks), as Nimbus builds them. Nimbus's release flags (-d:release
# --opt:speed --threads:on) without its -march=native and without LTO (the
# other adapters link without it, and gcc's LTO cannot go through the lld
# section shuffle); ORC rather than Nimbus's refc, so that freeing is
# deterministic and the driver can release results outside the timed
# window; the allocation counters of Nim's allocator, which the driver
# reads to tell an allocating operation.
NIMFLAGS=(c -d:release --opt:speed --mm:orc --threads:on -d:nimAllocStats
  --hints:off --warnings:off "--parallelBuild:${BENCH_JOBS:-2}"
  "--nimcache:$PWD/nimcache"
  --path:"$PWD/work/ssz" --path:"$PWD/work/stew" --path:"$PWD/work/stint" --path:"$PWD/work/intops/src"
  --path:"$PWD/work/nimcrypto" --path:"$PWD/work/results" --path:"$PWD/work/serialization"
  --path:"$PWD/work/json_serialization" --path:"$PWD/work/faststreams" --path:"$PWD/work/blscurve"
  --path:"$PWD/work/taskpools" --path:"$PWD/work/hashtree")

mkdir -p "$OUT"
IFS=',' read -r -a SEEDS <<< "${BENCH_SEEDS:-101}"
if command -v ld.lld >/dev/null 2>&1; then
  for seed in "${SEEDS[@]}"; do
    nim "${NIMFLAGS[@]}" --passL:-fuse-ld=lld "--passL:-Wl,--shuffle-sections=*=$seed" -o:"$OUT/bench-$seed" src/bench.nim
    for fork in fulu gloas; do
      printf '#!/bin/sh\nexec "%s/bench-%s" --fork %s "$@"\n' "$OUT" "$seed" "$fork" > "$OUT/$fork-$seed"
      chmod +x "$OUT/$fork-$seed"
    done
  done
  for fork in fulu gloas; do
    ln -sf "$fork-${SEEDS[0]}" "$OUT/$fork"
  done
else
  # No lld: one layout, the launcher out/<fork> only.
  nim "${NIMFLAGS[@]}" -o:"$OUT/bench" src/bench.nim
  for fork in fulu gloas; do
    printf '#!/bin/sh\nexec "%s/bench" --fork %s "$@"\n' "$OUT" "$fork" > "$OUT/$fork"
    chmod +x "$OUT/$fork"
  done
fi
