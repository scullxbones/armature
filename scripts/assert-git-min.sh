#!/usr/bin/env bash
# Fail closed when PATH git is below Armature's documented floor (2.34.0).
# Used by every CI leg so runners cannot silently drift below the product min.
# Prerelease markers on the same X.Y.Z (for example 2.34.0-rc1) are below the
# matching release — aligned with adapters.Version.AtLeast / ParseGitVersionOutput.
set -euo pipefail

REQUIRED_MAJOR=2
REQUIRED_MINOR=34
REQUIRED_PATCH=0

if ! command -v git >/dev/null 2>&1; then
  echo "FAIL: git not found on PATH (required ≥${REQUIRED_MAJOR}.${REQUIRED_MINOR}.${REQUIRED_PATCH})" >&2
  exit 1
fi

raw="$(git --version 2>&1)"
line="${raw%%$'\n'*}"
case "$line" in
  "git version "*)
    rest="${line#git version }"
    ;;
  *)
    echo "FAIL: unrecognized git --version output: $raw" >&2
    exit 1
    ;;
esac
# First token only — drop " (Apple Git-…)" and similar.
token="${rest%% *}"
if [[ -z "$token" ]]; then
  echo "FAIL: unrecognized git --version output: $raw" >&2
  exit 1
fi

IFS=. read -r major minor patch_part _ <<<"${token}."
major=${major:-0}
minor=${minor:-0}
patch_part=${patch_part:-0}

# Patch may be "0", "0-rc1", or "0rc1". Digits then a non-empty remainder ⇒ prerelease.
# Extra dotted segments (2.34.0.windows.1) leave patch_part as plain "0".
pre=0
if [[ "$patch_part" =~ ^([0-9]+)(.*)$ ]]; then
  patch="${BASH_REMATCH[1]}"
  if [[ -n "${BASH_REMATCH[2]}" ]]; then
    pre=1
  fi
else
  echo "FAIL: unrecognized git version token: $token (from: $raw)" >&2
  exit 1
fi

# Same ordering as adapters.Version.AtLeast: a prerelease of the exact floor
# X.Y.Z is below the release; a prerelease of a higher X.Y.Z is above the floor.
ok=0
if (( major > REQUIRED_MAJOR )); then
  ok=1
elif (( major == REQUIRED_MAJOR && minor > REQUIRED_MINOR )); then
  ok=1
elif (( major == REQUIRED_MAJOR && minor == REQUIRED_MINOR && patch > REQUIRED_PATCH )); then
  ok=1
elif (( major == REQUIRED_MAJOR && minor == REQUIRED_MINOR && patch == REQUIRED_PATCH && pre == 0 )); then
  ok=1
fi

have="${major}.${minor}.${patch}"
if (( pre == 1 )); then
  have="${have}-pre"
fi

if (( ok != 1 )); then
  echo "FAIL: installed git ${have} is below the required minimum git ${REQUIRED_MAJOR}.${REQUIRED_MINOR}.${REQUIRED_PATCH}" >&2
  echo "  git --version: $raw" >&2
  exit 1
fi

echo "OK: git ${have} meets minimum ${REQUIRED_MAJOR}.${REQUIRED_MINOR}.${REQUIRED_PATCH}"
