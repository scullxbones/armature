#!/usr/bin/env bash
# Sanctioned tool resolver for production guard scripts.
#
# CLASS: guards never resolve tools from ambient PATH / `command -v`.
# Every shell-out must go through guard_resolve_tool, which returns an
# absolute path under /usr/bin or /bin only — fail-closed if unpinned.
#
# Source from a guard (absolute path via SCRIPT_DIR):
#   . "$SCRIPT_DIR/guard-resolve-tool.sh"
#   GREP="$(guard_resolve_tool grep)" || exit 1
#
# When executed as a script: prints the path for argv[1] (same rules).
# shellcheck shell=bash

guard_resolve_tool() {
  local tool="${1:-}"
  local dir cand
  if [ -z "$tool" ]; then
    echo "FAIL: guard_resolve_tool: tool name required" >&2
    return 1
  fi
  case "$tool" in
    *[!A-Za-z0-9._-]*)
      echo "FAIL: guard_resolve_tool: invalid tool name: $tool" >&2
      return 1
      ;;
  esac
  for dir in /usr/bin /bin; do
    cand="$dir/$tool"
    if [ -x "$cand" ]; then
      printf '%s\n' "$cand"
      return 0
    fi
  done
  echo "FAIL: cannot pin tool '$tool' to /usr/bin or /bin" >&2
  return 1
}

# Allow `bash scripts/guard-resolve-tool.sh grep` for static checks / demos.
if [ "${BASH_SOURCE[0]-}" = "$0" ]; then
  set -euo pipefail
  guard_resolve_tool "${1:?tool name required}"
fi
