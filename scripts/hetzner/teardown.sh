#!/bin/bash
# Delete the benchmark server and its SSH key by name. Idempotent and
# best-effort: never exits non-zero, so it is safe in an `if: always()` step.
#
# Required env:
#   HCLOUD_TOKEN  - Hetzner Cloud API token
#   SERVER_NAME   - name used at provision time
set -uo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$DIR/lib.sh"

: "${HCLOUD_TOKEN:?HCLOUD_TOKEN is required}"
: "${SERVER_NAME:?SERVER_NAME is required}"

echo ">> Tearing down '$SERVER_NAME'"

server_id="$(hcloud_api GET "/servers?name=$SERVER_NAME" 2>/dev/null \
    | jq -r '.servers[0].id // empty')"
if [ -n "$server_id" ]; then
    hcloud_api DELETE "/servers/$server_id" >/dev/null 2>&1 \
        && echo "   deleted server $server_id" \
        || echo "   WARNING: failed to delete server $server_id"
else
    echo "   no server named '$SERVER_NAME'"
fi

key_id="$(hcloud_api GET "/ssh_keys?name=$SERVER_NAME" 2>/dev/null \
    | jq -r '.ssh_keys[0].id // empty')"
if [ -n "$key_id" ]; then
    hcloud_api DELETE "/ssh_keys/$key_id" >/dev/null 2>&1 \
        && echo "   deleted ssh key $key_id" \
        || echo "   WARNING: failed to delete ssh key $key_id"
else
    echo "   no ssh key named '$SERVER_NAME'"
fi

exit 0
