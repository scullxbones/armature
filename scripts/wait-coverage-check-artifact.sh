#!/usr/bin/env bash
# Poll until EXACTLY ONE coverage-check artifact matches this head SHA + repo,
# then download coverage-check.log into the cwd.
#
# Usage: wait-coverage-check-artifact.sh <repo> <head-sha> [out-log]
#
# Env:
#   GH_TOKEN   — required by gh
#   GH_BIN     — gh executable (default: gh); stubbable in tests
#   MAX_POLLS — default 120
#   SLEEP_SECS — default 30
#   SELECTOR   — path to select-coverage-check-artifact-id.sh (default: beside this script)
#
# Exit: 0 on success; 1 on timeout or hard selector/API failure.
# Zero matches (selector exit 2) keep polling; any other non-zero from the
# selector aborts immediately on first hard-fail (no || true). Two live
# check-core artifacts for one head SHA are a selector hard-fail.

set -euo pipefail

REPO=${1:?usage: wait-coverage-check-artifact.sh <repo> <head-sha> [out-log]}
HEAD_SHA=${2:?usage: wait-coverage-check-artifact.sh <repo> <head-sha> [out-log]}
OUT_LOG=${3:-coverage-check.log}

GH_BIN=${GH_BIN:-gh}
MAX_POLLS=${MAX_POLLS:-120}
SLEEP_SECS=${SLEEP_SECS:-30}
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
SELECTOR=${SELECTOR:-"$SCRIPT_DIR/select-coverage-check-artifact-id.sh"}

name="coverage-check-${HEAD_SHA}"
REPO_ID="$("$GH_BIN" api "repos/${REPO}" --jq .id)"
echo "Waiting for artifact name=${name} repo_id=${REPO_ID} head_sha=${HEAD_SHA}"

TMPDIR_ART=$(mktemp -d)
trap 'rm -rf "$TMPDIR_ART"' EXIT
JSON="$TMPDIR_ART/artifacts.json"
LAST_GH_ERR=""

id=""
i=0
while [ "$i" -lt "$MAX_POLLS" ]; do
	i=$((i + 1))
	set +e
	"$GH_BIN" api "repos/${REPO}/actions/artifacts?name=${name}&per_page=20" \
		>"$JSON" 2>"$TMPDIR_ART/gh.err"
	api_st=$?
	set -e
	if [ "$api_st" -ne 0 ]; then
		LAST_GH_ERR=$(tr '\n' ' ' <"$TMPDIR_ART/gh.err" 2>/dev/null || true)
		echo "poll ${i}/${MAX_POLLS}: gh api failed (status ${api_st}); retrying"
		sleep "$SLEEP_SECS"
		continue
	fi

	set +e
	id=$("$SELECTOR" "$HEAD_SHA" "$REPO_ID" "$JSON" 2>"$TMPDIR_ART/sel.err")
	sel_st=$?
	set -e
	if [ "$sel_st" -eq 0 ] && [ -n "$id" ]; then
		echo "Found artifact id=${id} after ${i} poll(s)"
		break
	fi
	if [ "$sel_st" -eq 2 ]; then
		echo "poll ${i}/${MAX_POLLS}: artifact not ready yet"
		id=""
		sleep "$SLEEP_SECS"
		continue
	fi
	# First hard-fail from selector aborts — never continue the poll loop.
	# (A continue-after-hard-fail mutant is killed by call-count fixtures.)
	cat "$TMPDIR_ART/sel.err" >&2 || true
	echo "FAIL: coverage-check artifact selector hard-failed (exit ${sel_st})" >&2
	exit 1
done

if [ -z "$id" ]; then
	echo "FAIL: timed out waiting for coverage-check artifact ${name}" >&2
	if [ -n "$LAST_GH_ERR" ]; then
		echo "FAIL: last gh api error: ${LAST_GH_ERR}" >&2
	fi
	exit 1
fi

"$GH_BIN" api "repos/${REPO}/actions/artifacts/${id}/zip" >"$TMPDIR_ART/coverage-check.zip"
unzip -o -d "$TMPDIR_ART/art" "$TMPDIR_ART/coverage-check.zip"
test -f "$TMPDIR_ART/art/coverage-check.log"
cp "$TMPDIR_ART/art/coverage-check.log" "$OUT_LOG"
exit 0
