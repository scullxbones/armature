#!/usr/bin/env bash
# Pick a non-expired coverage-check artifact id for a head SHA + repo id.
#
# Usage: select-coverage-check-artifact-id.sh <head-sha> <repo-id> <artifacts-json>
#
# Reads the GitHub Actions artifacts list JSON (the response body of
# GET /repos/{owner}/{repo}/actions/artifacts). Filters on
# workflow_run.head_sha AND workflow_run.repository_id. Prints the newest
# matching id, or empty if none.
#
# IMPORTANT: do not pass --arg/--argjson to `gh api --jq` — gh's --jq flag
# does not accept jq variable flags (CI failure class on #432). Pipe to jq.

set -euo pipefail

HEAD_SHA=${1:?usage: select-coverage-check-artifact-id.sh <head-sha> <repo-id> <artifacts-json>}
REPO_ID=${2:?usage: select-coverage-check-artifact-id.sh <head-sha> <repo-id> <artifacts-json>}
JSON=${3:?usage: select-coverage-check-artifact-id.sh <head-sha> <repo-id> <artifacts-json>}

if [ ! -f "$JSON" ]; then
	echo "FAIL: artifacts json not found: $JSON" >&2
	exit 1
fi

# jq (not gh --jq) owns --arg/--argjson.
jq -r --arg sha "$HEAD_SHA" --argjson rid "$REPO_ID" '
	[.artifacts[]?
		| select(
			(.expired | not)
			and (.workflow_run.head_sha // "") == $sha
			and (.workflow_run.repository_id // 0) == $rid
		)
	]
	| .[0].id // empty
' "$JSON"
