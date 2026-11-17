#!/usr/bin/env bash
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
      curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 -o "$fn" "$BASE_URL/$fn"
    fi
    done <SHA256SUMS
  grep -vE '^(#|$)' SHA256SUMS | sha256sum -c -
)

echo "OK: alpine git apk set verified in $DEST"
