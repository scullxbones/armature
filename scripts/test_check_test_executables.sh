#!/bin/bash

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"

echo "Test 1: go test ./scripts/repoguard (test-executables fixtures + real repo)..."
(cd "$REPO_ROOT" && go test ./scripts/repoguard -count=1 -run 'TestExecutables|RealRepoPasses')
echo "  PASS"

echo "Test 2: Makefile wires git-test-exec-check to scripts/repoguard..."
if ! rg -q 'scripts/repoguard -check test-executables' "$REPO_ROOT/Makefile"; then
    echo "FAIL: Makefile git-test-exec-check must run scripts/repoguard" >&2
    exit 1
fi
echo "  PASS"

echo "check-test-executables self-test OK"
