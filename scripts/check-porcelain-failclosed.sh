#!/usr/bin/env bash
# Guard: only scripts/porcelain-git.sh may invoke porcelain git subcommands.
#
# CLASS: Outside the helper, any (flattened) line with word-bounded `git` and
# later word-bounded `status`|`hash-object` MUST fail — any flags / -c / -C /
# --no-pager / punctuation between. Token end: ([^A-Za-z0-9_-]|$).
# Also bans var-held command forms ("$GIT" … / $g …) and xargs→git
# (e.g. `echo status | xargs git`).
# False positives → narrow per-line allowlist + written reason (never loosen).
#
# Selftest fixtures live under scripts/testdata/porcelain-bypass/; the selftest
# drives THIS guard against each fixture (--skip-selftest) so weakening these
# regexes fails the selftest. Production never reads tool/skip overrides from env.
set -euo pipefail

SKIP_SELFTEST=0
ROOT="."
while [ "$#" -gt 0 ]; do
  case "$1" in
    --skip-selftest) SKIP_SELFTEST=1; shift ;;
    -*)
      echo "FAIL: unknown flag $1" >&2
      exit 2
      ;;
    *) ROOT="$1"; shift; break ;;
  esac
done
# Env skip/tool overrides are never honored (GREP=true must not disable scanning).
if [ "${ARM_PORCELAIN_SKIP_SELFTEST:-0}" = "1" ]; then
  echo "FAIL: ARM_PORCELAIN_SKIP_SELFTEST env is banned; use --skip-selftest" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LISTER="$SCRIPT_DIR/list-guard-scan-files.sh"
cd "$ROOT"

HELPER="scripts/porcelain-git.sh"
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

# Build class regex without embedding both tokens on one source line.
_gw='gi'
_gw="${_gw}t"
_st='sta'
_st="${_st}tus"
_ho='hash-obj'
_ho="${_ho}ect"
_tok_end='([^A-Za-z0-9_-]|$)'
_tok_start='(^|[^A-Za-z0-9_-])'
# git + delimiter + optional junk ending in delimiter + status|hash-object + end
CLASS_RE="${_tok_start}${_gw}([^A-Za-z0-9_-](.*[^A-Za-z0-9_-])?)(${_st}|${_ho})${_tok_end}"
# Var-held command: "$GIT" status / $g status / ${GIT} hash-object
VAR_RE="${_tok_start}(\\\$\\{?[A-Za-z_][A-Za-z0-9_]*\\}?|\"\\\$\\{?[A-Za-z_][A-Za-z0-9_]*\\}?\")[[:space:]]+(${_st}|${_ho})${_tok_end}"
# xargs … git (args may precede git on the pipeline; CLASS_RE alone misses this)
_xa='xar'
_xa="${_xa}gs"
XARGS_RE="${_tok_start}${_xa}([^A-Za-z0-9_-](.*[^A-Za-z0-9_-])?)${_gw}${_tok_end}"
BAN_RE="$CLASS_RE|$VAR_RE|$XARGS_RE"

# Per-line allowlist: exact "file:linenumber" → reason (never broaden the regex).
allow_line() {
  local key="$1"
  case "$key" in
    # Git-hermetic fixtures embed exec.CommandContext(..., "git", ..., "status")
    # as source text inside string literals; package has its own repoguard check.
    scripts/repoguard/repoguard_test.go:486) return 0 ;;
    scripts/repoguard/repoguard_test.go:501) return 0 ;;
    *) return 1 ;;
  esac
}

require_readable() {
  local f="$1"
  if [ ! -f "$f" ] || [ ! -r "$f" ]; then
    echo "FAIL: $f is not a readable file" >&2
    exit 1
  fi
}

# Fail-closed grep to tempfile: exit 0 = hits, 1 = none, ≥2 → FAIL unscannable.
# Never `|| true` — that lets missing/garbage paths pass.
# Each -ge 2 site uses a unique status var + FAIL call-site tag (B3 per-site kill).
grep_hits() {
  local re="$1" src="$2" out="$3" gh_st
  set +e
  LC_ALL=C grep -a -nE "$re" "$src" >"$out"
  gh_st=$?
  set -e
  if [ "$gh_st" -ge 2 ]; then
    echo "FAIL: $4 unscannable (grep_hits)" >&2
    fail=1
    return 1
  fi
  return 0
}

# Flatten \-continuations to a tempfile so NULs/\xff survive (no bash $(...) truncate).
flatten_to() {
  awk '{
    if (sub(/\\$/, "")) { printf "%s", $0; next }
    print
  }' "$1" >"$2"
}

# True if $1 has any non-whitespace character.
has_non_ws() {
  case "$1" in
    *[![:space:]]*) return 0 ;;
    *) return 1 ;;
  esac
}

scan_file() {
  local f="$1"
  local flat hits_file raw_hits line_no key kept="" hit sf_vst
  flat="$(mktemp)"
  hits_file="$(mktemp)"
  raw_hits="$(mktemp)"
  flatten_to "$f" "$flat"
  # LC_ALL=C + grep -a: non-UTF8 (\xff) on a hit line must still fail
  # (UTF-8 locale + -a alone still drops invalid-byte lines).
  # Write hits to a file — bash $(...) strips NUL and can hide mid-file hits.
  if ! grep_hits "$BAN_RE" "$flat" "$raw_hits" "$f"; then
    rm -f "$flat" "$hits_file" "$raw_hits"
    return 0
  fi
  # Distinct argv shape (-vE) vs silencers (-v -E) for per-site PATH shims.
  set +e
  LC_ALL=C grep -a -vE '^[[:digit:]]+:[[:space:]]*#' "$raw_hits" >"$hits_file"
  sf_vst=$?
  set -e
  rm -f "$flat" "$raw_hits"
  if [ "$sf_vst" -ge 2 ]; then
    echo "FAIL: $f unscannable (scan_file:comment-filter)" >&2
    fail=1
    rm -f "$hits_file"
    return 0
  fi
  if [ ! -s "$hits_file" ]; then
    rm -f "$hits_file"
    return 0
  fi
  if [ "$f" = "$HELPER" ]; then
    rm -f "$hits_file"
    return 0
  fi
  while IFS= read -r hit || [ -n "${hit:-}" ]; do
    [ -z "${hit:-}" ] && continue
    line_no="${hit%%:*}"
    key="$f:$line_no"
    if allow_line "$key"; then
      continue
    fi
    kept=1
    echo "FAIL: $f invokes porcelain ${_st}/${_ho} outside $HELPER:" >&2
    # Print remaining hits for this file (re-read from start once).
    break
  done <"$hits_file"
  if [ -n "$kept" ]; then
    while IFS= read -r hit || [ -n "${hit:-}" ]; do
      [ -z "${hit:-}" ] && continue
      line_no="${hit%%:*}"
      key="$f:$line_no"
      if allow_line "$key"; then
        continue
      fi
      printf '  %s\n' "$hit" >&2
    done <"$hits_file"
    fail=1
  fi
  rm -f "$hits_file"
}

scan_silencers() {
  local f="$1"
  local flat hits_file raw_hits kept="" hit line_no key sil_hits sil_vst sil_sst
  if [ "$f" = "$HELPER" ]; then
    return 0
  fi
  flat="$(mktemp)"
  hits_file="$(mktemp)"
  raw_hits="$(mktemp)"
  sil_hits="$(mktemp)"
  flatten_to "$f" "$flat"
  if ! grep_hits "$BAN_RE" "$flat" "$raw_hits" "$f"; then
    rm -f "$flat" "$hits_file" "$raw_hits" "$sil_hits"
    return 0
  fi
  # Distinct from scan_file (-vE): use -v -E so PATH shims can target one site.
  set +e
  LC_ALL=C grep -a -v -E '^[[:digit:]]+:[[:space:]]*#' "$raw_hits" >"$sil_hits"
  sil_vst=$?
  set -e
  if [ "$sil_vst" -ge 2 ]; then
    echo "FAIL: $f unscannable (scan_silencers:comment-filter)" >&2
    fail=1
    rm -f "$flat" "$hits_file" "$raw_hits" "$sil_hits"
    return 0
  fi
  set +e
  LC_ALL=C grep -a -E '2>/dev/null|2>&-|set \+e|\|\|[[:space:]]*(true|:|echo|exit[[:space:]]+0)' \
    "$sil_hits" >"$hits_file"
  sil_sst=$?
  set -e
  rm -f "$flat" "$raw_hits" "$sil_hits"
  if [ "$sil_sst" -ge 2 ]; then
    echo "FAIL: $f unscannable (scan_silencers:silencer-filter)" >&2
    fail=1
    rm -f "$hits_file"
    return 0
  fi
  if [ ! -s "$hits_file" ]; then
    rm -f "$hits_file"
    return 0
  fi
  while IFS= read -r hit || [ -n "${hit:-}" ]; do
    [ -z "${hit:-}" ] && continue
    line_no="${hit%%:*}"
    key="$f:$line_no"
    if allow_line "$key"; then
      continue
    fi
    kept=1
    break
  done <"$hits_file"
  if [ -n "$kept" ]; then
    echo "FAIL: $f silences porcelain ${_st}/${_ho}:" >&2
    while IFS= read -r hit || [ -n "${hit:-}" ]; do
      [ -z "${hit:-}" ] && continue
      line_no="${hit%%:*}"
      key="$f:$line_no"
      if allow_line "$key"; then
        continue
      fi
      printf '  %s\n' "$hit" >&2
    done <"$hits_file"
    fail=1
  fi
  rm -f "$hits_file"
}

# Shared selector: every scan-eligible file under scripts/ (no ext/shebang filter).
# Scope = main: only scripts/testdata/* excluded — do NOT skip scripts/repoguard/*.
# Fail-closed: write list to file; nonzero lister exit → fail (never swallow).
# Count files actually scanned — blank/whitespace-only selector print must FAIL.
scan_list=$(mktemp)
trap 'rm -f "$scan_list"' EXIT
set +e
bash "$LISTER" --exclude-testdata scripts >"$scan_list"
lister_st=$?
set -e
if [ "$lister_st" -ne 0 ]; then
  echo "FAIL: list-guard-scan-files.sh exited $lister_st (porcelain scan aborted)" >&2
  exit 1
fi

while IFS= read -r f || [ -n "${f:-}" ]; do
  # Blank or whitespace-only selector lines do not count as scanned files.
  if ! has_non_ws "${f:-}"; then
    continue
  fi
  require_readable "$f"
  scanned=$((scanned + 1))
  scan_file "$f"
  scan_silencers "$f"
done <"$scan_list"

if [ "$scanned" -eq 0 ]; then
  echo "FAIL: scanned 0 files (porcelain)" >&2
  exit 1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: scripts/ porcelain git confined to $HELPER"

if [ "$SKIP_SELFTEST" -eq 1 ]; then
  exit 0
fi
bash "$ROOT/scripts/testdata/porcelain-bypass/run-selftest.sh" "$ROOT"
