#!/bin/bash
# Shared helpers for talking to the Hetzner Cloud API.
# Source this file; it expects HCLOUD_TOKEN to be set in the environment.

HCLOUD_API="https://api.hetzner.cloud/v1"
HCLOUD_TIMEOUT="${HCLOUD_TIMEOUT:-30}"        # per-request timeout (seconds)
HCLOUD_MAX_RETRIES="${HCLOUD_MAX_RETRIES:-4}" # attempts on transient failures

# Label applied to every resource we create, so the reaper can find orphans.
# (Referenced by the scripts that source this file.)
# shellcheck disable=SC2034
HCLOUD_LABEL_KEY="purpose"
# shellcheck disable=SC2034
HCLOUD_LABEL_VALUE="ssz-benchmark"

# hcloud_api METHOD PATH [JSON_BODY]
# Performs an authenticated request and prints the JSON response body on stdout.
# Retries transient failures (network errors and HTTP 429 rate limits) with a
# linear backoff. Other 4xx/5xx responses are returned to the caller as-is so it
# can inspect the JSON `.error`. Returns non-zero when all attempts fail.
#
# Note: `resp=$(hcloud_api ...)` masks the exit code under `set -e`, so callers
# also validate the body (e.g. with `jq -r '.server.id // empty'`).
hcloud_api() {
    local method="$1" path="$2" data="${3:-}"
    local attempt resp http body

    for (( attempt = 1; attempt <= HCLOUD_MAX_RETRIES; attempt++ )); do
        local args=(-sS -m "$HCLOUD_TIMEOUT" -w $'\n%{http_code}' -X "$method"
            -H "Authorization: Bearer ${HCLOUD_TOKEN}"
            -H "Content-Type: application/json")
        [ -n "$data" ] && args+=(-d "$data")

        if resp="$(curl "${args[@]}" "${HCLOUD_API}${path}" 2>/dev/null)"; then
            http="${resp##*$'\n'}"
            body="${resp%$'\n'*}"
            # Anything but a rate-limit is final (success or a real error body).
            if [ "$http" != "429" ]; then
                printf '%s' "$body"
                return 0
            fi
        else
            body=""  # transport failure: no HTTP response received
        fi

        if [ "$attempt" -lt "$HCLOUD_MAX_RETRIES" ]; then
            sleep $(( attempt * 3 ))
        fi
    done

    printf '%s' "${body:-}"
    return 1
}

# hcloud_error JSON  -> prints the API error message (or a fallback) for logging.
hcloud_error() {
    local msg
    msg="$(printf '%s' "$1" | jq -r '.error.message // empty' 2>/dev/null)"
    printf '%s' "${msg:-${1:-unknown error}}"
}
