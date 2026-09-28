#!/bin/bash
set -euo pipefail

REPO_ROOT="${1:-.}"
SCRIPT="$REPO_ROOT/scripts/check-git-test-hermetic.sh"
chmod +x "$SCRIPT"

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

expect_hit() {
    local label="$1"
    local file="$2"
    local needle="$3"
    echo "$label"
    mkdir -p "$(dirname "$file")"
    printf '%s\n' "$4" > "$file"
    if "$SCRIPT" "$WORKDIR" >/dev/null 2>"$WORKDIR/err"; then
        echo "FAIL: expected non-zero on injected git init" >&2
        exit 1
    fi
    if ! rg -q --fixed-strings "$needle" "$WORKDIR/err"; then
        echo "FAIL: expected hit in diagnostic, got:" >&2
        cat "$WORKDIR/err" >&2
        exit 1
    fi
    echo "  PASS"
    rm -f "$file"
}

echo "Test 1: clean checkout passes..."
if ! "$SCRIPT" "$REPO_ROOT" >/dev/null; then
    echo "FAIL: expected exit 0 on clean tree" >&2
    exit 1
fi
echo "  PASS"

expect_hit \
    "Test 2: injected raw git init is rejected..." \
    "$WORKDIR/cmd/fake/x_test.go" \
    '"git", "init"' \
    'package fake
func f() { _ = []string{"git", "init"} }'

expect_hit \
    "Test 3: injected git -C dir init is rejected..." \
    "$WORKDIR/cmd/fake/dashc_test.go" \
    '"git", "-C", dir, "init"' \
    'package fake
func f() { _ = exec.CommandContext(ctx, "git", "-C", dir, "init") }'

expect_hit \
    "Test 4: injected git -c key=val init is rejected..." \
    "$WORKDIR/cmd/fake/dashcfg_test.go" \
    'git -c commit.gpgsign=true init' \
    'package fake
func f() { _ = "git -c commit.gpgsign=true init" }'

echo "Test 5: non-matching git -C status is allowed when TestMain isolates..."
mkdir -p "$WORKDIR/cmd/fake"
printf '%s\n' 'package fake
import (
	"os"
	"testing"
	"github.com/scullxbones/armature/internal/gittest"
)
func TestMain(m *testing.M) { os.Exit(gittest.Main(m)) }
func f() { _ = exec.CommandContext(ctx, "git", "-C", dir, "status") }
func g() { _ = []string{"git", "commit", "--allow-empty", "-m", "init"} }' \
    > "$WORKDIR/cmd/fake/status_test.go"
if ! "$SCRIPT" "$WORKDIR" >/dev/null 2>"$WORKDIR/err"; then
    echo "FAIL: expected exit 0 on isolated non-matching git usage, got:" >&2
    cat "$WORKDIR/err" >&2
    exit 1
fi
echo "  PASS"
rm -f "$WORKDIR/cmd/fake/status_test.go"

echo "Test 6: missing TestMain isolation is rejected..."
mkdir -p "$WORKDIR/cmd/fake"
printf '%s\n' 'package fake
func f() { _ = exec.CommandContext(ctx, "git", "-C", dir, "status") }' \
    > "$WORKDIR/cmd/fake/no_testmain_test.go"
if "$SCRIPT" "$WORKDIR" >/dev/null 2>"$WORKDIR/err"; then
    echo "FAIL: expected non-zero when TestMain isolation is missing" >&2
    exit 1
fi
if ! rg -q 'without TestMain isolation' "$WORKDIR/err"; then
    echo "FAIL: expected missing-TestMain diagnostic, got:" >&2
    cat "$WORKDIR/err" >&2
    exit 1
fi
echo "  PASS"

echo "check-git-test-hermetic self-test OK"
