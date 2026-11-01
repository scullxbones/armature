#!/usr/bin/env bash
# Prove Bash 4+ fixtures fail under the Bash currently on PATH (CI pins 3.2).
# Fixtures live in scripts/testdata/bash32-mutations/ so this driver stays
# free of Bash-4 tokens (regex pre-check + bash -n under 3.2).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FIX="$ROOT/scripts/testdata/bash32-mutations"
fail=0
count=0

for f in "$FIX"/*.sh; do
  if [ ! -f "$f" ]; then
    echo "FAIL: no mutation fixtures under $FIX" >&2
    exit 1
  fi
  count=$((count + 1))
  name=$(basename "$f")
  if bash "$f" >/dev/null 2>&1; then
    echo "FAIL: $name mutation unexpectedly succeeded under $(bash --version | head -n 1)" >&2
    fail=1
  else
    echo "OK: $name mutation failed (expected under Bash 3.2)"
  fi
done

if [ "$count" -lt 3 ]; then
  echo "FAIL: expected at least 3 mutation fixtures, found $count" >&2
  exit 1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: all Bash 4+ mutations failed under $(bash --version | head -n 1)"
