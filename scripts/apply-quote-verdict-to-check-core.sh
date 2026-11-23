#!/usr/bin/env bash
# Move the required check-core conclusion to match a PR-body quote verdict.
#
# check-core is the ruleset context that fail-closes missing quotes. It does
# not re-run on pull_request edited (that would mark the other required jobs
# skipped). This script is the edit path: when check-core is red only because
# the quote step failed, or green while the live body no longer matches, plan
# a quote-only rerun. The rerun reuses the head SHA's coverage-check.log; it
# does not recompute coverage. A suite failure is left alone.
#
# Usage:
#   apply-quote-verdict-to-check-core.sh plan <repo> <head-sha> <match|mismatch>
#   apply-quote-verdict-to-check-core.sh commit <repo> <run-id> <job-id>
#
# plan stdout (exactly one line):
#   rerun=false
#   rerun=true run_id=<id> job_id=<id> next_attempt=<n>
# When rerun=true, plan writes quote-recheck-marker.txt in the cwd and, if
# GITHUB_OUTPUT is set, appends rerun/run_id/job_id/next_attempt there.
# The edit workflow uploads that file as artifact
# quote-recheck-<run_id>-<next_attempt> BEFORE commit. check-core's next
# attempt skips the suite only when that artifact exists.
#
# commit waits until the workflow run is completed, then POSTs
# /actions/jobs/<job_id>/rerun. Stdout: "reran job <id>".
#
# Env: GH_BIN (default gh), GH_TOKEN (for real gh), MAX_POLLS (default 40),
# SLEEP_SECS (default 15). Bash 3.2-safe.

set -euo pipefail

QUOTE_STEP="PR body coverage quotes match this job's coverage.out"
GH_BIN=${GH_BIN:-gh}
MAX_POLLS=${MAX_POLLS:-40}
SLEEP_SECS=${SLEEP_SECS:-15}

RUN_JQ='if (.workflow_runs | length) == 0 then "none" else .workflow_runs[0] | [.id, .status, (.run_attempt | tostring)] | @tsv end'
JOB_JQ='if ([.jobs[] | select(.name == "check-core")] | length) == 0 then "absent" else ([.jobs[] | select(.name == "check-core")][0] | [.id, .status, (.conclusion // "none"), ([.steps[]? | select(.conclusion == "failure") | .name] | join("|"))] | @tsv) end'

usage() {
	echo "usage: apply-quote-verdict-to-check-core.sh plan <repo> <head-sha> <match|mismatch>" >&2
	echo "       apply-quote-verdict-to-check-core.sh commit <repo> <run-id> <job-id>" >&2
	exit 1
}

require_repo() {
	case "${1:-}" in
	*..*|*/*/*|''|/*|*[!A-Za-z0-9_./-]*)
		echo "FAIL: repo must be owner/name, got: ${1:-}" >&2
		exit 1
		;;
	*/*) ;;
	*)
		echo "FAIL: repo must be owner/name, got: ${1:-}" >&2
		exit 1
		;;
	esac
}

require_digits() {
	case "${1:-}" in
	''|*[!0-9]*)
		echo "FAIL: expected numeric $2, got: ${1:-}" >&2
		exit 1
		;;
	esac
}

emit_plan() {
	rm -f quote-recheck-marker.txt
	if [ "$1" = "true" ]; then
		printf 'quote-only check-core rerun\n' > quote-recheck-marker.txt
		printf 'rerun=true run_id=%s job_id=%s next_attempt=%s\n' "$2" "$3" "$4"
		if [ -n "${GITHUB_OUTPUT:-}" ]; then
			{
				printf 'rerun=true\n'
				printf 'run_id=%s\n' "$2"
				printf 'job_id=%s\n' "$3"
				printf 'next_attempt=%s\n' "$4"
			} >> "$GITHUB_OUTPUT"
		fi
		return 0
	fi
	printf 'rerun=false\n'
	if [ -n "${GITHUB_OUTPUT:-}" ]; then
		printf 'rerun=false\n' >> "$GITHUB_OUTPUT"
	fi
}

decide() {
	conclusion=$1
	failed=$2
	verdict=$3
	if [ "$conclusion" = "failure" ] && [ "$failed" = "$QUOTE_STEP" ] && [ "$verdict" = "match" ]; then
		return 0
	fi
	if [ "$conclusion" = "success" ] && [ "$verdict" = "mismatch" ]; then
		return 0
	fi
	return 1
}

cmd_plan() {
	if [ "$#" -ne 3 ]; then
		usage
	fi
	REPO=$1
	HEAD_SHA=$2
	verdict=$3
	require_repo "$REPO"
	case "$HEAD_SHA" in
	''|*[!0-9a-fA-F]*)
		echo "FAIL: head sha must be hex, got: $HEAD_SHA" >&2
		exit 1
		;;
	esac
	case "$verdict" in
	match|mismatch) ;;
	*)
		echo "FAIL: verdict must be match or mismatch, got: $verdict" >&2
		exit 1
		;;
	esac

	i=0
	while [ "$i" -lt "$MAX_POLLS" ]; do
		i=$((i + 1))
		run_line=$("$GH_BIN" api "repos/${REPO}/actions/workflows/ci.yml/runs?head_sha=${HEAD_SHA}&event=pull_request&per_page=5" --jq "$RUN_JQ") || {
			echo "FAIL: listing CI runs failed" >&2
			exit 1
		}
		if [ "$run_line" = "none" ]; then
			echo "poll ${i}/${MAX_POLLS}: no pull_request CI run for ${HEAD_SHA} yet" >&2
			sleep "$SLEEP_SECS"
			continue
		fi
		run_id=$(printf '%s\n' "$run_line" | awk -F '\t' '{print $1}')
		run_attempt=$(printf '%s\n' "$run_line" | awk -F '\t' '{print $3}')
		require_digits "$run_id" "run id"
		require_digits "$run_attempt" "run attempt"

		job_line=$("$GH_BIN" api "repos/${REPO}/actions/runs/${run_id}/jobs?filter=latest&per_page=100" --jq "$JOB_JQ") || {
			echo "FAIL: listing check-core job failed" >&2
			exit 1
		}
		if [ "$job_line" = "absent" ]; then
			echo "poll ${i}/${MAX_POLLS}: check-core not in run ${run_id} yet" >&2
			sleep "$SLEEP_SECS"
			continue
		fi
		job_id=$(printf '%s\n' "$job_line" | awk -F '\t' '{print $1}')
		job_status=$(printf '%s\n' "$job_line" | awk -F '\t' '{print $2}')
		conclusion=$(printf '%s\n' "$job_line" | awk -F '\t' '{print $3}')
		failed=$(printf '%s\n' "$job_line" | awk -F '\t' '{print $4}')
		require_digits "$job_id" "job id"
		if [ "$job_status" != "completed" ]; then
			echo "poll ${i}/${MAX_POLLS}: check-core phase=${job_status}" >&2
			sleep "$SLEEP_SECS"
			continue
		fi

		if decide "$conclusion" "$failed" "$verdict"; then
			next=$((run_attempt + 1))
			echo "quote verdict ${verdict} disagrees with check-core conclusion=${conclusion}; rerun job ${job_id} as attempt ${next}" >&2
			emit_plan true "$run_id" "$job_id" "$next"
			return 0
		fi
		echo "check-core conclusion=${conclusion} failed_steps=${failed:-<none>} verdict=${verdict}; not rerunning" >&2
		emit_plan false
		return 0
	done
	echo "FAIL: timed out waiting for check-core on ${HEAD_SHA}" >&2
	exit 1
}

cmd_commit() {
	if [ "$#" -ne 3 ]; then
		usage
	fi
	REPO=$1
	RUN_ID=$2
	JOB_ID=$3
	require_repo "$REPO"
	require_digits "$RUN_ID" "run id"
	require_digits "$JOB_ID" "job id"

	i=0
	while [ "$i" -lt "$MAX_POLLS" ]; do
		i=$((i + 1))
		st=$("$GH_BIN" api "repos/${REPO}/actions/runs/${RUN_ID}" --jq '.status') || {
			echo "FAIL: reading CI run ${RUN_ID} failed" >&2
			exit 1
		}
		if [ "$st" != "completed" ]; then
			echo "poll ${i}/${MAX_POLLS}: workflow run ${RUN_ID} phase=${st}; waiting to rerun check-core" >&2
			sleep "$SLEEP_SECS"
			continue
		fi
		if "$GH_BIN" api -X POST "repos/${REPO}/actions/jobs/${JOB_ID}/rerun" >/dev/null; then
			echo "reran job ${JOB_ID}"
			return 0
		fi
		echo "poll ${i}/${MAX_POLLS}: rerun of job ${JOB_ID} was rejected; retrying" >&2
		sleep "$SLEEP_SECS"
	done
	echo "FAIL: timed out rerunning check-core job ${JOB_ID} on run ${RUN_ID}" >&2
	exit 1
}

if [ "$#" -lt 1 ]; then
	usage
fi
mode=$1
shift
case "$mode" in
plan) cmd_plan "$@" ;;
commit) cmd_commit "$@" ;;
*) usage ;;
esac
