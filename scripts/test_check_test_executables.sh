#!/bin/bash

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
SCRIPT="$REPO_ROOT/scripts/check-test-executables.sh"
chmod +x "$SCRIPT"

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

echo "Test 1: clean checkout passes..."
if ! "$SCRIPT" "$REPO_ROOT" >/dev/null; then
    echo "FAIL: expected exit 0 on clean tree" >&2
    exit 1
fi
echo "  PASS"

echo "Test 2: injected os.WriteFile(..., 0o755) is rejected and names testexec.Write..."
mkdir -p "$WORKDIR/cmd/fake"
printf '%s\n' 'package fake
func f() { _ = os.WriteFile(path, body, 0o755) }' \
    > "$WORKDIR/cmd/fake/x_test.go"
if "$SCRIPT" "$WORKDIR" >/dev/null 2>"$WORKDIR/err"; then
    echo "FAIL: expected non-zero on injected executable write" >&2
    exit 1
fi
if ! rg -q 'testexec.Write' "$WORKDIR/err"; then
    echo "FAIL: expected the diagnostic to name testexec.Write, got:" >&2
    cat "$WORKDIR/err" >&2
    exit 1
fi
echo "  PASS"
rm -f "$WORKDIR/cmd/fake/x_test.go"

echo "Test 3: os.WriteFile(..., 0o644) is allowed..."
printf '%s\n' 'package fake
func f() { _ = os.WriteFile(path, body, 0o644) }' \
    > "$WORKDIR/cmd/fake/ok_test.go"
if ! "$SCRIPT" "$WORKDIR" >/dev/null 2>"$WORKDIR/err"; then
    echo "FAIL: expected exit 0 for non-executable WriteFile, got:" >&2
    cat "$WORKDIR/err" >&2
    exit 1
fi
echo "  PASS"

echo "Test 4: multiline WriteFile with 0o755 is rejected..."
printf '%s\n' 'package fake
func f() {
	_ = os.WriteFile(
		path,
		body,
		0o755,
	)
}' > "$WORKDIR/cmd/fake/multi_test.go"
if "$SCRIPT" "$WORKDIR" >/dev/null 2>"$WORKDIR/err"; then
    echo "FAIL: expected non-zero on multiline executable write" >&2
    exit 1
fi
echo "  PASS"

echo "Test 5: check-test-executables.sh is committed executable..."
mode=$(git -C "$REPO_ROOT" ls-files -s -- "scripts/check-test-executables.sh" | awk '{print $1}')
if [[ "$mode" != "100755" ]]; then
    echo "FAIL: git mode is '${mode:-missing}', expected 100755" >&2
    exit 1
fi
echo "  PASS"

echo "check-test-executables self-test OK"
