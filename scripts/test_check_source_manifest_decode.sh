#!/bin/bash

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"

echo "Test 1: go test ./scripts/repoguard (source-manifest fixtures + real repo)..."
(cd "$REPO_ROOT" && go test ./scripts/repoguard -count=1 -run 'SourceManifest|RealRepoPasses')
echo "  PASS"

echo "Test 2: Makefile wires source-manifest-decode-check to scripts/repoguard..."
if ! rg -q 'scripts/repoguard -check source-manifest' "$REPO_ROOT/Makefile"; then
    echo "FAIL: Makefile source-manifest-decode-check must run scripts/repoguard" >&2
    exit 1
fi
echo "  PASS"

echo "All source-manifest-decode tests passed"
