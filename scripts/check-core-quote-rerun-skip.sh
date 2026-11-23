#!/usr/bin/env bash
# Decide whether this check-core attempt must skip the suite and reuse the
# head SHA's coverage-check.log.
#
# skip=true only when RUN_ATTEMPT > 1 and a non-expired Actions artifact
# named quote-recheck-<RUN_ID>-<RUN_ATTEMPT> exists. The edit workflow
# uploads that artifact only after plan says the quote verdict disagrees
# with check-core and the suite itself already passed. Attempt 1 always
# runs the suite, even if a stray marker exists.
#
# A missing marker (manual rerun, or a rerun after a non-quote failure)
# prints skip=false so the suite runs again. A gh/API error exits 1:
# failing closed beats either skipping the suite or recomputing coverage
# on a quote-only rerun we could not identify.
#
# Stdout: skip=true or skip=false. Also appends that line to GITHUB_OUTPUT
# when the variable is set.
#
# Env: REPO, RUN_ID, RUN_ATTEMPT, GH_TOKEN, GH_BIN (default gh),
# MAX_POLLS (default 5), SLEEP_SECS (default 2). Bash 3.2-safe.

set -euo pipefail

GH_BIN=${GH_BIN:-gh}
MAX_POLLS=${MAX_POLLS:-5}
SLEEP_SECS=${SLEEP_SECS:-2}
REPO=${REPO:?REPO is required}
RUN_ID=${RUN_ID:?RUN_ID is required}
RUN_ATTEMPT=${RUN_ATTEMPT:?RUN_ATTEMPT is required}

write_skip() {
	printf 'skip=%s\n' "$1"
	if [ -n "${GITHUB_OUTPUT:-}" ]; then
		printf 'skip=%s\n' "$1" >> "$GITHUB_OUTPUT"
	fi
}

case "$RUN_ATTEMPT" in
''|*[!0-9]*)
	echo "FAIL: RUN_ATTEMPT must be numeric, got: $RUN_ATTEMPT" >&2
	exit 1
	;;
esac
case "$RUN_ID" in
''|*[!0-9]*)
	echo "FAIL: RUN_ID must be numeric, got: $RUN_ID" >&2
	exit 1
	;;
esac
case "$REPO" in
*..*|*/*/*|''|/*|*[!A-Za-z0-9_./-]*)
	echo "FAIL: REPO must be owner/name, got: $REPO" >&2
	exit 1
	;;
*/*) ;;
*)
	echo "FAIL: REPO must be owner/name, got: $REPO" >&2
	exit 1
	;;
esac

# First attempt is the suite. Do not consult the API.
if [ "$RUN_ATTEMPT" -le 1 ]; then
	write_skip false
	exit 0
fi

name="quote-recheck-${RUN_ID}-${RUN_ATTEMPT}"
# Name is digits and hyphens only; safe to embed in the jq program.
# gh --jq does not accept --arg (see test_check_jobs.sh).
jqprog='[.artifacts[] | select(.expired == false) | select(.name == "'"$name"'") | .id] | length'

i=0
while [ "$i" -lt "$MAX_POLLS" ]; do
	i=$((i + 1))
	set +e
	count=$("$GH_BIN" api "repos/${REPO}/actions/artifacts?name=${name}&per_page=20" --jq "$jqprog" 2>"${TMPDIR:-/tmp}/quote-rerun-skip.err")
	api_st=$?
	set -e
	if [ "$api_st" -ne 0 ]; then
		echo "FAIL: could not list quote-recheck marker ${name}" >&2
		cat "${TMPDIR:-/tmp}/quote-rerun-skip.err" >&2 || true
		exit 1
	fi
	case "$count" in
	*[!0-9]*)
		echo "FAIL: marker count was not numeric: ${count}" >&2
		exit 1
		;;
	esac
	if [ "$count" -ge 1 ]; then
		echo "quote-only rerun marker ${name} present; reusing coverage-check.log" >&2
		write_skip true
		exit 0
	fi
	if [ "$i" -ge "$MAX_POLLS" ]; then
		break
	fi
	echo "poll ${i}/${MAX_POLLS}: marker ${name} not visible yet" >&2
	sleep "$SLEEP_SECS"
done

echo "no quote-recheck marker ${name}; this attempt runs the suite" >&2
write_skip false
exit 0
