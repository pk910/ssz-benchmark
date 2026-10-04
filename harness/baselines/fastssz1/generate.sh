#!/bin/bash
# Generates the SSZ code of this adapter with the fastssz generator of the
# given library version (default: the pinned one) and points the module at
# that version.
#
#   generate.sh [version]
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
MODULE=github.com/ferranbt/fastssz
VERSION="${1:-v1.0.0}"
export GOFLAGS=-mod=mod
go get "$MODULE@$VERSION"
for fork in fulu gloas; do
  objs=$(grep -oE "^type (\w+) struct" "$fork/types.go" | awk '{print $2}' | paste -sd,)
  rm -f "$fork/encoding_gen.go"
  go run "$MODULE/sszgen@$VERSION" --output "$fork/encoding_gen.go" --path "$fork/types.go" --objs "$objs"
done
go mod tidy
