#!/bin/bash
# Update zrnt and ztyp to their latest releases

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Get latest zrnt tag
ZRNT_TAG=$(git ls-remote --tags "https://github.com/protolambda/zrnt.git" 'v*' \
    | grep -v '\^{}' \
    | awk '{print $2}' \
    | sed 's|refs/tags/||' \
    | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
    | sort -V \
    | tail -1)

if [ -z "$ZRNT_TAG" ]; then
    echo "Error: No release tags found for protolambda/zrnt"
    exit 1
fi

# Get latest ztyp tag
ZTYP_TAG=$(git ls-remote --tags "https://github.com/protolambda/ztyp.git" 'v*' \
    | grep -v '\^{}' \
    | awk '{print $2}' \
    | sed 's|refs/tags/||' \
    | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
    | sort -V \
    | tail -1)

if [ -z "$ZTYP_TAG" ]; then
    echo "Error: No release tags found for protolambda/ztyp"
    exit 1
fi

echo "Latest zrnt: $ZRNT_TAG, ztyp: $ZTYP_TAG"

# Update go.mod
sed -i -E "s|(github.com/protolambda/zrnt) v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1 ${ZRNT_TAG}|g" "$SCRIPT_DIR/go.mod"
sed -i -E "s|(github.com/protolambda/ztyp) v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1 ${ZTYP_TAG}|g" "$SCRIPT_DIR/go.mod"

# Tidy
cd "$SCRIPT_DIR"
go mod tidy

echo "ztyp updated to zrnt=$ZRNT_TAG, ztyp=$ZTYP_TAG"
