#!/usr/bin/env bash
# Guard: only scripts/porcelain-git.sh may invoke git status / git hash-object.
# Bans direct calls anywhere under scripts/ (any depth), including git -C,
# line continuations, and silencers (|| true, || :, 2>/dev/null, set +e, …).
set -euo pipefail
ROOT="${1:-.}"
cd "$ROOT"

HELPER="scripts/porcelain-git.sh"
fail=0

# Command-shaped hits only (not prose like "dirt in git status --porcelain").
CMD_RE='(^|\$\(|`|[[:space:]]|;|&)git[[:space:]]+(-C[[:space:]]+[^[:space:]]+[[:space:]]+)?(status|hash-object)([[:space:]]|$)'

scan_file() {
  local f="$1"
  local flat hits
  flat="$(awk '{
    if (sub(/\\$/, "")) { printf "%s", $0; next }
    print
  }' "$f")"
  hits="$(printf '%s\n' "$flat" | grep -nE "$CMD_RE" \
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
  flat="$(awk '{
    if (sub(/\\$/, "")) { printf "%s", $0; next }
    print
  }' "$f")"
  hits="$(printf '%s\n' "$flat" | grep -nE "$CMD_RE" \
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
