#!/usr/bin/env bash
# Ban wrapper bypasses under scripts/ and .github/:
#   1) engine-path env override name (word-boundary)
#   2) `command -p docker` (and peers) — PATH-shadow only catches bare lookups
#   3) absolute-path docker (/usr/bin/docker, /usr/local/bin/docker) outside
#      scripts/run-container-image.sh (the sole intentional allowlist use)
# Fixtures under scripts/testdata/engine-bin-env/ prove each ban.
# Production never reads tool overrides from env (GREP=true must not disable scanning).
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
scanned=0

# Absolute grep for fixture proofs; scanning uses bare `grep` so PATH shims work.
# Never read GREP= from the environment.
REAL_GREP=""
for cand in /usr/bin/grep /bin/grep; do
  if [ -x "$cand" ]; then
    REAL_GREP="$cand"
    break
  fi
done
if [ -z "$REAL_GREP" ]; then
  REAL_GREP="$(command -v grep)"
fi

# Fail-closed grep: exit 0 = hits, 1 = no hits, ≥2 = error → FAIL unscannable.
# Never `|| true` — that lets missing/garbage paths pass.
grep_hits() {
  local re="$1" f="$2" out="$3" gh_st
  set +e
  LC_ALL=C grep -a -nE "$re" "$f" >"$out"
  gh_st=$?
  set -e
  if [ "$gh_st" -ge 2 ]; then
    echo "FAIL: $f unscannable (grep_hits)" >&2
    fail=1
    return 1
  fi
  return 0
}

require_readable() {
  local f="$1"
  if [ ! -f "$f" ] || [ ! -r "$f" ]; then
    echo "FAIL: $f is not a readable file" >&2
    exit 1
  fi
}

# True if $1 has any non-whitespace character.
has_non_ws() {
  case "$1" in
    *[![:space:]]*) return 0 ;;
    *) return 1 ;;
  esac
}

scan_env() {
  local f="$1"
  local hits_file
  hits_file="$(mktemp)"
  # LC_ALL=C + grep -a: non-UTF8 (\xff) on a hit line must still fail.
  if ! grep_hits "$RE" "$f" "$hits_file"; then
    rm -f "$hits_file"
    return 0
  fi
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
  if ! grep_hits "$CMD_P_RE" "$f" "$hits_file"; then
    rm -f "$hits_file"
    return 0
  fi
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
  if ! grep_hits "$ABS_RE" "$f" "$hits_file"; then
    rm -f "$hits_file"
    return 0
  fi
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
# Count files actually scanned — blank/whitespace-only selector print must FAIL.
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
  if ! has_non_ws "${f:-}"; then
    continue
  fi
  require_readable "$f"
  case "$f" in
    scripts/check-no-engine-bin-env.sh) continue ;;
    scripts/list-guard-scan-files.sh) continue ;;
  esac
  scanned=$((scanned + 1))
  scan_env "$f"
  scan_cmd_p "$f"
  scan_abs_docker "$f"
done <"$scan_list"

if [ "$scanned" -eq 0 ]; then
  echo "FAIL: scanned 0 files (engine)" >&2
  exit 1
fi

# Fixture must exist and must match the ban pattern (proves the regex).
FIXTURE="scripts/testdata/engine-bin-env/banned-export.sh"
if [ ! -f "$FIXTURE" ]; then
  echo "FAIL: missing ban fixture $FIXTURE" >&2
  exit 1
fi
if ! LC_ALL=C "$REAL_GREP" -a -qE "$RE" "$FIXTURE"; then
  echo "FAIL: ban fixture does not contain ${BANNED} word" >&2
  exit 1
fi
# Prove scanner would flag it if it lived under scripts/ (not testdata).
# Fixture proofs use REAL_GREP so PATH shims do not break ban proofs.
tmp=$(mktemp)
cp "$FIXTURE" "$tmp"
hits_file=$(mktemp)
set +e
LC_ALL=C "$REAL_GREP" -a -nE "$RE" "$tmp" >"$hits_file"
fx_st=$?
set -e
rm -f "$tmp"
if [ "$fx_st" -ge 2 ]; then
  echo "FAIL: $FIXTURE unscannable (fixture)" >&2
  rm -f "$hits_file"
  exit 1
fi
if [ ! -s "$hits_file" ]; then
  echo "FAIL: ban regex did not match fixture contents" >&2
  rm -f "$hits_file"
  exit 1
fi
rm -f "$hits_file"

FIXTURE_CMDP="scripts/testdata/engine-bin-env/banned-command-p-docker.sh"
if [ ! -f "$FIXTURE_CMDP" ]; then
  echo "FAIL: missing ban fixture $FIXTURE_CMDP" >&2
  exit 1
fi
if ! LC_ALL=C "$REAL_GREP" -a -qE "$CMD_P_RE" "$FIXTURE_CMDP"; then
  echo "FAIL: command -p fixture not matched by ban regex" >&2
  exit 1
fi

FIXTURE_ABS="scripts/testdata/engine-bin-env/banned-abs-docker.sh"
if [ ! -f "$FIXTURE_ABS" ]; then
  echo "FAIL: missing ban fixture $FIXTURE_ABS" >&2
  exit 1
fi
if ! LC_ALL=C "$REAL_GREP" -a -qE "$ABS_RE" "$FIXTURE_ABS"; then
  echo "FAIL: abs-docker fixture not matched by ban regex" >&2
  exit 1
fi
# Prove abs-path scanner would flag the fixture outside the wrapper allowlist.
tmp=$(mktemp)
cp "$FIXTURE_ABS" "$tmp"
hits_file=$(mktemp)
set +e
LC_ALL=C "$REAL_GREP" -a -nE "$ABS_RE" "$tmp" >"$hits_file"
fx_st=$?
set -e
rm -f "$tmp"
if [ "$fx_st" -ge 2 ]; then
  echo "FAIL: $FIXTURE_ABS unscannable (fixture)" >&2
  rm -f "$hits_file"
  exit 1
fi
if [ ! -s "$hits_file" ]; then
  echo "FAIL: abs-docker ban regex did not match fixture contents" >&2
  rm -f "$hits_file"
  exit 1
fi
rm -f "$hits_file"

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: engine-path env override + command -p/abs docker banned (wrapper allowlist only)"
