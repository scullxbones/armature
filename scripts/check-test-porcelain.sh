#!/usr/bin/env bash
# Fail if the test suite dirtied the git working tree — or changed the
# *content* of paths that were already dirty/untracked before the suite.
#
# Usage:
#   check-test-porcelain.sh --write-baseline [repo_root] [outfile]
#   check-test-porcelain.sh [repo_root] [baseline_file]
#   check-test-porcelain.sh [repo_root]   # refuse any dirt (no baseline)
#
# Baseline format (v2): one line per dirty/untracked path:
#   <status><TAB><path><TAB><content_hash>
# Paths come from `git status --porcelain -z` (NUL-separated), so spaces,
# quotes, and non-ASCII names round-trip safely.
#
# Bash 3.2 compatible (macOS /bin/bash).
set -euo pipefail

MODE="check"
if [ "${1:-}" = "--write-baseline" ]; then
  MODE="write"
  shift
fi

ROOT="${1:-.}"
BASELINE="${2:-}"
cd "$ROOT"

path_digest() {
  local path="$1"
  case "$path" in
    */) path="${path%/}" ;;
  esac
  if [ -d "$path" ]; then
    find "$path" \( -type f -o -type l \) -print 2>/dev/null \
      | LC_ALL=C sort \
      | while IFS= read -r f || [ -n "$f" ]; do
          [ -z "$f" ] && continue
          h=$(git hash-object -- "$f" 2>/dev/null || printf 'unreadable')
          printf '%s %s\n' "$h" "$f"
        done \
      | git hash-object --stdin
  elif [ -e "$path" ] || [ -L "$path" ]; then
    git hash-object -- "$path" 2>/dev/null || printf 'unreadable\n'
  else
    printf 'missing\n'
  fi
}

# Emit sorted baseline lines from porcelain -z.
emit_baseline() {
  local entry status path dig skip_next
  skip_next=0
  # Process substitution is Bash 3.2-safe.
  while IFS= read -r -d '' entry || [ -n "$entry" ]; do
    if [ "$skip_next" -eq 1 ]; then
      # Rename/copy origin path record (no XY prefix).
      skip_next=0
      continue
    fi
    [ -z "$entry" ] && continue
    status="${entry:0:2}"
    path="${entry:3}"
    case "$status" in
      R*|C*)
        skip_next=1
        ;;
    esac
    dig="$(path_digest "$path")"
    printf '%s\t%s\t%s\n' "$status" "$path" "$dig"
  done < <(git status --porcelain -z --untracked-files=all 2>/dev/null || true) \
    | LC_ALL=C sort
}

if [ "$MODE" = "write" ]; then
  if [ -z "$BASELINE" ]; then
    echo "FAIL: --write-baseline requires an outfile" >&2
    exit 1
  fi
  emit_baseline >"$BASELINE"
  exit 0
fi

current_fp="$(emit_baseline)"

if [ -z "$BASELINE" ]; then
  if [ -n "$current_fp" ]; then
    echo "FAIL: go test left dirt in git status --porcelain (explicit TempDir/root required):" >&2
    printf '%s\n' "$current_fp" | sed 's/\t[^	]*$//' | sed 's/^/  /' >&2
    exit 1
  fi
  echo "OK: git status --porcelain empty after tests"
  exit 0
fi

if [ ! -f "$BASELINE" ]; then
  echo "FAIL: porcelain baseline file missing: $BASELINE" >&2
  exit 1
fi

baseline_fp="$(cat "$BASELINE")"

if [ "$current_fp" = "$baseline_fp" ]; then
  echo "OK: git status --porcelain content-hash baseline unchanged"
  exit 0
fi

echo "FAIL: go test changed dirty/untracked content vs pre-test baseline (status and/or hashes):" >&2
echo "  --- baseline ---" >&2
if [ -n "$baseline_fp" ]; then
  printf '%s\n' "$baseline_fp" | sed 's/^/  /' >&2
else
  echo "  (clean)" >&2
fi
echo "  --- current ---" >&2
if [ -n "$current_fp" ]; then
  printf '%s\n' "$current_fp" | sed 's/^/  /' >&2
else
  echo "  (clean)" >&2
fi
exit 1
