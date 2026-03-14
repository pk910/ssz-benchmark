#!/bin/bash
# Update prysmssz to the version used by OffchainLabs/prysm
# prysmaticlabs/fastssz doesn't have releases; we track whatever version
# OffchainLabs/prysm uses in their go.mod on the develop branch.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Fetch OffchainLabs/prysm's go.mod from the develop branch
PRYSM_GOMOD=$(curl -sfL "https://raw.githubusercontent.com/OffchainLabs/prysm/develop/go.mod")

if [ -z "$PRYSM_GOMOD" ]; then
    echo "Error: Could not fetch OffchainLabs/prysm go.mod"
    exit 1
fi

# Extract the prysmaticlabs/fastssz version (skip replace directives)
FASTSSZ_VERSION=$(echo "$PRYSM_GOMOD" \
    | grep 'github.com/prysmaticlabs/fastssz ' \
    | grep -v '=>' \
    | head -1 \
    | awk '{print $2}')

if [ -z "$FASTSSZ_VERSION" ]; then
    echo "Error: Could not find prysmaticlabs/fastssz in OffchainLabs/prysm go.mod"
    exit 1
fi

echo "OffchainLabs/prysm uses prysmaticlabs/fastssz $FASTSSZ_VERSION"

# Update go.mod
sed -i -E "s|(github.com/prysmaticlabs/fastssz) v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1 ${FASTSSZ_VERSION}|g" "$SCRIPT_DIR/go.mod"

# Update generate.go
sed -i -E "s|(github.com/prysmaticlabs/fastssz/sszgen)@v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1@${FASTSSZ_VERSION}|g" "$SCRIPT_DIR/generate.go"

# Regenerate
cd "$SCRIPT_DIR"
go mod tidy
rm -f gen_*.go
go generate .

echo "prysmssz updated to $FASTSSZ_VERSION"
