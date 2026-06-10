#!/bin/bash
# Provision an ephemeral Hetzner Cloud server for running benchmarks.
#
# Required env:
#   HCLOUD_TOKEN     - Hetzner Cloud API token
#   SERVER_NAME      - unique name for the server + ssh key (e.g. ssz-bench-<run_id>)
#   SSH_PUBLIC_KEY   - public key to inject (matching /tmp/bench_key)
# Optional env:
#   SERVER_TYPE      - default ccx13 (2 dedicated vCPU, 8GB)
#   LOCATION         - default fsn1
#   SERVER_IMAGE     - default ubuntu-24.04
#   SSH_KEY_FILE     - private key for the SSH readiness check (default /tmp/bench_key)
#
# Outputs (when running under GitHub Actions): writes ip / server_id to $GITHUB_OUTPUT.
# Always prints the server IP as the last line of stdout.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$DIR/lib.sh"

: "${HCLOUD_TOKEN:?HCLOUD_TOKEN is required}"
: "${SERVER_NAME:?SERVER_NAME is required}"
: "${SSH_PUBLIC_KEY:?SSH_PUBLIC_KEY is required}"
SERVER_TYPE="${SERVER_TYPE:-ccx23}"
LOCATION="${LOCATION:-fsn1}"
SERVER_IMAGE="${SERVER_IMAGE:-ubuntu-24.04}"
SSH_KEY_FILE="${SSH_KEY_FILE:-/tmp/bench_key}"

SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null
    -o ConnectTimeout=5 -i "$SSH_KEY_FILE")

echo ">> Creating SSH key '$SERVER_NAME'"
key_payload="$(jq -n --arg n "$SERVER_NAME" --arg k "$SSH_PUBLIC_KEY" \
    --arg lk "$HCLOUD_LABEL_KEY" --arg lv "$HCLOUD_LABEL_VALUE" \
    '{name:$n, public_key:$k, labels:{($lk):$lv}}')"
resp="$(hcloud_api POST /ssh_keys "$key_payload")"
key_id="$(echo "$resp" | jq -r '.ssh_key.id // empty')"
if [ -z "$key_id" ]; then
    echo "ERROR: failed to create SSH key: $(hcloud_error "$resp")" >&2
    exit 1
fi

echo ">> Creating server '$SERVER_NAME' ($SERVER_TYPE @ $LOCATION, $SERVER_IMAGE)"
srv_payload="$(jq -n \
    --arg n "$SERVER_NAME" --arg t "$SERVER_TYPE" --arg i "$SERVER_IMAGE" \
    --arg l "$LOCATION" --argjson k "$key_id" \
    --arg lk "$HCLOUD_LABEL_KEY" --arg lv "$HCLOUD_LABEL_VALUE" \
    '{name:$n, server_type:$t, image:$i, location:$l, ssh_keys:[$k],
      public_net:{enable_ipv4:true, enable_ipv6:true},
      labels:{($lk):$lv}}')"
resp="$(hcloud_api POST /servers "$srv_payload")"
server_id="$(echo "$resp" | jq -r '.server.id // empty')"
if [ -z "$server_id" ]; then
    echo "ERROR: failed to create server: $(hcloud_error "$resp")" >&2
    # Best-effort: remove the key we just created so we don't leak it.
    hcloud_api DELETE "/ssh_keys/$key_id" >/dev/null 2>&1 || true
    exit 1
fi

echo ">> Waiting for server to reach 'running' and get an IPv4..."
ip=""
for _ in $(seq 1 60); do
    resp="$(hcloud_api GET "/servers/$server_id")"
    status="$(echo "$resp" | jq -r '.server.status // empty')"
    ip="$(echo "$resp" | jq -r '.server.public_net.ipv4.ip // empty')"
    if [ "$status" = "running" ] && [ -n "$ip" ]; then
        break
    fi
    sleep 5
done
if [ -z "$ip" ]; then
    echo "ERROR: server $server_id did not become ready in time" >&2
    exit 1
fi
echo ">> Server running at $ip (id $server_id)"

echo ">> Waiting for SSH..."
ssh_ready=false
for _ in $(seq 1 60); do
    if ssh "${SSH_OPTS[@]}" "root@$ip" true 2>/dev/null; then
        ssh_ready=true
        break
    fi
    sleep 5
done
if [ "$ssh_ready" != true ]; then
    echo "ERROR: SSH never became reachable on $ip" >&2
    exit 1
fi
echo ">> SSH is ready"

if [ -n "${GITHUB_OUTPUT:-}" ]; then
    {
        echo "ip=$ip"
        echo "server_id=$server_id"
    } >> "$GITHUB_OUTPUT"
fi

# Last line of stdout is the bare IP, for non-Actions callers.
echo "$ip"
