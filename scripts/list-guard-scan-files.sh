#!/usr/bin/env bash
# Shared file selector for porcelain / engine (and same-pattern shell) guards.
#
# CLASS: list every scan-eligible regular file under the given directories.
# Do NOT filter by extension or shebang — `scripts/zz` with no shebang and
# `scripts/foo.txt` must be scanned. Callers exclude their own allowlisted paths.
#
# Fail-closed bytes:
#   - Non-UTF8 / odd bytes are still listed; callers must scan with grep -a
#     (or LC_ALL=C) so a hit line carrying \xff still fails.
#   - NUL: skip only when bash refuses the file (first byte is NUL). A NUL
#     later in the file is still listed so "NUL later + git status" fails.
#   - --exclude-testdata matches main scope exactly: only scripts/testdata/*
#     (never nested other-package testdata dirs; never scripts/repoguard/*).
#
# Usage: list-guard-scan-files.sh [--exclude-testdata] DIR [DIR...]
# stdout: one relative path per line (sorted). Nonzero exit propagates errors.
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

# Bash refuses a normal text read when the first byte is NUL. Empty files and
# files whose first NUL appears later remain listed (scan anyway).
bash_refuses() {
  local f="$1" first
  [ -f "$f" ] || return 0
  [ ! -s "$f" ] && return 1
  first=$(dd if="$f" bs=1 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n')
  [ "$first" = "00" ]
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
    # Main scope: scripts/testdata/* only — never */testdata/* broadly.
    case "$f" in
      scripts/testdata/*|scripts/testdata) continue ;;
    esac
  fi
  if bash_refuses "$f"; then
    continue
  fi
  printf '%s\n' "$f"
done <"$list_tmp"
