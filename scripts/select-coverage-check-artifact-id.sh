#!/usr/bin/env bash
# Pick EXACTLY ONE non-expired coverage-check artifact id for a head SHA + repo.
#
# Usage: select-coverage-check-artifact-id.sh <head-sha> <repo-id> <artifacts-json>
#
# Reads the GitHub Actions artifacts list JSON (GET …/actions/artifacts).
# A match requires ALL of:
#   expired == false
#   workflow_run.head_sha == <head-sha>
#   workflow_run.repository_id == <repo-id>
#   workflow_run.head_repository_id == <repo-id>
# (head_repository_id blocks fork-PR artifacts that share repository_id.)
#
# Exit codes (fail-closed):
#   0  — exactly one match; id printed on stdout
#   2  — zero matches (not ready yet — caller may poll)
#   1  — hard fail (missing .artifacts, malformed JSON, multiple matches, …)
#        always prints "FAIL: <reason>" on stderr
#
# IMPORTANT: do not pass --arg/--argjson to `gh api --jq` — pipe to jq.

set -euo pipefail

HEAD_SHA=${1:?usage: select-coverage-check-artifact-id.sh <head-sha> <repo-id> <artifacts-json>}
REPO_ID=${2:?usage: select-coverage-check-artifact-id.sh <head-sha> <repo-id> <artifacts-json>}
JSON=${3:?usage: select-coverage-check-artifact-id.sh <head-sha> <repo-id> <artifacts-json>}

if [ ! -f "$JSON" ]; then
	echo "FAIL: artifacts json not found: $JSON" >&2
	exit 1
fi

# Validate REPO_ID is an integer (jq --argjson).
case "$REPO_ID" in
	'' | *[!0-9]*)
		echo "FAIL: repo-id must be a non-negative integer (got ${REPO_ID})" >&2
		exit 1
		;;
esac

# Parse + filter. Capture jq failures as hard FAIL (not raw exit 5).
set +e
FILTERED=$(jq -c --arg sha "$HEAD_SHA" --argjson rid "$REPO_ID" '
	if (type != "object") then
		error("top-level JSON must be an object")
	elif (has("artifacts") | not) then
		error("input missing .artifacts array")
	elif ((.artifacts | type) != "array") then
		error(".artifacts must be an array")
	else
		[.artifacts[]
			| select(
				(.expired | not)
				and ((.workflow_run.head_sha // "") == $sha)
				and ((.workflow_run.repository_id // 0) == $rid)
				and ((.workflow_run.head_repository_id // 0) == $rid)
			)
		]
	end
' "$JSON" 2>"$JSON.jq.err")
jq_st=$?
set -e

if [ "$jq_st" -ne 0 ]; then
	err=$(tr '\n' ' ' <"$JSON.jq.err" 2>/dev/null || true)
	rm -f "$JSON.jq.err"
	echo "FAIL: malformed artifacts JSON (${err:-jq failed})" >&2
	exit 1
fi
rm -f "$JSON.jq.err"

COUNT=$(printf '%s\n' "$FILTERED" | jq 'length')
if [ "$COUNT" -eq 0 ]; then
	# Not ready yet — distinct from hard fail so the poller can continue.
	exit 2
fi
if [ "$COUNT" -ne 1 ]; then
	echo "FAIL: expected exactly one matching coverage-check artifact, got ${COUNT}" >&2
	exit 1
fi

printf '%s\n' "$FILTERED" | jq -r '.[0].id'
exit 0
