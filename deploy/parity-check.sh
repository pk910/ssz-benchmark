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
HB=$(ls -dt "$DATA"/hb/*-[0-9a-f]*/ 2>/dev/null | grep -v -- '-[a-z]' | head -1)
[ -n "$HB" ] || { echo "no cached harness build under $DATA/hb"; exit 1; }
SCRATCH=$(mktemp -d "$DATA/work/parity.XXXX")
trap 'rm -rf "$SCRATCH"' EXIT
cp -r "$HB" "$SCRATCH/h"
chown -R "$USER_NAME:$USER_NAME" "$SCRATCH"
as_bench() { setpriv --reuid="$USER_NAME" --regid="$USER_NAME" --init-groups --no-new-privs -- env HOME=/home/$USER_NAME PATH=/usr/local/go/bin:$PATH GOFLAGS="-mod=mod -buildvcs=false" GOTOOLCHAIN=local GOCACHE=$DATA/work/gocache GOMODCACHE=$DATA/work/go "$@"; }
cd "$SCRATCH/h"
as_bench go build -o benchwrap ./benchwrap
as_bench go build -o adapter ./adapters/dynssz
BIN=$(ls seed-101-fulu.test)
LEAVES='^BenchmarkReal$/^Codegen$/^(FuluBlock|FuluMinState)$/^(Unmarshal|Marshal|HashTreeRoot)$'
ITERS='Unmarshal=20,Marshal=40,HashTreeRoot=4'
ENV="REAL_DATA=$DATA/res/real GOGC=off GOMAXPROCS=1 BENCH_MUTATOR_CPU=2 BENCH_ITERS=$ITERS"
run() { as_bench env $ENV setarch x86_64 -R taskset -c 2,3,4 "$@" -test.run '^$' -test.bench "$LEAVES" -test.benchmem -test.benchtime 1x 2>/dev/null | grep '^Benchmark' | sed -E 's/ +/ /g'; }
for round in 1 2; do
  echo "== kit, round $round"; run "./$BIN"
  echo "== wrapper, round $round"; run ./benchwrap -adapter ./adapter
done
