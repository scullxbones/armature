#!/usr/bin/env bash
# Fixture test for scripts/pr-coverage-quote-check.sh.
#
# Locks in:
#   1. match — body quotes equal log → exit 0
#   2. mismatch — body internal % disagrees with log → exit 1
#   3. no-quote — empty / docs-only body → soft-pass exit 0
#   4. table form — "| check-core coverage | **internal …**, cmd … |" match
#   5. cmd-only mismatch still fails when only cmd is quoted
#
# Wired into make check-static via test-pr-coverage-quote-check (not into
# check-core: local make check has no PR body).

set -euo pipefail

REPO_ROOT="${1:-.}"
SCRIPT="$REPO_ROOT/scripts/pr-coverage-quote-check.sh"

FAILURES=0
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

LOG="$WORKDIR/coverage.log"
{
	echo "cmd coverage: 83.79%"
	echo "internal coverage: 86.07%"
} >"$LOG"

run_check() {
	local body="$1"
	set +e
	OUTPUT=$("$SCRIPT" "$body" "$LOG" 2>&1)
	STATUS=$?
	set -e
}

# ----------------------------------------------------------------------------
# Test 1: matching quotes pass
# ----------------------------------------------------------------------------
echo "Test 1: pr-coverage-quote-check passes when body matches log..."

MATCH_BODY="$WORKDIR/match.md"
cat >"$MATCH_BODY" <<'EOF'
## Coverage
cmd coverage: 83.79%
internal coverage: 86.07%
EOF

run_check "$MATCH_BODY"
if [[ $STATUS -ne 0 ]]; then
	echo "  FAIL: expected exit 0, got $STATUS"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
elif ! grep -q "OK: PR body coverage quotes match log" <<<"$OUTPUT"; then
	echo "  FAIL: expected OK match message"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 2: mismatched internal % fails
# ----------------------------------------------------------------------------
echo "Test 2: pr-coverage-quote-check fails when body internal % disagrees..."

MISMATCH_BODY="$WORKDIR/mismatch.md"
cat >"$MISMATCH_BODY" <<'EOF'
Ready note quoted **internal 86.08%**, cmd 83.75% from an older check-core.
EOF

run_check "$MISMATCH_BODY"
if [[ $STATUS -eq 0 ]]; then
	echo "  FAIL: expected non-zero exit"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: body quotes internal coverage 86.08% but log has 86.07%" <<<"$OUTPUT"; then
	echo "  FAIL: expected internal mismatch message"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 3: no coverage quote soft-passes
# ----------------------------------------------------------------------------
echo "Test 3: pr-coverage-quote-check soft-passes when body has no coverage quotes..."

EMPTY_BODY="$WORKDIR/empty.md"
cat >"$EMPTY_BODY" <<'EOF'
Docs-only PR. No numbers here.
EOF

run_check "$EMPTY_BODY"
if [[ $STATUS -ne 0 ]]; then
	echo "  FAIL: expected exit 0 soft-pass, got $STATUS"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
elif ! grep -q "OK: no coverage quotes in body (soft-pass)" <<<"$OUTPUT"; then
	echo "  FAIL: expected soft-pass message"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 4: table form from ready notes matches
# ----------------------------------------------------------------------------
echo "Test 4: pr-coverage-quote-check accepts table-form coverage quotes..."

TABLE_BODY="$WORKDIR/table.md"
cat >"$TABLE_BODY" <<'EOF'
| check-core coverage | **internal 86.07%**, cmd 83.79% |
EOF

run_check "$TABLE_BODY"
if [[ $STATUS -ne 0 ]]; then
	echo "  FAIL: expected exit 0 for matching table form, got $STATUS"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
elif ! grep -q "OK: PR body coverage quotes match log" <<<"$OUTPUT"; then
	echo "  FAIL: expected OK match message (not soft-pass) for table form"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 5: cmd-only quote that disagrees still fails
# ----------------------------------------------------------------------------
echo "Test 5: pr-coverage-quote-check fails on cmd-only mismatch..."

CMD_ONLY="$WORKDIR/cmd_only.md"
cat >"$CMD_ONLY" <<'EOF'
cmd coverage: 83.75%
EOF

run_check "$CMD_ONLY"
if [[ $STATUS -eq 0 ]]; then
	echo "  FAIL: expected non-zero exit for cmd-only mismatch"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: body quotes cmd coverage 83.75% but log has 83.79%" <<<"$OUTPUT"; then
	echo "  FAIL: expected cmd mismatch message"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
	echo "All pr-coverage-quote-check tests passed"
	exit 0
else
	echo "FAIL: $FAILURES pr-coverage-quote-check test(s) failed"
	exit 1
fi
