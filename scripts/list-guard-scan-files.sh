#!/usr/bin/env bash
# Shared file selector for porcelain / engine (and same-pattern shell) guards.
#
# CLASS: list every non-binary regular file under the given directories.
# Do NOT filter by extension or shebang — `scripts/zz` with no shebang and
# `scripts/foo.txt` must be scanned. Callers exclude their own allowlisted paths.
#
# Usage: list-guard-scan-files.sh [--exclude-testdata] DIR [DIR...]
# stdout: one relative path per line (sorted).
set -euo pipefail

EXCLUDE_TESTDATA=0
dirs=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    --exclude-testdata) EXCLUDE_TESTDATA=1; shift ;;
    -*) echo "FAIL: list-guard-scan-files.sh: unknown flag $1" >&2; exit 2 ;;
    *) dirs+=("$1"); shift ;;
  esac
done
if [ "${#dirs[@]}" -eq 0 ]; then
  echo "FAIL: list-guard-scan-files.sh: need at least one DIR" >&2
  exit 2
fi

# Non-binary: no NUL byte. Empty files are scanned.
# Do not pass NUL to grep -F (C-string truncates to empty pattern → every file
# looks like a hit). Compare byte counts before/after deleting NULs instead.
is_non_binary() {
  local f="$1" raw stripped
  [ -f "$f" ] || return 1
  [ ! -s "$f" ] && return 0
  raw=$(wc -c <"$f" | tr -d ' ')
  stripped=$(tr -d '\000' <"$f" | wc -c | tr -d ' ')
  [ "$raw" = "$stripped" ]
}

list_tmp=$(mktemp)
trap 'rm -f "$list_tmp"' EXIT
{
  for d in "${dirs[@]}"; do
    if [ ! -d "$d" ]; then
      continue
    fi
    find "$d" -type f
  done
} | LC_ALL=C sort -u >"$list_tmp"

while IFS= read -r f || [ -n "${f:-}" ]; do
  [ -z "${f:-}" ] && continue
  if [ "$EXCLUDE_TESTDATA" -eq 1 ]; then
    case "$f" in
      */testdata/*|*/testdata) continue ;;
    esac
  fi
  if is_non_binary "$f"; then
    printf '%s\n' "$f"
  fi
done <"$list_tmp"
