#!/bin/bash

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
SCRIPT="$REPO_ROOT/scripts/check-source-manifest-decode.sh"

FAILURES=0
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

make_fixture() {
    local destination="$1"
    mkdir -p "$destination/scripts" "$destination/internal/sources" "$destination/internal/validate"
    cp "$SCRIPT" "$destination/scripts/check-source-manifest-decode.sh"
    chmod +x "$destination/scripts/check-source-manifest-decode.sh"
    printf 'module fixture\n\ngo 1.22\n' > "$destination/go.mod"
    cat > "$destination/internal/sources/types.go" <<'EOF'
package sources

type SourceEntry struct {
	LastSynced string `json:"last_synced"`
}

type Manifest struct {
	Entries map[string]SourceEntry `json:"entries"`
}
EOF
    printf 'package validate\n\nfunc Validate() {}\n' > "$destination/internal/validate/validate.go"
}

FIXTURE="$WORKDIR/fixture"
make_fixture "$FIXTURE"
CHECK="$FIXTURE/scripts/check-source-manifest-decode.sh"

echo "Test 1: check passes when only sources decodes the manifest..."
CLEAN_OUT=$(mktemp)
if "$CHECK" "$FIXTURE" > "$CLEAN_OUT" 2>&1; then
    echo "  PASS"
else
    echo "  FAIL: expected exit 0 on a sources-only decoder"
    cat "$CLEAN_OUT"
    FAILURES=$((FAILURES + 1))
fi
rm -f "$CLEAN_OUT"

echo "Test 2: duplicate last_synced/entries tags are rejected..."
cat > "$FIXTURE/internal/validate/validate.go" <<'EOF'
package validate

type sourceEntry struct {
	LastSynced string `json:"last_synced"`
}

func parseManifestData(data []byte) {
	var m struct {
		Entries map[string]sourceEntry `json:"entries"`
	}
	_ = m
	_ = data
}
EOF
set +e
OUTPUT=$("$CHECK" "$FIXTURE" 2>&1)
STATUS=$?
set -e
if [[ $STATUS -eq 0 ]]; then
    echo "  FAIL: expected non-zero exit for a copied decoder"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q 'sources.Manifest.Unmarshal' <<< "$OUTPUT"; then
    echo "  FAIL: error must name sources.Manifest.Unmarshal"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

echo "Test 3: json.RawMessage entries (PR #333/#362) are rejected..."
cat > "$FIXTURE/internal/validate/validate.go" <<'EOF'
package validate

import "encoding/json"

func parseManifestData(data []byte) {
	var m struct {
		Entries map[string]json.RawMessage `json:"entries"`
	}
	_ = m
	_ = data
}
EOF
set +e
OUTPUT=$("$CHECK" "$FIXTURE" 2>&1)
STATUS=$?
set -e
if [[ $STATUS -eq 0 ]]; then
    echo "  FAIL: expected non-zero exit for json.RawMessage entries"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q 'KnownSources' <<< "$OUTPUT"; then
    echo "  FAIL: error must name validate.KnownSources"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

echo "Test 4: check-source-manifest-decode.sh is committed executable..."
mode=$(git -C "$REPO_ROOT" ls-files -s -- "scripts/check-source-manifest-decode.sh" | awk '{print $1}')
if [[ "$mode" == "100755" ]]; then
    echo "  PASS: git mode is 100755"
else
    echo "  FAIL: git mode is '${mode:-missing}', expected 100755"
    FAILURES=$((FAILURES + 1))
fi
if [[ -x "$SCRIPT" ]]; then
    echo "  PASS: executable on disk"
else
    echo "  FAIL: not executable on disk"
    FAILURES=$((FAILURES + 1))
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All source-manifest-decode tests passed"
    exit 0
else
    echo "FAIL: $FAILURES source-manifest-decode test(s) failed"
    exit 1
fi
