#!/usr/bin/env bash
# Behavioral + mutant-kill tests for scripts/wait-coverage-check-artifact.sh.
#
# 1. Timeout → exit 1 with FAIL: timed out (kills exit 1→0 mutant)
# 2. PATH-shadow self-check required in edit workflow (kills removal mutant)
# 3. Selector exit 2 keeps polling; selector exit 1 aborts (no || true)

set -euo pipefail

REPO_ROOT="${1:-.}"
WAIT="$REPO_ROOT/scripts/wait-coverage-check-artifact.sh"
EDIT_WF="$REPO_ROOT/.github/workflows/pr-coverage-quotes-on-edit.yml"
SELECTOR="$REPO_ROOT/scripts/select-coverage-check-artifact-id.sh"

FAILURES=0
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

pass() { echo "  PASS${1:+ ($1)}"; }
fail() { echo "  FAIL: $1"; FAILURES=$((FAILURES + 1)); }

# Stub gh: always return empty artifact list + fixed repo id.
STUB_GH="$WORKDIR/gh"
cat >"$STUB_GH" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
# gh api repos/X --jq .id
if [[ "${1:-}" == "api" && "${2:-}" == repos/* && "${3:-}" == "--jq" && "${4:-}" == ".id" ]]; then
	echo 42
	exit 0
fi
# gh api …/artifacts?name=…
if [[ "${1:-}" == "api" && "${2:-}" == *"/actions/artifacts"* ]]; then
	echo '{"artifacts":[]}'
	exit 0
fi
echo "unexpected gh argv: $*" >&2
exit 99
EOF
chmod +x "$STUB_GH"

echo "Test 1: REQ timeout exits 1 (not 0)..."
set +e
OUT=$(
	GH_BIN="$STUB_GH" SELECTOR="$SELECTOR" MAX_POLLS=2 SLEEP_SECS=0 \
		"$WAIT" scullxbones/armature deadbeef "$WORKDIR/out.log" 2>&1
)
ST=$?
set -e
if [[ $ST -ne 1 ]]; then
	fail "expected exit 1 on timeout, got $ST"
	echo "$OUT"
elif ! grep -q 'FAIL: timed out waiting for coverage-check artifact' <<<"$OUT"; then
	fail "expected timed-out FAIL message"
	echo "$OUT"
else
	pass
fi

echo "Test 2: kill mutant timeout exit 1→0..."
MUT="$WORKDIR/wait_exit0.sh"
python3 - "$WAIT" "$MUT" <<'PY'
import sys
src, dst = sys.argv[1], sys.argv[2]
t = open(src).read()
old = 'echo "FAIL: timed out waiting for coverage-check artifact ${name}" >&2\n\texit 1'
new = 'echo "FAIL: timed out waiting for coverage-check artifact ${name}" >&2\n\texit 0'
if old not in t:
    raise SystemExit("timeout exit site not found")
open(dst, "w").write(t.replace(old, new, 1))
PY
chmod +x "$MUT"
set +e
MOUT=$(
	GH_BIN="$STUB_GH" SELECTOR="$SELECTOR" MAX_POLLS=2 SLEEP_SECS=0 \
		"$MUT" scullxbones/armature deadbeef "$WORKDIR/out2.log" 2>&1
)
MST=$?
ROUT=$(
	GH_BIN="$STUB_GH" SELECTOR="$SELECTOR" MAX_POLLS=2 SLEEP_SECS=0 \
		"$WAIT" scullxbones/armature deadbeef "$WORKDIR/out3.log" 2>&1
)
RST=$?
set -e
if [[ $RST -ne 1 ]]; then
	fail "oracle timeout must exit 1"
elif [[ $MST -eq 0 ]]; then
	pass "killed timeout exit→0 (mutant false-pass; oracle fail)"
else
	fail "mutant did not false-pass on timeout (mst=$MST)"
	echo "$MOUT"
fi

echo "Test 3: kill mutant removing PATH-shadow self-check from edit workflow..."
if [[ ! -f "$EDIT_WF" ]]; then
	fail "missing edit workflow"
else
	# Behavioral lock: workflow must invoke the self-check script by name.
	if ! rg -q 'ci-path-shadow-selfcheck\.sh' "$EDIT_WF"; then
		fail "PATH-shadow self-check missing from edit workflow"
	else
		# Mutant: drop the self-check step; a grep-based oracle must notice.
		MUT_WF="$WORKDIR/edit_no_shadow.yml"
		# Remove the self-check step block (name + run lines).
		python3 - "$EDIT_WF" "$MUT_WF" <<'PY'
import re, sys
src, dst = sys.argv[1], sys.argv[2]
t = open(src).read()
mut, n = re.subn(
    r"\n      - name: PATH-shadow self-check\n        run: bash scripts/ci-path-shadow-selfcheck\.sh\n",
    "\n",
    t,
    count=1,
)
if n != 1:
    raise SystemExit(f"PATH-shadow self-check site not found (n={n})")
open(dst, "w").write(mut)
PY
		if rg -q 'ci-path-shadow-selfcheck\.sh' "$MUT_WF"; then
			fail "mutant still has PATH-shadow self-check"
		elif ! rg -q 'ci-path-shadow-selfcheck\.sh' "$EDIT_WF"; then
			fail "oracle workflow unexpectedly lacks PATH-shadow self-check"
		else
			pass "killed PATH-shadow self-check removal"
		fi
	fi
fi

echo "Test 4: selector hard-fail aborts poll (no || true)..."
# Stub gh returns multi-match JSON so selector exits 1.
STUB2="$WORKDIR/gh_multi"
cat >"$STUB2" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "api" && "${2:-}" == repos/* && "${3:-}" == "--jq" ]]; then
	echo 42; exit 0
fi
if [[ "${1:-}" == "api" && "${2:-}" == *"/actions/artifacts"* ]]; then
	cat <<'JSON'
{"artifacts":[
  {"id":1,"expired":false,"workflow_run":{"head_sha":"deadbeef","repository_id":42,"head_repository_id":42}},
  {"id":2,"expired":false,"workflow_run":{"head_sha":"deadbeef","repository_id":42,"head_repository_id":42}}
]}
JSON
	exit 0
fi
exit 99
EOF
chmod +x "$STUB2"
set +e
HOUT=$(
	GH_BIN="$STUB2" SELECTOR="$SELECTOR" MAX_POLLS=3 SLEEP_SECS=0 \
		"$WAIT" scullxbones/armature deadbeef "$WORKDIR/out4.log" 2>&1
)
HST=$?
set -e
if [[ $HST -ne 1 ]]; then
	fail "expected hard-fail abort exit 1, got $HST"
	echo "$HOUT"
elif ! grep -q 'FAIL:' <<<"$HOUT"; then
	fail "expected FAIL from hard selector abort"
	echo "$HOUT"
else
	pass
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
	echo "All wait-coverage-check-artifact tests passed"
	exit 0
else
	echo "FAIL: $FAILURES wait-coverage-check-artifact test(s) failed"
	exit 1
fi
