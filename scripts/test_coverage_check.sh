#!/bin/bash
# Fixture test for scripts/coverage-check.sh — the per-tree statement
# coverage threshold logic factored out of the Makefile `coverage-check`
# target (docs/adr/0015-recalibrate-mutation-and-coverage-gates.md Decision
# 1: internal/** >= 86, cmd/** >= 83).
#
# Feeds synthetic `go test -coverprofile` profiles directly to
# scripts/coverage-check.sh and locks in:
#   1. pass case (both trees above threshold)
#   2. cmd-below-83 failure message and non-zero exit
#   3. internal-below-86 failure message and non-zero exit
#   4. the "no coverage lines matched armature/cmd/" empty-tree guard
#   5. the same guard for internal
#   6. the missing-profile guard (nonexistent profile path -> non-zero exit
#      and a "not found" message naming the actual path)
#
# This is wired into `make check` via the `test-coverage-check` target,
# mirroring how test-census-drift-check exercises census-drift-check.sh.

set -euo pipefail

REPO_ROOT="${1:-.}"
SCRIPT="$REPO_ROOT/scripts/coverage-check.sh"

FAILURES=0
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

# profile_line <path> <total-statements> <covered:0|1>
profile_line() {
    printf 'github.com/armature/%s:1.1,2.2 %s %s\n' "$1" "$2" "$3"
}

run_check() {
    local profile="$1"
    set +e
    OUTPUT=$("$SCRIPT" "$REPO_ROOT" "$profile" 2>&1)
    STATUS=$?
    set -e
}

# ----------------------------------------------------------------------------
# Test 1: pass case — both trees comfortably above threshold
# ----------------------------------------------------------------------------
echo "Test 1: coverage-check passes when both trees are above threshold..."

PASS_PROFILE="$WORKDIR/pass.out"
{
    echo "mode: set"
    profile_line "cmd/armature/foo.go" 100 1
    profile_line "cmd/armature/bar.go" 10 0
    profile_line "internal/materialize/baz.go" 100 1
    profile_line "internal/materialize/qux.go" 5 0
} > "$PASS_PROFILE"

run_check "$PASS_PROFILE"
if [[ $STATUS -ne 0 ]]; then
    echo "  FAIL: expected exit 0, got $STATUS"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "cmd coverage: 90.91%" <<< "$OUTPUT" || ! grep -q "internal coverage: 95.24%" <<< "$OUTPUT"; then
    echo "  FAIL: expected both percentages printed"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 2: cmd below 83% fails with the expected message and non-zero exit
# ----------------------------------------------------------------------------
echo "Test 2: coverage-check fails when cmd is below 83%..."

CMD_FAIL_PROFILE="$WORKDIR/cmd_fail.out"
{
    echo "mode: set"
    profile_line "cmd/armature/foo.go" 80 1
    profile_line "cmd/armature/bar.go" 20 0
    profile_line "internal/materialize/baz.go" 100 1
} > "$CMD_FAIL_PROFILE"

run_check "$CMD_FAIL_PROFILE"
if [[ $STATUS -eq 0 ]]; then
    echo "  FAIL: expected non-zero exit"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: cmd coverage 80.00% is below 83% threshold" <<< "$OUTPUT"; then
    echo "  FAIL: expected cmd shortfall message"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 3: internal below 86% fails with the expected message and non-zero exit
# ----------------------------------------------------------------------------
echo "Test 3: coverage-check fails when internal is below 86%..."

INT_FAIL_PROFILE="$WORKDIR/internal_fail.out"
{
    echo "mode: set"
    profile_line "cmd/armature/foo.go" 100 1
    profile_line "internal/materialize/baz.go" 80 1
    profile_line "internal/materialize/qux.go" 20 0
} > "$INT_FAIL_PROFILE"

run_check "$INT_FAIL_PROFILE"
if [[ $STATUS -eq 0 ]]; then
    echo "  FAIL: expected non-zero exit"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: internal coverage 80.00% is below 86% threshold" <<< "$OUTPUT"; then
    echo "  FAIL: expected internal shortfall message"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 4: empty-tree guard — no cmd lines in the profile at all
# ----------------------------------------------------------------------------
echo "Test 4: coverage-check fails when armature/cmd/ is missing from the profile..."

NO_CMD_PROFILE="$WORKDIR/no_cmd.out"
{
    echo "mode: set"
    profile_line "internal/materialize/baz.go" 100 1
} > "$NO_CMD_PROFILE"

run_check "$NO_CMD_PROFILE"
if [[ $STATUS -eq 0 ]]; then
    echo "  FAIL: expected non-zero exit"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: no coverage lines matched armature/cmd/ — tree missing from profile" <<< "$OUTPUT"; then
    echo "  FAIL: expected cmd empty-tree guard message"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 5: empty-tree guard — no internal lines in the profile at all
# ----------------------------------------------------------------------------
echo "Test 5: coverage-check fails when armature/internal/ is missing from the profile..."

NO_INTERNAL_PROFILE="$WORKDIR/no_internal.out"
{
    echo "mode: set"
    profile_line "cmd/armature/foo.go" 100 1
} > "$NO_INTERNAL_PROFILE"

run_check "$NO_INTERNAL_PROFILE"
if [[ $STATUS -eq 0 ]]; then
    echo "  FAIL: expected non-zero exit"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: no coverage lines matched armature/internal/ — tree missing from profile" <<< "$OUTPUT"; then
    echo "  FAIL: expected internal empty-tree guard message"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 6: missing-profile guard — nonexistent profile path
# ----------------------------------------------------------------------------
echo "Test 6: coverage-check fails when the profile file does not exist..."

MISSING_PROFILE="$WORKDIR/does_not_exist.out"

run_check "$MISSING_PROFILE"
if [[ $STATUS -eq 0 ]]; then
    echo "  FAIL: expected non-zero exit"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: $MISSING_PROFILE not found; run 'make coverage' first" <<< "$OUTPUT"; then
    echo "  FAIL: expected not-found message naming the profile path"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All coverage-check tests passed"
    exit 0
else
    echo "FAIL: $FAILURES coverage-check test(s) failed"
    exit 1
fi
