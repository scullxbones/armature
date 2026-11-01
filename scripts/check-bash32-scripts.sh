#!/usr/bin/env bash
# Optional regex pre-check: reject Bash 4+ features in make-invoked shell
# scripts that must run under macOS /bin/bash 3.2. Real enforcement is the
# check-bash32 CI job (pinned bash:3.2 container + mutation fixtures).
set -euo pipefail
ROOT="${1:-.}"
cd "$ROOT"

# Cross-OS make paths (test / coverage / test-ci / check-fast and their deps).
# Do not list mutation fixtures (they intentionally contain Bash 4+ tokens).
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
scripts/ci-bash32.sh
scripts/prove-bash32-mutations.sh
scripts/check-workflow-image-pins.sh
scripts/check-porcelain-failclosed.sh
scripts/check-no-engine-bin-env.sh
scripts/run-container-image.sh
scripts/porcelain-git.sh
scripts/ci-path-shadow-engines.sh
scripts/ci-path-shadow-selfcheck.sh
scripts/fetch-alpine-git-apks.sh
"

# Build the token list without writing a single source line that is itself a hit.
_tok_map='map'
_tok_map="${_tok_map}file"
_tok_read='read'
_tok_read="${_tok_read}array"
_tok_decl='declare -A'
_tok_co='co'
_tok_co="${_tok_co}proc"
# [[ -v name ]] — match "-v" inside [[ ... ]] without naming the full token here.
_tok_vtest='\[\[.*[[:space:]]-v[[:space:]]'
_pat="${_tok_map}|[[:space:]]${_tok_read}([[:space:]]|$)|${_tok_decl}|${_tok_co}|${_tok_vtest}|\\\$\\{[A-Za-z_][A-Za-z0-9_]*,,\\}|\\\$\\{[A-Za-z_][A-Za-z0-9_]*\\^\\^\\}"

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
