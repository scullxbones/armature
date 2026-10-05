#!/bin/bash

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
SCRIPT="$REPO_ROOT/scripts/check-fast.sh"

FAILURES=0

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

FIXTURE="$WORKDIR/fixture"
mkdir -p "$FIXTURE"
git -C "$FIXTURE" init -q
git -C "$FIXTURE" config user.email "test@example.com"
git -C "$FIXTURE" config user.name "Test"
git -C "$FIXTURE" config commit.gpgsign false
git -C "$FIXTURE" config tag.gpgsign false
git -C "$FIXTURE" checkout -q -b main

mkdir -p "$FIXTURE/pkg1" "$FIXTURE/pkg2" "$FIXTURE/cmd" "$FIXTURE/docs" "$FIXTURE/skills"
cat > "$FIXTURE/go.mod" <<'EOF'
module fixture

go 1.21
EOF
cat > "$FIXTURE/pkg1/pkg1.go" <<'EOF'
package pkg1

func Hello() string { return "hello" }
EOF
cat > "$FIXTURE/pkg2/pkg2.go" <<'EOF'
package pkg2

import "fixture/pkg1"

func Greet() string { return pkg1.Hello() }
EOF
cat > "$FIXTURE/cmd/main.go" <<'EOF'
package main

func main() {}
EOF
cat > "$FIXTURE/docs/README.md" <<'EOF'
# docs
EOF

git -C "$FIXTURE" add -A
git -C "$FIXTURE" commit -q -m "base"
BASE_SHA=$(git -C "$FIXTURE" rev-parse HEAD)

FAKE_BIN="$WORKDIR/bin"
mkdir -p "$FAKE_BIN"
MAKE_LOG="$WORKDIR/make.log"
cat > "$FAKE_BIN/make" <<EOF
#!/bin/bash
echo "\$@" >> "$MAKE_LOG"
exit 0
EOF
chmod +x "$FAKE_BIN/make"

run_check_fast() {
    rm -f "$MAKE_LOG"
    touch "$MAKE_LOG"
    (
        cd "$FIXTURE"
        PATH="$FAKE_BIN:$PATH" MAKE="$FAKE_BIN/make" BASE="$BASE_SHA" "$SCRIPT" "$FIXTURE"
    )
}

assert_make_target_ran() {
    local target="$1" label="$2"
    if grep -q "$target" "$MAKE_LOG"; then
        echo "  PASS: $label"
    else
        echo "  FAIL: expected make target '$target' to run ($label)"
        echo "  make.log contents:"; sed 's/^/    /' "$MAKE_LOG"
        FAILURES=$((FAILURES + 1))
    fi
}

assert_make_target_not_ran() {
    local target="$1" label="$2"
    if grep -q "$target" "$MAKE_LOG"; then
        echo "  FAIL: expected make target '$target' NOT to run ($label)"
        echo "  make.log contents:"; sed 's/^/    /' "$MAKE_LOG"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS: $label"
    fi
}

echo "Test 1: docs-only change routes to adr-principles only..."
echo "more docs" >> "$FIXTURE/docs/README.md"
git -C "$FIXTURE" add docs/README.md

OUTPUT=$(run_check_fast)
assert_make_target_ran "adr-principles" "adr-principles runs for docs-only change"
assert_make_target_not_ran "mutate" "docs-only must not run mutate"
assert_make_target_not_ran "coverage" "docs-only must not run coverage"
assert_make_target_not_ran "crosscompile" "docs-only must not run crosscompile"
assert_make_target_not_ran "census-drift-check" "docs-only must not run census-drift-check"
assert_make_target_not_ran "validate-skills" "docs-only must not run validate-skills"
assert_make_target_not_ran "build" "docs-only must not run build (no go changes)"

git -C "$FIXTURE" reset -q --hard "$BASE_SHA"

echo "Test 2: Go change routes to build + test on changed pkg plus reverse importers..."
cat >> "$FIXTURE/pkg1/pkg1.go" <<'EOF'

func Bye() string { return "bye" }
EOF
git -C "$FIXTURE" add pkg1/pkg1.go

OUTPUT=$(run_check_fast)
assert_make_target_ran "build" "build runs for go change"
assert_make_target_not_ran "mutate" "go change (fast gate) must not run mutate"
assert_make_target_not_ran "crosscompile" "go change (fast gate) must not run crosscompile"
assert_make_target_not_ran "census-drift-check" "pkg1 change is not a census surface"
assert_make_target_not_ran "validate-skills" "pkg1 change does not touch skills"

if grep -q "fixture/pkg1" <<< "$OUTPUT" && grep -q "fixture/pkg2" <<< "$OUTPUT"; then
    echo "  PASS: go test targets include changed package and reverse importer"
else
    echo "  FAIL: expected go test to include fixture/pkg1 and fixture/pkg2 (reverse importer)"
    echo "$OUTPUT" | sed 's/^/    /'
    FAILURES=$((FAILURES + 1))
fi

git -C "$FIXTURE" reset -q --hard "$BASE_SHA"

echo "Test 3: cmd/ change routes to census-drift-check..."
cat >> "$FIXTURE/cmd/main.go" <<'EOF'
// touch
EOF
git -C "$FIXTURE" add cmd/main.go

OUTPUT=$(run_check_fast)
assert_make_target_ran "census-drift-check" "census-drift-check runs for cmd/ change"
assert_make_target_not_ran "mutate" "cmd/ change (fast gate) must not run mutate"
assert_make_target_not_ran "validate-skills" "cmd/ change alone does not touch skills"

git -C "$FIXTURE" reset -q --hard "$BASE_SHA"

echo "Test 4: skills/ change routes to validate-skills + validate-doc-examples..."
echo "# skill" > "$FIXTURE/skills/SKILL.md"
git -C "$FIXTURE" add skills/SKILL.md

OUTPUT=$(run_check_fast)
assert_make_target_ran "validate-skills" "validate-skills runs for skills/ change"
assert_make_target_ran "validate-doc-examples" "validate-doc-examples runs for skills/ change"
assert_make_target_not_ran "census-drift-check" "skills/ change alone is not a census surface"
assert_make_target_not_ran "mutate" "skills/ change (fast gate) must not run mutate"

git -C "$FIXTURE" reset -q --hard "$BASE_SHA"
git -C "$FIXTURE" clean -q -fd skills

echo "Test 4b: .agents/skills/ change routes to validate-skills + validate-doc-examples..."
mkdir -p "$FIXTURE/.agents/skills/capturing-dogfood-findings"
echo "# capture" > "$FIXTURE/.agents/skills/capturing-dogfood-findings/SKILL.md"
git -C "$FIXTURE" add .agents/skills/capturing-dogfood-findings/SKILL.md

OUTPUT=$(run_check_fast)
assert_make_target_ran "validate-skills" "validate-skills runs for .agents/skills/ change"
assert_make_target_ran "validate-doc-examples" "validate-doc-examples runs for .agents/skills/ change"
assert_make_target_not_ran "census-drift-check" ".agents/skills/ change alone is not a census surface"
assert_make_target_not_ran "mutate" ".agents/skills/ change (fast gate) must not run mutate"

git -C "$FIXTURE" reset -q --hard "$BASE_SHA"
git -C "$FIXTURE" clean -q -fd .agents

echo "Test 4c: internal/skillsembed/skills/ change routes to validate-skills..."
mkdir -p "$FIXTURE/internal/skillsembed/skills/example"
echo "# skill" > "$FIXTURE/internal/skillsembed/skills/example/SKILL.md"
git -C "$FIXTURE" add internal/skillsembed/skills/example/SKILL.md

OUTPUT=$(run_check_fast)
assert_make_target_ran "validate-skills" "validate-skills runs for internal/skillsembed/skills/ change"
assert_make_target_ran "validate-doc-examples" "validate-doc-examples runs for internal/skillsembed/skills/ change"
assert_make_target_not_ran "mutate" "internal/skillsembed/skills/ change (fast gate) must not run mutate"

git -C "$FIXTURE" reset -q --hard "$BASE_SHA"
git -C "$FIXTURE" clean -q -fd internal

echo "Test 5: BASE= override changes the diff base..."
echo "more docs" >> "$FIXTURE/docs/README.md"
git -C "$FIXTURE" add docs/README.md
git -C "$FIXTURE" commit -q -m "docs change"
HEAD_SHA=$(git -C "$FIXTURE" rev-parse HEAD)

rm -f "$MAKE_LOG"; touch "$MAKE_LOG"
(
    cd "$FIXTURE"
    PATH="$FAKE_BIN:$PATH" MAKE="$FAKE_BIN/make" BASE="$HEAD_SHA" "$SCRIPT" "$FIXTURE"
) > "$WORKDIR/base-override-out.txt" 2>&1 || true

if grep -q "nothing to route" "$WORKDIR/base-override-out.txt"; then
    echo "  PASS: BASE= override respected (no diff against HEAD yields nothing to route)"
else
    echo "  FAIL: expected 'nothing to route' when BASE=HEAD"
    cat "$WORKDIR/base-override-out.txt" | sed 's/^/    /'
    FAILURES=$((FAILURES + 1))
fi

echo "Test 6: routed gate scripts are committed executable..."
for script in scripts/check-fast.sh scripts/test-check-fast.sh; do
    mode=$(git -C "$REPO_ROOT" ls-files -s -- "$script" | awk '{print $1}')
    if [[ "$mode" == "100755" ]]; then
        echo "  PASS: $script git mode is 100755"
    else
        echo "  FAIL: $script git mode is '${mode:-missing}', expected 100755"
        FAILURES=$((FAILURES + 1))
    fi
    if [[ -x "$REPO_ROOT/$script" ]]; then
        echo "  PASS: $script is executable on disk"
    else
        echo "  FAIL: $script is not executable on disk"
        FAILURES=$((FAILURES + 1))
    fi
done

echo "Test 7: check-core membership runs coverage-check, not a second suite..."
MAKEFILE="$REPO_ROOT/Makefile"
CI_YML="$REPO_ROOT/.github/workflows/ci.yml"
CORE_RESULT=$(python3 - "$MAKEFILE" <<'PY'
import sys
from pathlib import Path

makefile = Path(sys.argv[1]).read_text().splitlines()
roots = None
for line in makefile:
    if line.startswith("CHECK_JOB_check-core"):
        _, _, rhs = line.partition(":=")
        roots = rhs.split()
        break
if roots is None:
    print("FAIL: Makefile has no CHECK_JOB_check-core")
    sys.exit(1)
if "coverage-check" not in roots:
    print("FAIL: check-core does not include coverage-check")
    sys.exit(1)
if "test" in roots or "coverage" in roots:
    print("FAIL: check-core lists test/coverage as a sibling root (would duplicate the suite)")
    sys.exit(1)
print("PASS")
PY
) && CORE_STATUS=0 || CORE_STATUS=$?

if [[ $CORE_STATUS -eq 0 ]]; then
    echo "  PASS: check-core owns coverage-check only (no sibling test/coverage root)"
else
    echo "  $CORE_RESULT"
    FAILURES=$((FAILURES + 1))
fi

echo "Test 8: coverage-check depends on coverage (parallel-safe, single-run)..."
DEP_RESULT=$(python3 - "$MAKEFILE" "$CI_YML" <<'PY'
import sys
from pathlib import Path

makefile = Path(sys.argv[1]).read_text().splitlines()
ci = Path(sys.argv[2]).read_text()

prereqs = None
for line in makefile:
    if line.startswith("coverage-check:"):
        prereqs = line.split(":", 1)[1].split()
        break
if prereqs is None:
    print("FAIL: Makefile has no coverage-check target")
    sys.exit(1)
if "coverage" not in prereqs:
    print("FAIL: coverage-check does not depend on coverage; make -j can race")
    sys.exit(1)

runs = []
for line in ci.splitlines():
    stripped = line.strip()
    if stripped.startswith("run: make "):
        runs.append(stripped[len("run: make "):])
if any(r == "coverage" or r.startswith("coverage ") for r in runs):
    print("FAIL: CI still runs standalone make coverage; phony dep would re-run the suite")
    sys.exit(1)
if any(r == "test" or r.startswith("test ") for r in runs):
    print("FAIL: standalone make test still present in CI")
    sys.exit(1)
print("PASS")
PY
) && DEP_STATUS=0 || DEP_STATUS=$?

if [[ $DEP_STATUS -eq 0 ]]; then
    echo "  PASS: coverage-check depends on coverage; CI has no sibling make coverage/test"
else
    echo "  $DEP_RESULT"
    FAILURES=$((FAILURES + 1))
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All check-fast routing tests passed"
    exit 0
else
    echo "FAIL: $FAILURES check-fast routing test(s) failed"
    exit 1
fi
