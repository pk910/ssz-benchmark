#!/bin/bash
# Ship the working tree to the benchmark server, run the benchmarks there,
# and pull the results back into ./bench-output. Runs on the GitHub runner.
#
# Required env:
#   SERVER_IP      - IP of the provisioned server
# Optional env:
#   SSH_KEY_FILE   - private key (default /tmp/bench_key)
#   REMOTE_DIR     - path on the server (default /root/ssz-benchmark)
#   BENCH_COUNT    - -count for go test (passed through)
#   BENCH_CPUS     - taskset CPU list (passed through; auto-derived if empty)
#   GO_VERSION     - Go toolchain to install on the box (passed through)
set -euo pipefail

: "${SERVER_IP:?SERVER_IP is required}"
SSH_KEY_FILE="${SSH_KEY_FILE:-/tmp/bench_key}"
REMOTE_DIR="${REMOTE_DIR:-/root/ssz-benchmark}"
REMOTE="root@${SERVER_IP}"

# Keepalives keep the long-running benchmark session (~45 min) from being
# dropped, and detect a dead connection instead of hanging forever.
SSH_CMD="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
-o ConnectTimeout=15 -o ServerAliveInterval=30 -o ServerAliveCountMax=10 \
-i $SSH_KEY_FILE"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

echo ">> Syncing working tree to $REMOTE:$REMOTE_DIR"
rsync -az --delete \
    --exclude '.git' \
    --exclude 'tmp-*' \
    --exclude 'results' \
    --exclude 'bench-output' \
    --exclude 'res/generator/generator' \
    -e "$SSH_CMD" \
    ./ "$REMOTE:$REMOTE_DIR/"

echo ">> Running benchmarks on remote"
# shellcheck disable=SC2029  # we deliberately expand these values locally
$SSH_CMD "$REMOTE" \
    "cd $REMOTE_DIR && \
     BENCH_COUNT='${BENCH_COUNT:-10}' \
     BENCH_CPUS='${BENCH_CPUS:-}' \
     GO_VERSION='${GO_VERSION:-1.25.0}' \
     bash scripts/remote-bench.sh"

echo ">> Pulling results back to ./bench-output"
rm -rf bench-output
rsync -az -e "$SSH_CMD" "$REMOTE:$REMOTE_DIR/bench-output/" ./bench-output/

echo ">> Done. Contents:"
find bench-output -type f | sort
