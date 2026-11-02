#!/usr/bin/env bash
# Compare coverage percentages quoted in a PR body against a coverage-check log.
#
# Usage: pr-coverage-quote-check.sh <body-file> <coverage-log>
#
# Soft-pass (exit 0) when the body has no coverage quote patterns — docs-only
# / empty bodies are fine. Fail when the body quotes cmd and/or internal %
# that disagree with the log lines produced by coverage-check.sh:
#   cmd coverage: 83.79%
#   internal coverage: 86.07%
#
# Accepted body shapes (non-exhaustive):
#   cmd coverage: 83.75%
#   internal coverage: 86.08%
#   **internal 86.08%**, cmd 83.75%
#   | check-core coverage | **internal 86.08%**, cmd 83.75% |
#
# Bash 3.2-safe. No network / gh API — body and log are plain files (CI can
# feed GITHUB_EVENT_PATH via jq; agents can tee make check-job JOB=check-core).

set -euo pipefail

BODY=${1:?usage: pr-coverage-quote-check.sh <body-file> <coverage-log>}
LOG=${2:?usage: pr-coverage-quote-check.sh <body-file> <coverage-log>}

if [ ! -f "$BODY" ]; then
	echo "FAIL: body file not found: $BODY" >&2
	exit 1
fi
if [ ! -f "$LOG" ]; then
	echo "FAIL: coverage log not found: $LOG" >&2
	exit 1
fi

# Extract log percentages (must be present when comparing).
LOG_CMD=$(sed -n 's/^cmd coverage: \([0-9][0-9]*\.[0-9][0-9]*\)%$/\1/p' "$LOG" | head -n 1)
LOG_INT=$(sed -n 's/^internal coverage: \([0-9][0-9]*\.[0-9][0-9]*\)%$/\1/p' "$LOG" | head -n 1)

# Collect body quotes in one awk pass (no shared-stdin multi-sed).
# Prefer "cmd coverage:" / "internal coverage:"; also accept "cmd N.NN%" /
# "internal N.NN%" on a line that mentions coverage (table / bold forms).
EVAL=$(awk '
function take_pct(s,   m) {
	if (match(s, /[0-9]+\.[0-9]+%/)) {
		return substr(s, RSTART, RLENGTH-1)
	}
	return ""
}
{
	line = $0
	low = tolower(line)
	# Explicit "cmd coverage: N.NN%" / "internal coverage: N.NN%"
	if (cmd == "" && match(low, /cmd[[:space:]]+coverage:[[:space:]]*[0-9]+\.[0-9]+%/)) {
		cmd = take_pct(substr(line, RSTART, RLENGTH))
	}
	if (intp == "" && match(low, /internal[[:space:]]+coverage:[[:space:]]*[0-9]+\.[0-9]+%/)) {
		intp = take_pct(substr(line, RSTART, RLENGTH))
	}
	# Short forms: "internal 86.08%" / "cmd 83.75%" (optional ** around token).
	# Also covers table cells that mention coverage elsewhere on the line.
	if (intp == "" && match(low, /internal[[:space:]]+[0-9]+\.[0-9]+%/)) {
		intp = take_pct(substr(line, RSTART, RLENGTH))
	}
	if (cmd == "" && match(low, /(^|[^a-z])cmd[[:space:]]+[0-9]+\.[0-9]+%/)) {
		cmd = take_pct(substr(line, RSTART, RLENGTH))
	}
}
END {
	printf "cmd=%s\nint=%s\n", cmd, intp
}' "$BODY")
BODY_CMD=$(printf '%s\n' "$EVAL" | sed -n 's/^cmd=//p' | head -n 1)
BODY_INT=$(printf '%s\n' "$EVAL" | sed -n 's/^int=//p' | head -n 1)

if [ -z "${BODY_CMD}" ] && [ -z "${BODY_INT}" ]; then
	echo "OK: no coverage quotes in body (soft-pass)"
	exit 0
fi

if [ -z "${LOG_CMD}" ] || [ -z "${LOG_INT}" ]; then
	echo "FAIL: coverage log missing cmd/internal percentage lines" >&2
	echo "  log: $LOG" >&2
	exit 1
fi

FAIL=0
if [ -n "${BODY_CMD}" ] && [ "${BODY_CMD}" != "${LOG_CMD}" ]; then
	echo "FAIL: body quotes cmd coverage ${BODY_CMD}% but log has ${LOG_CMD}%" >&2
	FAIL=1
fi
if [ -n "${BODY_INT}" ] && [ "${BODY_INT}" != "${LOG_INT}" ]; then
	echo "FAIL: body quotes internal coverage ${BODY_INT}% but log has ${LOG_INT}%" >&2
	FAIL=1
fi

if [ "$FAIL" -ne 0 ]; then
	exit 1
fi

echo "OK: PR body coverage quotes match log (cmd ${LOG_CMD}%, internal ${LOG_INT}%)"
exit 0
