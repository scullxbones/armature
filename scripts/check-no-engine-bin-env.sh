#!/usr/bin/env bash
# Ban wrapper bypasses under scripts/ and .github/:
#   1) engine-path env override name (word-boundary)
#   2) `command -p docker` (and peers) — PATH-shadow only catches bare lookups
#   3) absolute-path docker (/usr/bin/docker, /usr/local/bin/docker) outside
#      scripts/run-container-image.sh (the sole intentional allowlist use)
# Fixtures under scripts/testdata/engine-bin-env/ prove each ban.
set -euo pipefail
ROOT="${1:-.}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LISTER="$SCRIPT_DIR/list-guard-scan-files.sh"
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

# Grep hits to a tempfile (bash $(...) strips NUL / can hide mid-file hits).
grep_hits() {
  local re="$1" f="$2" out="$3"
  LC_ALL=C grep -a -nE "$re" "$f" >"$out" || true
}

scan_env() {
  local f="$1"
  local hits_file
  hits_file="$(mktemp)"
  # LC_ALL=C + grep -a: non-UTF8 (\xff) on a hit line must still fail.
  grep_hits "$RE" "$f" "$hits_file"
  if [ -s "$hits_file" ]; then
    echo "FAIL: $f mentions ${BANNED} (banned; wrapper uses allowlist paths):" >&2
    sed 's/^/  /' "$hits_file" >&2
    fail=1
  fi
  rm -f "$hits_file"
}

scan_cmd_p() {
  local f="$1"
  local hits_file
  hits_file="$(mktemp)"
  grep_hits "$CMD_P_RE" "$f" "$hits_file"
  if [ -s "$hits_file" ]; then
    echo "FAIL: $f uses command -p <engine> (banned; use $WRAPPER):" >&2
    sed 's/^/  /' "$hits_file" >&2
    fail=1
  fi
  rm -f "$hits_file"
}

scan_abs_docker() {
  local f="$1"
  local hits_file
  if [ "$f" = "$WRAPPER" ]; then
    return 0
  fi
  hits_file="$(mktemp)"
  grep_hits "$ABS_RE" "$f" "$hits_file"
  if [ -s "$hits_file" ]; then
    echo "FAIL: $f uses absolute-path docker outside $WRAPPER:" >&2
    sed 's/^/  /' "$hits_file" >&2
    fail=1
  fi
  rm -f "$hits_file"
}

# Shared selector: every scan-eligible file under scripts/ and .github/.
# Scope = main: only scripts/testdata/* excluded via --exclude-testdata.
# Fail-closed: write list to file; nonzero lister exit → fail (never swallow).
scan_list=$(mktemp)
trap 'rm -f "$scan_list"' EXIT
set +e
bash "$LISTER" --exclude-testdata scripts .github >"$scan_list"
lister_st=$?
set -e
if [ "$lister_st" -ne 0 ]; then
  echo "FAIL: list-guard-scan-files.sh exited $lister_st (engine scan aborted)" >&2
  exit 1
fi

while IFS= read -r f || [ -n "${f:-}" ]; do
  [ -z "${f:-}" ] && continue
  case "$f" in
    scripts/check-no-engine-bin-env.sh) continue ;;
    scripts/list-guard-scan-files.sh) continue ;;
  esac
  scan_env "$f"
  scan_cmd_p "$f"
  scan_abs_docker "$f"
done <"$scan_list"

# Fixture must exist and must match the ban pattern (proves the regex).
FIXTURE="scripts/testdata/engine-bin-env/banned-export.sh"
if [ ! -f "$FIXTURE" ]; then
  echo "FAIL: missing ban fixture $FIXTURE" >&2
  exit 1
fi
if ! LC_ALL=C grep -a -qE "$RE" "$FIXTURE"; then
  echo "FAIL: ban fixture does not contain ${BANNED} word" >&2
  exit 1
fi
# Prove scanner would flag it if it lived under scripts/ (not testdata).
tmp=$(mktemp)
cp "$FIXTURE" "$tmp"
hits="$(LC_ALL=C grep -a -nE "$RE" "$tmp" || true)"
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
if ! LC_ALL=C grep -a -qE "$CMD_P_RE" "$FIXTURE_CMDP"; then
  echo "FAIL: command -p fixture not matched by ban regex" >&2
  exit 1
fi

FIXTURE_ABS="scripts/testdata/engine-bin-env/banned-abs-docker.sh"
if [ ! -f "$FIXTURE_ABS" ]; then
  echo "FAIL: missing ban fixture $FIXTURE_ABS" >&2
  exit 1
fi
if ! LC_ALL=C grep -a -qE "$ABS_RE" "$FIXTURE_ABS"; then
  echo "FAIL: abs-docker fixture not matched by ban regex" >&2
  exit 1
fi
# Prove abs-path scanner would flag the fixture outside the wrapper allowlist.
tmp=$(mktemp)
cp "$FIXTURE_ABS" "$tmp"
hits="$(LC_ALL=C grep -a -nE "$ABS_RE" "$tmp" || true)"
rm -f "$tmp"
if [ -z "$hits" ]; then
  echo "FAIL: abs-docker ban regex did not match fixture contents" >&2
  exit 1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: engine-path env override + command -p/abs docker banned (wrapper allowlist only)"
