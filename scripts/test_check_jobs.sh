#!/bin/bash
# Drift test for required-check fan-out (Design D).
#
# Source of job ids: `make print-check-jobs`. This script does not embed the
# membership target list. It fails when .github/workflows/ci.yml's make
# check-job jobs disagree with that table, when a leftover job named `check`
# exists, or when `make check` has prerequisites (which would let `make -j
# check` become in-process fan-out).
#
# Wired into `make check` via the `test-check-jobs` target.

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
CI_YML="$REPO_ROOT/.github/workflows/ci.yml"
MAKEFILE="$REPO_ROOT/Makefile"
SCRIPT="$REPO_ROOT/scripts/test_check_jobs.sh"

FAILURES=0

# Nested `make` from inside `make check-job JOB=…` inherits MAKEOVERRIDES
# (JOB=check-static). Clear that so empty/unknown JOB probes are real, and so
# this script cannot recurse into check-static.
run_make() {
    env -u MAKEFLAGS -u MAKEOVERRIDES -u MFLAGS make -C "$REPO_ROOT" -s "$@"
}

fail() {
    echo "  FAIL: $1"
    FAILURES=$((FAILURES + 1))
}

pass() {
    echo "  PASS: $1"
}

# ----------------------------------------------------------------------------
# Test 1: print-check-jobs is the source set of ids
# ----------------------------------------------------------------------------
echo "Test 1: make print-check-jobs emits one record per CHECK_JOBS id..."

PRINT_OUT=$(mktemp)
PRINT_ERR=$(mktemp)
IDS=()
set +e
run_make print-check-jobs >"$PRINT_OUT" 2>"$PRINT_ERR"
PRINT_STATUS=$?
set -e

if [[ $PRINT_STATUS -ne 0 ]]; then
    fail "print-check-jobs exited $PRINT_STATUS"
    cat "$PRINT_ERR"
else
    while IFS= read -r line; do
        case "$line" in
            job\ *)
                id="${line#job }"
                if [[ -z "$id" || "$id" == *" "* ]]; then
                    fail "print-check-jobs job line is not a single id: $line"
                else
                    IDS+=("$id")
                fi
                ;;
        esac
    done < "$PRINT_OUT"
    if [[ ${#IDS[@]} -eq 0 ]]; then
        fail "print-check-jobs printed no job records"
        echo "stdout:"
        cat "$PRINT_OUT"
    else
        UNIQUE=$(printf '%s\n' "${IDS[@]}" | sort -u | wc -l | tr -d ' ')
        if [[ "$UNIQUE" -ne "${#IDS[@]}" ]]; then
            fail "print-check-jobs repeated a job id"
        else
            pass "print-check-jobs ids: ${IDS[*]}"
        fi
    fi
fi
rm -f "$PRINT_OUT" "$PRINT_ERR"

# ----------------------------------------------------------------------------
# Test 2–5: ci.yml fan-out jobs vs the Makefile table
# ----------------------------------------------------------------------------
echo "Test 2: ci.yml make check-job jobs match print-check-jobs (no check, no needs)..."

PARSE_RESULT=$(python3 - "$CI_YML" "${IDS[@]+"${IDS[@]}"}" <<'PY'
import re
import sys
from pathlib import Path

ci_path = Path(sys.argv[1])
expected = sys.argv[2:]
text = ci_path.read_text()
lines = text.splitlines()

if "@latest" in text:
    print("FAIL: ci.yml still contains @latest")
    sys.exit(1)

# Workflow-level pull_request paths: forbidden on the fan-out (Design D: no
# paths: filters). A job-level paths: is also forbidden for check-job jobs.
jobs = {}
current = None
buf = []
in_jobs = False
for line in lines:
    if not in_jobs:
        if line == "jobs:" or line.startswith("jobs:"):
            in_jobs = True
        continue
    if line[:1] not in (" ", "\t", "") and line.strip() and not line.strip().startswith("#"):
        break
    m = re.match(r"^  ([A-Za-z0-9_-]+):\s*$", line)
    if m:
        if current is not None:
            jobs[current] = buf
        current = m.group(1)
        buf = []
        continue
    if current is not None:
        buf.append(line)
if current is not None:
    jobs[current] = buf

if "check" in jobs:
    print("FAIL: leftover job id 'check' exists")
    sys.exit(1)

fanout = {}
for jid, body in jobs.items():
    make_runs = []
    for raw in body:
        stripped = raw.strip()
        if stripped.startswith("#"):
            continue
        m = re.match(r"^(?:-\s*)?run:\s*(.*)$", stripped)
        if not m:
            continue
        val = m.group(1).strip()
        if val.startswith("make ") or val == "make":
            make_runs.append(val)
    if any(r.startswith("make check-job") for r in make_runs):
        fanout[jid] = (body, make_runs)

got = sorted(fanout)
want = sorted(expected)
if got != want:
    print(f"FAIL: check-job jobs {got} != print-check-jobs {want}")
    sys.exit(1)

for jid, (body, make_runs) in fanout.items():
    if make_runs != [f"make check-job JOB={jid}"]:
        print(f"FAIL: job {jid} make steps {make_runs!r} want exactly ['make check-job JOB={jid}']")
        sys.exit(1)
    for raw in body:
        stripped = raw.strip()
        if stripped.startswith("#"):
            continue
        if stripped == "needs:" or stripped.startswith("needs:"):
            print(f"FAIL: job {jid} has needs:")
            sys.exit(1)
        if stripped == "paths:" or stripped.startswith("paths:"):
            print(f"FAIL: job {jid} has paths:")
            sys.exit(1)
    blob = "\n".join(body)
    if jid in ("check-core", "check-static") and "ripgrep" not in blob:
        print(f"FAIL: job {jid} must install ripgrep (ubuntu-latest has no rg; git-test-hermetic and test-check-jobs need it)")
        sys.exit(1)

print("PASS")
PY
) && PARSE_STATUS=0 || PARSE_STATUS=$?

if [[ $PARSE_STATUS -eq 0 ]]; then
    pass "ci.yml check-job jobs are ${IDS[*]:-?} with JOB=<id>, no needs/paths, no leftover check"
else
    fail "${PARSE_RESULT:-ci.yml parse failed}"
fi

# ----------------------------------------------------------------------------
# Test 3: make check has no prerequisites
# ----------------------------------------------------------------------------
echo "Test 3: Makefile check target has no prerequisites..."

CHECK_LINE=$(awk '/^check:/{print; exit}' "$MAKEFILE")
if [[ -z "$CHECK_LINE" ]]; then
    fail "Makefile has no check: target"
elif [[ "$CHECK_LINE" != "check:" ]]; then
    fail "check has prerequisites ($CHECK_LINE); make -j check would fan out in-process"
else
    pass "check has no prerequisites"
fi

# ----------------------------------------------------------------------------
# Test 4: unknown / empty JOB is rejected without running a partition
# ----------------------------------------------------------------------------
echo "Test 4: check-job rejects empty and unknown JOB..."

set +e
EMPTY_OUT=$(run_make check-job JOB= 2>&1)
EMPTY_STATUS=$?
BOGUS_OUT=$(run_make check-job JOB=not-a-job 2>&1)
BOGUS_STATUS=$?
set -e

if [[ $EMPTY_STATUS -eq 0 ]]; then
    fail "check-job with empty JOB exited 0"
else
    pass "empty JOB exits $EMPTY_STATUS"
fi
if [[ $BOGUS_STATUS -eq 0 ]]; then
    fail "check-job JOB=not-a-job exited 0"
else
    pass "unknown JOB exits $BOGUS_STATUS"
fi
if echo "$BOGUS_OUT" | rg -q "not-a-job"; then
    pass "unknown JOB names the rejected id"
else
    fail "unknown JOB output does not name not-a-job: $BOGUS_OUT"
fi

# GNU make $(filter) treats % as a wildcard. JOB=% / check-% / mut% must not
# pass the membership gate and then run an empty target list (exit 0).
echo "Test 4b: check-job rejects Make-filter wildcard JOB values..."
for pat in '%' 'check-%' 'mut%'; do
    set +e
    WILD_OUT=$(run_make check-job JOB="$pat" 2>&1)
    WILD_STATUS=$?
    set -e
    if [[ $WILD_STATUS -eq 0 ]]; then
        fail "check-job JOB=$pat exited 0 (wildcard must not hollow-succeed)"
        echo "$WILD_OUT"
    else
        pass "JOB=$pat exits $WILD_STATUS"
    fi
done

# ----------------------------------------------------------------------------
# Test 5: ubuntu test-os must not re-run the unit suite on PRs (check-core
# owns it), but tag releases call this workflow with os-matrix-only and skip
# check-core — Linux must still run make test-ci there.
# ----------------------------------------------------------------------------
echo "Test 5: ubuntu test-os skips make test-ci except os-matrix-only..."

OS_RESULT=$(python3 - "$CI_YML" <<'PY'
import re
import sys
from pathlib import Path

text = Path(sys.argv[1]).read_text()
lines = text.splitlines()
jobs = {}
current = None
buf = []
in_jobs = False
for line in lines:
    if not in_jobs:
        if line == "jobs:" or line.startswith("jobs:"):
            in_jobs = True
        continue
    if line[:1] not in (" ", "\t", "") and line.strip() and not line.strip().startswith("#"):
        break
    m = re.match(r"^  ([A-Za-z0-9_-]+):\s*$", line)
    if m:
        if current is not None:
            jobs[current] = buf
        current = m.group(1)
        buf = []
        continue
    if current is not None:
        buf.append(line)
if current is not None:
    jobs[current] = buf

body = jobs.get("test-os")
if body is None:
    print("FAIL: no test-os job")
    sys.exit(1)

# Split into steps on lines matching "      - " at the start of a step.
steps = []
cur = []
for raw in body:
    if re.match(r"^      - ", raw):
        if cur:
            steps.append(cur)
        cur = [raw]
    elif cur:
        cur.append(raw)
if cur:
    steps.append(cur)

saw_test_ci = False
for step in steps:
    blob = "\n".join(step)
    if not re.search(r"(?:^|\n)\s*(?:-\s*)?run:\s*make test-ci\b", blob):
        continue
    saw_test_ci = True
    ifs = [ln.strip() for ln in step if ln.strip().startswith("if:")]
    joined = " ".join(ifs)
    if "Linux" not in joined:
        print("FAIL: make test-ci step has no Linux exclusion in if:")
        sys.exit(1)
    if "os-matrix-only" not in joined:
        print("FAIL: Linux test-ci skip does not restore tests when os-matrix-only (tag releases skip check-core)")
        sys.exit(1)
    if "||" not in joined and "||" not in blob:
        print(f"FAIL: make test-ci if: must OR os-matrix-only with the Linux skip ({joined})")
        sys.exit(1)

if not saw_test_ci:
    print("FAIL: test-os never runs make test-ci (windows/macos must keep it)")
    sys.exit(1)
print("PASS")
PY
) && OS_STATUS=0 || OS_STATUS=$?

if [[ $OS_STATUS -eq 0 ]]; then
    pass "test-os skips Linux test-ci on PRs and restores it for os-matrix-only"
else
    fail "${OS_RESULT:-test-os assertion failed}"
fi

# ----------------------------------------------------------------------------
# Test 6: this script is committed executable
# ----------------------------------------------------------------------------
echo "Test 6: test_check_jobs.sh is committed executable..."
mode=$(git -C "$REPO_ROOT" ls-files -s -- "scripts/test_check_jobs.sh" | awk '{print $1}')
if [[ "$mode" == "100755" ]]; then
    pass "git mode is 100755"
else
    fail "git mode is '${mode:-missing}', expected 100755 (stage as executable)"
fi
if [[ -x "$SCRIPT" ]]; then
    pass "executable on disk"
else
    fail "not executable on disk"
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All check-job drift tests passed"
    exit 0
else
    echo "FAIL: $FAILURES check-job drift test(s) failed"
    exit 1
fi
