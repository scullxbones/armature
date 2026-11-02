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
# Any shallowing of origin/main + full tip history widens origin/main..HEAD.
shallow_re = re.compile(
    r"git fetch[^\n]*(--depth(?:=\d+|\s+\d+)|--shallow-since=|--shallow-exclude=|--deepen(?:=\d+|\s+\d+))[^\n]*main:refs/remotes/origin/main"
)
if shallow_re.search(static):
    print(
        "FAIL: check-static must fetch origin/main without shallowing "
        "(--depth/--shallow-since/--shallow-exclude/--deepen); checkout already fetch-depth: 0"
    )
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

echo "Test 10b: own edited workflow rechecks quotes; CI never skips required jobs on edit..."

QUOTE_EDIT_RESULT=$(python3 - "$REPO_ROOT" <<'PY'
import re
import sys
from pathlib import Path

root = Path(sys.argv[1])
ci = (root / ".github/workflows/ci.yml").read_text()
edit_wf = root / ".github/workflows/pr-coverage-quotes-on-edit.yml"
if not edit_wf.is_file():
    print("FAIL: missing .github/workflows/pr-coverage-quotes-on-edit.yml")
    sys.exit(1)
edit = edit_wf.read_text()

# CI must NOT subscribe to edited (would emit required jobs as skipped).
if re.search(r"(?m)^  pull_request:\n(?:.*\n)*?    types:\s*\[[^\]]*edited", ci):
    print("FAIL: ci.yml pull_request must not list edited (own workflow owns that)")
    sys.exit(1)
if "pr-coverage-quotes-on-edit:" in ci:
    print("FAIL: pr-coverage-quotes-on-edit must not live in ci.yml")
    sys.exit(1)
if "github.event.action != 'edited'" in ci or 'github.event.action != "edited"' in ci:
    print("FAIL: ci.yml must not skip jobs on edited (never emit required names as skipped)")
    sys.exit(1)

# check-core: live body via gh api; upload before compare; always() upload.
if 'gh api "repos/${REPO}/pulls/${PR_NUMBER}"' not in ci and \
   "gh api \"repos/${REPO}/pulls/${PR_NUMBER}\"" not in ci:
    print("FAIL: check-core must fetch CURRENT body via gh api pulls/$N")
    sys.exit(1)
if "GITHUB_EVENT_PATH" in ci and "pull_request.body" in ci:
    print("FAIL: check-core must not read coverage body from GITHUB_EVENT_PATH")
    sys.exit(1)
# Upload step must use always() and precede the compare step.
upload_idx = ci.find("Upload coverage-check.log for late PR body paste")
compare_idx = ci.find("PR body coverage quotes match this job's coverage.out")
write_idx = ci.find("Write coverage-check.log")
if min(write_idx, upload_idx, compare_idx) < 0:
    print("FAIL: check-core missing write/upload/compare coverage-check steps")
    sys.exit(1)
if not (write_idx < upload_idx < compare_idx):
    print("FAIL: check-core must write log, upload (always), then compare")
    sys.exit(1)
upload_block = ci[upload_idx:compare_idx]
if "always()" not in upload_block:
    print("FAIL: coverage-check.log upload must use always() so failed compare still leaves artifact")
    sys.exit(1)
if "coverage-check-${{ github.event.pull_request.head.sha }}" not in ci and \
   "coverage-check-${{github.event.pull_request.head.sha}}" not in ci:
    print("FAIL: check-core must upload coverage-check-<head.sha> artifact")
    sys.exit(1)

# Own edited workflow contract.
if "types: [edited]" not in edit and 'types: ["edited"]' not in edit:
    print("FAIL: edit workflow must be on: pull_request types: [edited]")
    sys.exit(1)
if "pr-coverage-quotes-on-edit:" not in edit:
    print("FAIL: edit workflow missing pr-coverage-quotes-on-edit job")
    sys.exit(1)
if "assert-git-min.sh" not in edit:
    print("FAIL: edit workflow must run assert-git-min.sh")
    sys.exit(1)
if "ci-path-shadow-engines.sh" not in edit:
    print("FAIL: edit workflow must PATH-shadow container engines")
    sys.exit(1)
if "pr-coverage-quote-check.sh" not in edit:
    print("FAIL: edit workflow must invoke pr-coverage-quote-check.sh")
    sys.exit(1)
if "wait-coverage-check-artifact.sh" not in edit:
    print("FAIL: edit workflow must call wait-coverage-check-artifact.sh (poll loop not inline YAML)")
    sys.exit(1)
if "|| true" in edit and "select-coverage-check-artifact-id" in edit:
    print("FAIL: edit workflow must not || true the artifact selector")
    sys.exit(1)
if re.search(r"gh api[^\n]*--jq[^\n]*(--arg|--argjson)", edit):
    print("FAIL: gh api --jq does not accept --arg/--argjson (CI class failure on #432)")
    sys.exit(1)
if "ci-path-shadow-selfcheck.sh" not in edit:
    print("FAIL: edit workflow must run PATH-shadow self-check")
    sys.exit(1)
if 'gh api "repos/${REPO}/pulls/${PR_NUMBER}"' not in edit:
    print("FAIL: edit workflow must fetch CURRENT body via gh api")
    sys.exit(1)
# SHA-pinned checkout only (no @vN floating tags).
for m in re.finditer(r"uses:\s*(\S+)", edit):
    uses = m.group(1)
    if uses.startswith("./"):
        continue
    if "@" not in uses or not re.search(r"@[0-9a-f]{40}\b", uses):
        print(f"FAIL: edit workflow action not SHA-pinned: {uses}")
        sys.exit(1)

print("PASS")
PY
) && QUOTE_EDIT_STATUS=0 || QUOTE_EDIT_STATUS=$?

if [[ $QUOTE_EDIT_STATUS -eq 0 ]]; then
    pass "own edited workflow rechecks quotes; CI never skips required jobs on edit"
else
    fail "${QUOTE_EDIT_RESULT:-PR edited quote-check workflow assertion failed}"
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

echo "Test 13: content-hash porcelain; bash:3.2.57 digest pin; baseline fail-closed..."

PORC_DEMO=$(mktemp -d)
cleanup_porc() { rm -rf "$PORC_DEMO"; }
(
    set -e
    cd "$PORC_DEMO"
    git init -q -b main
    git config user.email "porc@example.com"
    git config user.name "porc"
    printf 'orig\n' > tracked.txt
    git add tracked.txt
    git commit -q -m "feat(PORC): seed tracked"
    # Pre-existing dirty tracked file — status line stays " M" across rewrite.
    printf 'dirty-v1\n' > tracked.txt
    baseline=$(mktemp)
    bash "$REPO_ROOT/scripts/check-test-porcelain.sh" --write-baseline . "$baseline"
    if ! grep -q $'\t' "$baseline"; then
        echo "FAIL: content-hash baseline missing TAB+hash: $(cat "$baseline")"
        exit 1
    fi

    # Rewrite already-modified tracked file (porcelain status unchanged, hash changes).
    printf 'dirty-v2\n' > tracked.txt
    set +e
    out=$(bash "$REPO_ROOT/scripts/check-test-porcelain.sh" . "$baseline" 2>&1)
    st=$?
    set -e
    if [[ $st -eq 0 ]]; then
        echo "FAIL: rewrite of already-modified tracked file did not fail: $out"
        exit 1
    fi
    if ! printf '%s\n' "$out" | rg -q 'tracked\.txt|hash|baseline'; then
        echo "FAIL: rewrite failure missing path/hash signal: $out"
        exit 1
    fi
    printf 'dirty-v1\n' > tracked.txt
    bash "$REPO_ROOT/scripts/check-test-porcelain.sh" . "$baseline" >/dev/null

    # -z path fixtures: spaces, quotes, non-ASCII must round-trip in baseline.
    printf 'x\n' > "spa ce.txt"
    printf 'y\n' > $'quot"ed.txt'
    printf 'z\n' > $'unicodé.txt'
    set +e
    out=$(bash "$REPO_ROOT/scripts/check-test-porcelain.sh" . "$baseline" 2>&1)
    st=$?
    set -e
    if [[ $st -eq 0 ]]; then
        echo "FAIL: special-name leaks did not fail check-test-porcelain: $out"
        exit 1
    fi
    for needle in 'spa ce.txt' 'quot"ed.txt' 'unicodé.txt'; do
        if ! printf '%s\n' "$out" | rg -F -q "$needle"; then
            echo "FAIL: special path missing from porcelain output ($needle): $out"
            exit 1
        fi
    done
    rm -f "spa ce.txt" $'quot"ed.txt' $'unicodé.txt'

    printf 'x\n' > leaked-file.txt
    set +e
    out=$(bash "$REPO_ROOT/scripts/check-test-porcelain.sh" . "$baseline" 2>&1)
    st=$?
    set -e
    rm -f leaked-file.txt
    if [[ $st -eq 0 ]]; then
        echo "FAIL: leaked file did not fail check-test-porcelain: $out"
        exit 1
    fi
    if ! printf '%s\n' "$out" | rg -q 'leaked-file\.txt'; then
        echo "FAIL: leaked file missing from porcelain output: $out"
        exit 1
    fi

    mkdir -p leaked-dir
    printf 'y\n' > leaked-dir/inner.txt
    set +e
    out=$(bash "$REPO_ROOT/scripts/check-test-porcelain.sh" . "$baseline" 2>&1)
    st=$?
    set -e
    rm -rf leaked-dir
    if [[ $st -eq 0 ]]; then
        echo "FAIL: leaked dir did not fail check-test-porcelain: $out"
        exit 1
    fi
    if ! printf '%s\n' "$out" | rg -q 'leaked-dir'; then
        echo "FAIL: leaked dir missing from porcelain output: $out"
        exit 1
    fi
    rm -f "$baseline"

    # Non-repo root: no-baseline and --write-baseline must fail closed.
    nonrepo=$(mktemp -d)
    set +e
    out=$(bash "$REPO_ROOT/scripts/check-test-porcelain.sh" "$nonrepo" 2>&1)
    st=$?
    set -e
    if [[ $st -eq 0 ]]; then
        echo "FAIL: no-baseline on non-repo exited 0: $out"
        exit 1
    fi
    set +e
    out=$(bash "$REPO_ROOT/scripts/check-test-porcelain.sh" --write-baseline "$nonrepo" "$nonrepo/base" 2>&1)
    st=$?
    set -e
    if [[ $st -eq 0 ]]; then
        echo "FAIL: --write-baseline on non-repo exited 0: $out"
        exit 1
    fi
    rm -rf "$nonrepo"

    # 0-byte baseline file must fail on check.
    zero=$(mktemp)
    : >"$zero"
    set +e
    out=$(bash "$REPO_ROOT/scripts/check-test-porcelain.sh" . "$zero" 2>&1)
    st=$?
    set -e
    rm -f "$zero"
    if [[ $st -eq 0 ]]; then
        echo "FAIL: 0-byte baseline check exited 0: $out"
        exit 1
    fi

    # Host Bash 4+: fixtures must exit 0 (dual self-check; 3.2 failure is CI).
    bash "$REPO_ROOT/scripts/prove-bash32-mutations.sh" >/dev/null

    for mut in readarray coproc v-test; do
        f="$REPO_ROOT/scripts/testdata/bash32-mutations/${mut}.sh"
        if ! rg -q 'set -euo pipefail' "$f"; then
            echo "FAIL: $mut fixture missing set -euo pipefail"
            exit 1
        fi
    done

    # Manifest-keyed bash:3.2.57 + apk sha256 + sole wrapper + PATH-shadow in every job.
    MANIFEST="$REPO_ROOT/scripts/container-images.manifest"
    if ! rg -q 'bash-3\.2\.57[[:space:]]+bash:3\.2\.57@sha256:b6252c62fc86114f93f02104a486c71e6569371d95950ea7479a2ffd6b47f16a' "$MANIFEST"; then
        echo "FAIL: container-images.manifest missing bash-3.2.57 digest"
        exit 1
    fi
    if ! rg -q 'run-container-image\.sh bash-3\.2\.57' "$CI_YML"; then
        echo "FAIL: ci.yml must invoke run-container-image.sh bash-3.2.57"
        exit 1
    fi
    if ! rg -q 'ci-path-shadow-engines\.sh' "$CI_YML"; then
        echo "FAIL: ci.yml missing PATH-shadow install"
        exit 1
    fi
    shadow_jobs=$(rg -c 'ci-path-shadow-engines\.sh' "$CI_YML" || true)
    if [[ "${shadow_jobs:-0}" -lt 9 ]]; then
        echo "FAIL: PATH-shadow missing from some CI jobs (found $shadow_jobs)"
        exit 1
    fi
    if rg -n -e 'docker' -e 'podman' -e 'buildx' "$CI_YML" \
        | rg -v 'path-shadow|run-container-image|container-images|^\s*#' \
        | rg -q .; then
        echo "FAIL: ci.yml still contains raw docker|podman|buildx token"
        exit 1
    fi
    if [[ ! -f "$REPO_ROOT/scripts/alpine-v322-git-apks.sha256" ]]; then
        echo "FAIL: missing scripts/alpine-v322-git-apks.sha256 closed apk set"
        exit 1
    fi
    if ! rg -q '42f4573799ffce0c7dd4954d2247eb882eb87360dc18637930624484ecfd1c90  git-2\.49\.1-r0\.apk' \
        "$REPO_ROOT/scripts/alpine-v322-git-apks.sha256"; then
        echo "FAIL: alpine-v322-git-apks.sha256 missing git-2.49.1-r0 pin"
        exit 1
    fi
    mode=$(git -C "$REPO_ROOT" ls-files -s -- "scripts/fetch-alpine-git-apks.sh" | awk '{print $1}')
    if [[ "$mode" != "100755" ]]; then
        echo "FAIL: scripts/fetch-alpine-git-apks.sh must be committed executable (got ${mode:-missing})"
        exit 1
    fi
    if ! rg -q 'fetch-alpine-git-apks\.sh' "$CI_YML"; then
        echo "FAIL: ci.yml must fetch closed Alpine git apk set via fetch-alpine-git-apks.sh"
        exit 1
    fi
    if rg -n '/apks/\*\.apk' "$CI_YML" | rg -v '^\s*[0-9]+:\s*#' | rg -q .; then
        echo "FAIL: ci.yml still uses /apks/*.apk glob (must list basenames from sha256 file)"
        exit 1
    fi
    if ! rg -q 'alpine-v322-git-apks\.sha256' "$CI_YML"; then
        echo "FAIL: ci.yml must derive apk basenames from alpine-v322-git-apks.sha256"
        exit 1
    fi
    if ! rg -q 'apk add --no-network --allow-untrusted \$apks' "$CI_YML"; then
        echo "FAIL: ci.yml missing apk add --no-network over explicit \$apks basenames"
        exit 1
    fi
    if ! rg -q 'apk add --no-network' "$CI_YML"; then
        echo "FAIL: ci.yml missing apk add --no-network"
        exit 1
    fi
    RELEASE_YML="$REPO_ROOT/.github/workflows/release.yml"
    if ! rg -q 'ci-path-shadow-engines\.sh' "$RELEASE_YML"; then
        echo "FAIL: release.yml missing PATH-shadow install"
        exit 1
    fi
    if ! rg -q 'ci-path-shadow-selfcheck\.sh' "$RELEASE_YML"; then
        echo "FAIL: release.yml missing PATH-shadow self-check"
        exit 1
    fi
    if ! rg -q 'check-bash32:' "$CI_YML"; then
        echo "FAIL: ci.yml missing check-bash32 job"
        exit 1
    fi
    if [[ ! -f "$REPO_ROOT/scripts/porcelain-git.sh" ]]; then
        echo "FAIL: missing scripts/porcelain-git.sh helper"
        exit 1
    fi
    up=$(cd "$REPO_ROOT" && GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/armature-gocache}" GOFLAGS="${GOFLAGS:--buildvcs=false}" go list ./... | rg 'workflowimagepins' || true)
    if [[ -z "$up" ]]; then
        echo "FAIL: scripts/workflowimagepins not listable"
        exit 1
    fi
    if rg -q "grep -v '/scripts/workflowimagepins" "$MAKEFILE"; then
        echo "FAIL: Makefile still excludes workflowimagepins from UNIT_PACKAGES"
        exit 1
    fi

    # Porcelain confined + image-pin selftest (refs blocking; engine words advisory).
    # Guards must exit 0 here (assertion strings below avoid banned class tokens).
    bash "$REPO_ROOT/scripts/check-porcelain-failclosed.sh" "$REPO_ROOT" >/dev/null
    bash "$REPO_ROOT/scripts/check-no-engine-bin-env.sh" "$REPO_ROOT" >/dev/null
    bash "$REPO_ROOT/scripts/check-workflow-image-pins.sh" "$REPO_ROOT" >/dev/null
    # Ban proof lives in scripts/check-no-engine-bin-env.sh (fixture + scan).
    if ! rg -q 'check-no-engine-bin-env\.sh' "$REPO_ROOT/Makefile"; then
        echo "FAIL: Makefile lint missing check-no-engine-bin-env.sh"
        exit 1
    fi
    for bypass in git-c-status.sh var-git-status.sh alias-git-status.sh \
        eval-git-status.sh xargs-git.sh \
        cmdsub-status-or-true.sh status-semicolon.sh status-pipe-cat.sh \
        func-git-status.sh no-pager-status-or-true.sh extless-git-status; do
        if [[ ! -f "$REPO_ROOT/scripts/testdata/porcelain-bypass/$bypass" ]]; then
            echo "FAIL: missing porcelain bypass fixture $bypass"
            exit 1
        fi
    done
    # Split tokens so this file is not itself a porcelain/xargs/command-p hit.
    _alias_pat="alias gs='gi"
    _alias_pat="${_alias_pat}t sta"
    _alias_pat="${_alias_pat}tus'"
    if ! rg -F -q "$_alias_pat" "$REPO_ROOT/scripts/testdata/porcelain-bypass/alias-git-status.sh"; then
        echo "FAIL: alias-git-status.sh must be a real alias fixture"
        exit 1
    fi
    _xa='xar'; _xa="${_xa}gs"
    _gw='gi'; _gw="${_gw}t"
    if ! rg -q "${_xa}[[:space:]]+${_gw}" "$REPO_ROOT/scripts/testdata/porcelain-bypass/xargs-git.sh"; then
        echo "FAIL: xargs-git.sh fixture missing ${_xa}→${_gw} form"
        exit 1
    fi
    if ! rg -q 'XARGS_RE=' "$REPO_ROOT/scripts/check-porcelain-failclosed.sh"; then
        echo "FAIL: porcelain guard missing XARGS_RE ban"
        exit 1
    fi
    # Shared non-binary file selector (no ext/shebang filter) — class for porcelain + engine.
    if [[ ! -f "$REPO_ROOT/scripts/list-guard-scan-files.sh" ]]; then
        echo "FAIL: missing scripts/list-guard-scan-files.sh shared selector"
        exit 1
    fi
    if ! rg -q 'list-guard-scan-files\.sh' "$REPO_ROOT/scripts/check-porcelain-failclosed.sh"; then
        echo "FAIL: porcelain guard must use list-guard-scan-files.sh"
        exit 1
    fi
    if ! rg -q 'list-guard-scan-files\.sh' "$REPO_ROOT/scripts/check-no-engine-bin-env.sh"; then
        echo "FAIL: engine guard must use list-guard-scan-files.sh"
        exit 1
    fi
    # Mutation: restoring ext/shebang filters must fail this REQ.
    if rg -q 'is_scanned_script' "$REPO_ROOT/scripts/check-porcelain-failclosed.sh"; then
        echo "FAIL: porcelain must not restore is_scanned_script ext/shebang filter"
        exit 1
    fi
    if rg -q -- '-name '\''\*\.sh'\''' "$REPO_ROOT/scripts/check-no-engine-bin-env.sh"; then
        echo "FAIL: engine guard must not restore extension find filter"
        exit 1
    fi
    if ! rg -q 'ARM_PORCELAIN_SKIP_SELFTEST' "$REPO_ROOT/scripts/check-porcelain-failclosed.sh"; then
        echo "FAIL: porcelain guard must support SKIP_SELFTEST for real-guard selftest"
        exit 1
    fi
    if ! rg -q 'real guard' "$REPO_ROOT/scripts/testdata/porcelain-bypass/run-selftest.sh"; then
        echo "FAIL: porcelain selftest must drive the real guard"
        exit 1
    fi
    # Selftest integrity: require specific FAIL: scripts/<fixture> (not any nonzero).
    if ! rg -q 'FAIL: scripts/' "$REPO_ROOT/scripts/testdata/porcelain-bypass/run-selftest.sh"; then
        echo "FAIL: porcelain selftest must require FAIL: scripts/<fixture> line"
        exit 1
    fi
    # REQ: extensionless no-shebang + .txt must be scanned by both guards.
    _gw='gi'; _gw="${_gw}t"
    _st='sta'; _st="${_st}tus"
    demo_sel=$(mktemp -d)
    mkdir -p "$demo_sel/scripts" "$demo_sel/.github"
    # Repro: printf '<git> <status>\n' > scripts/zz (no shebang) must fail porcelain.
    printf '%s %s\n' "$_gw" "$_st" >"$demo_sel/scripts/zz"
    printf '%s %s\n' "$_gw" "$_st" >"$demo_sel/scripts/leak.txt"
    # Engine tokens in extensionless + .txt must fail engine guard.
    _u='/usr'; _db='/bin/doc'; _db="${_db}ker"
    printf '%s\n' "${_u}${_db}" >"$demo_sel/scripts/zz_engine"
    printf '%s\n' "${_u}${_db}" >"$demo_sel/scripts/engine.txt"
    cp "$REPO_ROOT/scripts/list-guard-scan-files.sh" "$demo_sel/scripts/"
    cp "$REPO_ROOT/scripts/check-porcelain-failclosed.sh" "$demo_sel/scripts/"
    cp "$REPO_ROOT/scripts/check-no-engine-bin-env.sh" "$demo_sel/scripts/"
    chmod +x "$demo_sel/scripts/"*.sh
    set +e
    pout=$(ARM_PORCELAIN_SKIP_SELFTEST=1 bash "$demo_sel/scripts/check-porcelain-failclosed.sh" "$demo_sel" 2>&1)
    pst=$?
    eout=$(bash "$demo_sel/scripts/check-no-engine-bin-env.sh" "$demo_sel" 2>&1)
    est=$?
    set -e
    rm -rf "$demo_sel"
    if [ "$pst" -eq 0 ] || ! printf '%s\n' "$pout" | grep -q 'FAIL: scripts/zz'; then
        echo "FAIL: porcelain must flag extensionless no-shebang scripts/zz: $pout"
        exit 1
    fi
    if ! printf '%s\n' "$pout" | grep -q 'FAIL: scripts/leak.txt'; then
        echo "FAIL: porcelain must flag scripts/leak.txt: $pout"
        exit 1
    fi
    if [ "$est" -eq 0 ] || ! printf '%s\n' "$eout" | grep -q 'FAIL: scripts/zz_engine'; then
        echo "FAIL: engine must flag extensionless scripts/zz_engine: $eout"
        exit 1
    fi
    if ! printf '%s\n' "$eout" | grep -q 'FAIL: scripts/engine.txt'; then
        echo "FAIL: engine must flag scripts/engine.txt: $eout"
        exit 1
    fi
    # REQ fail-closed 1–4 (PJ block on 9b6038e8): non-UTF8, NUL-later, main scope, lister crash.
    # Structural guards + live demos; each mutation must make the REQ demo exit 0 incorrectly.
    if ! rg -q 'LC_ALL=C grep -a' "$REPO_ROOT/scripts/check-porcelain-failclosed.sh"; then
        echo "FAIL: porcelain must scan with LC_ALL=C grep -a (non-UTF8 fail-closed)"
        exit 1
    fi
    if ! rg -q 'LC_ALL=C grep -a' "$REPO_ROOT/scripts/check-no-engine-bin-env.sh"; then
        echo "FAIL: engine must scan with LC_ALL=C grep -a (non-UTF8 fail-closed)"
        exit 1
    fi
    if ! rg -q 'bash_refuses' "$REPO_ROOT/scripts/list-guard-scan-files.sh"; then
        echo "FAIL: selector must only skip bash-refused (NUL-first) files"
        exit 1
    fi
    if rg -q "tr -d '\\\\000'|tr -d '\\\\0'" "$REPO_ROOT/scripts/list-guard-scan-files.sh"; then
        echo "FAIL: selector must not skip every NUL-bearing file (tr -d NUL raw/stripped)"
        exit 1
    fi
    if rg -q 'is_non_binary' "$REPO_ROOT/scripts/list-guard-scan-files.sh"; then
        echo "FAIL: selector must not restore is_non_binary any-NUL skip"
        exit 1
    fi
    if ! rg -q 'scripts/testdata/\*' "$REPO_ROOT/scripts/list-guard-scan-files.sh"; then
        echo "FAIL: --exclude-testdata must match scripts/testdata/* only (main scope)"
        exit 1
    fi
    if rg -q 'case "\$f" in' -A2 "$REPO_ROOT/scripts/list-guard-scan-files.sh" \
        | rg -q '\*/testdata/\*'; then
        echo "FAIL: selector case must not exclude nested */testdata/* (narrower than main)"
        exit 1
    fi
    if rg -q 'scripts/repoguard/\*\) continue' "$REPO_ROOT/scripts/check-porcelain-failclosed.sh"; then
        echo "FAIL: porcelain must not skip scripts/repoguard/* (narrower than main)"
        exit 1
    fi
    if ! rg -q 'lister_st' "$REPO_ROOT/scripts/check-porcelain-failclosed.sh" \
        || ! rg -q 'lister_st' "$REPO_ROOT/scripts/check-no-engine-bin-env.sh"; then
        echo "FAIL: guards must check lister exit status (fail-closed crash)"
        exit 1
    fi
    # Live demos (parallel-safe tempdirs).
    _gw='gi'; _gw="${_gw}t"
    _st='sta'; _st="${_st}tus"
    _u='/usr'; _db='/bin/doc'; _db="${_db}ker"
    # 1) Non-UTF8 \xff on hit line — porcelain + engine must fail.
    demo_ff=$(mktemp -d)
    mkdir -p "$demo_ff/scripts" "$demo_ff/.github"
    printf '%s %s' "$_gw" "$_st" >"$demo_ff/scripts/zz"; printf '\xff\n' >>"$demo_ff/scripts/zz"
    printf '%s' "${_u}${_db}" >"$demo_ff/scripts/ez"; printf '\xff\n' >>"$demo_ff/scripts/ez"
    cp "$REPO_ROOT/scripts/list-guard-scan-files.sh" "$demo_ff/scripts/"
    cp "$REPO_ROOT/scripts/check-porcelain-failclosed.sh" "$demo_ff/scripts/"
    cp "$REPO_ROOT/scripts/check-no-engine-bin-env.sh" "$demo_ff/scripts/"
    chmod +x "$demo_ff/scripts/"*.sh
    set +e
    pout=$(ARM_PORCELAIN_SKIP_SELFTEST=1 bash "$demo_ff/scripts/check-porcelain-failclosed.sh" "$demo_ff" 2>&1)
    pst=$?
    eout=$(bash "$demo_ff/scripts/check-no-engine-bin-env.sh" "$demo_ff" 2>&1)
    est=$?
    set -e
    if [ "$pst" -eq 0 ] || ! printf '%s\n' "$pout" | grep -q 'FAIL: scripts/zz'; then
        echo "FAIL: REQ1 porcelain must flag ${_gw} ${_st} + \\xff: $pout"
        rm -rf "$demo_ff"; exit 1
    fi
    if [ "$est" -eq 0 ] || ! printf '%s\n' "$eout" | grep -q 'FAIL: scripts/ez'; then
        echo "FAIL: REQ1 engine must flag abs docker + \\xff: $eout"
        rm -rf "$demo_ff"; exit 1
    fi
    # Mutation: drop LC_ALL=C → UTF-8 locale misses \xff hit → exit 0.
    sed -i.bak 's/LC_ALL=C grep -a/grep -a/g' "$demo_ff/scripts/check-porcelain-failclosed.sh"
    set +e
    pout=$(ARM_PORCELAIN_SKIP_SELFTEST=1 bash "$demo_ff/scripts/check-porcelain-failclosed.sh" "$demo_ff" 2>&1)
    pst=$?
    set -e
    if [ "$pst" -ne 0 ]; then
        echo "FAIL: REQ1 mutation (no LC_ALL=C) must miss \\xff hit (want exit 0): $pout"
        rm -rf "$demo_ff"; exit 1
    fi
    rm -rf "$demo_ff"
    # 2) NUL later + porcelain tokens must fail (selector must list the file).
    demo_nul=$(mktemp -d)
    mkdir -p "$demo_nul/scripts"
    printf 'x\0' >"$demo_nul/scripts/zz"; printf '%s %s\n' "$_gw" "$_st" >>"$demo_nul/scripts/zz"
    cp "$REPO_ROOT/scripts/list-guard-scan-files.sh" "$demo_nul/scripts/"
    cp "$REPO_ROOT/scripts/check-porcelain-failclosed.sh" "$demo_nul/scripts/"
    chmod +x "$demo_nul/scripts/"*.sh
    set +e
    pout=$(ARM_PORCELAIN_SKIP_SELFTEST=1 bash "$demo_nul/scripts/check-porcelain-failclosed.sh" "$demo_nul" 2>&1)
    pst=$?
    set -e
    if [ "$pst" -eq 0 ] || ! printf '%s\n' "$pout" | grep -q 'FAIL: scripts/zz'; then
        echo "FAIL: REQ2 porcelain must flag NUL-later + ${_gw} ${_st}: $pout"
        rm -rf "$demo_nul"; exit 1
    fi
    # Mutation: restore any-NUL skip → REQ fails (exit 0).
    cat >"$demo_nul/scripts/list-guard-scan-files.sh" <<'MUT_NUL'
#!/usr/bin/env bash
set -euo pipefail
EXCLUDE_TESTDATA=0; dirs=()
while [ "$#" -gt 0 ]; do
  case "$1" in --exclude-testdata) EXCLUDE_TESTDATA=1; shift ;; *) dirs+=("$1"); shift ;; esac
done
is_non_binary() {
  local f="$1" raw stripped
  [ -f "$f" ] || return 1
  [ ! -s "$f" ] && return 0
  raw=$(wc -c <"$f" | tr -d ' '); stripped=$(tr -d '\000' <"$f" | wc -c | tr -d ' ')
  [ "$raw" = "$stripped" ]
}
list_tmp=$(mktemp); trap 'rm -f "$list_tmp"' EXIT
{ for d in "${dirs[@]}"; do [ -d "$d" ] && find "$d" -type f; done; } | LC_ALL=C sort -u >"$list_tmp"
while IFS= read -r f || [ -n "${f:-}" ]; do
  [ -z "${f:-}" ] && continue
  if [ "$EXCLUDE_TESTDATA" -eq 1 ]; then case "$f" in scripts/testdata/*|scripts/testdata) continue ;; esac; fi
  if is_non_binary "$f"; then printf '%s\n' "$f"; fi
done <"$list_tmp"
MUT_NUL
    chmod +x "$demo_nul/scripts/list-guard-scan-files.sh"
    set +e
    pout=$(ARM_PORCELAIN_SKIP_SELFTEST=1 bash "$demo_nul/scripts/check-porcelain-failclosed.sh" "$demo_nul" 2>&1)
    pst=$?
    set -e
    if [ "$pst" -ne 0 ]; then
        echo "FAIL: REQ2 mutation (any-NUL skip) must miss NUL-later hit (want exit 0): $pout"
        rm -rf "$demo_nul"; exit 1
    fi
    rm -rf "$demo_nul"
    # 3) Scope = main: nested */testdata/* and scripts/repoguard/* scanned; only scripts/testdata/* excluded.
    demo_scope=$(mktemp -d)
    mkdir -p "$demo_scope/scripts/other/testdata" "$demo_scope/scripts/repoguard" "$demo_scope/scripts/testdata"
    printf '%s %s\n' "$_gw" "$_st" >"$demo_scope/scripts/other/testdata/leak.sh"
    printf '%s %s\n' "$_gw" "$_st" >"$demo_scope/scripts/repoguard/leak.go"
    printf '%s %s\n' "$_gw" "$_st" >"$demo_scope/scripts/testdata/ok.sh"
    cp "$REPO_ROOT/scripts/list-guard-scan-files.sh" "$demo_scope/scripts/"
    cp "$REPO_ROOT/scripts/check-porcelain-failclosed.sh" "$demo_scope/scripts/"
    chmod +x "$demo_scope/scripts/"*.sh
    set +e
    pout=$(ARM_PORCELAIN_SKIP_SELFTEST=1 bash "$demo_scope/scripts/check-porcelain-failclosed.sh" "$demo_scope" 2>&1)
    pst=$?
    set -e
    if [ "$pst" -eq 0 ] \
        || ! printf '%s\n' "$pout" | grep -q 'FAIL: scripts/other/testdata/leak.sh' \
        || ! printf '%s\n' "$pout" | grep -q 'FAIL: scripts/repoguard/leak.go'; then
        echo "FAIL: REQ3 must scan nested testdata + repoguard: $pout"
        rm -rf "$demo_scope"; exit 1
    fi
    if printf '%s\n' "$pout" | grep -q 'scripts/testdata/ok.sh'; then
        echo "FAIL: REQ3 must NOT scan scripts/testdata/*: $pout"
        rm -rf "$demo_scope"; exit 1
    fi
    # Mutation: exclude */testdata/* broadly → nested leak missed.
    sed -i.bak 's|scripts/testdata/\*|*/testdata/*|g' "$demo_scope/scripts/list-guard-scan-files.sh"
    set +e
    pout=$(ARM_PORCELAIN_SKIP_SELFTEST=1 bash "$demo_scope/scripts/check-porcelain-failclosed.sh" "$demo_scope" 2>&1)
    pst=$?
    set -e
    if printf '%s\n' "$pout" | grep -q 'FAIL: scripts/other/testdata/leak.sh'; then
        echo "FAIL: REQ3 mutation (*/testdata/*) must miss nested leak: $pout"
        rm -rf "$demo_scope"; exit 1
    fi
    rm -rf "$demo_scope"
    # 4) Lister exit 2 + violating zz → guard must NOT exit 0.
    demo_crash=$(mktemp -d)
    mkdir -p "$demo_crash/scripts"
    printf '%s %s\n' "$_gw" "$_st" >"$demo_crash/scripts/zz"
    cat >"$demo_crash/scripts/list-guard-scan-files.sh" <<'MUT_CRASH'
#!/usr/bin/env bash
exit 2
MUT_CRASH
    cp "$REPO_ROOT/scripts/check-porcelain-failclosed.sh" "$demo_crash/scripts/"
    chmod +x "$demo_crash/scripts/"*.sh
    set +e
    pout=$(ARM_PORCELAIN_SKIP_SELFTEST=1 bash "$demo_crash/scripts/check-porcelain-failclosed.sh" "$demo_crash" 2>&1)
    pst=$?
    set -e
    if [ "$pst" -eq 0 ]; then
        echo "FAIL: REQ4 porcelain must fail when lister exits 2: $pout"
        rm -rf "$demo_crash"; exit 1
    fi
    # Mutation: restore heredoc $(lister) without exit check → exit 0 despite crash.
    cat >"$demo_crash/scripts/check-porcelain-failclosed.sh" <<'MUT_HEREDOC'
#!/usr/bin/env bash
set -euo pipefail
ROOT="${1:-.}"; SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"; LISTER="$SCRIPT_DIR/list-guard-scan-files.sh"
cd "$ROOT"; fail=0
HELPER="scripts/porcelain-git.sh"
_gw='gi'; _gw="${_gw}t"; _st='sta'; _st="${_st}tus"
CLASS_RE="(^|[^A-Za-z0-9_-])${_gw}([^A-Za-z0-9_-].*)?${_st}([^A-Za-z0-9_-]|$)"
scan_file() {
  local f="$1" hits
  hits="$(LC_ALL=C grep -a -nE "$CLASS_RE" "$f" || true)"
  if [ -n "$hits" ] && [ "$f" != "$HELPER" ]; then
    echo "FAIL: $f invokes porcelain" >&2; fail=1
  fi
}
while IFS= read -r f || [ -n "${f:-}" ]; do
  [ -z "${f:-}" ] && continue
  scan_file "$f"
done <<EOF
$(bash "$LISTER" --exclude-testdata scripts)
EOF
if [ "$fail" -ne 0 ]; then exit 1; fi
echo "OK: swallowed"
MUT_HEREDOC
    chmod +x "$demo_crash/scripts/check-porcelain-failclosed.sh"
    set +e
    pout=$(ARM_PORCELAIN_SKIP_SELFTEST=1 bash "$demo_crash/scripts/check-porcelain-failclosed.sh" "$demo_crash" 2>&1)
    pst=$?
    set -e
    if [ "$pst" -ne 0 ]; then
        echo "FAIL: REQ4 mutation (heredoc swallow) must exit 0 with crashing lister: st=$pst $pout"
        rm -rf "$demo_crash"; exit 1
    fi
    rm -rf "$demo_crash"
    for engfix in banned-export.sh banned-command-p-docker.sh banned-abs-docker.sh; do
        if [[ ! -f "$REPO_ROOT/scripts/testdata/engine-bin-env/$engfix" ]]; then
            echo "FAIL: missing engine-bin ban fixture $engfix"
            exit 1
        fi
    done
    if ! rg -q 'CMD_P_RE=' "$REPO_ROOT/scripts/check-no-engine-bin-env.sh"; then
        echo "FAIL: check-no-engine-bin-env.sh missing command -p <engine> ban"
        exit 1
    fi
    if ! rg -q 'ABS_RE=' "$REPO_ROOT/scripts/check-no-engine-bin-env.sh"; then
        echo "FAIL: check-no-engine-bin-env.sh missing absolute-path engine ban"
        exit 1
    fi
    if ! rg -q 'alpine:latest' "$REPO_ROOT/scripts/testdata/workflow-image-pins/unpinned/container-string.yml"; then
        echo "FAIL: container-string fixture must use alpine:latest"
        exit 1
    fi
    if ! rg -q 'docker://alpine:latest' "$REPO_ROOT/scripts/testdata/workflow-image-pins/unpinned/uses-docker.yml"; then
        echo "FAIL: uses-docker fixture must use docker://alpine:latest"
        exit 1
    fi

    # Wrapper airtight: reject positionals-before-key, image-shaped keys, missing --,
    # and dropped docker-run flags outside the closed allowlist.
    set +e
    wout=$(bash "$REPO_ROOT/scripts/run-container-image.sh" --rm bash-3.2.57 -- true 2>&1)
    wst=$?
    set -e
    if [[ $wst -eq 0 ]]; then
        echo "FAIL: wrapper accepted flag before key: $wout"
        exit 1
    fi
    set +e
    wout=$(bash "$REPO_ROOT/scripts/run-container-image.sh" 'alpine:latest' -- true 2>&1)
    wst=$?
    set -e
    if [[ $wst -eq 0 ]]; then
        echo "FAIL: wrapper accepted image-shaped key: $wout"
        exit 1
    fi
    set +e
    wout=$(bash "$REPO_ROOT/scripts/run-container-image.sh" bash-3.2.57 --rm 2>&1)
    wst=$?
    set -e
    if [[ $wst -eq 0 ]]; then
        echo "FAIL: wrapper accepted missing -- separator: $wout"
        exit 1
    fi
    set +e
    wout=$(bash "$REPO_ROOT/scripts/run-container-image.sh" bash-3.2.57 --privileged -- true 2>&1)
    wst=$?
    set -e
    if [[ $wst -eq 0 ]]; then
        echo "FAIL: wrapper accepted dropped --privileged flag: $wout"
        exit 1
    fi
    if ! printf '%s\n' "$wout" | rg -q 'unknown/disallowed docker-run flag'; then
        echo "FAIL: wrapper --privileged rejection missing disallow message: $wout"
        exit 1
    fi
    if ! rg -q 'Closed docker-run flag set' "$REPO_ROOT/scripts/run-container-image.sh"; then
        echo "FAIL: run-container-image.sh missing closed flag-set documentation"
        exit 1
    fi
    # Closed set must not re-admit dropped flags.
    flag_allow=$(sed -n '/^flag_bool/,/^}/p; /^flag_value/,/^}/p' "$REPO_ROOT/scripts/run-container-image.sh")
    if printf '%s\n' "$flag_allow" | rg -q 'privileged|publish|health-cmd|--device'; then
        echo "FAIL: wrapper flag allowlist still contains dropped flags"
        exit 1
    fi

    # PJ repro forms must fail via Go selftest fixtures.
    for form in cmd-sub subshell docker-context compose-run bare-digest uses-docker uses-docker-empty \
        container-string container-image shell-key services-image; do
        if [[ ! -f "$REPO_ROOT/scripts/testdata/workflow-image-pins/unpinned/${form}.yml" ]]; then
            echo "FAIL: missing unpinned PJ/form fixture $form"
            exit 1
        fi
        if [[ ! -f "$REPO_ROOT/scripts/testdata/workflow-image-pins/pinned/${form}.yml" ]]; then
            echo "FAIL: missing pinned twin $form"
            exit 1
        fi
    done
    for dform in dockerfile-from from-platform from-scratch from-stage; do
        if [[ ! -f "$REPO_ROOT/scripts/testdata/workflow-image-pins/unpinned-dirs/${dform}/Dockerfile" ]]; then
            echo "FAIL: missing unpinned-dirs/$dform fixture"
            exit 1
        fi
    done

    # Makefile write-baseline failure must stop make (not ignored via ';').
    if ! rg -q 'check-test-porcelain\.sh --write-baseline .* \|\| \{' "$MAKEFILE"; then
        echo "FAIL: Makefile write-baseline missing || fail-closed"
        exit 1
    fi
    mkdemo=$(mktemp -d)
    mkdir -p "$mkdemo/scripts"
    printf '%s\n' '#!/usr/bin/env bash' 'echo forced-baseline-failure >&2' 'exit 1' \
        >"$mkdemo/scripts/check-test-porcelain.sh"
    chmod +x "$mkdemo/scripts/check-test-porcelain.sh"
    cat >"$mkdemo/Makefile" <<'EOF'
test:
	@tmp=$$(mktemp); \
	pre=$$(mktemp); \
	scripts/check-test-porcelain.sh --write-baseline . "$$pre" || { rm -f "$$pre" "$$tmp"; exit 1; }; \
	echo SHOULD_NOT_REACH; \
	exit 0
EOF
    set +e
    mkout=$(make -C "$mkdemo" test 2>&1)
    mkst=$?
    set -e
    rm -rf "$mkdemo"
    if [[ $mkst -eq 0 ]]; then
        echo "FAIL: forced write-baseline failure did not stop make: $mkout"
        exit 1
    fi
    if printf '%s\n' "$mkout" | rg -q 'SHOULD_NOT_REACH'; then
        echo "FAIL: make continued after write-baseline failure: $mkout"
        exit 1
    fi

    echo "PASS: content-hash + manifest image pin + baseline fail-closed"
) && PORC_STATUS=0 || PORC_STATUS=$?
cleanup_porc

if [[ $PORC_STATUS -eq 0 ]]; then
    pass "content-hash porcelain + manifest image pin + baseline fail-closed"
else
    fail "porcelain / bash32 pin / baseline fail-closed demo failed"
fi


echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All check-job drift tests passed"
    exit 0
else
    echo "FAIL: $FAILURES check-job drift test(s) failed"
    exit 1
fi
