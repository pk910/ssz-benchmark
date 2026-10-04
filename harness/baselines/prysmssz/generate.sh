#!/bin/bash
# Generates the SSZ code of this adapter with the methodical-ssz generator
# of the given library version (default: the pinned one, which is what the
# latest Prysm release uses) and points the module at that version.
#
#   generate.sh [version]
#
# The generator loads the package with the Go toolchain it was built for,
# in a scratch module holding only the types and their yaml config.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
MODULE=github.com/OffchainLabs/methodical-ssz
VERSION="${1:-v0.0.0-20260703104215-9be4f5c6a334}"
export GOFLAGS=-mod=mod
go get "$MODULE@$VERSION"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf 'module baseline/prysmssz\n\ngo 1.25.1\n\nrequire github.com/OffchainLabs/go-bitfield v0.0.0-20260504143531-5cbb6d0f5f2e\n' > "$tmp/go.mod"
(cd "$tmp" && GOTOOLCHAIN=go1.25.1 go get "$MODULE@$VERSION" >/dev/null 2>&1)
for fork in fulu gloas; do
  rm -f "$fork/encoding_gen.go"
  mkdir -p "$tmp/$fork" && cp "$fork/types.go" "$fork/deps.go" "$tmp/$fork/" && cp "$fork.yaml" "$tmp/"
  (cd "$tmp" && export GOTOOLCHAIN=go1.25.1 && go mod tidy >/dev/null 2>&1 && go run "$MODULE/cmd/ssz@$VERSION" gen --config="$fork.yaml" --output="$fork/encoding_gen.go" 2>&1 | grep -v "level=info\|^Parsing\|^Rendering\|^Generating" || true)
  cp "$tmp/$fork/encoding_gen.go" "$fork/"
done
go mod tidy
