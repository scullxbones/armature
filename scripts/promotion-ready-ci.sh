#!/usr/bin/env bash
# promotion-ready CI entry (ADR 0026 N1, issue #480).
#
# pull_request: HEAD must be github.event.pull_request.head.sha. GITHUB_SHA on
# that event is the synthetic merge commit and is not the delivery tip.
# Any other event (push to main, workflow_call): log N/A and exit 0.
#
# ID-scoped deliveries are derived by `arm promotion-ready scope` from ops
# state (done-leaf tips in boundary..head). The PR body is not read.
# Normative order: record the assessment for (base, head), arm push-ops,
# then push the branch. This job refetches _armature until that assessment
# is visible or PROMOTION_OPS_WAIT_SECONDS (default 45) elapses. It does not
# refetch after `arm gate run full`.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ARM_BIN="${ARM_BIN:-${ROOT}/bin/arm}"

die() {
	echo "promotion-ready-ci: $*" >&2
	exit 1
}

scope_kind() {
	printf '%s\n' "$1" | sed -n 's/^promotion-scope: kind=\([^ ]*\).*/\1/p' | head -n 1
}

scope_satisfied() {
	local out="$1"
	local kind line assessed any
	kind="$(scope_kind "$out")"
	[[ "$kind" == "scoped" ]] || return 1
	any=0
	while IFS= read -r line; do
		[[ -n "$line" ]] || continue
		any=1
		assessed="$(printf '%s\n' "$line" | sed -n 's/^promotion-scope: delivery base=[^ ]* issues=[^ ]* assessed=\([^ ]*\).*/\1/p')"
		[[ "$assessed" == "true" ]] || return 1
	done <<EOF
$(printf '%s\n' "$out" | grep '^promotion-scope: delivery ' || true)
EOF
	[[ "$any" -eq 1 ]]
}

refresh_ops() {
	git fetch --no-tags origin "+refs/heads/_armature:refs/remotes/origin/_armature"
	local ops
	ops="$(git config --get armature.ops-worktree-path)"
	[[ -n "$ops" ]] || die "armature.ops-worktree-path is unset after bootstrap"
	if git -C "$ops" -c core.hooksPath=/dev/null merge --ff-only origin/_armature; then
		return 0
	fi
	git -C "$ops" -c core.hooksPath=/dev/null rebase origin/_armature
}

if [[ "${PROMOTION_READY_CI_SKIP_PROOF:-}" != "1" ]]; then
	bash "$ROOT/scripts/promotion-ready-pr-proof.sh"
fi

event="${GITHUB_EVENT_NAME:-local}"
if [[ "$event" != "pull_request" ]]; then
	echo "promotion-ready: N/A on ${event} (no pull_request delivery; GITHUB_SHA is not evaluated)"
	exit 0
fi

head="${PROMOTION_HEAD:-}"
boundary="${PROMOTION_BASE_SHA:-}"
boundary_ref="${PROMOTION_BASE_REF:-}"
[[ -n "$head" ]] || die "PROMOTION_HEAD is empty; refusing to evaluate GITHUB_SHA"
[[ -n "$boundary" ]] || die "PROMOTION_BASE_SHA is empty"
[[ -n "$boundary_ref" ]] || die "PROMOTION_BASE_REF is empty"
case "$boundary_ref" in
	-* | *[!A-Za-z0-9._/-]* | *..*)
		die "refusing base ref ${boundary_ref}"
		;;
esac

cd "$ROOT"
[[ -x "$ARM_BIN" ]] || make -C "$ROOT" build
[[ -x "$ARM_BIN" ]] || die "arm binary not executable at $ARM_BIN"

actual="$(git rev-parse HEAD)"
if [[ "$actual" != "$head" ]]; then
	die "HEAD ${actual} != pull_request head ${head} (GITHUB_SHA is the merge commit on pull_request; this check is bound to head.sha)"
fi
echo "promotion-ready: bound to pull_request head ${head}; GITHUB_SHA=${GITHUB_SHA:-unset} is not the delivery tip"

git fetch --no-tags origin "+refs/heads/${boundary_ref}:refs/remotes/origin/${boundary_ref}"
if [[ "$boundary_ref" != "main" ]]; then
	git fetch --no-tags origin "+refs/heads/main:refs/remotes/origin/main"
fi
git fetch --no-tags origin "+refs/heads/_armature:refs/remotes/origin/_armature"
if ! git cat-file -e "${boundary}^{commit}" >/dev/null 2>&1; then
	git fetch --no-tags origin "$boundary"
fi

"$ARM_BIN" bootstrap --global
worker_id="ci-promo-${GITHUB_RUN_ID:-local}"
if [[ -n "${GITHUB_RUN_ATTEMPT:-}" ]]; then
	worker_id="${worker_id}-${GITHUB_RUN_ATTEMPT}"
fi
"$ARM_BIN" worker-init --id "$worker_id" --reuse

wait_seconds="${PROMOTION_OPS_WAIT_SECONDS:-45}"
interval="${PROMOTION_OPS_WAIT_INTERVAL:-5}"
case "$wait_seconds" in
	'' | *[!0-9]*) die "PROMOTION_OPS_WAIT_SECONDS must be a non-negative integer" ;;
esac
case "$interval" in
	'' | *[!0-9]*) die "PROMOTION_OPS_WAIT_INTERVAL must be a non-negative integer" ;;
esac

deadline=$((SECONDS + wait_seconds))
scope_out=""
while true; do
	refresh_ops
	scope_out="$("$ARM_BIN" promotion-ready scope --format human \
		--head "$head" --boundary "$boundary" --boundary-ref "$boundary_ref")"
	printf '%s\n' "$scope_out"
	if scope_satisfied "$scope_out"; then
		break
	fi
	if [[ "$SECONDS" -ge "$deadline" ]]; then
		echo "promotion-ready: ops wait elapsed (${wait_seconds}s)"
		break
	fi
	if [[ "$interval" -eq 0 ]]; then
		break
	fi
	sleep "$interval"
done

kind="$(scope_kind "$scope_out")"
if [[ "$kind" == "no-scope" ]]; then
	echo "promotion-ready: no ID-scoped delivery; exiting 0"
	exit 0
fi
[[ "$kind" == "scoped" ]] || die "unrecognized scope kind: ${kind:-empty}"

if ! command -v rg >/dev/null 2>&1; then
	dest="${HOME}/.local/bin"
	mkdir -p "$dest"
	make -C "$ROOT" install-rg-release DEST="$dest"
	export PATH="${dest}:${PATH}"
fi
export CHECK_INSTALL_TOOLS=1
# Evidence is appended locally and is not pushed. Do not refetch _armature
# after this command.
"$ARM_BIN" gate run full

deliveries="$(printf '%s\n' "$scope_out" | sed -n 's/^promotion-scope: delivery base=\([^ ]*\) issues=\([^ ]*\) assessed=\([^ ]*\).*/\1 \2 \3/p')"
[[ -n "$deliveries" ]] || die "scoped result had no delivery lines"
while read -r base issues assessed; do
	[[ -n "${base:-}" ]] || continue
	echo "promotion-ready: checking base ${base} issues ${issues} assessed=${assessed}"
	set -- "$ARM_BIN" promotion-ready --format human --base "$base" --head "$head"
	if [[ -n "${GITHUB_RUN_ID:-}" ]]; then
		set -- "$@" --actions-run-id "$GITHUB_RUN_ID"
	fi
	old_ifs="$IFS"
	IFS=','
	for id in $issues; do
		[[ -n "$id" ]] || continue
		set -- "$@" --issue "$id"
	done
	IFS="$old_ifs"
	"$@"
done <<EOF
$deliveries
EOF

echo "promotion-ready: all deliveries passed at ${head}"
