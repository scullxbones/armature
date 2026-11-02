#!/usr/bin/env bash
# Shared file selector for porcelain / engine (and same-pattern shell) guards.
#
# CLASS: list every scan-eligible regular file under the given directories.
# Do NOT filter by extension or shebang — `scripts/zz` with no shebang and
# `scripts/foo.txt` must be scanned. Callers exclude their own allowlisted paths.
#
# Fail-closed bytes:
#   - Non-UTF8 / odd bytes are still listed; callers must scan with
#     LC_ALL=C grep -a so a hit line carrying \xff still fails.
#   - NUL: NEVER skip silently. Files whose first byte is NUL are listed;
#     callers scan with LC_ALL=C grep -a (or FAIL: <f> unscannable). A NUL
#     later in the file is likewise listed so "NUL later + git status" fails.
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
  printf '%s\n' "$f"
done <"$list_tmp"
