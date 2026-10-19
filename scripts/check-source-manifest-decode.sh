#!/bin/bash

set -euo pipefail

REPO_ROOT="${1:-.}"
cd "$REPO_ROOT"

FIX_MSG="Decode source-manifest bytes with sources.Manifest.Unmarshal or sources.ReadManifest, then pass validate.KnownSources / validate.UnreadableSources into validate and decompose."

if ! command -v rg >/dev/null 2>&1; then
    echo "FAIL: rg is required for check-source-manifest-decode" >&2
    exit 1
fi

if [[ ! -f go.mod ]]; then
    echo "FAIL: go.mod not found in $REPO_ROOT" >&2
    exit 1
fi

ERRORS=0

fail_hits() {
    local label="$1"
    local hits="$2"
    if [[ -n "$hits" ]]; then
        echo "FAIL: $label" >&2
        echo "$hits" >&2
        echo "$FIX_MSG" >&2
        ERRORS=$((ERRORS + 1))
    fi
}

ENTRIES_HITS=$(rg -n --glob '*.go' --glob '!internal/sources/**' --glob '!**/testdata/**' \
    'json:"entries"' . || true)
fail_hits "json:\"entries\" is only allowed on sources.Manifest in internal/sources" "$ENTRIES_HITS"

LAST_SYNCED_HITS=$(rg -n --glob '*.go' --glob '!internal/sources/**' --glob '!**/testdata/**' \
    'json:"last_synced"' . || true)
fail_hits "json:\"last_synced\" is only allowed on sources.SourceEntry in internal/sources" "$LAST_SYNCED_HITS"

RAW_ENTRIES_HITS=$(rg -n --glob '*.go' --glob '!**/testdata/**' \
    'map\[string\]json\.RawMessage\s+`json:"entries"`' . || true)
fail_hits "map[string]json.RawMessage json:\"entries\" reintroduces the PR #333/#362 decode hole" "$RAW_ENTRIES_HITS"

NAME_HITS=$(rg -n --glob '*.go' --glob '!**/testdata/**' \
    -e 'func parseManifestData\(' \
    -e 'func sourceIDsFromManifest\(' \
    -e 'type sourceEntry struct' \
    . || true)
fail_hits "duplicate source-manifest decoder names are banned; use sources.Manifest.Unmarshal" "$NAME_HITS"

if [[ "$ERRORS" -ne 0 ]]; then
    exit 1
fi

echo "OK: source-manifest JSON is decoded only by sources.Manifest.Unmarshal"
