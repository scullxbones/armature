#!/usr/bin/env bash
# Planted: if grep -q must FAIL ambient check
SCRIPT_DIR="$(cd "${0%/*}" && pwd)"
. "$SCRIPT_DIR/guard-resolve-tool.sh"
GREP="$(guard_resolve_tool grep)" || exit 1
if grep -q foo /etc/passwd; then
  true
fi
