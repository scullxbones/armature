#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(CDPATH= cd -- "${1:-.}" && pwd)
APPLY="$REPO_ROOT/scripts/apply-quote-verdict-to-check-core.sh"
SKIP="$REPO_ROOT/scripts/check-core-quote-rerun-skip.sh"
CI="$REPO_ROOT/.github/workflows/ci.yml"
EDIT="$REPO_ROOT/.github/workflows/pr-coverage-quotes-on-edit.yml"

FAILURES=0
pass() { echo "  PASS${1:+ ($1)}"; }
fail() { echo "  FAIL: $1"; FAILURES=$((FAILURES + 1)); }

QUOTE_STEP="PR body coverage quotes match this job's coverage.out"

echo "Test 1: workflow contract — edit path reruns required check-core, suite stays put..."
CONTRACT=$(python3 - "$CI" "$EDIT" <<'PY'
import sys
from pathlib import Path

ci = Path(sys.argv[1]).read_text()
edit = Path(sys.argv[2]).read_text()
errors = []

def job_slice(text, key):
    marker = f"\n  {key}:\n"
    start = text.find(marker)
    if start < 0:
        return ""
    start += 1
    rest = text[start + len(marker) - 1 :]
    # next top-level job key at column 2
    lines = text[start:].splitlines(True)
    out = []
    for i, ln in enumerate(lines):
        if i > 0 and len(ln) > 2 and ln[2] != " " and ln.endswith(":\n") and not ln.startswith("   "):
            break
        out.append(ln)
    return "".join(out)

core = job_slice(ci, "check-core")
if not core:
    errors.append("ci.yml missing check-core job")

def steps(job):
    blocks = []
    cur = None
    for ln in job.splitlines():
        if ln.startswith("      - "):
            if cur is not None:
                blocks.append(cur)
            cur = [ln]
        elif cur is not None:
            cur.append(ln)
    if cur is not None:
        blocks.append(cur)
    return ["\n".join(b) for b in blocks]

blocks = steps(core)

def block_with(needle):
    hits = [b for b in blocks if needle in b]
    return hits

make_hits = block_with("make check-job JOB=check-core")
if len(make_hits) != 1:
    errors.append(f"expected one make check-job JOB=check-core step, got {len(make_hits)}")
elif "steps.quote_rerun.outputs.skip != 'true'" not in make_hits[0] or "success()" not in make_hits[0]:
    errors.append("make check-job must be success() and gated on quote_rerun skip != true")

cov_hits = block_with("scripts/coverage-check.sh")
if len(cov_hits) != 1:
    errors.append(f"expected one coverage-check.sh step, got {len(cov_hits)}")
elif "steps.quote_rerun.outputs.skip != 'true'" not in cov_hits[0]:
    errors.append("coverage-check.sh must not run on a quote-only rerun")

upload_hits = block_with("Upload coverage-check.log for late PR body paste")
if len(upload_hits) != 1:
    errors.append("missing coverage-check.log upload step")
elif "always()" not in upload_hits[0] or "skip != 'true'" not in upload_hits[0]:
    errors.append("upload must stay always() and must not replace the log on quote-only rerun")

cmp_hits = block_with("scripts/pr-coverage-quote-check.sh")
if len(cmp_hits) != 1:
    errors.append("check-core must still run pr-coverage-quote-check.sh")
elif "skip" in cmp_hits[0]:
    errors.append("quote compare must run on quote-only reruns (do not gate it on skip)")
elif "github.event_name == 'pull_request'" not in cmp_hits[0] and 'github.event_name == "pull_request"' not in cmp_hits[0]:
    errors.append("quote compare must stay pull_request-only")

skip_hits = block_with("check-core-quote-rerun-skip.sh")
if len(skip_hits) != 1:
    errors.append("check-core must run check-core-quote-rerun-skip.sh")
else:
    if "github.run_attempt" not in skip_hits[0] or "github.run_id" not in skip_hits[0]:
        errors.append("skip script must see github.run_id and github.run_attempt")
    if "id: quote_rerun" not in skip_hits[0]:
        errors.append("skip step id must be quote_rerun")

restore_hits = block_with("wait-coverage-check-artifact.sh")
if len(restore_hits) != 1:
    errors.append("quote-only rerun must restore coverage-check.log via wait-coverage-check-artifact.sh")
elif "skip == 'true'" not in restore_hits[0]:
    errors.append("restore must run only when skip == true")

if "types:" in ci and "edited" in ci.split("jobs:", 1)[0]:
    # on: block only — jobs may mention the word in comments
    header = ci.split("\njobs:", 1)[0]
    if "edited" in header:
        errors.append("ci.yml on: must not list pull_request edited")

for required in ("check-core", "check-static", "mutate"):
    if f"\n  {required}:\n" not in ci:
        errors.append(f"ci.yml lost required job {required}")

if "\n  check-core:\n" in edit:
    errors.append("edit workflow must not define a job named check-core")
if "make check-job" in edit or "coverage-check.sh" in edit:
    errors.append("edit workflow must not recompute coverage")
if "types: [edited]" not in edit and 'types: ["edited"]' not in edit:
    errors.append("edit workflow must stay on pull_request types: [edited]")
if "apply-quote-verdict-to-check-core.sh plan" not in edit:
    errors.append("edit workflow must plan the check-core verdict")
if "apply-quote-verdict-to-check-core.sh commit" not in edit:
    errors.append("edit workflow must commit the check-core rerun")
if "verdict=match" not in edit or "verdict=mismatch" not in edit:
    errors.append("edit workflow must pass the quote script's match/mismatch verdict")
if "quote-recheck-" not in edit:
    errors.append("edit workflow must publish a quote-recheck marker before rerun")
if "actions: write" not in edit:
    errors.append("edit workflow needs actions: write to rerun check-core")
if "pr-coverage-quote-check.sh" not in edit:
    errors.append("edit workflow must run pr-coverage-quote-check.sh")
# Marker upload is gated on the plan output, including when the quote step failed.
if "steps.align.outputs.rerun == 'true'" not in edit:
    errors.append("marker upload and rerun must follow steps.align.outputs.rerun")
if "always()" not in edit:
    errors.append("rerun steps must use always() so a mismatch verdict still flips check-core")

if errors:
    print("FAIL: " + "; ".join(errors))
    sys.exit(1)
print("PASS")
PY
) && CONTRACT_STATUS=0 || CONTRACT_STATUS=$?

if [[ $CONTRACT_STATUS -eq 0 ]]; then
    pass "edit path owns the check-core quote verdict"
else
    fail "${CONTRACT:-workflow contract failed}"
fi

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

write_stub() {
    cat >"$WORKDIR/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${GH_LOG:?}"
if [ "${1:-}" != "api" ]; then
    echo "unexpected gh argv: $*" >&2
    exit 99
fi
shift
method=GET
url=""
prog=""
while [ $# -gt 0 ]; do
    case "$1" in
        -X) method=$2; shift 2 ;;
        --jq) prog=$2; shift 2 ;;
        -f|-H) shift 2 ;;
        --silent|--fail|-s) shift ;;
        *) url=$1; shift ;;
    esac
done
printf '%s %s\n' "$method" "$url" >> "${GH_LOG}"
case "$url" in
    *"/actions/jobs/"*"/rerun")
        printf '%s %s\n' "$method" "$url" >> "${GH_POSTS:?}"
        exit 0
        ;;
esac
fixture=""
case "$url" in
    *"/actions/workflows/ci.yml/runs"*) fixture="runs.json" ;;
    *"/actions/runs/"*"/jobs"*) fixture="jobs.json" ;;
    *"/actions/runs/"*) fixture="run.json" ;;
    *"/actions/artifacts"*) fixture="artifacts.json" ;;
    *)
        echo "unmapped url: $url" >&2
        exit 99
        ;;
esac
if [ -n "${GH_COUNT_FILE:-}" ]; then
    n=0
    if [ -f "$GH_COUNT_FILE" ]; then
        n=$(cat "$GH_COUNT_FILE")
    fi
    # Number only the fixture family under test so a runs poll and a jobs
    # poll do not share one counter.
    if [ "${GH_COUNT_KIND:-$fixture}" = "$fixture" ]; then
        n=$((n + 1))
        printf '%s\n' "$n" > "$GH_COUNT_FILE"
        alt="${GH_FIX:?}/${fixture%.json}-${n}.json"
        if [ -f "$alt" ]; then
            fixture="${fixture%.json}-${n}.json"
        fi
    fi
fi
if [ ! -f "${GH_FIX:?}/$fixture" ]; then
    echo "missing fixture $fixture for $url" >&2
    exit 99
fi
if [ -n "$prog" ]; then
    jq -r "$prog" "${GH_FIX}/$fixture"
else
    cat "${GH_FIX}/$fixture"
fi
EOF
    chmod +x "$WORKDIR/gh"
}

runs_completed='{"workflow_runs":[{"id":100,"status":"completed","run_attempt":1,"event":"pull_request"}]}'
job_quote_fail=$(cat <<EOF
{"jobs":[{"id":55,"name":"check-core","status":"completed","conclusion":"failure","steps":[
  {"name":"Run make check-job JOB=check-core","conclusion":"success"},
  {"name":"Write coverage-check.log","conclusion":"success"},
  {"name":"$QUOTE_STEP","conclusion":"failure"}
]}]}
EOF
)
job_suite_fail=$(cat <<EOF
{"jobs":[{"id":55,"name":"check-core","status":"completed","conclusion":"failure","steps":[
  {"name":"Run make check-job JOB=check-core","conclusion":"failure"},
  {"name":"$QUOTE_STEP","conclusion":"failure"}
]}]}
EOF
)
job_success=$(cat <<EOF
{"jobs":[{"id":55,"name":"check-core","status":"completed","conclusion":"success","steps":[
  {"name":"Run make check-job JOB=check-core","conclusion":"success"},
  {"name":"$QUOTE_STEP","conclusion":"success"}
]}]}
EOF
)
job_in_progress='{"jobs":[{"id":55,"name":"check-core","status":"in_progress","conclusion":null,"steps":[]}]}'

run_plan() {
    local verdict="$1"
    mkdir -p "$WORKDIR/planout"
    (
        cd "$WORKDIR/planout"
        GH_BIN="$WORKDIR/gh" GH_LOG="$WORKDIR/gh.log" GH_POSTS="$WORKDIR/posts" GH_FIX="$WORKDIR/fix" \
            MAX_POLLS="${MAX_POLLS:-3}" SLEEP_SECS=0 \
            "$APPLY" plan scullxbones/armature abcdef "${verdict}"
    )
}

prep_fix() {
    rm -rf "$WORKDIR/fix" "$WORKDIR/gh.log" "$WORKDIR/posts"
    mkdir -p "$WORKDIR/fix"
    : >"$WORKDIR/posts"
    printf '%s\n' "$runs_completed" >"$WORKDIR/fix/runs.json"
}

if [[ ! -x "$APPLY" || ! -x "$SKIP" ]]; then
    fail "missing apply/skip scripts (contract above is the main-branch failure)"
else
    write_stub

    echo "Test 2: quote-only red check-core + matching body reruns check-core..."
    prep_fix
    printf '%s\n' "$job_quote_fail" >"$WORKDIR/fix/jobs.json"
    OUT=$(run_plan match)
    if [[ "$OUT" != "rerun=true run_id=100 job_id=55 next_attempt=2" ]]; then
        fail "expected quote-only match to request rerun, got: $OUT"
    elif [[ ! -f "$WORKDIR/planout/quote-recheck-marker.txt" ]]; then
        fail "plan did not write the quote-recheck marker file"
    else
        pass
    fi
    rm -f "$WORKDIR/planout/quote-recheck-marker.txt"

    echo "Test 3: quote-only red + still-wrong body does not rerun..."
    prep_fix
    printf '%s\n' "$job_quote_fail" >"$WORKDIR/fix/jobs.json"
    OUT=$(run_plan mismatch)
    if [[ "$OUT" != "rerun=false" ]]; then
        fail "expected already-red quote failure to stay, got: $OUT"
    else
        pass
    fi

    echo "Test 4: suite failure is not cleared by a matching quote..."
    prep_fix
    printf '%s\n' "$job_suite_fail" >"$WORKDIR/fix/jobs.json"
    OUT=$(run_plan match)
    if [[ "$OUT" != "rerun=false" ]]; then
        fail "suite failure must not rerun, got: $OUT"
    else
        pass
    fi

    echo "Test 5: green check-core + wrong body reruns so the required check goes red..."
    prep_fix
    printf '%s\n' "$job_success" >"$WORKDIR/fix/jobs.json"
    OUT=$(run_plan mismatch)
    if [[ "$OUT" != "rerun=true run_id=100 job_id=55 next_attempt=2" ]]; then
        fail "expected success+mismatch to rerun, got: $OUT"
    else
        pass
    fi
    rm -f "$WORKDIR/planout/quote-recheck-marker.txt"

    echo "Test 6: green check-core + matching body does not rerun..."
    prep_fix
    printf '%s\n' "$job_success" >"$WORKDIR/fix/jobs.json"
    OUT=$(run_plan match)
    if [[ "$OUT" != "rerun=false" ]]; then
        fail "expected aligned success to stay, got: $OUT"
    else
        pass
    fi

    echo "Test 7: plan waits until check-core completes..."
    prep_fix
    printf '%s\n' "$job_in_progress" >"$WORKDIR/fix/jobs-1.json"
    printf '%s\n' "$job_quote_fail" >"$WORKDIR/fix/jobs-2.json"
    OUT=$(GH_COUNT_FILE="$WORKDIR/count" GH_COUNT_KIND=jobs.json run_plan match)
    rm -f "$WORKDIR/count"
    if [[ "$OUT" != "rerun=true run_id=100 job_id=55 next_attempt=2" ]]; then
        fail "expected poll-then-rerun, got: $OUT"
    else
        pass
    fi
    rm -f "$WORKDIR/planout/quote-recheck-marker.txt"

    echo "Test 8: commit posts the job rerun only after the workflow run completes..."
    prep_fix
    printf '%s\n' '{"status":"in_progress"}' >"$WORKDIR/fix/run-1.json"
    printf '%s\n' '{"status":"completed"}' >"$WORKDIR/fix/run-2.json"
    # run.json fallback unused; counter serves run-N
    printf '%s\n' '{"status":"completed"}' >"$WORKDIR/fix/run.json"
    COUT=$(GH_BIN="$WORKDIR/gh" GH_LOG="$WORKDIR/gh.log" GH_POSTS="$WORKDIR/posts" GH_FIX="$WORKDIR/fix" \
        GH_COUNT_FILE="$WORKDIR/rcount" GH_COUNT_KIND=run.json MAX_POLLS=4 SLEEP_SECS=0 \
        "$APPLY" commit scullxbones/armature 100 55)
    rm -f "$WORKDIR/rcount"
    if [[ "$COUT" != "reran job 55" ]]; then
        fail "commit stdout: $COUT"
    elif ! grep -qx 'POST repos/scullxbones/armature/actions/jobs/55/rerun' "$WORKDIR/posts"; then
        fail "rerun POST missing: $(cat "$WORKDIR/posts")"
    else
        nposts=$(grep -c '/rerun' "$WORKDIR/posts" || true)
        if [[ "$nposts" -ne 1 ]]; then
            fail "expected one rerun POST, got $nposts"
        else
            pass
        fi
    fi

    echo "Test 9: attempt 1 never skips and does not call gh..."
    BOOM="$WORKDIR/boom"
    cat >"$BOOM" <<'EOF'
#!/usr/bin/env bash
echo "gh should not be called on attempt 1" >&2
exit 99
EOF
    chmod +x "$BOOM"
    SOUT=$(GH_BIN="$BOOM" RUN_ID=100 RUN_ATTEMPT=1 REPO=scullxbones/armature "$SKIP")
    if [[ "$SOUT" != "skip=false" ]]; then
        fail "attempt 1 skip output: $SOUT"
    else
        pass
    fi

    echo "Test 10: attempt>1 with marker skips the suite..."
    prep_fix
    printf '%s\n' '{"artifacts":[{"id":7,"name":"quote-recheck-100-2","expired":false}]}' >"$WORKDIR/fix/artifacts.json"
    SOUT=$(GH_BIN="$WORKDIR/gh" GH_LOG="$WORKDIR/gh.log" GH_POSTS="$WORKDIR/posts" GH_FIX="$WORKDIR/fix" \
        MAX_POLLS=2 SLEEP_SECS=0 RUN_ID=100 RUN_ATTEMPT=2 REPO=scullxbones/armature "$SKIP")
    if [[ "$SOUT" != "skip=true" ]]; then
        fail "expected skip=true, got: $SOUT"
    else
        pass
    fi

    echo "Test 11: attempt>1 without marker runs the suite..."
    prep_fix
    printf '%s\n' '{"artifacts":[]}' >"$WORKDIR/fix/artifacts.json"
    SOUT=$(GH_BIN="$WORKDIR/gh" GH_LOG="$WORKDIR/gh.log" GH_POSTS="$WORKDIR/posts" GH_FIX="$WORKDIR/fix" \
        MAX_POLLS=2 SLEEP_SECS=0 RUN_ID=100 RUN_ATTEMPT=2 REPO=scullxbones/armature "$SKIP")
    if [[ "$SOUT" != "skip=false" ]]; then
        fail "expected skip=false without marker, got: $SOUT"
    else
        pass
    fi

    echo "Test 12: marker that appears on a later poll still skips..."
    prep_fix
    printf '%s\n' '{"artifacts":[]}' >"$WORKDIR/fix/artifacts-1.json"
    printf '%s\n' '{"artifacts":[{"id":7,"name":"quote-recheck-100-2","expired":false}]}' >"$WORKDIR/fix/artifacts-2.json"
    SOUT=$(GH_BIN="$WORKDIR/gh" GH_LOG="$WORKDIR/gh.log" GH_POSTS="$WORKDIR/posts" GH_FIX="$WORKDIR/fix" \
        GH_COUNT_FILE="$WORKDIR/acount" GH_COUNT_KIND=artifacts.json MAX_POLLS=3 SLEEP_SECS=0 \
        RUN_ID=100 RUN_ATTEMPT=2 REPO=scullxbones/armature "$SKIP")
    rm -f "$WORKDIR/acount"
    if [[ "$SOUT" != "skip=true" ]]; then
        fail "expected late marker to skip, got: $SOUT"
    else
        pass
    fi

    echo "Test 13: expired marker does not skip..."
    prep_fix
    printf '%s\n' '{"artifacts":[{"id":7,"name":"quote-recheck-100-2","expired":true}]}' >"$WORKDIR/fix/artifacts.json"
    SOUT=$(GH_BIN="$WORKDIR/gh" GH_LOG="$WORKDIR/gh.log" GH_POSTS="$WORKDIR/posts" GH_FIX="$WORKDIR/fix" \
        MAX_POLLS=1 SLEEP_SECS=0 RUN_ID=100 RUN_ATTEMPT=2 REPO=scullxbones/armature "$SKIP")
    if [[ "$SOUT" != "skip=false" ]]; then
        fail "expired marker must not skip, got: $SOUT"
    else
        pass
    fi

    echo "Test 14: gh failure on attempt>1 fails closed..."
    cat >"$BOOM" <<'EOF'
#!/usr/bin/env bash
echo "api down" >&2
exit 1
EOF
    chmod +x "$BOOM"
    set +e
    SOUT=$(GH_BIN="$BOOM" MAX_POLLS=1 SLEEP_SECS=0 RUN_ID=100 RUN_ATTEMPT=2 REPO=scullxbones/armature "$SKIP" 2>&1)
    SST=$?
    set -e
    if [[ $SST -eq 0 ]]; then
        fail "gh failure must not exit 0 ($SOUT)"
    else
        pass
    fi

    echo "Test 15: mutant — treating every failure as quote-only must not pass suite-failure..."
    MUT="$WORKDIR/apply-mut.sh"
    python3 - "$APPLY" "$MUT" <<'PY'
import sys
src, dst = sys.argv[1], sys.argv[2]
t = open(src).read()
old = 'if [ "$conclusion" = "failure" ] && [ "$failed" = "$QUOTE_STEP" ] && [ "$verdict" = "match" ]; then'
new = 'if [ "$conclusion" = "failure" ] && [ "$verdict" = "match" ]; then'
if old not in t:
    raise SystemExit("decision site not found")
open(dst, "w").write(t.replace(old, new, 1))
PY
    chmod +x "$MUT"
    prep_fix
    printf '%s\n' "$job_suite_fail" >"$WORKDIR/fix/jobs.json"
    set +e
    MOUT=$(
        cd "$WORKDIR/planout"
        GH_BIN="$WORKDIR/gh" GH_LOG="$WORKDIR/gh.log" GH_POSTS="$WORKDIR/posts" GH_FIX="$WORKDIR/fix" \
            MAX_POLLS=2 SLEEP_SECS=0 "$MUT" plan scullxbones/armature abcdef match
    )
    set -e
    if [[ "$MOUT" == "rerun=false" ]]; then
        fail "mutant still refused a suite failure; decision site is not what the test locks"
    else
        pass "mutant reran a suite failure ($MOUT)"
    fi
fi

if [[ "$FAILURES" -eq 0 ]]; then
    echo "All quote-verdict gate tests passed"
    exit 0
fi
echo "FAIL: $FAILURES quote-verdict gate test(s) failed"
exit 1
