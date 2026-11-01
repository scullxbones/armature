#!/usr/bin/env bash
# Download the closed Alpine v3.22 git .apk set and verify each sha256.
# Usage: fetch-alpine-git-apks.sh <dest-dir>
# Bash 3.2 compatible.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SUMS="$ROOT/scripts/alpine-v322-git-apks.sha256"
BASE_URL="${ALPINE_APK_BASE_URL:-https://dl-cdn.alpinelinux.org/alpine/v3.22/main/x86_64}"
DEST="${1:-}"

if [ -z "$DEST" ]; then
  echo "FAIL: usage: fetch-alpine-git-apks.sh <dest-dir>" >&2
  exit 2
fi
if [ ! -f "$SUMS" ]; then
  echo "FAIL: missing checksums $SUMS" >&2
  exit 1
fi

mkdir -p "$DEST"
# Copy checksums into dest for sha256sum -c (paths are basenames).
cp "$SUMS" "$DEST/SHA256SUMS"

(
  cd "$DEST"
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ''|\#*) continue ;;
    esac
    # sha256  filename
    fn="${line##*  }"
    fn="${fn##* }"
    if [ ! -f "$fn" ]; then
      curl -fsSL -o "$fn" "$BASE_URL/$fn"
    fi
  done <SHA256SUMS
  # Verify only data lines (sha256sum -c ignores comments? — filter).
  grep -vE '^(#|$)' SHA256SUMS | sha256sum -c -
)

echo "OK: alpine git apk set verified in $DEST"
