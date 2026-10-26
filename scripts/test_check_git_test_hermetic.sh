#!/bin/bash

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"

echo "Test 1: go test ./scripts/repoguard (git-hermetic fixtures + real repo)..."
(cd "$REPO_ROOT" && go test ./scripts/repoguard -count=1 -run 'GitHermetic|RealRepoPasses')
echo "  PASS"

echo "Test 2: Makefile wires git-test-hermetic-check to scripts/repoguard..."
if ! rg -q 'scripts/repoguard -check git-hermetic' "$REPO_ROOT/Makefile"; then
    echo "FAIL: Makefile git-test-hermetic-check must run scripts/repoguard" >&2
    exit 1
fi
echo "  PASS"

echo "check-git-test-hermetic self-test OK"
