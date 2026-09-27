#!/bin/bash
# Exercises check-git-test-hermetic.sh: clean tree passes; injected raw git init fails.
set -euo pipefail

REPO_ROOT="${1:-.}"
SCRIPT="$REPO_ROOT/scripts/check-git-test-hermetic.sh"
chmod +x "$SCRIPT"

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

echo "Test 1: clean checkout passes..."
if ! "$SCRIPT" "$REPO_ROOT" >/dev/null; then
    echo "FAIL: expected exit 0 on clean tree" >&2
    exit 1
fi
echo "  PASS"

echo "Test 2: injected raw git init is rejected..."
mkdir -p "$WORKDIR/cmd/fake"
printf 'package fake\nfunc f() { _ = []string{"git", "init"} }\n' > "$WORKDIR/cmd/fake/x_test.go"
if "$SCRIPT" "$WORKDIR" >/dev/null 2>"$WORKDIR/err"; then
    echo "FAIL: expected non-zero on injected git init" >&2
    exit 1
fi
if ! grep -q '"git", "init"' "$WORKDIR/err"; then
    echo "FAIL: expected hit in diagnostic, got:" >&2
    cat "$WORKDIR/err" >&2
    exit 1
fi
echo "  PASS"

echo "check-git-test-hermetic self-test OK"
