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
    # Lean (b): check-static alone may also run commit-subjects-check (exactly once).
    want_runs = [f"make check-job JOB={jid}"]
    if jid == "check-static":
        want_runs = ["make commit-subjects-check", f"make check-job JOB={jid}"]
    if make_runs != want_runs:
        print(f"FAIL: job {jid} make steps {make_runs!r} want exactly {want_runs!r}")
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
        sha_part, _, comment = rest.partition("#")
        sha_part = sha_part.strip()
        comment = comment.strip()
        if not sha_re.match(sha_part):
            problems.append(f"{rel}:{i+1} uses: {spec} is not a 40-char SHA")
        if not comment:
            problems.append(f"{rel}:{i+1} uses: {spec} missing trailing # vX.Y.Z comment")
        elif not re.search(r"v?\d+", comment):
            problems.append(f"{rel}:{i+1} uses: {spec} comment {comment!r} is not a version")
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

echo "Test 9: every CI job asserts Git ≥2.34; check-git-2-34 pins exact 2.34.0 source..."

GIT_MIN_RESULT=$(python3 - "$CI_YML" <<'PY'
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
            jobs[current] = "\n".join(buf)
        current = m.group(1)
        buf = []
        continue
    if current is not None:
        buf.append(line)
if current is not None:
    jobs[current] = "\n".join(buf)

if "check-git-2-34" not in jobs:
    print("FAIL: missing check-git-2-34 job")
    sys.exit(1)
for jid, body in jobs.items():
    if "assert-git-min.sh" not in body:
        print(f"FAIL: job {jid} must run scripts/assert-git-min.sh")
        sys.exit(1)
body = jobs["check-git-2-34"]
for needle in (
    "install-git-from-source.sh",
    "GIT_MIN_VERSION",
    "GIT_MIN_TARBALL_SHA256",
    "make check-git-floor",
):
    if needle not in body:
        print(f"FAIL: check-git-2-34 missing {needle}")
        sys.exit(1)
# Lean (b): version-pinned leg must NOT run bare make check / commit-subjects.
cleaned = "\n".join(ln for ln in body.splitlines() if not ln.lstrip().startswith("#"))
if re.search(r"(^|[\s;|&])make check(?!-)($|[\s;|&])", cleaned, re.M):
    print("FAIL: check-git-2-34 must not run bare make check (use check-git-floor)")
    sys.exit(1)
if "commit-subjects-check" in cleaned or "check-commit-subjects" in cleaned:
    print("FAIL: check-git-2-34 must not own commit-subjects (lean b)")
    sys.exit(1)
if "0ce6222bfd31938b29360150286b51c77c643fa97740b1d35b6d1ceef8b0ecd7" not in text:
    print("FAIL: workflow must pin git-2.34.0.tar.gz sha256")
    sys.exit(1)
if "2.34.0" not in text:
    print("FAIL: workflow must name Git 2.34.0")
    sys.exit(1)
print("PASS")
PY
) && GIT_MIN_STATUS=0 || GIT_MIN_STATUS=$?

if [[ $GIT_MIN_STATUS -eq 0 ]]; then
    pass "CI asserts Git floor on every job and pins exact 2.34.0 for check-git-2-34"
else
    fail "${GIT_MIN_RESULT:-git min CI assertions failed}"
fi

for s in assert-git-min.sh install-git-from-source.sh; do
    mode=$(git -C "$REPO_ROOT" ls-files -s -- "scripts/$s" | awk '{print $1}')
    if [[ "$mode" == "100755" ]]; then
        pass "scripts/$s git mode is 100755"
    else
        fail "scripts/$s git mode is '${mode:-missing}', expected 100755"
    fi
    if [[ -x "$REPO_ROOT/scripts/$s" ]]; then
        pass "scripts/$s executable on disk"
    else
        fail "scripts/$s not executable on disk"
    fi
done

echo "Test 10a: assert-git-min rejects prereleases and accepts windows/Apple suffixes..."

ASSERT_GIT_MIN="$REPO_ROOT/scripts/assert-git-min.sh"
AGM_DIR=$(mktemp -d)
write_fake_git() {
    local ver=$1
    cat >"$AGM_DIR/git" <<EOF
#!/bin/sh
echo 'git version ${ver}'
EOF
    chmod +x "$AGM_DIR/git"
}
run_assert_git_min() {
    env PATH="$AGM_DIR:$PATH" bash "$ASSERT_GIT_MIN"
}

write_fake_git "2.34.0"
if ! out=$(run_assert_git_min 2>&1); then
    fail "assert-git-min must accept 2.34.0 (got: $out)"
else
    pass "assert-git-min accepts 2.34.0"
fi

write_fake_git "2.34.0-rc1"
if out=$(run_assert_git_min 2>&1); then
    fail "assert-git-min must reject 2.34.0-rc1 (got: $out)"
else
    pass "assert-git-min rejects 2.34.0-rc1"
fi

write_fake_git "2.35.0-rc1"
if ! out=$(run_assert_git_min 2>&1); then
    fail "assert-git-min must accept 2.35.0-rc1 above floor (got: $out)"
else
    pass "assert-git-min accepts 2.35.0-rc1 above floor"
fi

write_fake_git "2.34.0.windows.1"
if ! out=$(run_assert_git_min 2>&1); then
    fail "assert-git-min must accept 2.34.0.windows.1 (got: $out)"
else
    pass "assert-git-min accepts 2.34.0.windows.1"
fi

write_fake_git "2.43.0 (Apple Git-155)"
if ! out=$(run_assert_git_min 2>&1); then
    fail "assert-git-min must accept Apple Git suffix (got: $out)"
else
    pass "assert-git-min accepts Apple Git suffix"
fi

write_fake_git "2.33.0"
if out=$(run_assert_git_min 2>&1); then
    fail "assert-git-min must reject 2.33.0 (got: $out)"
else
    pass "assert-git-min rejects 2.33.0"
fi
rm -rf "$AGM_DIR"

echo "Test 10: CI jobs whose make targets need origin/main must fetch that ref..."

ORIGIN_MAIN_RESULT=$(python3 - "$CI_YML" <<'PY'
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
            jobs[current] = "\n".join(buf)
        current = m.group(1)
        buf = []
        continue
    if current is not None:
        buf.append(line)
if current is not None:
    jobs[current] = "\n".join(buf)

# Targets that read origin/main (see Makefile check / commit-subjects-check / check-fast).
# Do not match make check-job / check-git-floor when looking for bare make check.
need_re = re.compile(
    r"(^|[\s;|&])make (?:check-fast|commit-subjects-check|check)(?!-)($|[\s;|&])",
    re.MULTILINE,
)
fetch_ok_re = re.compile(
    r"main:refs/remotes/origin/main|fetch-depth:\s*0\b",
)
subjects_re = re.compile(
    r"(^|[\s;|&])make commit-subjects-check($|[\s;|&])",
    re.MULTILINE,
)

needed_by = []
subjects_owners = []
for jid, body in jobs.items():
    cleaned = "\n".join(
        ln for ln in body.splitlines() if not ln.lstrip().startswith("#")
    )
    if need_re.search(cleaned):
        needed_by.append(jid)
        if not fetch_ok_re.search(cleaned):
            print(
                f"FAIL: job {jid} runs a make target that needs origin/main "
                "but neither fetches main:refs/remotes/origin/main nor sets fetch-depth: 0"
            )
            sys.exit(1)
    if subjects_re.search(cleaned):
        subjects_owners.append(jid)

if "check-git-2-34" in needed_by:
    print("FAIL: check-git-2-34 must not depend on origin/main (lean b: check-git-floor only)")
    sys.exit(1)
if subjects_owners != ["check-static"]:
    print(f"FAIL: exactly one job must own commit-subjects-check (want ['check-static'], got {subjects_owners})")
    sys.exit(1)
static = jobs.get("check-static", "")
if "pull_request.head.sha" not in static:
    print("FAIL: check-static must pin github.event.pull_request.head.sha for commit-subjects range")
    sys.exit(1)
if "fetch-depth: 0" not in static and "fetch-depth:0" not in static:
    print("FAIL: check-static must use fetch-depth: 0 for commit-subjects history")
    sys.exit(1)
# Push-to-main must not run the empty origin/main..HEAD gate.
if "github.event_name == 'pull_request'" not in static and 'github.event_name == "pull_request"' not in static:
    print("FAIL: check-static commit-subjects/fetch must be gated to pull_request (skip empty range on main push)")
    sys.exit(1)
# Shallow main fetch + full tip history widens origin/main..HEAD past the branch point.
if re.search(r"git fetch[^\n]*--depth(?:=\d+|\s+\d+)[^\n]*main:refs/remotes/origin/main", static):
    print("FAIL: check-static must fetch origin/main without --depth (checkout already fetch-depth: 0)")
    sys.exit(1)
if "main:refs/remotes/origin/main" not in static:
    print("FAIL: check-static must fetch main:refs/remotes/origin/main for commit-subjects")
    sys.exit(1)
print("PASS: origin/main mapped for " + ",".join(needed_by) + "; commit-subjects owner=check-static (PR-only; unshallow main)")
PY
) && ORIGIN_MAIN_STATUS=0 || ORIGIN_MAIN_STATUS=$?

if [[ $ORIGIN_MAIN_STATUS -eq 0 ]]; then
    pass "CI origin/main + single commit-subjects owner (${ORIGIN_MAIN_RESULT#PASS: })"
else
    fail "${ORIGIN_MAIN_RESULT:-origin/main fetch mapping failed}"
fi

echo "Test 11: check-commit-subjects fails loud on empty and merge-only ranges..."

CCS_DEMO=$(mktemp -d)
cleanup_ccs() { rm -rf "$CCS_DEMO"; }
(
    set -e
    cd "$CCS_DEMO"
    git init -q -b main
    git config user.email "ccs@example.com"
    git config user.name "ccs"
    # Isolated fixture: no global hooks / templates.
    git commit -q --allow-empty -m "feat(GIT234-MIN): base on main"
    git branch -M main
    git update-ref refs/remotes/origin/main HEAD

    # Empty range: HEAD == origin/main
    set +e
    out=$(bash "$REPO_ROOT/scripts/check-commit-subjects.sh" origin/main..HEAD 2>&1)
    st=$?
    set -e
    if [[ $st -eq 0 ]]; then
        echo "FAIL: empty range exited 0: $out"
        exit 1
    fi
    if ! printf '%s\n' "$out" | rg -q 'empty|refusing to pass while checking nothing'; then
        echo "FAIL: empty range missing loud message: $out"
        exit 1
    fi

    # Merge-only range: point origin/main at the side tip so origin/main..HEAD
    # is only the merge commit (both parents are reachable from the side tip).
    git checkout -q -b side
    git commit -q --allow-empty -m "feat(GIT234-MIN): side tip"
    side=$(git rev-parse HEAD)
    git checkout -q main
    git merge -q --no-ff -m "Merge ${side} into main" side
    git update-ref refs/remotes/origin/main "$side"

    set +e
    out=$(bash "$REPO_ROOT/scripts/check-commit-subjects.sh" origin/main..HEAD 2>&1)
    st=$?
    set -e
    if [[ $st -eq 0 ]]; then
        echo "FAIL: merge-only range exited 0: $out"
        exit 1
    fi
    if ! printf '%s\n' "$out" | rg -q 'only merge commits|merge-only'; then
        echo "FAIL: merge-only range missing loud message: $out"
        exit 1
    fi
) && CCS_STATUS=0 || CCS_STATUS=$?
cleanup_ccs

if [[ $CCS_STATUS -eq 0 ]]; then
    pass "check-commit-subjects fails on empty and merge-only ranges"
else
    fail "check-commit-subjects empty/merge-only demo failed"
fi

echo "Test 12: shallow --depth=50 origin/main widens range past branch point (>50 behind)..."

DEPTH_DEMO=$(mktemp -d)
cleanup_depth() { rm -rf "$DEPTH_DEMO"; }
(
    set -e
    # Bare "upstream" with 60 commits on main; PR branch forks early so it is
    # >50 commits behind tip of main.
    upstream="$DEPTH_DEMO/upstream.git"
    work="$DEPTH_DEMO/work"
    git init -q --bare "$upstream"
    git clone -q "$upstream" "$work"
    cd "$work"
    git config user.email "depth@example.com"
    git config user.name "depth"
    git commit -q --allow-empty -m "chore(DEPTHDEMO): main commit 1"
    git branch -M main
    for i in $(seq 2 60); do
        git commit -q --allow-empty -m "chore(DEPTHDEMO): main commit $i"
    done
    fork=$(git rev-parse HEAD~55)
    git push -q origin main
    git checkout -q -b pr-branch "$fork"
    git commit -q --allow-empty -m "feat(GIT234-MIN): only PR tip commit"
    git push -q -u origin pr-branch

    # Two fresh PR-tip clones (checkout already fetch-depth:0). Compare a
    # depth-capped main fetch vs an uncapped one — do not reuse a shallow
    # repo for the uncapped case (.git/shallow poisons merge-base).
    shallow_clone="$DEPTH_DEMO/shallow"
    git clone -q --branch pr-branch "$upstream" "$shallow_clone"
    git -C "$shallow_clone" fetch -q --no-tags --depth=50 origin main:refs/remotes/origin/main
    shallow_count=$(git -C "$shallow_clone" rev-list --count origin/main..HEAD)

    full_clone="$DEPTH_DEMO/full"
    git clone -q --branch pr-branch "$upstream" "$full_clone"
    git -C "$full_clone" fetch -q --no-tags origin main:refs/remotes/origin/main
    full_count=$(git -C "$full_clone" rev-list --count origin/main..HEAD)

    if [[ "$full_count" -ne 1 ]]; then
        echo "FAIL: unshallow main fetch should leave origin/main..HEAD = 1 PR commit (got $full_count)"
        exit 1
    fi
    if [[ "$shallow_count" -le 1 ]]; then
        echo "FAIL: depth=50 main fetch should widen origin/main..HEAD past the branch point (got $shallow_count)"
        exit 1
    fi
    echo "PASS: depth=50 range=$shallow_count commits; unshallow range=$full_count (PR-only)"
) && DEPTH_STATUS=0 || DEPTH_STATUS=$?
cleanup_depth

if [[ $DEPTH_STATUS -eq 0 ]]; then
    pass "shallow origin/main fetch widens subject range; unshallow keeps PR-only"
else
    fail "depth=50 vs unshallow origin/main range demo failed"
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All check-job drift tests passed"
    exit 0
else
    echo "FAIL: $FAILURES check-job drift test(s) failed"
    exit 1
fi
