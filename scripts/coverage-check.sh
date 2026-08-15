#!/bin/bash
# Per-tree statement coverage threshold check.
#
# Reads a `go test -coverprofile` profile (default: coverage.out in the repo
# root passed as $1, default ".") and fails if either tree is under its
# threshold: cmd/** >= 83, internal/** >= 86. Both percentages print
# unconditionally so drift is visible even when passing. Thresholds recorded
# in docs/adr/0015-recalibrate-mutation-and-coverage-gates.md.
#
# Usage: coverage-check.sh [repo-root] [profile]. `repo-root` defaults to
# `.`; `profile` defaults to `<repo-root>/coverage.out`.
#
# Factored out of the Makefile `coverage-check` target so the threshold logic
# is independently testable (see scripts/test_coverage_check.sh). The
# `make coverage-check` target's own output and exit code are unchanged.

set -euo pipefail

REPO_ROOT="${1:-.}"
PROFILE="${2:-$REPO_ROOT/coverage.out}"

if [ ! -f "$PROFILE" ]; then
    echo "FAIL: $PROFILE not found; run 'make coverage' first"
    exit 1
fi

awk 'NR>1{n=$2;c=$3;
	if($0 ~ /armature\/cmd\//){ct+=n; if(c>0) cc+=n}
	if($0 ~ /armature\/internal\//){it+=n; if(c>0) ic+=n}}
END{
	cmd_pct = (ct>0) ? 100*cc/ct : 0;
	int_pct = (it>0) ? 100*ic/it : 0;
	printf "cmd coverage: %.2f%%\n", cmd_pct;
	printf "internal coverage: %.2f%%\n", int_pct;
	fail=0;
	if (cmd_pct < 83) { printf "FAIL: cmd coverage %.2f%% is below 83%% threshold (short by %.2f points)\n", cmd_pct, 83-cmd_pct; fail=1 }
	if (int_pct < 86) { printf "FAIL: internal coverage %.2f%% is below 86%% threshold (short by %.2f points)\n", int_pct, 86-int_pct; fail=1 }
	if (ct==0) { print "FAIL: no coverage lines matched armature/cmd/ — tree missing from profile"; fail=1 }
	if (it==0) { print "FAIL: no coverage lines matched armature/internal/ — tree missing from profile"; fail=1 }
	exit fail
}' "$PROFILE"
