#!/bin/bash
# Fixture test for the internal-package surface-budget ratchet.
#
# Locks in:
#   1. check passes against a matching baseline
#   2. adding an exported func fails (growth)
#   3. removing an exported func still passes (shrink)
#   4. -write rewrites the baseline down after a shrink
#   5. a new internal package with exports fails until ratcheted
#
# Wired into make check via the test-surface-budget-check target.

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
TOOL="$REPO_ROOT/scripts/surfacebudget"

FAILURES=0
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

if [[ ! -d "$TOOL" ]]; then
    echo "FAIL: surfacebudget tool missing at $TOOL"
    exit 1
fi

make_tree() {
    local dest="$1"
    mkdir -p "$dest/internal/pkg" "$dest/docs/design"
    cat > "$dest/internal/pkg/pkg.go" <<'EOF'
package pkg

type Client struct {
	Name string
}

func Hello() string { return "hello" }

func (c *Client) Greet() string { return c.Name }
EOF
}

run_tool() {
    local root="$1"
    shift
    set +e
    OUTPUT=$(go run "$TOOL" "$@" "$root" 2>&1)
    STATUS=$?
    set -e
}

echo "Test 1: check passes when current surface matches the baseline..."
TREE="$WORKDIR/match"
make_tree "$TREE"
run_tool "$TREE" -write
run_tool "$TREE" -check
if [[ $STATUS -ne 0 ]]; then
    echo "  FAIL: expected exit 0 on matching baseline"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "OK:" <<< "$OUTPUT"; then
    echo "  FAIL: expected OK on matching baseline"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

echo "Test 2: adding an exported func fails the check..."
TREE="$WORKDIR/grow"
make_tree "$TREE"
run_tool "$TREE" -write
printf '\nfunc Extra() {}\n' >> "$TREE/internal/pkg/pkg.go"
run_tool "$TREE" -check
if [[ $STATUS -eq 0 ]]; then
    echo "  FAIL: expected non-zero exit on growth"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "grew" <<< "$OUTPUT" || ! grep -q "Extra" <<< "$OUTPUT"; then
    echo "  FAIL: expected growth message naming Extra"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

echo "Test 3: shrinking exported surface still passes..."
TREE="$WORKDIR/shrink"
make_tree "$TREE"
run_tool "$TREE" -write
# Drop Hello(); Client, Client.Name, Client.Greet remain.
cat > "$TREE/internal/pkg/pkg.go" <<'EOF'
package pkg

type Client struct {
	Name string
}

func (c *Client) Greet() string { return c.Name }
EOF
run_tool "$TREE" -check
if [[ $STATUS -ne 0 ]]; then
    echo "  FAIL: shrink must pass"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "below baseline" <<< "$OUTPUT"; then
    echo "  FAIL: expected a shrink hint"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    echo "  PASS"
fi

echo "Test 4: -write ratchets the baseline down after a shrink..."
run_tool "$TREE" -write
if [[ $STATUS -ne 0 ]]; then
    echo "  FAIL: -write exited $STATUS"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    run_tool "$TREE" -check
    if [[ $STATUS -ne 0 ]]; then
        echo "  FAIL: check after ratchet should be a clean OK"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    elif grep -q "below baseline" <<< "$OUTPUT"; then
        echo "  FAIL: ratchet did not lock the shrink (still hinting below baseline)"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS"
    fi
fi

echo "Test 5: a new internal package with exports fails until ratcheted..."
TREE="$WORKDIR/newpkg"
make_tree "$TREE"
run_tool "$TREE" -write
mkdir -p "$TREE/internal/newbie"
printf 'package newbie\n\nfunc Fresh() {}\n' > "$TREE/internal/newbie/newbie.go"
run_tool "$TREE" -check
if [[ $STATUS -eq 0 ]]; then
    echo "  FAIL: expected non-zero exit for a new exported package"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
elif ! grep -q "new package internal/newbie" <<< "$OUTPUT"; then
    echo "  FAIL: expected new-package message"
    echo "$OUTPUT"
    FAILURES=$((FAILURES + 1))
else
    run_tool "$TREE" -write
    run_tool "$TREE" -check
    if [[ $STATUS -ne 0 ]]; then
        echo "  FAIL: check after ratchet should accept the new package"
        echo "$OUTPUT"
        FAILURES=$((FAILURES + 1))
    else
        echo "  PASS"
    fi
fi

echo "Test 6: scripts/test_surface_budget_check.sh is committed executable..."
mode=$(git -C "$REPO_ROOT" ls-files -s -- "scripts/test_surface_budget_check.sh" | awk '{print $1}')
if [[ "$mode" == "100755" ]]; then
    echo "  PASS: git mode is 100755"
else
    echo "  FAIL: git mode is '${mode:-missing}', expected 100755 (stage as executable)"
    FAILURES=$((FAILURES + 1))
fi
if [[ -x "$REPO_ROOT/scripts/test_surface_budget_check.sh" ]]; then
    echo "  PASS: executable on disk"
else
    echo "  FAIL: not executable on disk"
    FAILURES=$((FAILURES + 1))
fi

echo ""
if [[ $FAILURES -eq 0 ]]; then
    echo "All surface-budget tests passed"
    exit 0
else
    echo "FAIL: $FAILURES surface-budget test(s) failed"
    exit 1
fi
