#!/usr/bin/env bash
# Guard: only scripts/porcelain-git.sh may invoke porcelain git subcommands.
#
# CLASS: Outside the helper, any (flattened) line with word-bounded `git` and
# later word-bounded `status`|`hash-object` MUST fail — any flags / -c / -C /
# --no-pager / punctuation between. Token end: ([^A-Za-z0-9_-]|$).
# Also bans var-held command forms ("$GIT" … / $g …) and xargs→git
# (e.g. `echo status | xargs git`).
# False positives → narrow per-line allowlist + written reason (never loosen).
#
# Selftest fixtures live under scripts/testdata/porcelain-bypass/; the selftest
# drives THIS guard against each fixture (ARM_PORCELAIN_SKIP_SELFTEST=1) so
# weakening these regexes fails the selftest.
set -euo pipefail
ROOT="${1:-.}"
cd "$ROOT"

HELPER="scripts/porcelain-git.sh"
fail=0

# Build class regex without embedding both tokens on one source line.
_gw='gi'
_gw="${_gw}t"
_st='sta'
_st="${_st}tus"
_ho='hash-obj'
_ho="${_ho}ect"
_tok_end='([^A-Za-z0-9_-]|$)'
_tok_start='(^|[^A-Za-z0-9_-])'
# git + delimiter + optional junk ending in delimiter + status|hash-object + end
CLASS_RE="${_tok_start}${_gw}([^A-Za-z0-9_-](.*[^A-Za-z0-9_-])?)(${_st}|${_ho})${_tok_end}"
# Var-held command: "$GIT" status / $g status / ${GIT} hash-object
VAR_RE="${_tok_start}(\\\$\\{?[A-Za-z_][A-Za-z0-9_]*\\}?|\"\\\$\\{?[A-Za-z_][A-Za-z0-9_]*\\}?\")[[:space:]]+(${_st}|${_ho})${_tok_end}"
# xargs … git (args may precede git on the pipeline; CLASS_RE alone misses this)
_xa='xar'
_xa="${_xa}gs"
XARGS_RE="${_tok_start}${_xa}([^A-Za-z0-9_-](.*[^A-Za-z0-9_-])?)${_gw}${_tok_end}"
BAN_RE="$CLASS_RE|$VAR_RE|$XARGS_RE"

# Per-line allowlist: exact "file:linenumber" → reason (never broaden the regex).
allow_line() {
  local key="$1"
  case "$key" in
    # none yet — add "path:N) return 0 ;;  # reason" entries here only
    *) return 1 ;;
  esac
}

flatten() {
  awk '{
    if (sub(/\\$/, "")) { printf "%s", $0; next }
    print
  }' "$1"
}

# Extensionless scripts under scripts/ (shebang) plus *.sh/*.bash/*.py.
is_scanned_script() {
  local f="$1" base
  base="${f##*/}"
  case "$base" in
    *.sh|*.bash|*.py) return 0 ;;
    *.*) return 1 ;;
  esac
  # Extensionless: require a shebang so binary/data files are skipped.
  head -n 1 "$f" 2>/dev/null | grep -q '^#!'
}

scan_file() {
  local f="$1"
  local flat hits line_no key kept=""
  flat="$(flatten "$f")"
  hits="$(printf '%s\n' "$flat" | grep -nE "$BAN_RE" \
    | grep -vE '^[[:digit:]]+:[[:space:]]*#' || true)"
  if [ -z "$hits" ]; then
    return 0
  fi
  if [ "$f" = "$HELPER" ]; then
    return 0
  fi
  while IFS= read -r hit || [ -n "$hit" ]; do
    [ -z "$hit" ] && continue
    line_no="${hit%%:*}"
    key="$f:$line_no"
    if allow_line "$key"; then
      continue
    fi
    kept="${kept}${hit}"$'\n'
  done <<EOF
$hits
EOF
  if [ -z "$kept" ]; then
    return 0
  fi
  echo "FAIL: $f invokes porcelain ${_st}/${_ho} outside $HELPER:" >&2
  printf '%s' "$kept" | sed 's/^/  /' >&2
  fail=1
}

scan_silencers() {
  local f="$1"
  local flat hits kept="" hit line_no key
  if [ "$f" = "$HELPER" ]; then
    return 0
  fi
  flat="$(flatten "$f")"
  hits="$(printf '%s\n' "$flat" | grep -nE "$BAN_RE" \
    | grep -vE '^[[:digit:]]+:[[:space:]]*#' \
    | grep -E '2>/dev/null|2>&-|set \+e|\|\|[[:space:]]*(true|:|echo|exit[[:space:]]+0)' || true)"
  if [ -z "$hits" ]; then
    return 0
  fi
  while IFS= read -r hit || [ -n "$hit" ]; do
    [ -z "$hit" ] && continue
    line_no="${hit%%:*}"
    key="$f:$line_no"
    if allow_line "$key"; then
      continue
    fi
    kept="${kept}${hit}"$'\n'
  done <<EOF
$hits
EOF
  if [ -n "$kept" ]; then
    echo "FAIL: $f silences porcelain ${_st}/${_ho}:" >&2
    printf '%s' "$kept" | sed 's/^/  /' >&2
    fail=1
  fi
}

while IFS= read -r f || [ -n "$f" ]; do
  [ -z "$f" ] && continue
  if ! is_scanned_script "$f"; then
    continue
  fi
  scan_file "$f"
  scan_silencers "$f"
done <<EOF
$(find scripts -type f ! -path 'scripts/testdata/*' | LC_ALL=C sort)
EOF

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: scripts/ porcelain git confined to $HELPER"

if [ "${ARM_PORCELAIN_SKIP_SELFTEST:-0}" = "1" ]; then
  exit 0
fi
bash "$ROOT/scripts/testdata/porcelain-bypass/run-selftest.sh" "$ROOT"
