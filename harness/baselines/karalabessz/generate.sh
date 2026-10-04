#!/bin/bash
# Generates the SSZ code of this adapter with the karalabe/ssz generator of
# the given library version (default: the pinned one) and points the module
# at that version.
#
#   generate.sh [version]
#
# The generator needs the Go 1.23 toolchain (its x/tools is older than the
# current compiler's export format) and so cannot load a module that
# requires the kit; it runs in a scratch module holding only the types.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
MODULE=github.com/karalabe/ssz
VERSION="${1:-v0.3.0}"
export GOFLAGS=-mod=mod
go get "$MODULE@$VERSION"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf 'module baseline/karalabessz\n\ngo 1.23.4\n\nrequire (\n\tgithub.com/holiman/uint256 v1.3.2\n\tgithub.com/prysmaticlabs/go-bitfield v0.0.0-20240618144021-706c95b2dd15\n)\n' > "$tmp/go.mod"
(cd "$tmp" && GOTOOLCHAIN=go1.23.4 go get "$MODULE@$VERSION" >/dev/null 2>&1)
for fork in fulu gloas; do
  rm -f "$fork"/gen_*_ssz.go
  mkdir -p "$tmp/$fork" && cp "$fork/types.go" "$tmp/$fork/"
  (cd "$tmp" && GOTOOLCHAIN=go1.23.4 go mod tidy >/dev/null 2>&1)
  # one type per run, dependencies first (order.txt)
  while read -r t; do
    (cd "$tmp/$fork" && GOTOOLCHAIN=go1.23.4 go run "$MODULE/cmd/sszgen@$VERSION" -type "$t" -out "gen_${t}_ssz.go" 2>&1 | grep -v "source-processing\|Generating" || true)
  done < "$fork/order.txt"
  cp "$tmp/$fork"/gen_*_ssz.go "$fork/"
done
go mod tidy
