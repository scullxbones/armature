#!/usr/bin/env bash
# Hermetic proof that promotion-ready fails closed on a bad delivery and
# passes on a conforming one, and that no-scope and stacked ranges finish
# with an explicit rule. Run by scripts/promotion-ready-ci.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

log="$(mktemp "${TMPDIR:-/tmp}/promotion-ready-pr-proof.XXXXXX")"
cleanup() { rm -f "$log"; }
trap cleanup EXIT

echo "promotion-ready-pr-proof: fixture cases"
go test ./cmd/armature/ -count=1 -timeout 20m -run 'TestPromotionReadyPRProof_REQ_480' -v | tee "$log"

needles='
PROOF empty-contract
PROMOTION-READY-1
empty contract
PROOF missing-assessment
no valid assessment
PROOF stale-head
PROOF missing-ci-evidence
missing CI gate evidence
PROOF conforming
promotion-ready: ok
PROOF no-scope
not a bypass of R2/R3/R4
PROOF stacked
stacked base ref
PROOF ordering
'
printf '%s\n' "$needles" | while IFS= read -r needle; do
	[[ -n "$needle" ]] || continue
	if ! grep -F -q -- "$needle" "$log"; then
		echo "promotion-ready-pr-proof: missing log line: $needle" >&2
		exit 1
	fi
	echo "promotion-ready-pr-proof: saw ${needle}"
done

echo "PROOF push-na:"
na="$(PROMOTION_READY_CI_SKIP_PROOF=1 GITHUB_EVENT_NAME=push bash "$ROOT/scripts/promotion-ready-ci.sh")"
printf '%s\n' "$na"
grep -F "promotion-ready: N/A" <<<"$na" >/dev/null
grep -F "GITHUB_SHA is not evaluated" <<<"$na" >/dev/null

echo "PROOF head-mismatch:"
actual="$(git rev-parse HEAD)"
first="${actual:0:1}"
if [[ "$first" == "0" ]]; then
	repl=1
else
	repl=0
fi
wrong="${repl}${actual:1}"
set +e
mismatch="$(
	PROMOTION_READY_CI_SKIP_PROOF=1 \
		GITHUB_EVENT_NAME=pull_request \
		PROMOTION_HEAD="$wrong" \
		PROMOTION_BASE_SHA="$actual" \
		PROMOTION_BASE_REF=main \
		bash "$ROOT/scripts/promotion-ready-ci.sh" 2>&1
)"
code=$?
set -e
printf '%s\n' "$mismatch"
[[ "$code" -ne 0 ]]
grep -F "HEAD" <<<"$mismatch" >/dev/null
grep -F "head.sha" <<<"$mismatch" >/dev/null

echo "promotion-ready-pr-proof: OK"
