#!/usr/bin/env bash
# Fail commits on RANGE whose subjects lack an issue-id scope.
# Keep the match rules aligned with commitref.subjectHasIssueScope.
#
# Usage: scripts/check-commit-subjects.sh [RANGE]
#        RANGE defaults to origin/main..HEAD

set -euo pipefail

RANGE="${1:-origin/main..HEAD}"
TYPES='feat|fix|refactor|test|docs|style|polish|ci'

if ! command -v rg >/dev/null 2>&1; then
    echo "FAIL: rg is required for check-commit-subjects" >&2
    exit 1
fi

strip_squash_suffix() {
    printf '%s\n' "$1" | sed -E 's/ \(#[0-9]+\)$//'
}

subject_has_issue_scope() {
    local subject="$1"
    local parent_count="$2"
    subject=$(strip_squash_suffix "$subject")
    if printf '%s\n' "$subject" | rg -q "^($TYPES)\([^)]+\)!?:[ \t]+\S"; then
        return 0
    fi
    if (( parent_count >= 2 )) && printf '%s\n' "$subject" | rg -q '^merge:[ \t]+\S+[ \t]+\S'; then
        return 0
    fi
    return 1
}

if ! git rev-parse --verify --quiet origin/main >/dev/null; then
    echo "FAIL: origin/main is missing; fetch it before check-commit-subjects" >&2
    exit 1
fi

if ! git rev-list --count "$RANGE" >/dev/null; then
    echo "FAIL: cannot list commits in $RANGE" >&2
    exit 1
fi

count=$(git rev-list --count "$RANGE")
if [[ "$count" -eq 0 ]]; then
    echo "check-commit-subjects: no commits in $RANGE; skip"
    exit 0
fi

echo "check-commit-subjects: checking $RANGE ($count commit(s))"

failed=0
while IFS=$'\t' read -r sha parents subject; do
    parent_count=0
    if [[ -n "$parents" ]]; then
        # shellcheck disable=SC2086
        set -- $parents
        parent_count=$#
    fi
    if subject_has_issue_scope "$subject" "$parent_count"; then
        continue
    fi
    echo "FAIL: unscoped commit subject: $sha $subject" >&2
    failed=1
done < <(git log --format='%h%x09%P%x09%s' "$RANGE")

if [[ "$failed" -ne 0 ]]; then
    echo "FAIL: commit subjects must be type(ISSUE-ID): description (or merge: ISSUE-ID description with two parents)" >&2
    exit 1
fi

echo "OK: commit subjects on $RANGE have issue-id scope"
