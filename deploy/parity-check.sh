#!/bin/bash
# Checks benchwrap against the kit on the benchmark box: the same leaves of
# dynamic-ssz measured by the kit (the Go test binary) and through the
# wrapper with the Go adapter speaking its protocol (harness/adapters/dynssz)
# must agree within the noise floor on time, cycles, instructions, bytes,
# allocations and page faults. Run it on the box as root between two jobs
# (stop benchd first: a running job shares the benchmark cpus):
#
#   systemctl stop benchd && deploy/parity-check.sh && systemctl start benchd
#
# It builds in a scratch copy of the deployed harness against the newest
# cached harness build's checkout and runs on cpu 3, as the sandbox user.
set -euo pipefail
DATA=/srv/benchd
USER_NAME="${SANDBOX_USER:-bench}"
# The newest build of the harness itself: <harness hash>-<40-digit commit>
# (a library adapter's build carries the library's name in between).
HB=""
for d in $(ls -dt "$DATA"/hb/*/ 2>/dev/null | grep -E '/[0-9a-f]{16}-[0-9a-f]{40}/$'); do
  if [ -f "$d/seed-101-fulu.test" ]; then HB="$d"; break; fi
done
[ -n "$HB" ] || { echo "no complete harness build under $DATA/hb"; exit 1; }
echo "harness build $HB"
SCRATCH=$(mktemp -d "$DATA/work/parity.XXXX")
trap 'rm -rf "$SCRATCH"' EXIT
cp -r "$HB" "$SCRATCH/h"
chown -R "$USER_NAME:$USER_NAME" "$SCRATCH"
as_bench() { setpriv --reuid="$USER_NAME" --regid="$USER_NAME" --init-groups --no-new-privs -- env HOME=/home/$USER_NAME PATH=/usr/local/go/bin:$PATH GOFLAGS="-mod=mod -buildvcs=false" GOTOOLCHAIN=local GOCACHE=$DATA/work/gocache GOMODCACHE=$DATA/work/go "$@"; }
cd "$SCRATCH/h"
as_bench go build -o wrap ./benchwrap
as_bench go build -o adapter ./adapters/dynssz
BIN=$(ls seed-101-fulu.test)
# The leaves, the counts and the rounds can be overridden from the
# environment (PARITY_LEAVES, PARITY_ITERS, PARITY_ROUNDS).
LEAVES="${PARITY_LEAVES:-^BenchmarkReal$/^Codegen$/^(FuluBlock|FuluMinState)$/^(Unmarshal|Marshal|HashTreeRoot)$}"
ITERS="${PARITY_ITERS:-Unmarshal=20,Marshal=40,HashTreeRoot=4}"
ROUNDS="${PARITY_ROUNDS:-2}"
ENV="REAL_DATA=$DATA/res/real GOGC=off GOMAXPROCS=1 BENCH_MUTATOR_CPU=2 BENCH_ITERS=$ITERS"
run() { as_bench env $ENV setarch x86_64 -R taskset -c 2,3,4 "$@" -test.run '^$' -test.bench "$LEAVES" -test.benchmem -test.benchtime 1x 2>/dev/null | grep '^Benchmark' | sed -E 's/ +/ /g'; }
for round in $(seq "$ROUNDS"); do
  echo "== kit, round $round"; run "./$BIN"
  echo "== wrapper, round $round"; run ./wrap -adapter ./adapter
done
