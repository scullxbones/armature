#!/bin/bash
# Real (non-manual) test for census-drift-check.sh.
#
# Verifies:
# 1. The check passes (exit 0) on a clean checkout.
# 2. The check fails (non-zero) and reports the expected message when a
#    surface exists in code but not in the census (injected op type).
#
# This is wired into `make check` via the `test-census-drift-check` target.

set -euo pipefail

REPO_ROOT="${1:-.}"
SCRIPT="$REPO_ROOT/scripts/census-drift-check.sh"

FAILURES=0

# ----------------------------------------------------------------------------
# Test 1: clean tree passes
# ----------------------------------------------------------------------------
echo "Test 1: census-drift-check passes on a clean tree..."
if "$SCRIPT" "$REPO_ROOT" > /tmp/census-drift-clean.out 2>&1; then
    echo "  PASS"
else
    echo "  FAIL: expected exit 0 on clean tree, got non-zero"
    cat /tmp/census-drift-clean.out
    FAILURES=$((FAILURES + 1))
fi

# ----------------------------------------------------------------------------
# Test 2: an op type in code but not in the census is detected as drift
# ----------------------------------------------------------------------------
echo "Test 2: census-drift-check detects an undocumented op type..."

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

# Copy the tracked tree into an isolated fixture so we can safely mutate it.
git -C "$REPO_ROOT" archive HEAD | (mkdir -p "$WORKDIR/fixture" && tar -x -C "$WORKDIR/fixture")

TYPES_FILE="$WORKDIR/fixture/internal/ops/types.go"
if [[ ! -f "$TYPES_FILE" ]]; then
    echo "  FAIL: fixture missing $TYPES_FILE"
    FAILURES=$((FAILURES + 1))
else
    # Inject a fake op type constant that has no corresponding census row.
    awk '
        /^const \(/ && !done { print; print "\tOpFakeDriftSurface = \"fake-drift-surface\""; done=1; next }
        { print }
    ' "$TYPES_FILE" > "$TYPES_FILE.new" && mv "$TYPES_FILE.new" "$TYPES_FILE"

    set +e
    OUTPUT=$("$SCRIPT" "$WORKDIR/fixture" 2>&1)
    STATUS=$?
    set -e

    if [[ $STATUS -eq 0 ]]; then
        echo "  FAIL: expected non-zero exit when an undocumented op type is injected"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    elif ! grep -q "Op type 'fake-drift-surface' in code but not in census" <<< "$OUTPUT"; then
        echo "  FAIL: expected drift message not found in output"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS"
    fi
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All census-drift-check tests passed"
    exit 0
else
    echo "FAIL: $FAILURES census-drift-check test(s) failed"
    exit 1
fi
