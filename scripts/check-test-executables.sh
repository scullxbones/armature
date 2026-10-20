#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="${1:-.}"
cd "$REPO_ROOT"

if ! command -v rg >/dev/null 2>&1; then
    echo "FAIL: rg is required for check-test-executables" >&2
    exit 1
fi

hits=$(rg -n -U --multiline --glob '*_test.go' --glob '!internal/testexec/**' \
    -e 'os\.(WriteFile|OpenFile)\([^;\n]*,\s*0o?[0-7]*[1357]\s*\)' \
    -e 'os\.(WriteFile|OpenFile)\(\s*\n(?:[ \t]*[^\n]+\n){1,8}[ \t]*0o?[0-7]*[1357]\s*,?\s*\n[ \t]*\)' \
    . || true)

if [[ -n "$hits" ]]; then
    echo "FAIL: _test.go writes an executable; use testexec.Write so tests never create an executable inode after init (golang/go#22315):" >&2
    echo "$hits" >&2
    exit 1
fi

echo "OK: no _test.go writes an executable outside testexec.Write"
