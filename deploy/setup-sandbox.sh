#!/bin/bash
# Prepares the benchmark VM for running builds and benchmark processes as
# an unprivileged user (benchd -sandbox-user / SANDBOX_USER):
#
#   setup-sandbox.sh [root@<box>] [user]
#
# - creates the user;
# - hands it the work directories (worktrees, harness builds, Go caches);
#   the database, the payload, job logs and credentials stay root's;
# - keeps that user off the internal networks: it may resolve names and
#   reach the internet (module downloads during a build), nothing private.
#   Benchmark processes get no network at all (the daemon unshares it).
# Idempotent. Restart benchd afterwards with SANDBOX_USER set in
# /srv/benchd/env.
set -euo pipefail
BOX="${1:-${BENCH_BOX:?set BENCH_BOX (root@<benchmark machine>) or pass it as the first argument}}"
USER_NAME="${2:-bench}"
ssh "$BOX" "set -e
  id -u $USER_NAME >/dev/null 2>&1 || useradd --system --create-home --shell /usr/sbin/nologin $USER_NAME
  mkdir -p /srv/benchd/work/wt /srv/benchd/work/hb /srv/benchd/work/go /srv/benchd/work/gocache
  chown -R $USER_NAME:$USER_NAME /srv/benchd/work/wt /srv/benchd/work/hb /srv/benchd/work/go /srv/benchd/work/gocache
  chmod 711 /srv/benchd
  chmod 600 /srv/benchd/env /srv/benchd/benchd.db* 2>/dev/null || true
  chmod -R a+rX /srv/benchd/res
  # root runs git in worktrees the sandbox user owns
  git config --global --get-all safe.directory | grep -qx '*' || git config --global --add safe.directory '*'
  apt-get install -y -q nftables >/dev/null 2>&1
  cat > /etc/nftables.d-bench.nft <<NFT
table inet bench_sandbox
delete table inet bench_sandbox
table inet bench_sandbox {
  chain output {
    type filter hook output priority 0; policy accept;
    meta skuid $USER_NAME oifname \"lo\" accept
    meta skuid $USER_NAME udp dport 53 accept
    meta skuid $USER_NAME tcp dport 53 accept
    meta skuid $USER_NAME ip daddr { 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16, 100.64.0.0/10 } drop
    meta skuid $USER_NAME ip6 daddr { fc00::/7, fe80::/10 } drop
    meta skuid $USER_NAME tcp dport != { 80, 443 } drop
    meta skuid $USER_NAME meta l4proto != tcp drop
  }
}
NFT
  nft -f /etc/nftables.d-bench.nft
  cat > /etc/systemd/system/bench-sandbox-net.service <<UNIT
[Unit]
Description=Network limits of the benchmark sandbox user
Before=benchd.service
After=network-pre.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/sbin/nft -f /etc/nftables.d-bench.nft

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable bench-sandbox-net >/dev/null 2>&1
  echo 'sandbox user $USER_NAME ready; set SANDBOX_USER=$USER_NAME in /srv/benchd/env and restart benchd at a job boundary'
"
