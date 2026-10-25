#!/bin/bash

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
CI_YML="$REPO_ROOT/.github/workflows/ci.yml"
MAKEFILE="$REPO_ROOT/Makefile"
SCRIPT="$REPO_ROOT/scripts/test_check_jobs.sh"

FAILURES=0

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

echo "Test 3: Makefile check target has no prerequisites..."

CHECK_LINE=$(awk '/^check:/{print; exit}' "$MAKEFILE")
if [[ -z "$CHECK_LINE" ]]; then
    fail "Makefile has no check: target"
elif [[ "$CHECK_LINE" != "check:" ]]; then
    fail "check has prerequisites ($CHECK_LINE); make -j check would fan out in-process"
else
    pass "check has no prerequisites"
fi

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

echo "Test 5b: test-os uploads go test JSON and keeps diagnose-on-failure..."

JSON_RESULT=$(python3 - "$CI_YML" <<'PY'
import re
import sys
from pathlib import Path

text = Path(sys.argv[1]).read_text()
if "retention-days: 14" not in text:
    print("FAIL: go test JSON artifact retention is not 14 days")
    sys.exit(1)
if "actions/upload-artifact@" not in text:
    print("FAIL: test-os does not upload a go test JSON artifact")
    sys.exit(1)
if "TEST_JSON_OUT" not in text:
    print("FAIL: Test step does not set TEST_JSON_OUT for the JSON artifact")
    sys.exit(1)
if not re.search(r"(?m)^\s*- name: Diagnose cmd/armature failures\s*$", text):
    print("FAIL: diagnose cmd/armature step is missing")
    sys.exit(1)
if "go test -count=1 -timeout 25m -v ./cmd/armature/" not in text:
    print("FAIL: diagnose rerun of ./cmd/armature/ changed")
    sys.exit(1)
print("PASS")
PY
) && JSON_STATUS=0 || JSON_STATUS=$?

if [[ $JSON_STATUS -eq 0 ]]; then
    pass "test-os keeps diagnose rerun and uploads go-test JSON (14d)"
else
    fail "${JSON_RESULT:-test-os JSON artifact assertion failed}"
fi

echo "Test 6: CHECK_INSTALL_TOOLS installs checksum-verified GitHub binaries, not go install golangci-lint..."

if rg -n 'CHECK_INSTALL_TOOLS' "$MAKEFILE" | rg -q 'go install.*golangci'; then
    fail "Makefile still go-installs golangci-lint on the CHECK_INSTALL_TOOLS path"
else
    pass "CHECK_INSTALL_TOOLS path does not go install golangci-lint"
fi
if ! rg -q 'install-\$\$t-release' "$MAKEFILE"; then
    fail "check-job CHECK_INSTALL_TOOLS path must call install-<tool>-release"
else
    pass "check-job installs tools via install-<tool>-release"
fi
if ! rg -q 'install-verified-github-release.sh' "$MAKEFILE"; then
    fail "Makefile must invoke install-verified-github-release.sh"
else
    pass "Makefile uses install-verified-github-release.sh"
fi
if ! rg -q 'checksum mismatch' "$REPO_ROOT/scripts/install-verified-github-release.sh"; then
    fail "installer must fail on checksum mismatch"
else
    pass "installer mentions checksum mismatch failure"
fi
if rg -q 'go install github.com/golangci/golangci-lint' "$CI_YML"; then
    fail "ci.yml go-installs golangci-lint"
else
    pass "ci.yml does not go install golangci-lint"
fi
if ! awk '/^GOLANGCI_LINT_VERSION[[:space:]]*:?=/{found=1} END{exit found?0:1}' "$MAKEFILE"; then
    fail "GOLANGCI_LINT_VERSION missing from Makefile"
else
    pass "GOLANGCI_LINT_VERSION remains Makefile source of truth"
fi
if rg -n 'version: v2\.14' "$CI_YML"; then
    fail "ci.yml duplicates the golangci-lint version number"
else
    pass "ci.yml does not duplicate GOLANGCI_LINT_VERSION"
fi

echo "Test 7: GitHub Actions are SHA-pinned with version comments, least privilege, persist-credentials: false..."

SUPPLY_RESULT=$(python3 - "$REPO_ROOT" <<'PY'
import re
import sys
from pathlib import Path

root = Path(sys.argv[1])
workflow_dir = root / ".github" / "workflows"
sha_re = re.compile(r"^[0-9a-f]{40}$")
uses_re = re.compile(r"^(\s*)uses:\s*(.+)$")
problems = []
uses_lines = []

for path in sorted(workflow_dir.glob("*.yml")) + sorted(workflow_dir.glob("*.yaml")):
    text = path.read_text()
    rel = path.relative_to(root).as_posix()
    lines = text.splitlines()
    top_perm = False
    for i, line in enumerate(lines):
        if line.startswith("jobs:"):
            break
        if line == "permissions:" or line.startswith("permissions:"):
            top_perm = True
            window = "\n".join(lines[i:i+8])
            if "contents: read" not in window and "contents: write" not in window:
                problems.append(f"{rel}: top-level permissions missing contents")
            if rel.endswith("ci.yml") and "contents: read" not in window:
                problems.append(f"{rel}: CI top-level permissions must be contents: read")
            if rel.endswith("release.yml") and "contents: read" not in window:
                problems.append(f"{rel}: release.yml top-level permissions must be contents: read (escalate on the release job)")
            break
    if not top_perm:
        problems.append(f"{rel}: missing top-level permissions")

    lines = text.splitlines()
    i = 0
    while i < len(lines):
        m = uses_re.match(lines[i])
        if not m:
            i += 1
            continue
        indent, spec = m.group(1), m.group(2).strip()
        uses_lines.append((rel, spec))
        if spec.startswith("./"):
            i += 1
            continue
        if "@" not in spec:
            problems.append(f"{rel}:{i+1} uses: {spec} is not pinned")
            i += 1
            continue
        action, _, rest = spec.partition("@")
        # rest may include comment
        sha_part, _, comment = rest.partition("#")
        sha_part = sha_part.strip()
        comment = comment.strip()
        if not sha_re.match(sha_part):
            problems.append(f"{rel}:{i+1} uses: {spec} is not a 40-char SHA")
        if not comment:
            problems.append(f"{rel}:{i+1} uses: {spec} missing trailing # vX.Y.Z comment")
        elif not re.search(r"v?\d+", comment):
            problems.append(f"{rel}:{i+1} uses: {spec} comment {comment!r} is not a version")
        # persist-credentials on checkout
        if action == "actions/checkout":
            j = i + 1
            block = []
            while j < len(lines) and (not lines[j].strip() or lines[j].startswith(indent + " ") or lines[j].startswith(indent + "\t")):
                block.append(lines[j])
                j += 1
            blob = "\n".join(block)
            if "persist-credentials:" not in blob:
                problems.append(f"{rel}:{i+1} actions/checkout missing persist-credentials")
            elif not re.search(r"persist-credentials:\s*false", blob):
                problems.append(f"{rel}:{i+1} actions/checkout persist-credentials is not false")
        i += 1

if problems:
    print("FAIL")
    for p in problems:
        print(p)
    sys.exit(1)
print("PASS")
PY
) && SUPPLY_STATUS=0 || SUPPLY_STATUS=$?

if [[ $SUPPLY_STATUS -eq 0 ]]; then
    pass "workflows pin actions to SHAs, set permissions, and disable checkout credentials"
else
    fail "${SUPPLY_RESULT:-workflow supply-chain assertions failed}"
fi

echo "Test 8: test_check_jobs.sh is committed executable..."
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

INSTALL_SCRIPT="$REPO_ROOT/scripts/install-verified-github-release.sh"
INSTALL_TEST="$REPO_ROOT/scripts/test_install_verified_github_release.sh"
install_mode=$(git -C "$REPO_ROOT" ls-files -s -- "scripts/install-verified-github-release.sh" | awk '{print $1}')
if [[ "$install_mode" == "100755" ]]; then
    pass "install-verified-github-release.sh git mode is 100755"
else
    fail "install-verified-github-release.sh git mode is '${install_mode:-missing}', expected 100755"
fi
if [[ -x "$INSTALL_SCRIPT" && -x "$INSTALL_TEST" ]]; then
    pass "install scripts executable on disk"
else
    fail "install scripts not executable on disk"
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All check-job drift tests passed"
    exit 0
else
    echo "FAIL: $FAILURES check-job drift test(s) failed"
    exit 1
fi
