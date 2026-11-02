#!/usr/bin/env bash
# Fixture test for scripts/select-coverage-check-artifact-id.sh (+ workflow contract).
#
# Locks in:
#   1. matching head_sha + repo_id → id
#   2. wrong head_sha → empty
#   3. wrong repo_id → empty
#   4. expired artifact ignored
#   5. edit workflow must pipe gh api → jq (never gh api --jq --arg*)

set -euo pipefail

REPO_ROOT="${1:-.}"
SCRIPT="$REPO_ROOT/scripts/select-coverage-check-artifact-id.sh"
EDIT_WF="$REPO_ROOT/.github/workflows/pr-coverage-quotes-on-edit.yml"

FAILURES=0
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

JSON="$WORKDIR/artifacts.json"
cat >"$JSON" <<'EOF'
{
  "artifacts": [
    {
      "id": 111,
      "name": "coverage-check-deadbeef",
      "expired": true,
      "workflow_run": {"head_sha": "deadbeef", "repository_id": 42}
    },
    {
      "id": 222,
      "name": "coverage-check-deadbeef",
      "expired": false,
      "workflow_run": {"head_sha": "deadbeef", "repository_id": 99}
    },
    {
      "id": 333,
      "name": "coverage-check-deadbeef",
      "expired": false,
      "workflow_run": {"head_sha": "deadbeef", "repository_id": 42}
    },
    {
      "id": 444,
      "name": "coverage-check-other",
      "expired": false,
      "workflow_run": {"head_sha": "cafebabe", "repository_id": 42}
    }
  ]
}
EOF

echo "Test 1: matching head_sha + repo_id returns id..."
got=$("$SCRIPT" deadbeef 42 "$JSON")
if [[ "$got" != "333" ]]; then
	echo "  FAIL: expected 333, got '$got'"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

echo "Test 2: wrong head_sha returns empty..."
got=$("$SCRIPT" nosuch 42 "$JSON")
if [[ -n "$got" ]]; then
	echo "  FAIL: expected empty, got '$got'"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

echo "Test 3: wrong repo_id returns empty..."
got=$("$SCRIPT" deadbeef 7 "$JSON")
if [[ -n "$got" ]]; then
	echo "  FAIL: expected empty, got '$got'"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

echo "Test 4: edit workflow must not pass --arg* to gh api --jq..."
if [[ ! -f "$EDIT_WF" ]]; then
	echo "  FAIL: missing $EDIT_WF"
	FAILURES=$((FAILURES + 1))
elif rg -n 'gh api[^\n]*--jq[^\n]*(--arg|--argjson)' "$EDIT_WF"; then
	echo "  FAIL: gh api --jq does not accept --arg/--argjson (CI class failure)"
	FAILURES=$((FAILURES + 1))
elif ! rg -q 'select-coverage-check-artifact-id\.sh' "$EDIT_WF"; then
	echo "  FAIL: edit workflow must call select-coverage-check-artifact-id.sh"
	FAILURES=$((FAILURES + 1))
else
	echo "  PASS"
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
	echo "All select-coverage-check-artifact-id tests passed"
	exit 0
else
	echo "FAIL: $FAILURES select-coverage-check-artifact-id test(s) failed"
	exit 1
fi
