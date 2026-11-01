#!/usr/bin/env bash
# Fail closed when PATH git is below Armature's documented floor (2.34.0).
# Used by every CI leg so runners cannot silently drift below the product min.
set -euo pipefail

REQUIRED_MAJOR=2
REQUIRED_MINOR=34
REQUIRED_PATCH=0

if ! command -v git >/dev/null 2>&1; then
  echo "FAIL: git not found on PATH (required ≥${REQUIRED_MAJOR}.${REQUIRED_MINOR}.${REQUIRED_PATCH})" >&2
  exit 1
fi

raw="$(git --version 2>&1)"
# shellcheck disable=SC2001
ver="$(printf '%s\n' "$raw" | sed -n 's/^git version \([0-9][0-9.]*\).*/\1/p' | head -n1)"
if [[ -z "$ver" ]]; then
  echo "FAIL: unrecognized git --version output: $raw" >&2
  exit 1
fi

IFS=. read -r major minor patch _ <<<"${ver}...."
major=${major:-0}
minor=${minor:-0}
patch=${patch:-0}
# strip any non-digit suffix on patch (e.g. windows builds already trimmed by sed)
patch="${patch%%[!0-9]*}"
patch=${patch:-0}

ok=0
if (( major > REQUIRED_MAJOR )); then
  ok=1
elif (( major == REQUIRED_MAJOR && minor > REQUIRED_MINOR )); then
  ok=1
elif (( major == REQUIRED_MAJOR && minor == REQUIRED_MINOR && patch >= REQUIRED_PATCH )); then
  ok=1
fi

if (( ok != 1 )); then
  echo "FAIL: installed git ${major}.${minor}.${patch} is below the required minimum git ${REQUIRED_MAJOR}.${REQUIRED_MINOR}.${REQUIRED_PATCH}" >&2
  echo "  git --version: $raw" >&2
  exit 1
fi

echo "OK: git ${major}.${minor}.${patch} meets minimum ${REQUIRED_MAJOR}.${REQUIRED_MINOR}.${REQUIRED_PATCH}"
