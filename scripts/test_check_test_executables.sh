#!/bin/bash

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
SCRIPT="$REPO_ROOT/scripts/check-test-executables.sh"
chmod +x "$SCRIPT"

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

write_fixture() {
    local name="$1"
    local body="$2"
    local dir="$WORKDIR/$name"
    rm -rf "$dir"
    mkdir -p "$dir/cmd/fake"
    printf '%s\n' "$body" > "$dir/cmd/fake/${name}_test.go"
    printf '%s\n' "$dir"
}

legacy_368_hits() {
    local tree="$1"
    rg -n -U --multiline --glob '*_test.go' --glob '!internal/testexec/**' \
        -e 'os\.(WriteFile|OpenFile)\([^;\n]*,\s*0o?[0-7]*[1357]\s*\)' \
        -e 'os\.(WriteFile|OpenFile)\(\s*\n(?:[ \t]*[^\n]+\n){1,8}[ \t]*0o?[0-7]*[1357]\s*,?\s*\n[ \t]*\)' \
        "$tree" || true
}

assert_rejected() {
    local label="$1"
    local body="$2"
    local tree
    tree=$(write_fixture "$label" "$body")
    if "$SCRIPT" "$tree" >/dev/null 2>"$WORKDIR/err"; then
        echo "FAIL: $label: expected non-zero, script passed" >&2
        exit 1
    fi
    if ! rg -q 'testexec.Write' "$WORKDIR/err"; then
        echo "FAIL: $label: expected the diagnostic to name testexec.Write, got:" >&2
        cat "$WORKDIR/err" >&2
        exit 1
    fi
    echo "  PASS reject $label"
}

assert_allowed() {
    local label="$1"
    local body="$2"
    local tree
    tree=$(write_fixture "$label" "$body")
    if ! "$SCRIPT" "$tree" >/dev/null 2>"$WORKDIR/err"; then
        echo "FAIL: $label: expected exit 0, got:" >&2
        cat "$WORKDIR/err" >&2
        exit 1
    fi
    echo "  PASS allow $label"
}

echo "Test 1: clean checkout passes..."
if ! "$SCRIPT" "$REPO_ROOT" >/dev/null; then
    echo "FAIL: expected exit 0 on clean tree" >&2
    exit 1
fi
echo "  PASS"

echo "Test 2: one fixture per executable mode is rejected and names testexec.Write..."
assert_rejected 0o700 'package fake
func f() { _ = os.WriteFile(path, body, 0o700) }'
assert_rejected 0o744 'package fake
func f() { _ = os.WriteFile(path, body, 0o744) }'
assert_rejected 0o750 'package fake
func f() { _ = os.WriteFile(path, body, 0o750) }'
assert_rejected 0o755 'package fake
func f() { _ = os.WriteFile(path, body, 0o755) }'
assert_rejected 0755 'package fake
func f() { _ = os.WriteFile(path, body, 0755) }'
assert_rejected chmod_plusx 'package fake
func f() {
	_ = os.WriteFile(path, body, 0o644)
	_ = os.Chmod(path, 0o111)
}'
assert_rejected modeperm 'package fake
func f() { _ = os.WriteFile(path, body, os.ModePerm) }'
assert_rejected filemode 'package fake
func f() { _ = os.WriteFile(path, body, fs.FileMode(0o700)) }'

echo "Test 3: os.WriteFile(..., 0o644) is allowed..."
assert_allowed 0o644 'package fake
func f() { _ = os.WriteFile(path, body, 0o644) }'

echo "Test 4: multiline WriteFile with 0o755 is rejected..."
assert_rejected multiline_0o755 'package fake
func f() {
	_ = os.WriteFile(
		path,
		body,
		0o755,
	)
}'

echo "Test 5: #368 last-digit guard misses 0o700/0o744/0o750..."
mkdir -p "$WORKDIR/legacy/cmd/fake"
for mode in 0o700 0o744 0o750; do
    printf '%s\n' "package fake
func f() { _ = os.WriteFile(path, body, ${mode}) }" \
        > "$WORKDIR/legacy/cmd/fake/${mode}_test.go"
done
legacy=$(legacy_368_hits "$WORKDIR/legacy")
if [[ -n "$legacy" ]]; then
    echo "FAIL: #368 guard should miss 0o700/0o744/0o750, but matched:" >&2
    echo "$legacy" >&2
    exit 1
fi
if ! "$SCRIPT" "$WORKDIR/legacy" >/dev/null 2>"$WORKDIR/err"; then
    :
else
    echo "FAIL: current guard must reject the #368-miss modes" >&2
    exit 1
fi
if ! rg -q 'testexec.Write' "$WORKDIR/err"; then
    echo "FAIL: current guard diagnostic must name testexec.Write" >&2
    cat "$WORKDIR/err" >&2
    exit 1
fi
echo "  PASS"

echo "Test 6: directory os.Chmod(..., 0o755) without a WriteFile of that path is allowed..."
assert_allowed dir_chmod 'package fake
func f() { _ = os.Chmod(opsDir, 0o755) }'

echo "Test 7: check-test-executables.sh is committed executable..."
mode=$(git -C "$REPO_ROOT" ls-files -s -- "scripts/check-test-executables.sh" | awk '{print $1}')
if [[ "$mode" != "100755" ]]; then
    echo "FAIL: git mode is '${mode:-missing}', expected 100755" >&2
    exit 1
fi
echo "  PASS"

echo "check-test-executables self-test OK"
