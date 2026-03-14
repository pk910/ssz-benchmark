#!/bin/bash
# Update karalabe/ssz to the latest release

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="karalabe/ssz"

# Get the latest semver tag
LATEST_TAG=$(git ls-remote --tags "https://github.com/$REPO.git" 'v*' \
    | grep -v '\^{}' \
    | awk '{print $2}' \
    | sed 's|refs/tags/||' \
    | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
    | sort -V \
    | tail -1)

if [ -z "$LATEST_TAG" ]; then
    echo "Error: No release tags found for $REPO"
    exit 1
fi

echo "Latest release: $LATEST_TAG"

# Update go.mod
sed -i -E "s|(github.com/karalabe/ssz) v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1 ${LATEST_TAG}|g" "$SCRIPT_DIR/go.mod"

# Update generate.sh
sed -i -E "s|(github.com/karalabe/ssz/cmd/sszgen)@v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1@${LATEST_TAG}|g" "$SCRIPT_DIR/generate.sh"

# Regenerate
cd "$SCRIPT_DIR"
go mod tidy
rm -f gen_*.go
./generate.sh

echo "karalabessz updated to $LATEST_TAG"
