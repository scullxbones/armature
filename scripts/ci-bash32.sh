#!/usr/bin/env bash
# CI entrypoint: run cross-OS make scripts under the real Bash that is on PATH.
# Intended host is the pinned bash:3.2 container (see .github/workflows/ci.yml).
# Bash 3.2 compatible — this file itself must parse under 3.2.
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

ver="$(bash --version | head -n 1)"
echo "bash: $ver"
case "$ver" in
  *"version 3.2"*)
    ;;
  *)
    echo "FAIL: expected Bash 3.2.x on PATH, got: $ver" >&2
    exit 1
    ;;
esac

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
scripts/check-bash32-scripts.sh
"

for f in $SCRIPTS; do
  if [ ! -f "$f" ]; then
    echo "FAIL: missing script $f" >&2
    exit 1
  fi
  bash -n "$f"
  echo "OK: bash -n $f"
done

# Smoke: content-hash porcelain write + check under Bash 3.2.
demo="$(mktemp -d "${TMPDIR:-/tmp}/armature-bash32-porc.XXXXXX")"
cleanup_demo() { rm -rf "$demo"; }
trap cleanup_demo EXIT
(
  cd "$demo"
  git init -q -b main
  git config user.email "bash32@example.com"
  git config user.name "bash32"
  printf 'seed\n' > tracked.txt
  git add tracked.txt
  git commit -q -m "feat(BASH32): seed"
  baseline="$(mktemp "${TMPDIR:-/tmp}/armature-bash32-base.XXXXXX")"
  bash "$ROOT/scripts/check-test-porcelain.sh" --write-baseline . "$baseline"
  bash "$ROOT/scripts/check-test-porcelain.sh" . "$baseline" >/dev/null
  rm -f "$baseline"
)
echo "OK: porcelain content-hash smoke under Bash 3.2"

bash "$ROOT/scripts/prove-bash32-mutations.sh"
echo "OK: ci-bash32 complete"
