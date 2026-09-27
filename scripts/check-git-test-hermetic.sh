#!/bin/bash
# Fail make check when a _test.go file runs raw `git init` / `git init -b`
# outside internal/gittest (the shared isolation + repo helper).
set -euo pipefail

REPO_ROOT="${1:-.}"
cd "$REPO_ROOT"

if ! command -v rg >/dev/null 2>&1; then
    echo "FAIL: rg is required for check-git-test-hermetic" >&2
    exit 1
fi

hits=$(rg -n --glob '*_test.go' --glob '!internal/gittest/**' \
    -e 'git init' \
    -e '"git", "init"' \
    -e '"init", "--bare"' \
    -e '"init", "-b"' \
    -e '"init", "-q"' \
    . || true)

if [[ -n "$hits" ]]; then
    echo "FAIL: raw git init in test files outside internal/gittest:" >&2
    echo "$hits" >&2
    echo "Use gittest.Init / gittest.InitRepo instead." >&2
    exit 1
fi

echo "OK: no raw git init in _test.go files outside internal/gittest"
