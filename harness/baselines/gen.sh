#!/bin/bash
# Regenerates the type definitions of every library adapter from the
# harness types (types/fulu, types/gloas), then its generated code at the
# pinned default version. Run it when the harness types change; the output
# is checked in next to this script and travels with the harness.
#
#   baselines/gen.sh [fastssz1|fastssz2|prysmssz|karalabessz ...]
#
# The generated code alone is produced by each adapter's generate.sh,
# which takes the library version to generate with and needs no Python:
# the benchmark machine runs it for every library commit it measures.
#
# What the external libraries can express:
#   prysmssz (methodical-ssz, the generator Prysm uses): everything, Gloas
#     included (progressive containers and lists, vectors of containers).
#   fastssz1, fastssz2:
#     Fulu   state, block      everything
#     Gloas  block, envelope   serialization only (no progressive
#                              merkleization)
#     Gloas  state             nothing (no vector of containers)
#   karalabessz: as fastssz, without the Fulu state (its vector lengths are
#     a closed list that lacks the proposer lookahead's 64 uint64).
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TYPES="$HERE/../types"
FULU_ROOTS="FuluBeaconState ElectraSignedBeaconBlock"
FULU_BLOCK_ROOTS="ElectraSignedBeaconBlock"
GLOAS_ROOTS="GloasSignedBeaconBlock GloasSignedExecutionPayloadEnvelope"
GLOAS_ALL_ROOTS="GloasBeaconState GloasSignedBeaconBlock GloasSignedExecutionPayloadEnvelope"

convert() { # lib dialect fork roots
  local dir="$HERE/$1/$3"
  mkdir -p "$dir"
  python3 "$HERE/convert_types.py" "$TYPES/$3/types.go" "$2" "$dir/types.go" $4
  sed -i "s/^package .*/package $3/" "$dir/types.go"
  gofmt -w "$dir/types.go"
}

fastssz_types() { # lib
  convert "$1" ferranbt fulu "$FULU_ROOTS"
  convert "$1" ferranbt gloas "$GLOAS_ROOTS"
}

karalabe_types() {
  convert karalabessz karalabe fulu "$FULU_BLOCK_ROOTS"
  convert karalabessz karalabe gloas "$GLOAS_ROOTS"
  # The generator takes one type per run and needs the nested types'
  # methods to exist already: the order is written down for generate.sh.
  for fork in fulu gloas; do
    python3 "$HERE/struct_order.py" "$HERE/karalabessz/$fork/types.go" > "$HERE/karalabessz/$fork/order.txt"
  done
}

# methodical-ssz reads the progressive containers and collections from a
# yaml config, written next to the types.
methodical_types() {
  YAML_OUT="$HERE/prysmssz/fulu.yaml" YAML_PKG="baseline/prysmssz/fulu" convert prysmssz methodical fulu "$FULU_ROOTS"
  YAML_OUT="$HERE/prysmssz/gloas.yaml" YAML_PKG="baseline/prysmssz/gloas" convert prysmssz methodical gloas "$GLOAS_ALL_ROOTS"
}

for lib in "${@:-fastssz1 fastssz2 prysmssz karalabessz}"; do
  for l in $lib; do
    case "$l" in
      fastssz1|fastssz2) fastssz_types "$l" ;;
      prysmssz) methodical_types ;;
      karalabessz) karalabe_types ;;
      *) echo "unknown baseline $l"; exit 1 ;;
    esac
    bash "$HERE/$l/generate.sh"
    (cd "$HERE/$l" && go vet ./fulu ./gloas)
    echo "generated $l"
  done
done
