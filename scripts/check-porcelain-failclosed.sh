#!/usr/bin/env bash
# Guard: scripts/ must not silence git status / git hash-object failures.
# Rejects 2>/dev/null and || true adjacent to those invocations.
set -euo pipefail
ROOT="${1:-.}"
cd "$ROOT"

fail=0
# Scan make-invoked and porcelain-adjacent shell under scripts/ (not testdata).
# Skip comment-only lines so this guard's own docs do not self-match.
while IFS= read -r f || [ -n "$f" ]; do
  [ -z "$f" ] && continue
  hits="$(grep -nE 'git[[:space:]]+(status|hash-object)' "$f" \
    | grep -vE '^[[:digit:]]+:[[:space:]]*#' \
    | grep -E '2>/dev/null|\|\|[[:space:]]*true' || true)"
  if [ -n "$hits" ]; then
    echo "FAIL: $f silences git status/hash-object (need fail-closed):" >&2
    printf '%s\n' "$hits" | sed 's/^/  /' >&2
    fail=1
  fi
done <<EOF
$(find scripts -maxdepth 1 -type f -name '*.sh' | LC_ALL=C sort)
EOF

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: scripts/ git status/hash-object are fail-closed"
