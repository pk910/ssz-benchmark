#!/bin/bash
# Commit the freshly generated results/ to the orphan `benchmark-results` branch,
# regenerate the SVGs, and force-push. Runs on the GitHub runner after the
# benchmarks have been processed into results/*.json.
#
# Includes a data-loss guard: if the new result set has dropped to less than half
# of what is already on the branch, it aborts rather than overwriting good data.
#
# Requires: git (with push credentials already configured, e.g. by
# actions/checkout), python3, and node (for the SVG generators).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$ROOT_DIR"

count_results() {
    # Sum the number of benchmark entries across all non-aggregation JSON files
    # in the given directory. Prints 0 if the directory is missing.
    python3 - "$1" <<'PY'
import json, os, sys
d = sys.argv[1]
total = 0
if os.path.isdir(d):
    for f in os.listdir(d):
        if f.endswith('.json') and 'aggregation' not in f:
            with open(os.path.join(d, f)) as fh:
                total += len(json.load(fh).get('benchmarks', []))
print(total)
PY
}

git config --local user.email "github-actions[bot]@users.noreply.github.com"
git config --local user.name "github-actions[bot]"

# Stash the freshly generated results before we switch branches.
rm -rf /tmp/benchmark-results
cp -r results /tmp/benchmark-results

git fetch origin

if git ls-remote --heads origin benchmark-results | grep -q benchmark-results; then
    # Safety check: compare new result count against existing to prevent data loss.
    NEW_COUNT="$(count_results /tmp/benchmark-results)"
    rm -rf /tmp/old-results
    mkdir -p /tmp/old-results
    git archive origin/benchmark-results -- results/ 2>/dev/null \
        | tar -x -C /tmp/old-results --strip-components=1 2>/dev/null || true
    OLD_COUNT="$(count_results /tmp/old-results)"
    rm -rf /tmp/old-results
    echo "Result count: new=$NEW_COUNT, existing=$OLD_COUNT"
    if [ "$OLD_COUNT" -gt 10 ] && [ "$NEW_COUNT" -lt $((OLD_COUNT / 2)) ]; then
        echo "ERROR: New results ($NEW_COUNT) are less than half of existing ($OLD_COUNT). Aborting to prevent data loss." >&2
        exit 1
    fi
    git checkout -B benchmark-results origin/benchmark-results
else
    git checkout --orphan benchmark-results
    git rm -rf . || true
fi

# Restore the generated results onto the branch.
rm -rf results
cp -r /tmp/benchmark-results results
git add results/

if git diff --staged --quiet; then
    echo "No changes to commit"
    exit 0
fi

# Rebuild the SVGs and include them in the commit.
./svg-gen/generate-svg-charts.js
./svg-gen/generate-svg-table.js
git add ./*.svg

if git log --oneline -1 2>/dev/null | grep -q .; then
    git commit --amend -m "Update benchmark results"
else
    git commit -m "Update benchmark results"
fi

git push --force origin benchmark-results
