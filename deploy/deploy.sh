#!/bin/bash
# Builds benchd and installs it on the benchmark box.
#
#   deploy.sh [all|ui|daemon] [box]
#
# ui      restarts only benchweb (the web UI and API): a running job is not
#         interrupted.
# daemon  syncs the harness and restarts benchd (the job runner): the running
#         job is queued again and restarts; the web service keeps serving.
# all     both (default).
set -euo pipefail
TARGET="${1:-all}"
BOX="${2:-${BENCH_BOX:?set BENCH_BOX (root@<benchmark machine>) or pass it as the second argument}}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC="$HERE/../benchd"
HARNESS="$HERE/../harness"
OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT
(cd "$SRC" && go vet ./... && go test ./... >/dev/null && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$OUT/benchd" .)
scp -q "$OUT/benchd" "$HERE/benchd.service" "$HERE/benchweb.service" "$HERE/bench-irq-affinity.service" "$HERE/bench-irq-affinity" "$BOX:/tmp/"
if [ "$TARGET" != ui ]; then
  rsync -a --delete --exclude 'gen_ssz.go' --exclude 'payload' --exclude 'dynssz-gen' --exclude '*.test' \
    --exclude 'baselines/*/work' --exclude 'baselines/*/target' --exclude 'baselines/*/node_modules' --exclude 'baselines/*/build' \
    --exclude 'baselines/*/.gradle' --exclude 'baselines/*/out' --exclude 'baselines/*/nimcache' --exclude 'baselines/*/Cargo.lock' \
    "$HARNESS/" "$BOX:/srv/benchd/harness/"
fi
ssh "$BOX" "set -e
  mkdir -p /srv/benchd
  install -m 755 /tmp/bench-irq-affinity /usr/local/sbin/bench-irq-affinity
  install -m 644 /tmp/benchd.service /etc/systemd/system/benchd.service
  install -m 644 /tmp/benchweb.service /etc/systemd/system/benchweb.service
  install -m 644 /tmp/bench-irq-affinity.service /etc/systemd/system/bench-irq-affinity.service
  systemctl daemon-reload
  systemctl enable bench-irq-affinity benchd benchweb >/dev/null 2>&1
  systemctl start bench-irq-affinity || true
  if [ '$TARGET' != ui ]; then
    systemctl stop benchd
    install -m 755 /tmp/benchd /srv/benchd/benchd
    systemctl start benchd
  fi
  if [ '$TARGET' != daemon ]; then
    install -m 755 /tmp/benchd /srv/benchd/benchweb
    systemctl restart benchweb
  fi
  rm -f /tmp/benchd /tmp/benchd.service /tmp/benchweb.service /tmp/bench-irq-affinity.service /tmp/bench-irq-affinity
  sleep 2
  systemctl --no-pager --lines=0 status benchd benchweb | grep -E 'service|Active'"
