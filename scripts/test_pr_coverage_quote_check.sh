#!/usr/bin/env bash
# Fixture test for scripts/pr-coverage-quote-check.sh.
#
# Locks in (REQ coverage-quote boundaries):
#   1. match — body quotes equal log → exit 0
#   2. REQ |diff|=0.10 — inclusive pass on cmd and internal
#   3. REQ |diff|=0.11 — fail
#   4. REQ missing quote — empty / docs-only / cmd-only → fail (no soft-pass)
#   5. REQ wrong-package — non-cmd/non-internal coverage token → fail
#   6. table form — "| check-core coverage | **internal …**, cmd … |" match
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
elif ! grep -q "OK: PR body coverage quotes within" <<<"$OUTPUT"; then
	echo "  FAIL: expected OK within-tol message"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 2: REQ |diff|=0.10 inclusive pass
# ----------------------------------------------------------------------------
echo "Test 2: REQ |body−log|=0.10 passes (inclusive)..."

TOL_PASS="$WORKDIR/tol_pass.md"
cat >"$TOL_PASS" <<'EOF'
cmd coverage: 83.89%
internal coverage: 85.97%
EOF

run_check "$TOL_PASS"
if [[ $STATUS -ne 0 ]]; then
	echo "  FAIL: expected exit 0 for |diff|=0.10, got $STATUS"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 3: REQ |diff|=0.11 fail
# ----------------------------------------------------------------------------
echo "Test 3: REQ |body−log|=0.11 fails..."

TOL_FAIL="$WORKDIR/tol_fail.md"
cat >"$TOL_FAIL" <<'EOF'
cmd coverage: 83.90%
internal coverage: 86.07%
EOF

run_check "$TOL_FAIL"
if [[ $STATUS -eq 0 ]]; then
	echo "  FAIL: expected non-zero exit for |diff|=0.11"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: body quotes cmd coverage 83.90% but log has 83.79%" <<<"$OUTPUT"; then
	echo "  FAIL: expected cmd |diff|>0.10 message"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 4: REQ missing quote fails (no soft-pass)
# ----------------------------------------------------------------------------
echo "Test 4: REQ missing coverage quotes fail (no soft-pass)..."

EMPTY_BODY="$WORKDIR/empty.md"
cat >"$EMPTY_BODY" <<'EOF'
Docs-only PR. No numbers here.
EOF

run_check "$EMPTY_BODY"
if [[ $STATUS -eq 0 ]]; then
	echo "  FAIL: expected non-zero exit for quote-less body"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: PR body must quote both cmd and internal coverage percentages" <<<"$OUTPUT"; then
	echo "  FAIL: expected missing-quote message"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

CMD_ONLY="$WORKDIR/cmd_only.md"
cat >"$CMD_ONLY" <<'EOF'
cmd coverage: 83.79%
EOF

run_check "$CMD_ONLY"
if [[ $STATUS -eq 0 ]]; then
	echo "  FAIL: expected non-zero exit for cmd-only body (internal missing)"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: PR body must quote both cmd and internal coverage percentages" <<<"$OUTPUT"; then
	echo "  FAIL: expected missing-quote message for cmd-only"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS (cmd-only)"
fi

# ----------------------------------------------------------------------------
# Test 5: REQ wrong-package quotes fail
# ----------------------------------------------------------------------------
echo "Test 5: REQ wrong-package coverage token fails..."

WRONG_PKG="$WORKDIR/wrong_pkg.md"
cat >"$WRONG_PKG" <<'EOF'
scripts coverage: 83.79%
mutate coverage: 86.07%
EOF

run_check "$WRONG_PKG"
if [[ $STATUS -eq 0 ]]; then
	echo "  FAIL: expected non-zero exit for wrong-package quotes"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
elif ! grep -q "FAIL: PR body must quote both cmd and internal coverage percentages" <<<"$OUTPUT"; then
	echo "  FAIL: expected missing cmd/internal message for wrong-package body"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

# ----------------------------------------------------------------------------
# Test 6: table form from ready notes matches
# ----------------------------------------------------------------------------
echo "Test 6: pr-coverage-quote-check accepts table-form coverage quotes..."

TABLE_BODY="$WORKDIR/table.md"
cat >"$TABLE_BODY" <<'EOF'
| check-core coverage | **internal 86.07%**, cmd 83.79% |
EOF

run_check "$TABLE_BODY"
if [[ $STATUS -ne 0 ]]; then
	echo "  FAIL: expected exit 0 for matching table form, got $STATUS"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
elif ! grep -q "OK: PR body coverage quotes within" <<<"$OUTPUT"; then
	echo "  FAIL: expected OK within-tol message for table form"
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
