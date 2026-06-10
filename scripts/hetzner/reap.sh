#!/bin/bash
# Safety net: delete any benchmark server (and orphaned ssh key) that has
# outlived a normal run. A successful run tears its own server down; this
# catches servers leaked by a hard-crashed or cancelled workflow.
#
# Required env:
#   HCLOUD_TOKEN  - Hetzner Cloud API token
# Optional env:
#   MAX_AGE_MIN   - delete servers older than this many minutes (default 90).
#                   Must exceed the benchmark job's timeout so it never reaps a
#                   server that a still-running job is legitimately using.
set -uo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$DIR/lib.sh"

: "${HCLOUD_TOKEN:?HCLOUD_TOKEN is required}"
MAX_AGE_MIN="${MAX_AGE_MIN:-90}"

selector="${HCLOUD_LABEL_KEY}=${HCLOUD_LABEL_VALUE}"
now="$(date -u +%s)"

age_minutes() {
    # age_minutes <iso8601> -> minutes since that timestamp, or empty on parse error
    local created="$1" cts
    cts="$(date -u -d "$created" +%s 2>/dev/null)" || return 0
    [ -n "$cts" ] && echo $(( (now - cts) / 60 ))
}

echo ">> Reaping benchmark servers older than ${MAX_AGE_MIN}m"
servers="$(hcloud_api GET "/servers?label_selector=$selector")"
echo "$servers" | jq -c '.servers[]?' | while read -r s; do
    id="$(echo "$s" | jq -r '.id')"
    name="$(echo "$s" | jq -r '.name')"
    created="$(echo "$s" | jq -r '.created')"
    age="$(age_minutes "$created")"
    if [ -n "$age" ] && [ "$age" -ge "$MAX_AGE_MIN" ]; then
        echo "   reaping server $name ($id), age ${age}m"
        hcloud_api DELETE "/servers/$id" >/dev/null 2>&1 || echo "   WARNING: delete failed for $id"
    else
        echo "   keeping server $name ($id), age ${age:-unknown}m"
    fi
done

# Orphaned ssh keys: labeled keys with no matching live server name.
echo ">> Reaping orphaned ssh keys"
live_names="$(hcloud_api GET "/servers?label_selector=$selector" | jq -r '.servers[]?.name')"
keys="$(hcloud_api GET "/ssh_keys?label_selector=$selector")"
echo "$keys" | jq -c '.ssh_keys[]?' | while read -r k; do
    id="$(echo "$k" | jq -r '.id')"
    name="$(echo "$k" | jq -r '.name')"
    if ! grep -qxF "$name" <<<"$live_names"; then
        echo "   reaping orphan ssh key $name ($id)"
        hcloud_api DELETE "/ssh_keys/$id" >/dev/null 2>&1 || echo "   WARNING: delete failed for key $id"
    fi
done

exit 0
