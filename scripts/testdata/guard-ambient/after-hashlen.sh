#!/usr/bin/env bash
SCRIPT_DIR="$(cd "${0%/*}" && pwd)"
. "$SCRIPT_DIR/guard-resolve-tool.sh"
GREP="$(guard_resolve_tool grep)" || exit 1
var=hello
# call after ${#var} — comment must not truncate; bare grep must still FAIL
echo "${#var}"
grep foo /etc/passwd
