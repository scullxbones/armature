#!/usr/bin/env bash
SCRIPT_DIR="$(cd "${0%/*}" && pwd)"
. "$SCRIPT_DIR/guard-resolve-tool.sh"
GREP="$(guard_resolve_tool grep)" || exit 1
LC_ALL=C grep foo /etc/passwd
