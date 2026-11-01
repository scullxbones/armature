#!/usr/bin/env bash
# Fail if the test suite dirtied the git working tree.
# Usage: check-test-porcelain.sh [repo_root] [baseline_porcelain_file]
#   With baseline: fail on any porcelain line not present before the suite
#   (allows pre-existing WIP). Without baseline: fail on any porcelain.
# Bash 3.2 compatible (macOS /bin/bash) — no mapfile / readarray / declare -A.
set -euo pipefail
ROOT="${1:-.}"
BASELINE="${2:-}"
cd "$ROOT"

current="$(git status --porcelain --untracked-files=all || true)"

if [ -n "$BASELINE" ]; then
  if [ ! -f "$BASELINE" ]; then
    echo "FAIL: porcelain baseline file missing: $BASELINE" >&2
    exit 1
  fi
  new_dirt="$(
    printf '%s\n' "$current" | while IFS= read -r line || [ -n "$line" ]; do
      [ -z "$line" ] && continue
      if ! grep -Fxq -- "$line" "$BASELINE"; then
        printf '%s\n' "$line"
      fi
    done
  )"
  if [ -n "$new_dirt" ]; then
    echo "FAIL: go test left new dirt in git status --porcelain (explicit TempDir/root required):" >&2
    printf '%s\n' "$new_dirt" | sed 's/^/  /' >&2
    exit 1
  fi
  echo "OK: git status --porcelain unchanged vs pre-test baseline"
  exit 0
fi

if [ -n "$current" ]; then
  echo "FAIL: go test left dirt in git status --porcelain (explicit TempDir/root required):" >&2
  printf '%s\n' "$current" | sed 's/^/  /' >&2
  exit 1
fi
echo "OK: git status --porcelain empty after tests"
