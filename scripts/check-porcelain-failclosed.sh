#!/usr/bin/env bash
# Guard: only scripts/porcelain-git.sh may invoke git status / git hash-object.
# Bans direct calls anywhere under scripts/ (any depth), including:
#   - git -C … status|hash-object
#   - git -c key=val … status|hash-object (config globals before subcmd)
#   - "$GIT" status / $g status (var-held command)
#   - line continuations and silencers (|| true, || :, 2>/dev/null, set +e, …)
# Selftest fixtures live under scripts/testdata/porcelain-bypass/.
set -euo pipefail
ROOT="${1:-.}"
cd "$ROOT"

HELPER="scripts/porcelain-git.sh"
fail=0

# Direct `git` with optional -C / repeated -c globals, then status|hash-object.
DIRECT_RE='(^|\$\(|`|[[:space:]]|;|&)git([[:space:]]+(-C[[:space:]]+[^[:space:]]+|-c[[:space:]]+[^[:space:]]+))*[[:space:]]+(status|hash-object)([[:space:]]|$)'
# Var-held command: "$GIT" status / $g status / ${GIT} hash-object
VAR_RE='(^|\$\(|`|[[:space:]]|;|&)(\$\{?[A-Za-z_][A-Za-z0-9_]*\}?|"\$\{?[A-Za-z_][A-Za-z0-9_]*\}?")[[:space:]]+(status|hash-object)([[:space:]]|$)'

flatten() {
  awk '{
    if (sub(/\\$/, "")) { printf "%s", $0; next }
    print
  }' "$1"
}

scan_file() {
  local f="$1"
  local flat hits
  flat="$(flatten "$f")"
  hits="$(printf '%s\n' "$flat" | grep -nE "$DIRECT_RE|$VAR_RE" \
    | grep -vE '^[[:digit:]]+:[[:space:]]*#' || true)"
  if [ -z "$hits" ]; then
    return 0
  fi
  if [ "$f" = "$HELPER" ]; then
    return 0
  fi
  echo "FAIL: $f invokes git status/hash-object outside $HELPER:" >&2
  printf '%s\n' "$hits" | sed 's/^/  /' >&2
  fail=1
}

scan_silencers() {
  local f="$1"
  local flat hits
  if [ "$f" = "$HELPER" ]; then
    return 0
  fi
  flat="$(flatten "$f")"
  hits="$(printf '%s\n' "$flat" | grep -nE "$DIRECT_RE|$VAR_RE" \
    | grep -vE '^[[:digit:]]+:[[:space:]]*#' \
    | grep -E '2>/dev/null|2>&-|set \+e|\|\|[[:space:]]*(true|:|echo|exit[[:space:]]+0)' || true)"
  if [ -n "$hits" ]; then
    echo "FAIL: $f silences git status/hash-object:" >&2
    printf '%s\n' "$hits" | sed 's/^/  /' >&2
    fail=1
  fi
}

while IFS= read -r f || [ -n "$f" ]; do
  [ -z "$f" ] && continue
  scan_file "$f"
  scan_silencers "$f"
done <<EOF
$(find scripts -type f \( -name '*.sh' -o -name '*.bash' -o -name '*.py' \) ! -path 'scripts/testdata/*' | LC_ALL=C sort)
EOF

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: scripts/ porcelain git confined to $HELPER"

bash "$ROOT/scripts/testdata/porcelain-bypass/run-selftest.sh" "$ROOT" "$(cd "$(dirname "$0")" && pwd)"
