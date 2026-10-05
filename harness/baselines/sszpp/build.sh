#!/bin/bash
# Builds the sszpp adapter at a commit of OffchainLabs/sszpp:
#
#   build.sh <commit> <out>
#
# installs or updates CMake (toolchains/cpp.sh; the compiler is the
# system's), fetches sszpp at the commit and its dependencies at the
# revisions pinned below into work/ (hashtree for SHA-256, built with its
# own Makefile; intx for uint256, header-only), generates the Fulu type
# definitions for both presets from the harness types, builds the driver
# once per layout seed when lld is there to shuffle the sections (once
# with the system linker, which cannot), and leaves the launcher out/fulu
# (and out/fulu-<seed> per shuffled build). The Gloas types are progressive
# containers and lists, which sszpp cannot express: no gloas launcher.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
COMMIT="$1"
OUT="$2"
HASHTREE_REV=76c54d6dcf3596c03788ea5bc0ef030e9f1b43f4
INTX_REV=4c1ca55d78777ffea7ede46e70cbd46a5beef008 # v0.9.3
source ../../toolchains/cpp.sh

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
mkdir -p work build
checkout https://github.com/OffchainLabs/sszpp work/sszpp "$COMMIT"
checkout https://github.com/prysmaticlabs/hashtree work/hashtree "$HASHTREE_REV"
checkout https://github.com/chfast/intx work/intx "$INTX_REV"

PAYLOAD="${REAL_DATA:-/srv/benchd/res/real}"
python3 ../convert_foreign.py cpp ../../types/fulu/types.go src/gen_fulu.hpp --roots FuluBeaconState,ElectraSignedBeaconBlock \
  --preset mainnet="$PAYLOAD/fulu/spec.json" --preset minimal="$PAYLOAD/fulu/minimal/spec.json"

# hashtree's Makefile picks the assembly for the architecture and the
# implementation at run time; its objects land under build/hashtree.
make -s -C work/hashtree/src -j"$BENCH_JOBS" OUT_DIR="$PWD/build/hashtree" "$PWD/build/hashtree/lib/libhashtree.a"

# build <link flags> <binary>: configures (a relink when only the flags
# changed) and builds the driver into out, with the system's g++ (gcc 14
# on the box) whatever CXX says.
build() {
  cmake -S . -B build -DCMAKE_BUILD_TYPE=Release -DCMAKE_CXX_COMPILER=g++ -DCMAKE_EXE_LINKER_FLAGS="$1" >/dev/null
  cmake --build build --parallel "$BENCH_JOBS" >/dev/null
  cp build/bench "$2"
}
mkdir -p "$OUT"
IFS=',' read -r -a SEEDS <<< "${BENCH_SEEDS:-101}"
if command -v ld.lld >/dev/null 2>&1; then
  for seed in "${SEEDS[@]}"; do
    build "-fuse-ld=lld -Wl,--shuffle-sections=*=$seed" "$OUT/bench-$seed"
    printf '#!/bin/sh\nexec "%s/bench-%s" --fork fulu "$@"\n' "$OUT" "$seed" > "$OUT/fulu-$seed"
    chmod +x "$OUT/fulu-$seed"
  done
  ln -sf "fulu-${SEEDS[0]}" "$OUT/fulu"
else
  build "" "$OUT/bench"
  printf '#!/bin/sh\nexec "%s/bench" --fork fulu "$@"\n' "$OUT" > "$OUT/fulu"
  chmod +x "$OUT/fulu"
fi
