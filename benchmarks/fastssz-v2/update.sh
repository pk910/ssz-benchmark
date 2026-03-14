#!/bin/bash
# Update fastssz-v2 to the latest v2.x.y release
# Since the v2 module path isn't set up properly in the repo, we grab the
# commit hash of the latest v2 release tag and use it as a pseudo-version.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="github.com/ferranbt/fastssz"
GIT_URL="https://${REPO}.git"

# Find the latest v2.x.y tag (exclude pre-releases and annotated tag refs)
LATEST_TAG=$(git ls-remote --tags "$GIT_URL" 'v2.*' \
    | grep -v '\^{}' \
    | awk '{print $2}' \
    | sed 's|refs/tags/||' \
    | grep -E '^v2\.[0-9]+\.[0-9]+$' \
    | sort -V \
    | tail -1)

if [ -z "$LATEST_TAG" ]; then
    echo "Error: No v2.x.y tags found for $REPO"
    exit 1
fi

echo "Latest v2 tag: $LATEST_TAG"

# Get the commit hash for this tag (dereference annotated tags)
COMMIT_HASH=$(git ls-remote --tags "$GIT_URL" "refs/tags/${LATEST_TAG}^{}" | awk '{print $1}')
if [ -z "$COMMIT_HASH" ]; then
    # Lightweight tag, use direct hash
    COMMIT_HASH=$(git ls-remote --tags "$GIT_URL" "refs/tags/${LATEST_TAG}" | grep -v '\^{}' | awk '{print $1}')
fi

if [ -z "$COMMIT_HASH" ]; then
    echo "Error: Could not resolve commit hash for tag $LATEST_TAG"
    exit 1
fi

SHORT_HASH="${COMMIT_HASH:0:12}"

# Get the commit date via shallow clone at the tag
TMP_DIR=$(mktemp -d)
git clone --depth 1 --bare "$GIT_URL" --branch "$LATEST_TAG" "$TMP_DIR/repo" 2>/dev/null
COMMIT_DATE=$(TZ=UTC git -C "$TMP_DIR/repo" log -1 --format='%cd' --date=format-local:'%Y%m%d%H%M%S' 2>/dev/null)
rm -rf "$TMP_DIR"

if [ -z "$COMMIT_DATE" ]; then
    echo "Error: Could not get commit date for tag $LATEST_TAG"
    exit 1
fi

PSEUDO_VERSION="v0.0.0-${COMMIT_DATE}-${SHORT_HASH}"
echo "Pseudo-version: $PSEUDO_VERSION"

# Update go.mod
sed -i -E "s|(github.com/ferranbt/fastssz) v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1 ${PSEUDO_VERSION}|g" "$SCRIPT_DIR/go.mod"

# Update generate.go
sed -i -E "s|(github.com/ferranbt/fastssz/sszgen)@v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1@${PSEUDO_VERSION}|g" "$SCRIPT_DIR/generate.go"

# Regenerate
cd "$SCRIPT_DIR"
go mod tidy
rm -f gen_*.go
go generate .

echo "fastssz-v2 updated to $PSEUDO_VERSION (tag: $LATEST_TAG)"
