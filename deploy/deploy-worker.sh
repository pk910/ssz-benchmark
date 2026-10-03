#!/bin/bash
# Sets up a worker machine for the controller: same box tuning as the
# controller, the payload copied from it, the harness synced, the worker
# unit installed.
#
#   deploy-worker.sh root@<worker> [root@<controller>]
#
# Before: give the worker the same kernel command line as the controller
# (isolcpus=nohz,managed_irq,2-7 nohz_full=2-7 rcu_nocbs=2-7
# transparent_hugepage=madvise), CPUAffinity=0 1 in
# /etc/systemd/system.conf, Go in /usr/local/go, and reboot.
set -euo pipefail
WORKER="$1"
CONTROLLER="${2:-${BENCH_BOX:?set BENCH_BOX (root@<controller>) or pass it as the second argument}}"
CONTROLLER_HOST="${CONTROLLER#*@}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC="$HERE/../benchd"
HARNESS="$HERE/../harness"
OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT
TOKEN="$(ssh "$CONTROLLER" "grep ^RUNNER_TOKEN= /srv/benchd/env | cut -d= -f2-")"
[ -n "$TOKEN" ] || { echo "no RUNNER_TOKEN on the controller"; exit 1; }
(cd "$SRC" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$OUT/benchd" .)
scp -q "$OUT/benchd" "$HERE/benchrunner.service" "$HERE/bench-irq-affinity.service" "$HERE/bench-irq-affinity" "$WORKER:/tmp/"
ssh "$WORKER" 'mkdir -p /srv/benchd/res /srv/benchd/harness'
rsync -a --delete --exclude 'gen_ssz.go' --exclude 'payload' --exclude 'dynssz-gen' --exclude '*.test' "$HARNESS/" "$WORKER:/srv/benchd/harness/"
# The payload goes controller -> worker directly (680 MB).
ssh "$CONTROLLER" "tar -C /srv/benchd/res -cf - real" | ssh "$WORKER" "tar -C /srv/benchd/res -xf -"
ssh "$WORKER" "set -e
  apt-get install -y -q git rsync curl util-linux >/dev/null 2>&1 || true
  install -m 755 /tmp/benchd /srv/benchd/benchd
  install -m 755 /tmp/bench-irq-affinity /usr/local/sbin/bench-irq-affinity
  install -m 644 /tmp/benchrunner.service /etc/systemd/system/benchrunner.service
  install -m 644 /tmp/bench-irq-affinity.service /etc/systemd/system/bench-irq-affinity.service
  printf 'RUNNER_TOKEN=%s\nCONTROLLER=http://%s\n' '$TOKEN' '$CONTROLLER_HOST' > /srv/benchd/env
  chmod 600 /srv/benchd/env
  systemctl daemon-reload
  systemctl enable --now bench-irq-affinity >/dev/null 2>&1 || true
  systemctl enable --now benchrunner
  sleep 2
  systemctl --no-pager --lines=3 status benchrunner | grep -E 'Active|benchd'"
