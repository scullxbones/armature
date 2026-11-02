#!/usr/bin/env bash
# Fixture test for scripts/select-coverage-check-artifact-id.sh (fail-closed).
#
# REQ:
#   exactly-one match (head_sha + repository_id + head_repository_id) → exit 0 + id
#   empty / expired-only / {} / wrong sha → exit 2 (not ready) OR exit 1 with FAIL
#   multiple matches → exit 1 FAIL
#   fork (head_repository_id != rid) → not selected (exit 2 if sole candidate)
#   malformed JSON → exit 1 FAIL (not raw jq 5)
#   edit workflow uses wait script; never || true on selector; no gh --arg*

set -euo pipefail

REPO_ROOT="${1:-.}"
SCRIPT="$REPO_ROOT/scripts/select-coverage-check-artifact-id.sh"
EDIT_WF="$REPO_ROOT/.github/workflows/pr-coverage-quotes-on-edit.yml"
WAIT="$REPO_ROOT/scripts/wait-coverage-check-artifact.sh"

FAILURES=0
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

run_sel() {
	local json="$1"
	shift
	set +e
	OUT=$("$SCRIPT" "$@" "$json" 2>"$WORKDIR/err.txt")
	ST=$?
	ERR=$(cat "$WORKDIR/err.txt")
	set -e
}

pass() { echo "  PASS${1:+ ($1)}"; }
fail() { echo "  FAIL: $1"; echo "  out='$OUT' st=$ST err='$ERR'"; FAILURES=$((FAILURES + 1)); }

# ----------------------------------------------------------------------------
echo "Test 1: exactly one match (sha+repo+head_repository_id) → id..."
ONE="$WORKDIR/one.json"
cat >"$ONE" <<'EOF'
{
  "artifacts": [
    {
      "id": 333,
      "name": "coverage-check-deadbeef",
      "expired": false,
      "workflow_run": {
        "head_sha": "deadbeef",
        "repository_id": 42,
        "head_repository_id": 42
      }
    }
  ]
}
EOF
run_sel "$ONE" deadbeef 42
if [[ $ST -ne 0 || "$OUT" != "333" ]]; then fail "expected exit 0 id=333"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 2: empty artifacts → exit 2 (not ready)..."
EMPTY="$WORKDIR/empty.json"
echo '{"artifacts":[]}' >"$EMPTY"
run_sel "$EMPTY" deadbeef 42
if [[ $ST -ne 2 ]]; then fail "expected exit 2 for empty list"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 3: expired-only → exit 2..."
EXP="$WORKDIR/expired.json"
cat >"$EXP" <<'EOF'
{
  "artifacts": [
    {
      "id": 111,
      "expired": true,
      "workflow_run": {
        "head_sha": "deadbeef",
        "repository_id": 42,
        "head_repository_id": 42
      }
    }
  ]
}
EOF
run_sel "$EXP" deadbeef 42
if [[ $ST -ne 2 ]]; then fail "expected exit 2 for expired-only"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 4: {} missing .artifacts → exit 1 FAIL..."
BRACE="$WORKDIR/brace.json"
echo '{}' >"$BRACE"
run_sel "$BRACE" deadbeef 42
if [[ $ST -ne 1 ]]; then fail "expected exit 1 for {}"
elif ! grep -q '^FAIL:' <<<"$ERR"; then fail "expected FAIL: reason on stderr"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 5: multiple matches → exit 1 FAIL..."
MULTI="$WORKDIR/multi.json"
cat >"$MULTI" <<'EOF'
{
  "artifacts": [
    {
      "id": 1,
      "expired": false,
      "workflow_run": {"head_sha": "deadbeef", "repository_id": 42, "head_repository_id": 42}
    },
    {
      "id": 2,
      "expired": false,
      "workflow_run": {"head_sha": "deadbeef", "repository_id": 42, "head_repository_id": 42}
    }
  ]
}
EOF
run_sel "$MULTI" deadbeef 42
if [[ $ST -ne 1 ]]; then fail "expected exit 1 for multiple"
elif ! grep -q 'FAIL: expected exactly one' <<<"$ERR"; then fail "expected exactly-one FAIL message"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 6: fork artifact (head_repository_id != rid) not selected → exit 2..."
FORK="$WORKDIR/fork.json"
cat >"$FORK" <<'EOF'
{
  "artifacts": [
    {
      "id": 777,
      "expired": false,
      "workflow_run": {
        "head_sha": "deadbeef",
        "repository_id": 42,
        "head_repository_id": 99
      }
    }
  ]
}
EOF
run_sel "$FORK" deadbeef 42
if [[ $ST -ne 2 ]]; then fail "expected exit 2 (fork filtered out)"
elif [[ -n "$OUT" ]]; then fail "fork must not print an id"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 7: wrong head_sha → exit 2..."
run_sel "$ONE" nosuch 42
if [[ $ST -ne 2 ]]; then fail "expected exit 2 for wrong sha"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 8: malformed JSON → exit 1 FAIL (not raw jq 5)..."
BAD="$WORKDIR/bad.json"
echo 'not-json{' >"$BAD"
run_sel "$BAD" deadbeef 42
if [[ $ST -ne 1 ]]; then fail "expected exit 1 for malformed (got $ST)"
elif ! grep -q '^FAIL: malformed' <<<"$ERR"; then fail "expected FAIL: malformed …"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 9: edit workflow uses wait script; no || true; no gh --arg*..."
if [[ ! -f "$EDIT_WF" ]]; then
	fail "missing edit workflow"
elif ! rg -q 'wait-coverage-check-artifact\.sh' "$EDIT_WF"; then
	fail "edit workflow must call wait-coverage-check-artifact.sh"
elif rg -n 'select-coverage-check-artifact-id\.sh[^\n]*\|\|\s*true' "$EDIT_WF"; then
	fail "edit workflow must not || true the selector"
elif rg -n 'gh api[^\n]*--jq[^\n]*(--arg|--argjson)' "$EDIT_WF"; then
	fail "gh api --jq must not take --arg/--argjson"
elif ! rg -q 'ci-path-shadow-selfcheck\.sh' "$EDIT_WF"; then
	fail "edit workflow must run PATH-shadow self-check"
else
	pass
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
	echo "All select-coverage-check-artifact-id tests passed"
	exit 0
else
	echo "FAIL: $FAILURES select-coverage-check-artifact-id test(s) failed"
	exit 1
fi
