#!/usr/bin/env bash
# Prove Bash 4+ fixtures behave correctly for the Bash currently on PATH.
# - Under Bash 3.2.x (CI pin): each fixture MUST exit non-zero.
# - Under Bash 4+: each fixture MUST exit 0 (self-check that fixtures are
#   real successes on modern bash — a fixture that "can't fail" under 3.2
#   because it never exercises a Bash-4 feature would also fail this host
#   check when the CI job runs the dual gate).
# Fixtures live in scripts/testdata/bash32-mutations/.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FIX="$ROOT/scripts/testdata/bash32-mutations"
fail=0
count=0

ver="$(bash --version | head -n 1)"
echo "prove-bash32-mutations under: $ver"

major=0
minor=0
# "GNU bash, version 3.2.57(1)-release ..."
case "$ver" in
  *"version "*)
    rest="${ver#*version }"
    major="${rest%%.*}"
    rest2="${rest#*.}"
    minor="${rest2%%.*}"
    ;;
esac

expect_fail=0
case "$major.$minor" in
  3.2)
    expect_fail=1
    ;;
esac

for f in "$FIX"/*.sh; do
  if [ ! -f "$f" ]; then
    echo "FAIL: no mutation fixtures under $FIX" >&2
    exit 1
  fi
  count=$((count + 1))
  name=$(basename "$f")
  set +e
  bash "$f" >/tmp/armature-bash32-mut.out 2>/tmp/armature-bash32-mut.err
  st=$?
  set -e
  if [ "$expect_fail" -eq 1 ]; then
    if [ "$st" -eq 0 ]; then
      echo "FAIL: $name exited 0 under Bash 3.2 (mutation must fail; check set -e on fixture)" >&2
      fail=1
    else
      echo "OK: $name failed under Bash 3.2 (exit $st)"
    fi
  else
    if [ "$st" -ne 0 ]; then
      echo "FAIL: $name exited $st under Bash ${major}.${minor}+ (fixture must succeed on Bash 4+)" >&2
      cat /tmp/armature-bash32-mut.err >&2 || true
      fail=1
    else
      echo "OK: $name succeeded under Bash ${major}.${minor}+ (exit 0)"
    fi
  fi
done

if [ "$count" -lt 3 ]; then
  echo "FAIL: expected at least 3 mutation fixtures, found $count" >&2
  exit 1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: mutation fixtures match expected outcome under $ver"
