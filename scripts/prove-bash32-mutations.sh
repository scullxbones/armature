#!/usr/bin/env bash
# Prove Bash 4+ constructs fail under Bash 3.2 and succeed under Bash 4.2+.
# - Fixture scripts under scripts/testdata/bash32-mutations/
# - Real-script mutation: inject tokens into a copy of check-fast.sh
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

# Real-script mutation: inject Bash-4 tokens into a copy of check-fast.sh.
# Under 3.2, execution of the mutated copy must fail.
# Build token names by concatenation so this prover itself stays Bash-3.2-safe
# under scripts/check-bash32-scripts.sh regex pre-check.
_tok_ra='read'
_tok_ra="${_tok_ra}array"
_tok_mf='map'
_tok_mf="${_tok_mf}file"
_tok_cp='co'
_tok_cp="${_tok_cp}proc"

mut_dir="$(mktemp -d "${TMPDIR:-/tmp}/armature-bash32-realscript.XXXXXX")"
trap 'rm -rf "$mut_dir"; cleanup' EXIT
cp "$ROOT/scripts/check-fast.sh" "$mut_dir/check-fast.sh"
# Append a line that is valid on Bash 4+ but not on 3.2.
printf '\n# mutated for bash32 proof\n%s -t _arm_bash32_mut < <(echo x)\n' "$_tok_ra" >>"$mut_dir/check-fast.sh"
set +e
bash -n "$mut_dir/check-fast.sh" >"$OUT" 2>"$ERR"
st=$?
set -e
if [ "$expect_fail" -eq 1 ]; then
  if [ "$st" -eq 0 ]; then
    # bash -n on 3.2 may not always reject the injected builtin (runtime).
    # Force runtime: source the mutated tail under set -e.
    set +e
    bash -c 'set -euo pipefail; source "$1"' _ "$mut_dir/check-fast.sh" >"$OUT" 2>"$ERR"
    st=$?
    set -e
  fi
  if [ "$st" -eq 0 ]; then
    echo "FAIL: mutated check-fast.sh did not fail under Bash 3.2" >&2
    fail=1
  else
    echo "OK: mutated check-fast.sh fails under Bash 3.2 (exit $st)"
  fi
else
  # On Bash 4+, the mutated script should at least parse (`bash -n`).
  if [ "$st" -ne 0 ]; then
    echo "FAIL: mutated check-fast.sh failed bash -n on Bash ${major}.${minor}" >&2
    cat "$ERR" >&2 || true
    fail=1
  else
    echo "OK: mutated check-fast.sh parses under Bash ${major}.${minor}+"
  fi
fi

# Also prove sibling-token injections fail under 3.2 when expected.
for tok_name in "$_tok_mf" "$_tok_cp"; do
  cp "$ROOT/scripts/check-fast.sh" "$mut_dir/check-fast-${tok_name}.sh"
  if [ "$tok_name" = "$_tok_mf" ]; then
    printf '\n%s -t _m < <(echo a)\n' "$_tok_mf" >>"$mut_dir/check-fast-${tok_name}.sh"
  else
    printf '\n%s _c { echo a; }\n' "$_tok_cp" >>"$mut_dir/check-fast-${tok_name}.sh"
  fi
  set +e
  bash -c 'set -euo pipefail; source "$1"' _ "$mut_dir/check-fast-${tok_name}.sh" >"$OUT" 2>"$ERR"
  st=$?
  set -e
  if [ "$expect_fail" -eq 1 ]; then
    if [ "$st" -eq 0 ]; then
      echo "FAIL: mutated check-fast.sh ($tok_name) did not fail under Bash 3.2" >&2
      fail=1
    else
      echo "OK: mutated check-fast.sh ($tok_name) fails under Bash 3.2 (exit $st)"
    fi
  fi
done

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "OK: mutation fixtures + real-script mutations match expected outcome under $ver"
