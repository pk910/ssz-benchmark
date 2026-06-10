#!/bin/bash
# Shared script to run all SSZ benchmarks
# Writes results to <library>_results.txt files in the root directory
# Usage: ./scripts/run-benchmarks.sh
#
# Optional env:
#   BENCH_COUNT  - value passed to `go test -count` (default 5)
#   BENCH_CPUS   - CPU list to pin `go test` to via taskset, e.g. "1".
#                  Empty (default) means no pinning. Used on dedicated hosts to
#                  reduce timing jitter. A single core is best: it keeps the
#                  benchmark goroutine cache-hot (pinning to 2+ cores lets it
#                  migrate and measured ~3x noisier on a dedicated box).

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"

BENCH_COUNT="${BENCH_COUNT:-5}"
BENCH_CPUS="${BENCH_CPUS:-}"

# Build an optional taskset prefix for pinning to dedicated cores.
RUN_PREFIX=()
if [ -n "$BENCH_CPUS" ]; then
    if command -v taskset >/dev/null 2>&1; then
        RUN_PREFIX=(taskset -c "$BENCH_CPUS")
        echo "Pinning benchmarks to CPUs: $BENCH_CPUS"
    else
        echo "WARNING: BENCH_CPUS=$BENCH_CPUS set but taskset not found; running unpinned"
    fi
fi
echo "Benchmark iterations (-count): $BENCH_COUNT"

# Libraries to benchmark, in run order.
LIBS="fastssz-v1 fastssz-v2 dynamicssz-codegen dynamicssz-reflection karalabessz prysmssz ztyp"

for lib in $LIBS; do
    echo "Running $lib benchmarks..."
    cd "benchmarks/$lib"
    go mod download
    "${RUN_PREFIX[@]}" go test -run=^$ -bench=. -benchmem -count="$BENCH_COUNT" \
        > "$ROOT_DIR/${lib}_results.txt"
    cd "$ROOT_DIR"
done

echo "All benchmarks completed!"
