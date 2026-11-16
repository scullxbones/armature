#!/usr/bin/env bash
# Host entry for ADR 0026 N1 (#481): run `arm land --pr` for one merged PR,
# or sweep recent merged PRs. No model, no extra secrets — GITHUB_TOKEN only
# (consumed by arm networkCmd as an ephemeral http.extraHeader; never written
# into remote URLs / git config by this script).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ARM_BIN="${ARM_BIN:-${ROOT}/bin/arm}"

die() {
	echo "arm-land-ci: $*" >&2
	exit 1
}

[[ -x "$ARM_BIN" ]] || die "arm binary not executable at $ARM_BIN"

land_one() {
	local pr="$1"
	[[ "$pr" =~ ^[0-9]+$ ]] || die "invalid PR number: ${pr}"
	echo "arm-land-ci: land --pr ${pr}"
	"$ARM_BIN" land --pr "$pr"
}

# Unit/fixture proof when invoked without a host event (local / check job).
if [[ "${ARM_LAND_PROOF_ONLY:-}" == "1" ]]; then
	cd "$ROOT"
	go test ./cmd/armature/ ./internal/adapters/ -count=1 -run 'REQ_481'
	echo "arm-land-ci: proof OK"
	exit 0
fi

EVENT_NAME="${EVENT_NAME:-${GITHUB_EVENT_NAME:-}}"
PR_NUMBER="${PR_NUMBER:-}"

case "$EVENT_NAME" in
pull_request|workflow_dispatch)
	if [[ -n "${PR_NUMBER}" ]]; then
		land_one "$PR_NUMBER"
		echo "arm-land-ci: OK"
		exit 0
	fi
	# workflow_dispatch with empty pr → fall through to sweep
	;;
esac

# Scheduled / dispatch sweep: attempt every recent merged PR. Do not abort the
# loop on the first failure — successful lands must still reach push-ops in the
# workflow. Exit non-zero after the loop so the job fails after publish.
command -v gh >/dev/null 2>&1 || die "gh is required for the merged-PR sweep"
mapfile -t prs < <(gh pr list --state merged --limit 30 --json number --jq '.[].number')
if [[ ${#prs[@]} -eq 0 ]]; then
	echo "arm-land-ci: no merged PRs to sweep"
	exit 0
fi
failed=0
failed_prs=()
for pr in "${prs[@]}"; do
	if land_one "$pr"; then
		continue
	fi
	echo "arm-land-ci: land --pr ${pr} failed (continuing sweep)" >&2
	failed=1
	failed_prs+=("$pr")
done
if [[ "$failed" -ne 0 ]]; then
	echo "arm-land-ci: sweep finished with failures: ${failed_prs[*]}" >&2
	exit 1
fi
echo "arm-land-ci: sweep OK (${#prs[@]} PRs)"
