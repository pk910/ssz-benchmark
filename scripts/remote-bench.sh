#!/bin/bash
# Runs ON the ephemeral Hetzner box. Installs Go, runs the benchmark suite for
# both the stable and dev library versions, and collects everything the runner
# needs into ./bench-output:
#
#   bench-output/stable/<lib>_results.txt   - go test output, stable versions
#   bench-output/dev/<lib>_results.txt      - go test output, dev versions
#   bench-output/dev/gomod/<lib>.mod        - go.mod after dev update (for version extraction)
#
# Optional env:
#   GO_VERSION   - Go toolchain to install (default 1.25.0)
#   BENCH_COUNT  - -count for go test (default 10)
#   BENCH_CPUS   - taskset CPU list for go test. Defaults to a single core
#                  (best stability; see run-benchmarks.sh / the pinning note).
#   SKIP_DEV     - set to 1 to run only the stable phase.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$ROOT_DIR"

GO_VERSION="${GO_VERSION:-1.25.0}"
export BENCH_COUNT="${BENCH_COUNT:-10}"
SKIP_DEV="${SKIP_DEV:-0}"

# Pin the benchmark to a single dedicated core (core 1), leaving every other
# core to the OS/background. Measured on ccx23: single-core pinning gives ~3%
# median intra-run spread vs ~9% when pinned to two cores -- with two cores the
# benchmark goroutine migrates between them and keeps hitting cold caches, which
# adds more jitter than the GC/runtime offload removes. The spare cores still
# absorb OS noise; they just don't run the hot benchmark goroutine.
if [ -z "${BENCH_CPUS:-}" ]; then
    if [ "$(nproc)" -ge 2 ]; then
        BENCH_CPUS="1"
    else
        BENCH_CPUS=""
    fi
fi
export BENCH_CPUS
# Let the Go toolchain auto-download the exact versions go.mod files ask for
# (notably karalabe/ssz needs go1.23.4 for code generation).
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
# Some libraries (dynamic-ssz via hashtree-bindings) use CGO for their optimized
# C SHA-256 hasher. Without a C compiler the build silently falls back to a much
# slower pure-Go hasher (~8x slower HashTreeRoot), so a C toolchain is required.
export CGO_ENABLED=1
export DEBIAN_FRONTEND=noninteractive

apt_wait() {
    # Cloud images run unattended-upgrades on first boot; wait for the lock.
    local i=0
    while fuser /var/lib/dpkg/lock-frontend >/dev/null 2>&1 \
        || fuser /var/lib/apt/lists/lock >/dev/null 2>&1; do
        echo "   waiting for apt/dpkg lock..."
        sleep 3
        i=$((i + 1))
        [ "$i" -ge 60 ] && break
    done
}

echo "=== Installing prerequisites (git, curl, C toolchain) ==="
if ! command -v git >/dev/null 2>&1 || ! command -v curl >/dev/null 2>&1 \
    || ! command -v gcc >/dev/null 2>&1; then
    apt_wait
    apt-get update -qq >/dev/null
    apt-get install -y -qq git curl ca-certificates build-essential >/dev/null
fi

echo "=== Installing Go ${GO_VERSION} ==="
if [ ! -x /usr/local/go/bin/go ] \
    || ! /usr/local/go/bin/go version | grep -q "go${GO_VERSION} "; then
    go_tarball="go${GO_VERSION}.linux-amd64.tar.gz"
    curl -fsSL "https://go.dev/dl/${go_tarball}" -o /tmp/go.tgz
    curl -fsSL "https://go.dev/dl/${go_tarball}.sha256" -o /tmp/go.tgz.sha256
    echo "$(cat /tmp/go.tgz.sha256)  /tmp/go.tgz" | sha256sum -c -
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tgz
    rm -f /tmp/go.tgz /tmp/go.tgz.sha256
fi
export GOPATH="/root/go"
export PATH="/usr/local/go/bin:${GOPATH}/bin:${PATH}"
go version

OUT="$ROOT_DIR/bench-output"
rm -rf "$OUT"
mkdir -p "$OUT/stable" "$OUT/dev/gomod"

echo "=== Phase 1: benchmarks on stable versions ==="
"$SCRIPT_DIR/run-benchmarks.sh"
mv "$ROOT_DIR"/*_results.txt "$OUT/stable/"

# Update to dev library versions and benchmark them. This phase is best-effort:
# a dev version that fails to build/generate must not discard the stable results,
# which are the primary output. Run under `if` so its failure is non-fatal.
run_dev_phase() {
    # Called from an `if`, which disables `set -e` inside the function, so each
    # critical step returns explicitly on failure to stop the phase.
    echo "--- updating to dev versions ---"
    "$SCRIPT_DIR/update-dev-versions.sh" || return 1
    echo "--- benchmarking dev versions ---"
    "$SCRIPT_DIR/run-benchmarks.sh" || return 1
    mv "$ROOT_DIR"/*_results.txt "$OUT/dev/" || return 1
    # Snapshot the dev go.mod files so the runner can extract the pseudo-versions
    # without having to run the (Go-dependent) update step itself.
    for d in benchmarks/*/; do
        lib="$(basename "$d")"
        [ "$lib" = "common" ] && continue
        [ -f "$d/go.mod" ] && cp "$d/go.mod" "$OUT/dev/gomod/${lib}.mod"
    done
    return 0
}

if [ "$SKIP_DEV" = "1" ]; then
    echo "=== Phase 2 skipped (SKIP_DEV=1) ==="
elif run_dev_phase; then
    echo "=== Phase 2 complete ==="
else
    echo "=== WARNING: dev phase failed; keeping stable results only ==="
    rm -f "$ROOT_DIR"/*_results.txt 2>/dev/null || true
fi

echo "=== Done. bench-output contents: ==="
find "$OUT" -type f | sort
