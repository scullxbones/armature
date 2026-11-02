#!/usr/bin/env bash
SCRIPT_DIR="$(cd "${0%/*}" && pwd)"
. "$SCRIPT_DIR/guard-resolve-tool.sh"
GREP="$(guard_resolve_tool grep)" || exit 1
! grep foo /etc/passwd
