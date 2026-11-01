#!/usr/bin/env bash
# Prove Bash 4+ constructs fail under Bash 3.2 and succeed under Bash 4.2+.
# - Fixture scripts under scripts/testdata/bash32-mutations/
# - Real-script mutation: run only the injected snippet under set -euo
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FIX="$ROOT/scripts/testdata/bash32-mutations"
fail=0
count=0

OUT="$(mktemp "${TMPDIR:-/tmp}/armature-bash32-mut-out.XXXXXX")"
ERR="$(mktemp "${TMPDIR:-/tmp}/armature-bash32-mut-err.XXXXXX")"
cleanup() { rm -f "$OUT" "$ERR"; }
trap cleanup EXIT

ver="$(bash --version | head -n 1)"
echo "prove-bash32-mutations under: $ver"

major=0
minor=0
case "$ver" in
  *"version "*)
    rest="${ver#*version }"
    major="${rest%%.*}"
    rest2="${rest#*.}"
    minor="${rest2%%.*}"
    ;;
esac

expect_fail=0
case "$major.$minor" in
  3.2)
    expect_fail=1
    ;;
esac

# [[ -v ]] is Bash 4.2+; on 4.0/4.1 treat like 3.2 for that fixture only.
vtest_expect_fail="$expect_fail"
case "$major.$minor" in
  4.0|4.1)
    vtest_expect_fail=1
    ;;
esac

for f in "$FIX"/*.sh; do
  if [ ! -f "$f" ]; then
    echo "FAIL: no mutation fixtures under $FIX" >&2
    exit 1
  fi
  count=$((count + 1))
  name=$(basename "$f")
  this_expect="$expect_fail"
  if [ "$name" = "v-test.sh" ]; then
    this_expect="$vtest_expect_fail"
  fi
  set +e
  bash "$f" >"$OUT" 2>"$ERR"
  st=$?
  set -e
  if [ "$this_expect" -eq 1 ]; then
    if [ "$st" -eq 0 ]; then
      echo "FAIL: $name exited 0 (expected fail under this bash)" >&2
      fail=1
    else
      echo "OK: $name failed (exit $st)"
    fi
  else
    if [ "$st" -ne 0 ]; then
      echo "FAIL: $name exited $st (expected success on Bash ${major}.${minor}+)" >&2
      cat "$ERR" >&2 || true
      fail=1
    else
      echo "OK: $name succeeded (exit 0)"
    fi
  fi
done

if [ "$count" -lt 3 ]; then
  echo "FAIL: expected at least 3 mutation fixtures, found $count" >&2
  exit 1
fi

# Token names via concatenation so this prover stays Bash-3.2-safe under
# scripts/check-bash32-scripts.sh regex pre-check.
_tok_ra='read'
_tok_ra="${_tok_ra}array"
_tok_mf='map'
_tok_mf="${_tok_mf}file"
_tok_cp='co'
_tok_cp="${_tok_cp}proc"

mut_dir="$(mktemp -d "${TMPDIR:-/tmp}/armature-bash32-realscript.XXXXXX")"
trap 'rm -rf "$mut_dir"; cleanup' EXIT

# Control: unmutated snippet under set -euo must exit 0 on every bash.
printf 'echo control_ok\n' >"$mut_dir/control.sh"
set +e
bash -c 'set -euo pipefail; source "$1"' _ "$mut_dir/control.sh" >"$OUT" 2>"$ERR"
st=$?
set -e
if [ "$st" -ne 0 ]; then
  echo "FAIL: unmutated control snippet exited $st" >&2
  cat "$ERR" >&2 || true
  fail=1
else
  echo "OK: unmutated control snippet exits 0"
fi

# Injected snippet only (derived from mutating check-fast.sh constructs).
run_injected() {
  local tok="$1"
  local snippet="$2"
  local expect_pat="$3"
  printf '%s\n' "$snippet" >"$mut_dir/inj-${tok}.sh"
  set +e
  bash -c 'set -euo pipefail; source "$1"' _ "$mut_dir/inj-${tok}.sh" >"$OUT" 2>"$ERR"
  st=$?
  set -e
  if [ "$expect_fail" -eq 1 ]; then
    if [ "$st" -eq 0 ]; then
      echo "FAIL: injected $tok snippet did not fail under Bash 3.2" >&2
      fail=1
      return
    fi
    if ! grep -q "$expect_pat" "$ERR"; then
      echo "FAIL: injected $tok missing expected error /$expect_pat/ in stderr:" >&2
      cat "$ERR" >&2 || true
      fail=1
      return
    fi
    echo "OK: injected $tok fails under Bash 3.2 with /$expect_pat/ (exit $st)"
  else
    if [ "$st" -ne 0 ]; then
      echo "FAIL: injected $tok exited $st on Bash ${major}.${minor}+" >&2
      cat "$ERR" >&2 || true
      fail=1
      return
    fi
    echo "OK: injected $tok succeeds under Bash ${major}.${minor}+"
  fi
}

run_injected "$_tok_ra" "${_tok_ra} -t _arm_bash32_mut < <(echo x)" "$_tok_ra"

# Sibling tokens: always assert (fail under 3.2, succeed under 4+).
run_injected "$_tok_mf" "${_tok_mf} -t _m < <(echo a)" "$_tok_mf"
run_injected "$_tok_cp" "${_tok_cp} _c { echo a; }" "$_tok_cp"

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: mutation fixtures + injected-snippet mutations match expected outcome under $ver"
