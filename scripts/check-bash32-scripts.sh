#!/usr/bin/env bash
# Reject Bash 4+ features in make-invoked shell scripts that must run under
# macOS /bin/bash 3.2 (and other Bash-3 environments).
set -euo pipefail
ROOT="${1:-.}"
cd "$ROOT"

# Cross-OS make paths (test / coverage / test-ci / check-fast and their deps).
# Do not list this guard script itself (its detector string names the tokens).
SCRIPTS="
scripts/check-test-porcelain.sh
scripts/check-commit-subjects.sh
scripts/check-fast.sh
scripts/check-mock-ban.sh
scripts/coverage-check.sh
scripts/assert-git-min.sh
scripts/install-git-from-source.sh
scripts/install-verified-github-release.sh
scripts/quickstart_check.sh
"

# Build the token list without writing a single source line that is itself a hit.
_tok_map='map'
_tok_map="${_tok_map}file"
_tok_read='read'
_tok_read="${_tok_read}array"
_tok_decl='declare -A'
_pat="${_tok_map}|[[:space:]]${_tok_read}([[:space:]]|$)|${_tok_decl}|\\\$\\{[A-Za-z_][A-Za-z0-9_]*,,\\}|\\\$\\{[A-Za-z_][A-Za-z0-9_]*\\^\\^\\}"

fail=0
for f in $SCRIPTS; do
  if [ ! -f "$f" ]; then
    echo "FAIL: missing make-invoked script $f" >&2
    fail=1
    continue
  fi
  hits="$(grep -nE "$_pat" "$f" | grep -vE '^[[:digit:]]+:[[:space:]]*#' || true)"
  if [ -n "$hits" ]; then
    echo "FAIL: $f uses Bash 4+ features (need Bash 3.2 for macOS make test):" >&2
    printf '%s\n' "$hits" | sed 's/^/  /' >&2
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: make-invoked cross-OS scripts are Bash 3.2-safe"
