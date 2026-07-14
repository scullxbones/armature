#!/bin/bash
# Real (non-manual) test for census-drift-check.sh.
#
# Verifies:
# 1. The check passes (exit 0) on a clean checkout.
# 2. The check fails (non-zero) and reports the expected message when a
#    surface exists in code but not in the census (injected op type, command,
#    or flag).
#
# This is wired into `make check` via the `test-census-drift-check` target.

set -euo pipefail

REPO_ROOT="${1:-.}"

FAILURES=0

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT
FIXTURE="$WORKDIR/fixture"

# Start from a tracked repository snapshot, then overlay the candidate checker
# and census. Every assertion below invokes the fixture's checker, so mutations
# cannot accidentally be checked by the mutable source-worktree script.
git -C "$REPO_ROOT" archive HEAD | (mkdir -p "$FIXTURE" && tar -x -C "$FIXTURE")
cp "$REPO_ROOT/scripts/census-drift-check.sh" "$FIXTURE/scripts/census-drift-check.sh"
cp "$REPO_ROOT/docs/design/surface-census.md" "$FIXTURE/docs/design/surface-census.md"
chmod +x "$FIXTURE/scripts/census-drift-check.sh"
SCRIPT="$FIXTURE/scripts/census-drift-check.sh"

# ----------------------------------------------------------------------------
# Test 1: the candidate checker passes against its clean fixture
# ----------------------------------------------------------------------------
echo "Test 1: census-drift-check passes on a clean fixture..."
if "$SCRIPT" "$FIXTURE" > /tmp/census-drift-clean.out 2>&1; then
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

TYPES_FILE="$FIXTURE/internal/ops/types.go"
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
    OUTPUT=$("$SCRIPT" "$FIXTURE" 2>&1)
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

# ----------------------------------------------------------------------------
# Test 3: a changed root Cobra command Use value is detected as drift
# ----------------------------------------------------------------------------
echo "Test 3: census-drift-check detects an undocumented CLI command..."

COMMAND_FILE="$FIXTURE/cmd/armature/dagsum.go"
if [[ ! -f "$COMMAND_FILE" ]]; then
    echo "  FAIL: fixture missing $COMMAND_FILE"
    FAILURES=$((FAILURES + 1))
else
    sed -i 's/Use:   "dag-summary"/Use:   "fake-drift-command"/' "$COMMAND_FILE"

    set +e
    OUTPUT=$("$SCRIPT" "$FIXTURE" 2>&1)
    STATUS=$?
    set -e

    if [[ $STATUS -eq 0 ]]; then
        echo "  FAIL: expected non-zero exit when an undocumented command is injected"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    elif ! grep -q "CLI command 'fake-drift-command' in code but not in census" <<< "$OUTPUT"; then
        echo "  FAIL: expected command drift message not found in output"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS"
    fi
fi

# ----------------------------------------------------------------------------
# Test 4: a Cobra flag missing from the census is detected as drift
# ----------------------------------------------------------------------------
echo "Test 4: census-drift-check detects an undocumented command flag..."

FLAG_FILE="$FIXTURE/cmd/armature/link.go"
if [[ ! -f "$FLAG_FILE" ]]; then
    echo "  FAIL: fixture missing $FLAG_FILE"
    FAILURES=$((FAILURES + 1))
else
    sed -i 's/"source", "", "source issue ID"/"fake-drift-flag", "", "source issue ID"/' "$FLAG_FILE"

    set +e
    OUTPUT=$("$SCRIPT" "$FIXTURE" 2>&1)
    STATUS=$?
    set -e

    if [[ $STATUS -eq 0 ]]; then
        echo "  FAIL: expected non-zero exit when an undocumented flag is injected"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    elif ! grep -q "Command flag '--fake-drift-flag' in code but not in census" <<< "$OUTPUT"; then
        echo "  FAIL: expected flag drift message not found in output"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS"
    fi
fi

# ----------------------------------------------------------------------------
# Test 5: a phantom command name in the census (that doesn't exist in code,
# including as a subcommand) is detected as drift
# ----------------------------------------------------------------------------
echo "Test 5: census-drift-check detects a phantom census command..."

CENSUS_FIXTURE="$FIXTURE/docs/design/surface-census.md"
if [[ ! -f "$CENSUS_FIXTURE" ]]; then
    echo "  FAIL: fixture missing $CENSUS_FIXTURE"
    FAILURES=$((FAILURES + 1))
else
    # Inject a phantom subcommand row that has no matching AddCommand/Use in
    # code, mirroring the real "review attest" drift this regression guards
    # against (a phantom subcommand name that was never a real command).
    sed -i 's/^| `review record` |/| `review attest` |/' "$CENSUS_FIXTURE"

    set +e
    OUTPUT=$("$SCRIPT" "$FIXTURE" 2>&1)
    STATUS=$?
    set -e

    if [[ $STATUS -eq 0 ]]; then
        echo "  FAIL: expected non-zero exit when a phantom census command is injected"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    elif ! grep -q "CLI command 'review attest' in census but not in code" <<< "$OUTPUT"; then
        echo "  FAIL: expected phantom command drift message not found in output"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS"
    fi

    # Restore the census fixture for any subsequent tests.
    git -C "$REPO_ROOT" show HEAD:docs/design/surface-census.md > "$CENSUS_FIXTURE" 2>/dev/null || true
    cp "$REPO_ROOT/docs/design/surface-census.md" "$CENSUS_FIXTURE"
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All census-drift-check tests passed"
    exit 0
else
    echo "FAIL: $FAILURES census-drift-check test(s) failed"
    exit 1
fi
