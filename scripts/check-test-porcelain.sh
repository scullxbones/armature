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
#   <porcelain_line><TAB><content_hash>
# Content hash covers file bytes (or a deterministic tree digest for dirs),
# so rewriting an already-modified tracked file fails even when the porcelain
# status line is unchanged.
#
# Bash 3.2 compatible (macOS /bin/bash) — no mapfile / readarray / declare -A /
# coproc / [[ -v ]].
set -euo pipefail

MODE="check"
if [ "${1:-}" = "--write-baseline" ]; then
  MODE="write"
  shift
fi

ROOT="${1:-.}"
BASELINE="${2:-}"
cd "$ROOT"

# Extract the path from a porcelain line (handles "XY path" and "R  old -> new").
porcelain_path() {
  local line="$1"
  local rest
  # Drop the two-char status and the following space.
  rest="${line#?? }"
  case "$rest" in
    *" -> "*)
      printf '%s\n' "${rest##* -> }"
      ;;
    *)
      printf '%s\n' "$rest"
      ;;
  esac
}

# Content digest for a path. Directories: hash of sorted "filehash path" lines.
path_digest() {
  local path="$1"
  # Trim a trailing slash from porcelain dir entries (e.g. "?? dir/").
  case "$path" in
    */) path="${path%/}" ;;
  esac
  if [ -d "$path" ]; then
    # shellcheck disable=SC2016
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

# Emit baseline lines for the current dirty/untracked tree (sorted).
emit_baseline() {
  local current line path dig
  current="$(git status --porcelain --untracked-files=all || true)"
  if [ -z "$current" ]; then
    return 0
  fi
  printf '%s\n' "$current" | while IFS= read -r line || [ -n "$line" ]; do
    [ -z "$line" ] && continue
    path="$(porcelain_path "$line")"
    dig="$(path_digest "$path")"
    # TAB separates porcelain status line from content hash.
    printf '%s\t%s\n' "$line" "$dig"
  done | LC_ALL=C sort
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
    printf '%s\n' "$current_fp" | sed 's/\t.*//' | sed 's/^/  /' >&2
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
