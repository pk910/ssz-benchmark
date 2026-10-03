#!/bin/bash
# Regenerates the type definitions and the generated code of every baseline
# library from the harness types (types/fulu, types/gloas). Run it when the
# harness types or a pinned library version change; the output is checked
# in next to this script and travels with the harness.
#
#   baselines/gen.sh [fastssz1|fastssz2|prysmssz|karalabessz ...]
#
# What the external libraries can express:
#   prysmssz (methodical-ssz, the generator Prysm uses): everything, Gloas
#     included (progressive containers and lists, vectors of containers).
#   fastssz1, fastssz2, karalabessz:
#     Fulu   state, block      everything
#     Gloas  block, envelope   serialization only (no progressive
#                              merkleization)
#     Gloas  state             nothing (no vector of containers)
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TYPES="$HERE/../types"
FULU_ROOTS="FuluBeaconState ElectraSignedBeaconBlock"
GLOAS_ROOTS="GloasSignedBeaconBlock GloasSignedExecutionPayloadEnvelope"
FASTSSZ1=github.com/ferranbt/fastssz/sszgen@v1.0.0
FASTSSZ2=github.com/ferranbt/fastssz/sszgen@v0.0.0-20250808103907-ac370aa5f7e4
METHODICAL=github.com/OffchainLabs/methodical-ssz
METHODICAL_VERSION=v0.0.0-20260703104215-9be4f5c6a334
GLOAS_ALL_ROOTS="GloasBeaconState GloasSignedBeaconBlock GloasSignedExecutionPayloadEnvelope"
KARALABE=github.com/karalabe/ssz
KARALABE_VERSION=v0.3.0

convert() { # lib dialect fork roots
  local dir="$HERE/$1/$3"
  mkdir -p "$dir"
  python3 "$HERE/convert_types.py" "$TYPES/$3/types.go" "$2" "$dir/types.go" $4
  sed -i "s/^package .*/package $3/" "$dir/types.go"
  gofmt -w "$dir/types.go"
}

fastssz_style() { # lib dialect generator
  for fork in fulu gloas; do
    local roots="$FULU_ROOTS"; [ "$fork" = gloas ] && roots="$GLOAS_ROOTS"
    convert "$1" "$2" "$fork" "$roots"
    local objs; objs=$(grep -oE "^type (\w+) struct" "$HERE/$1/$fork/types.go" | awk '{print $2}' | paste -sd,)
    (cd "$HERE/$1" && rm -f "$fork/encoding_gen.go" && GOFLAGS=-mod=mod go run "$3" --output "$fork/encoding_gen.go" --path "$fork/types.go" --objs "$objs")
  done
  (cd "$HERE/$1" && GOFLAGS=-mod=mod go mod tidy && go vet ./...)
}

karalabe() {
  local lib="$HERE/karalabessz"
  # The library's vector lengths are closed lists; Fulu's proposer lookahead
  # (64 uint64) is not among them, so the baseline builds against a copy
  # with that one length added.
  if [ ! -d "$lib/ssz" ]; then
    local src; src=$(cd "$lib" && GOFLAGS=-mod=mod go mod download -json "$KARALABE@$KARALABE_VERSION" | grep '"Dir"' | cut -d'"' -f4)
    cp -r "$src" "$lib/ssz" && chmod -R u+w "$lib/ssz"
    sed -i 's/^\t~\[8192\]uint64$/\t~[64]uint64 | ~[8192]uint64/' "$lib/ssz/generics.go"
    grep -q '~\[64\]uint64' "$lib/ssz/generics.go"
  fi
  # The generator needs the Go 1.23 toolchain (its x/tools is older than
  # the current compiler's export format) and so cannot load a module that
  # requires the kit; it runs in a scratch module holding only the types.
  local tmp; tmp=$(mktemp -d)
  printf 'module baseline/karalabessz\n\ngo 1.23.4\n\nrequire (\n\tgithub.com/holiman/uint256 v1.3.2\n\tgithub.com/karalabe/ssz %s\n\tgithub.com/prysmaticlabs/go-bitfield v0.0.0-20240618144021-706c95b2dd15\n)\n\nreplace github.com/karalabe/ssz => %s/ssz\n' "$KARALABE_VERSION" "$lib" > "$tmp/go.mod"
  for fork in fulu gloas; do
    local roots="$FULU_ROOTS"; [ "$fork" = gloas ] && roots="$GLOAS_ROOTS"
    convert karalabessz karalabe "$fork" "$roots"
    rm -f "$lib/$fork"/gen_*_ssz.go
    mkdir -p "$tmp/$fork" && cp "$lib/$fork/types.go" "$tmp/$fork/"
    (cd "$tmp" && GOTOOLCHAIN=go1.23.4 GOFLAGS=-mod=mod go mod tidy >/dev/null 2>&1)
    # one type per run, dependencies first: the generator needs the nested
    # types' methods to exist already
    python3 "$HERE/struct_order.py" "$tmp/$fork/types.go" | while read -r t; do
      (cd "$tmp/$fork" && GOTOOLCHAIN=go1.23.4 GOFLAGS=-mod=mod go run "$KARALABE/cmd/sszgen@$KARALABE_VERSION" -type "$t" -out "gen_${t}_ssz.go" 2>&1 | grep -v "source-processing\|Generating" || true)
    done
    cp "$tmp/$fork"/gen_*_ssz.go "$lib/$fork/"
  done
  rm -rf "$tmp"
  (cd "$lib" && GOFLAGS=-mod=mod go mod tidy && go vet ./fulu ./gloas)
}

# methodical-ssz reads the progressive containers and collections from a
# yaml config (emitted next to the types) and loads the package with the Go
# toolchain it was built for, in a scratch module like the one above.
methodical() {
  local lib="$HERE/prysmssz" tmp; tmp=$(mktemp -d)
  printf 'module baseline/prysmssz\n\ngo 1.25.1\n\nrequire (\n\tgithub.com/OffchainLabs/go-bitfield v0.0.0-20260504143531-5cbb6d0f5f2e\n\t%s %s\n)\n' "$METHODICAL" "$METHODICAL_VERSION" > "$tmp/go.mod"
  for fork in fulu gloas; do
    local roots="$FULU_ROOTS"; [ "$fork" = gloas ] && roots="$GLOAS_ALL_ROOTS"
    YAML_OUT="$tmp/$fork.yaml" YAML_PKG="baseline/prysmssz/$fork" convert prysmssz methodical "$fork" "$roots"
    rm -f "$lib/$fork/encoding_gen.go"
    mkdir -p "$tmp/$fork" && cp "$lib/$fork/types.go" "$lib/$fork/deps.go" "$tmp/$fork/"
    (cd "$tmp" && export GOTOOLCHAIN=go1.25.1 GOFLAGS=-mod=mod && go mod tidy >/dev/null 2>&1 && go run "$METHODICAL/cmd/ssz@$METHODICAL_VERSION" gen --config="$fork.yaml" --output="$fork/encoding_gen.go" 2>&1 | grep -v "level=info\|^Parsing\|^Rendering\|^Generating" || true)
    cp "$tmp/$fork/encoding_gen.go" "$lib/$fork/"
  done
  rm -rf "$tmp"
  (cd "$lib" && GOFLAGS=-mod=mod go mod tidy && go vet ./fulu ./gloas)
}

export KARALABE KARALABE_VERSION
for lib in "${@:-fastssz1 fastssz2 prysmssz karalabessz}"; do
  for l in $lib; do
    case "$l" in
      fastssz1) fastssz_style fastssz1 ferranbt "$FASTSSZ1" ;;
      fastssz2) fastssz_style fastssz2 ferranbt "$FASTSSZ2" ;;
      prysmssz) methodical ;;
      karalabessz) karalabe ;;
      *) echo "unknown baseline $l"; exit 1 ;;
    esac
    echo "generated $l"
  done
done
