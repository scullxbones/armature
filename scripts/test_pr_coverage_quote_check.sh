#!/usr/bin/env bash
# Fixture + mutant-kill tests for scripts/pr-coverage-quote-check.sh.
#
# REQ boundaries:
#   1. exact match → pass
#   2. 83.90 vs 83.80 (|diff|=0.10) → pass
#   3. body 0.11 BELOW log → fail (kills abs-removed + one-sided mutants)
#   4. |diff|=0.11 above → fail (kills <=→< at boundary via sweep companion)
#   5. missing quote / empty / cmd-only → fail (no soft-pass)
#   6. wrong-package + 'foo 86.09%' → fail (kills any-word internal short-form)
#   7. internal 50 vs 86.09 → fail
#   8. sweep 80.00–100.00: every exact-0.10 pair passes both dirs; 0.11 fails both
#   9. mutants: <=→< ; abs removed ; internal short-form any-word — each killed
#
# Wired into make check-static via test-pr-coverage-quote-check.

set -euo pipefail

REPO_ROOT="${1:-.}"
SCRIPT="$REPO_ROOT/scripts/pr-coverage-quote-check.sh"

FAILURES=0
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

LOG="$WORKDIR/coverage.log"
{
	echo "cmd coverage: 83.80%"
	echo "internal coverage: 86.09%"
} >"$LOG"

run_check() {
	local body="$1"
	local script="${2:-$SCRIPT}"
	local log="${3:-$LOG}"
	set +e
	OUTPUT=$("$script" "$body" "$log" 2>&1)
	STATUS=$?
	set -e
}

pass() { echo "  PASS${1:+ ($1)}"; }
fail() {
	echo "  FAIL: $1"
	echo "$OUTPUT"
	FAILURES=$((FAILURES + 1))
}

# ----------------------------------------------------------------------------
echo "Test 1: exact match passes..."
MATCH_BODY="$WORKDIR/match.md"
cat >"$MATCH_BODY" <<'EOF'
## Coverage
cmd coverage: 83.80%
internal coverage: 86.09%
EOF
run_check "$MATCH_BODY"
if [[ $STATUS -ne 0 ]]; then fail "expected exit 0, got $STATUS"
elif ! grep -q "OK: PR body coverage quotes within 0.10" <<<"$OUTPUT"; then fail "expected OK message"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 2: REQ 83.90 vs 83.80 (|diff|=0.10) passes..."
TOL10="$WORKDIR/tol10.md"
cat >"$TOL10" <<'EOF'
cmd coverage: 83.90%
internal coverage: 86.09%
EOF
run_check "$TOL10"
if [[ $STATUS -ne 0 ]]; then fail "expected exit 0 for |diff|=0.10"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 3: REQ body 0.11 BELOW log fails..."
BELOW="$WORKDIR/below.md"
cat >"$BELOW" <<'EOF'
cmd coverage: 83.69%
internal coverage: 86.09%
EOF
run_check "$BELOW"
if [[ $STATUS -eq 0 ]]; then fail "expected fail for body 0.11 below log"
elif ! grep -q "FAIL: body quotes cmd coverage 83.69%" <<<"$OUTPUT"; then fail "expected cmd below-log message"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 4: REQ |diff|=0.11 above fails..."
ABOVE="$WORKDIR/above.md"
cat >"$ABOVE" <<'EOF'
cmd coverage: 83.91%
internal coverage: 86.09%
EOF
run_check "$ABOVE"
if [[ $STATUS -eq 0 ]]; then fail "expected fail for |diff|=0.11"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 5: REQ missing quotes fail (no soft-pass)..."
EMPTY_BODY="$WORKDIR/empty.md"
cat >"$EMPTY_BODY" <<'EOF'
Docs-only PR. No numbers here.
EOF
run_check "$EMPTY_BODY"
if [[ $STATUS -eq 0 ]]; then fail "expected fail for quote-less body"
elif ! grep -q "FAIL: PR body must quote both cmd and internal coverage percentages" <<<"$OUTPUT"; then fail "expected missing-quote message"
else pass; fi

CMD_ONLY="$WORKDIR/cmd_only.md"
cat >"$CMD_ONLY" <<'EOF'
cmd coverage: 83.80%
EOF
run_check "$CMD_ONLY"
if [[ $STATUS -eq 0 ]]; then fail "expected fail for cmd-only"
else pass "cmd-only"; fi

# ----------------------------------------------------------------------------
echo "Test 6: REQ wrong-package / foo 86.09% fails..."
WRONG="$WORKDIR/wrong.md"
cat >"$WRONG" <<'EOF'
cmd coverage: 83.80%
foo 86.09%
EOF
run_check "$WRONG"
if [[ $STATUS -eq 0 ]]; then fail "expected fail for foo 86.09% (not internal)"
elif ! grep -q "FAIL: PR body must quote both cmd and internal coverage percentages" <<<"$OUTPUT"; then fail "expected missing internal for foo short-form"
else pass "foo 86.09%"; fi

SCRIPTS_PKG="$WORKDIR/scripts_pkg.md"
cat >"$SCRIPTS_PKG" <<'EOF'
scripts coverage: 83.80%
mutate coverage: 86.09%
EOF
run_check "$SCRIPTS_PKG"
if [[ $STATUS -eq 0 ]]; then fail "expected fail for wrong-package quotes"
else pass "wrong-package"; fi

# ----------------------------------------------------------------------------
echo "Test 7: REQ internal 50 vs 86.09 fails..."
INT50="$WORKDIR/int50.md"
cat >"$INT50" <<'EOF'
cmd coverage: 83.80%
internal coverage: 50.00%
EOF
run_check "$INT50"
if [[ $STATUS -eq 0 ]]; then fail "expected fail for internal 50 vs 86.09"
elif ! grep -q "FAIL: body quotes internal coverage 50.00%" <<<"$OUTPUT"; then fail "expected internal mismatch message"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 8: REQ sweep 80.00–100.00 exact-0.10 pass / 0.11 fail both dirs..."
SWEEP_OUT="$WORKDIR/sweep.out"
# Generate pairs in awk; invoke the real within_tol logic by writing tiny bodies.
python3 - "$SCRIPT" "$WORKDIR" <<'PY' >"$SWEEP_OUT"
import subprocess, sys, tempfile, os
script, work = sys.argv[1], sys.argv[2]
# hundredths from 8000..10000 inclusive → 80.00..100.00
fails = []
checks = 0
for h in range(8000, 10001):
    base = h / 100.0
    for delta, expect_ok, direction in (
        (10, True, "above"),
        (10, True, "below"),
        (11, False, "above"),
        (11, False, "below"),
    ):
        if direction == "above":
            body_h = h + delta
        else:
            body_h = h - delta
        if body_h < 0 or body_h > 10000:
            continue
        body_pct = f"{body_h / 100.0:.2f}"
        log_pct = f"{base:.2f}"
        body = f"cmd coverage: {body_pct}%\ninternal coverage: {log_pct}%\n"
        log = f"cmd coverage: {log_pct}%\ninternal coverage: {log_pct}%\n"
        bf = os.path.join(work, "sweep_body.md")
        lf = os.path.join(work, "sweep_log.md")
        open(bf, "w").write(body)
        open(lf, "w").write(log)
        r = subprocess.run([script, bf, lf], capture_output=True, text=True)
        ok = r.returncode == 0
        checks += 1
        if ok != expect_ok:
            fails.append(f"base={log_pct} body_cmd={body_pct} dir={direction} delta={delta/100:.2f} expect_ok={expect_ok} got={ok} out={r.stdout!r}{r.stderr!r}")
            if len(fails) >= 5:
                break
    if len(fails) >= 5:
        break
if fails:
    print("FAIL")
    for f in fails:
        print(f)
    sys.exit(1)
print(f"PASS checks={checks}")
PY
SWEEP_STATUS=$?
if [[ $SWEEP_STATUS -ne 0 ]]; then
	echo "  FAIL: sweep"
	cat "$SWEEP_OUT"
	FAILURES=$((FAILURES + 1))
else
	pass "$(cat "$SWEEP_OUT")"
fi

# ----------------------------------------------------------------------------
echo "Test 9: kill mutants (<=→<, abs removed, internal any-word)..."
kill_mutant() {
	local name="$1"
	local mutant="$2"
	local body="$3"
	local log="${4:-$LOG}"
	local expect_status="$5" # mutant should get this wrong vs oracle: we expect mutant STATUS != oracle, specifically mutant should PASS when oracle FAILS or vice versa — we assert mutant fails the killing fixture (expect_status non-zero means mutant must incorrectly pass for kill proof... )
	# Convention: kill proof = mutant returns 0 on a fixture the real script fails (or returns non-zero on a fixture the real script passes).
	local mode="$6" # "false_pass" | "false_fail"
	set +e
	MOUT=$("$mutant" "$body" "$log" 2>&1)
	MST=$?
	ROUT=$("$SCRIPT" "$body" "$log" 2>&1)
	RST=$?
	set -e
	if [[ "$mode" == "false_pass" ]]; then
		# Real must fail; mutant must wrongly pass → killed when we detect that divergence and keep real failing.
		if [[ $RST -eq 0 ]]; then
			echo "  FAIL mutant $name: oracle unexpectedly passed"
			echo "$ROUT"
			FAILURES=$((FAILURES + 1))
			return
		fi
		if [[ $MST -ne 0 ]]; then
			echo "  FAIL mutant $name: mutant did not false-pass (not a live mutant on this fixture)"
			echo "$MOUT"
			FAILURES=$((FAILURES + 1))
			return
		fi
		pass "killed $name (mutant false-pass; oracle fail)"
	else
		# false_fail: real passes; mutant wrongly fails
		if [[ $RST -ne 0 ]]; then
			echo "  FAIL mutant $name: oracle unexpectedly failed"
			echo "$ROUT"
			FAILURES=$((FAILURES + 1))
			return
		fi
		if [[ $MST -eq 0 ]]; then
			echo "  FAIL mutant $name: mutant did not false-fail"
			echo "$MOUT"
			FAILURES=$((FAILURES + 1))
			return
		fi
		pass "killed $name (mutant false-fail; oracle pass)"
	fi
}

# Mutant 1: <= → <  in hundredths compare (exact 0.10 false-fails)
M1="$WORKDIR/m_lt.sh"
python3 - "$SCRIPT" "$M1" <<'PY'
import sys
src, dst = sys.argv[1], sys.argv[2]
t = open(src).read()
old = "exit (d <= tol) ? 0 : 1"
new = "exit (d < tol) ? 0 : 1"
if old not in t:
    raise SystemExit("mutant site <= not found")
open(dst, "w").write(t.replace(old, new, 1))
PY
chmod +x "$M1"
kill_mutant "<=→<" "$M1" "$TOL10" "$LOG" 1 "false_fail"

# Mutant 2: abs removed (one-sided); body 0.11 below false-passes
M2="$WORKDIR/m_noabs.sh"
python3 - "$SCRIPT" "$M2" <<'PY'
import sys
src, dst = sys.argv[1], sys.argv[2]
t = open(src).read()
# Remove the abs correction lines inside within_tol.
old = "\t\td = bh - lh\n\t\tif (d < 0) d = -d\n\t\texit (d <= tol) ? 0 : 1"
new = "\t\td = bh - lh\n\t\texit (d <= tol) ? 0 : 1"
if old not in t:
    raise SystemExit("mutant site abs not found")
open(dst, "w").write(t.replace(old, new, 1))
PY
chmod +x "$M2"
kill_mutant "abs-removed" "$M2" "$BELOW" "$LOG" 0 "false_pass"

# Mutant 3: internal short-form accepts any word
M3="$WORKDIR/m_anyword.sh"
python3 - "$SCRIPT" "$M3" <<'PY'
import sys
src, dst = sys.argv[1], sys.argv[2]
t = open(src).read()
old = "if (intp == \"\" && match(low, /(^|[^a-z])internal[[:space:]]+[0-9]+\\.[0-9]+%/)) {"
new = "if (intp == \"\" && match(low, /(^|[^a-z])[a-z]+[[:space:]]+[0-9]+\\.[0-9]+%/)) {"
if old not in t:
    raise SystemExit("mutant site internal short-form not found")
open(dst, "w").write(t.replace(old, new, 1))
PY
chmod +x "$M3"
kill_mutant "internal-any-word" "$M3" "$WRONG" "$LOG" 0 "false_pass"

# ----------------------------------------------------------------------------
echo "Test 10: table form still matches..."
TABLE_BODY="$WORKDIR/table.md"
cat >"$TABLE_BODY" <<'EOF'
| check-core coverage | **internal 86.09%**, cmd 83.80% |
EOF
run_check "$TABLE_BODY"
if [[ $STATUS -ne 0 ]]; then fail "expected exit 0 for table form"
else pass; fi

# ----------------------------------------------------------------------------
echo "Test 11: REQ A — stale push-event body fails; live body (edit) passes..."
# Models check-core reading a stale event payload vs edit recheck / gh api live body.
STALE="$WORKDIR/stale_event_body.md"
LIVE="$WORKDIR/live_api_body.md"
cat >"$STALE" <<'EOF'
cmd coverage: 99.99%
internal coverage: 99.99%
EOF
cat >"$LIVE" <<'EOF'
cmd coverage: 83.80%
internal coverage: 86.09%
EOF
run_check "$STALE"
if [[ $STATUS -eq 0 ]]; then fail "stale event body must fail against log"
else
	run_check "$LIVE"
	if [[ $STATUS -ne 0 ]]; then fail "live body after edit must pass against same log"
	else pass "stale fail → live/edit pass"; fi
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
	echo "All pr-coverage-quote-check tests passed"
	exit 0
else
	echo "FAIL: $FAILURES pr-coverage-quote-check test(s) failed"
	exit 1
fi
