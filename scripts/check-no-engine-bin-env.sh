#!/usr/bin/env bash
# Ban wrapper bypasses under scripts/ and .github/:
#   1) engine-path env override name (word-boundary)
#   2) `command -p docker` (and peers) — PATH-shadow only catches bare lookups
#   3) absolute-path docker (/usr/bin/docker, /usr/local/bin/docker) outside
#      scripts/run-container-image.sh (the sole intentional allowlist use)
# Fixtures under scripts/testdata/engine-bin-env/ prove each ban.
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

# command -p <engine> — build without a single contiguous banned phrase.
_cmd='com'
_cmd="${_cmd}mand"
_pflag='-p'
_engines='doc'
_engines="${_engines}ker|podman|buildx|nerdctl|ctr|finch|lima|crictl"
CMD_P_RE="${_cmd}[[:space:]]+${_pflag}[[:space:]]+(${_engines})"

# Absolute docker paths (wrapper allowlist only).
_u='/usr'
_db='/bin/doc'
_db="${_db}ker"
ABS_DOCKER_1="${_u}${_db}"
ABS_DOCKER_2="${_u}/local${_db}"
ABS_RE="(^|[^A-Za-z0-9_])(${ABS_DOCKER_1}|${ABS_DOCKER_2})([^A-Za-z0-9_]|$)"

WRAPPER="scripts/run-container-image.sh"
fail=0

scan_env() {
  local f="$1"
  local hits
  hits="$(grep -nE "$RE" "$f" || true)"
  if [ -n "$hits" ]; then
    echo "FAIL: $f mentions ${BANNED} (banned; wrapper uses allowlist paths):" >&2
    printf '%s\n' "$hits" | sed 's/^/  /' >&2
    fail=1
  fi
}

scan_cmd_p() {
  local f="$1"
  local hits
  hits="$(grep -nE "$CMD_P_RE" "$f" || true)"
  if [ -n "$hits" ]; then
    echo "FAIL: $f uses command -p <engine> (banned; use $WRAPPER):" >&2
    printf '%s\n' "$hits" | sed 's/^/  /' >&2
    fail=1
  fi
}

scan_abs_docker() {
  local f="$1"
  local hits
  if [ "$f" = "$WRAPPER" ]; then
    return 0
  fi
  hits="$(grep -nE "$ABS_RE" "$f" || true)"
  if [ -n "$hits" ]; then
    echo "FAIL: $f uses absolute-path docker outside $WRAPPER:" >&2
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
  scan_env "$f"
  scan_cmd_p "$f"
  scan_abs_docker "$f"
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

FIXTURE_CMDP="scripts/testdata/engine-bin-env/banned-command-p-docker.sh"
if [ ! -f "$FIXTURE_CMDP" ]; then
  echo "FAIL: missing ban fixture $FIXTURE_CMDP" >&2
  exit 1
fi
if ! grep -qE "$CMD_P_RE" "$FIXTURE_CMDP"; then
  echo "FAIL: command -p fixture not matched by ban regex" >&2
  exit 1
fi

FIXTURE_ABS="scripts/testdata/engine-bin-env/banned-abs-docker.sh"
if [ ! -f "$FIXTURE_ABS" ]; then
  echo "FAIL: missing ban fixture $FIXTURE_ABS" >&2
  exit 1
fi
if ! grep -qE "$ABS_RE" "$FIXTURE_ABS"; then
  echo "FAIL: abs-docker fixture not matched by ban regex" >&2
  exit 1
fi
# Prove abs-path scanner would flag the fixture outside the wrapper allowlist.
tmp=$(mktemp)
cp "$FIXTURE_ABS" "$tmp"
hits="$(grep -nE "$ABS_RE" "$tmp" || true)"
rm -f "$tmp"
if [ -z "$hits" ]; then
  echo "FAIL: abs-docker ban regex did not match fixture contents" >&2
  exit 1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: engine-path env override + command -p/abs docker banned (wrapper allowlist only)"
