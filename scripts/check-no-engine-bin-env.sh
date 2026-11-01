#!/usr/bin/env bash
# Ban the engine-path env override name anywhere under scripts/ and .github/
# (word-boundary). Wrapper resolves docker from a fixed absolute-path allowlist
# only. Fixture: scripts/testdata/engine-bin-env/banned-export.sh must be flagged.
set -euo pipefail
ROOT="${1:-.}"
cd "$ROOT"

# Exact env name as a word. Built without writing the banned token as a
# contiguous scanned identifier in unrelated files.
_A='ARM_'
_B='DOCKER'
_C='_BIN'
BANNED="${_A}${_B}${_C}"
RE="(^|[^A-Za-z0-9_])${BANNED}([^A-Za-z0-9_]|$)"

fail=0
scan() {
  local f="$1"
  local hits
  hits="$(grep -nE "$RE" "$f" || true)"
  if [ -n "$hits" ]; then
    echo "FAIL: $f mentions ${BANNED} (banned; wrapper uses allowlist paths):" >&2
    printf '%s\n' "$hits" | sed 's/^/  /' >&2
    fail=1
  fi
}

while IFS= read -r f || [ -n "$f" ]; do
  [ -z "$f" ] && continue
  case "$f" in
    scripts/check-no-engine-bin-env.sh) continue ;;
    scripts/testdata/*) continue ;;
  esac
  scan "$f"
done <<EOF
$(find scripts .github -type f \( -name '*.sh' -o -name '*.bash' -o -name '*.yml' -o -name '*.yaml' -o -name '*.go' -o -name 'Makefile' \) 2>/dev/null | LC_ALL=C sort)
EOF

# Fixture must exist and must match the ban pattern (proves the regex).
FIXTURE="scripts/testdata/engine-bin-env/banned-export.sh"
if [ ! -f "$FIXTURE" ]; then
  echo "FAIL: missing ban fixture $FIXTURE" >&2
  exit 1
fi
if ! grep -qE "$RE" "$FIXTURE"; then
  echo "FAIL: ban fixture does not contain ${BANNED} word" >&2
  exit 1
fi
# Prove scanner would flag it if it lived under scripts/ (not testdata).
tmp=$(mktemp)
cp "$FIXTURE" "$tmp"
hits="$(grep -nE "$RE" "$tmp" || true)"
rm -f "$tmp"
if [ -z "$hits" ]; then
  echo "FAIL: ban regex did not match fixture contents" >&2
  exit 1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: engine-path env override banned (wrapper allowlist only)"
