#!/usr/bin/env bash
# Fail if the test suite dirtied the git working tree — or changed the
# *content* of paths that were already dirty/untracked before the suite.
#
# Usage:
#   check-test-porcelain.sh --write-baseline [repo_root] [outfile]
#   check-test-porcelain.sh [repo_root] [baseline_file]
#   check-test-porcelain.sh [repo_root]   # refuse any dirt (no baseline)
#
# Baseline format (v2): header line + one line per dirty/untracked path:
#   # armature-porcelain-baseline v2
#   <status><TAB><path><TAB><content_hash>
# Paths come from `git status --porcelain -z` (NUL-separated), so spaces,
# quotes, and non-ASCII names round-trip safely.
#
# All porcelain git goes through scripts/porcelain-git.sh (fail-closed).
# Non-repo roots fail. A 0-byte baseline file is never a valid write/read.
#
# Bash 3.2 compatible (macOS /bin/bash).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/porcelain-git.sh
. "$SCRIPT_DIR/porcelain-git.sh"

MODE="check"
if [ "${1:-}" = "--write-baseline" ]; then
  MODE="write"
  shift
fi

ROOT="${1:-.}"
BASELINE="${2:-}"
cd "$ROOT"

# Refuse non-git directories (no silenced git).
porcelain_git_rev_parse --git-dir >/dev/null

path_digest() {
  local path="$1"
  case "$path" in
    */) path="${path%/}" ;;
  esac
  if [ -d "$path" ]; then
    find "$path" \( -type f -o -type l \) -print \
      | LC_ALL=C sort \
      | while IFS= read -r f || [ -n "$f" ]; do
          [ -z "$f" ] && continue
          # Fail-closed: hash-object errors must propagate (no soft marker).
          h=$(porcelain_git_hash_object -- "$f")
          printf '%s %s\n' "$h" "$f"
        done \
      | porcelain_git_hash_object --stdin
  elif [ -e "$path" ] || [ -L "$path" ]; then
    porcelain_git_hash_object -- "$path"
  else
    printf 'missing\n'
  fi
}

# Emit sorted baseline body lines from porcelain -z (no header).
# Status stream is a temp file so a non-zero porcelain_git_status_z exit
# is not dropped (process substitution would swallow exit 128 under set -e).
# Body is written to a temp file (not piped) so path_digest / hash-object
# failures are not masked by a trailing `rm` or `sort` success.
emit_baseline_body() {
  local entry status path dig skip_next status_file body_file loop_st
  status_file=$(mktemp)
  body_file=$(mktemp)
  porcelain_git_status_z >"$status_file" || {
    rm -f "$status_file" "$body_file"
    exit 1
  }
  skip_next=0
  set +e
  while IFS= read -r -d '' entry || [ -n "$entry" ]; do
    if [ "$skip_next" -eq 1 ]; then
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
    dig="$(path_digest "$path")" || {
      rm -f "$status_file" "$body_file"
      exit 1
    }
    printf '%s\t%s\t%s\n' "$status" "$path" "$dig"
  done <"$status_file" >"$body_file"
  loop_st=$?
  set -e
  rm -f "$status_file"
  if [ "$loop_st" -ne 0 ]; then
    rm -f "$body_file"
    exit 1
  fi
  LC_ALL=C sort <"$body_file"
  rm -f "$body_file"
}

BASELINE_HEADER='# armature-porcelain-baseline v2'

if [ "$MODE" = "write" ]; then
  if [ -z "$BASELINE" ]; then
    echo "FAIL: --write-baseline requires an outfile" >&2
    exit 1
  fi
  body="$(emit_baseline_body)"
  {
    printf '%s\n' "$BASELINE_HEADER"
    if [ -n "$body" ]; then
      printf '%s\n' "$body"
    fi
  } >"$BASELINE"
  if [ ! -s "$BASELINE" ]; then
    echo "FAIL: --write-baseline produced 0-byte file" >&2
    rm -f "$BASELINE"
    exit 1
  fi
  exit 0
fi

current_body="$(emit_baseline_body)"
current_fp="$BASELINE_HEADER"
if [ -n "$current_body" ]; then
  current_fp="$BASELINE_HEADER
$current_body"
fi

if [ -z "$BASELINE" ]; then
  if [ -n "$current_body" ]; then
    echo "FAIL: go test left dirt in porcelain baseline (explicit TempDir/root required):" >&2
    printf '%s\n' "$current_body" | sed 's/\t[^	]*$//' | sed 's/^/  /' >&2
    exit 1
  fi
  echo "OK: porcelain baseline empty after tests"
  exit 0
fi

if [ ! -f "$BASELINE" ]; then
  echo "FAIL: porcelain baseline file missing: $BASELINE" >&2
  exit 1
fi
if [ ! -s "$BASELINE" ]; then
  echo "FAIL: porcelain baseline is 0 bytes (invalid)" >&2
  exit 1
fi

baseline_fp="$(cat "$BASELINE")"

if [ "$current_fp" = "$baseline_fp" ]; then
  echo "OK: porcelain content-hash baseline unchanged"
  exit 0
fi

echo "FAIL: go test changed dirty/untracked content vs pre-test baseline (status and/or hashes):" >&2
echo "  --- baseline ---" >&2
printf '%s\n' "$baseline_fp" | sed 's/^/  /' >&2
echo "  --- current ---" >&2
printf '%s\n' "$current_fp" | sed 's/^/  /' >&2
exit 1
