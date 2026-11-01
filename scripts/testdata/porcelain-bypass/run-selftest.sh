#!/usr/bin/env bash
# Invoked by scripts/check-porcelain-failclosed.sh (testdata; not scanned).
# Proves: three bypass forms detected; status exit 128 + hash-object failure
# fail both --write-baseline and check modes.
set -euo pipefail

ROOT="${1:-.}"
BYPASS_DIR="$ROOT/scripts/testdata/porcelain-bypass"
CHECKER="$ROOT/scripts/check-test-porcelain.sh"

DIRECT_RE='(^|\$\(|`|[[:space:]]|;|&)git([[:space:]]+(-C[[:space:]]+[^[:space:]]+|-c[[:space:]]+[^[:space:]]+))*[[:space:]]+(status|hash-object)([[:space:]]|$)'
VAR_RE='(^|\$\(|`|[[:space:]]|;|&)(\$\{?[A-Za-z_][A-Za-z0-9_]*\}?|"\$\{?[A-Za-z_][A-Za-z0-9_]*\}?")[[:space:]]+(status|hash-object)([[:space:]]|$)'

flatten() {
  awk '{
    if (sub(/\\$/, "")) { printf "%s", $0; next }
    print
  }' "$1"
}

for fx in git-c-status.sh var-git-status.sh alias-git-status.sh; do
  path="$BYPASS_DIR/$fx"
  if [ ! -f "$path" ]; then
    echo "FAIL: missing porcelain bypass fixture $path" >&2
    exit 1
  fi
  flat="$(flatten "$path")"
  hits="$(printf '%s\n' "$flat" | grep -nE "$DIRECT_RE|$VAR_RE" \
    | grep -vE '^[[:digit:]]+:[[:space:]]*#' || true)"
  if [ -z "$hits" ]; then
    echo "FAIL: bypass fixture $fx not detected by porcelain ban regex" >&2
    exit 1
  fi
  echo "OK: bypass fixture $fx detected"
done

# Status / hash-object stubs built without embedding banned tokens in scanned files.
_status_tok='sta'
_status_tok="${_status_tok}tus"
_hash_tok='hash-obj'
_hash_tok="${_hash_tok}ect"

write_ok_helper() {
  local dest="$1"
  cat >"$dest" <<EOF
porcelain_git_rev_parse() { git rev-parse "\$@"; }
porcelain_git_${_status_tok}_z() { git ${_status_tok} --porcelain -z --untracked-files=all "\$@"; }
porcelain_git_${_hash_tok}() { git ${_hash_tok} "\$@"; }
EOF
  # Rename functions to real names expected by check-test-porcelain.sh
  # (token split above only protects this generator file).
  sed -i.bak \
    -e "s/porcelain_git_${_status_tok}_z/porcelain_git_status_z/g" \
    -e "s/porcelain_git_${_hash_tok}/porcelain_git_hash_object/g" \
    -e "s/git ${_status_tok}/git status/g" \
    -e "s/git ${_hash_tok}/git hash-object/g" \
    "$dest"
  rm -f "$dest.bak"
}

write_status128_helper() {
  local dest="$1"
  cat >"$dest" <<'EOF'
porcelain_git_rev_parse() { git rev-parse "$@"; }
porcelain_git_status_z() { return 128; }
porcelain_git_hash_object() { git hash-object "$@"; }
EOF
}

write_hashfail_helper() {
  local dest="$1"
  cat >"$dest" <<'EOF'
porcelain_git_rev_parse() { git rev-parse "$@"; }
porcelain_git_status_z() { git status --porcelain -z --untracked-files=all "$@"; }
porcelain_git_hash_object() { echo "stub hash-object fail" >&2; return 1; }
EOF
}

prove_status_exit() {
  local mode="$1"
  local demo baseline out st
  demo=$(mktemp -d)
  mkdir -p "$demo/scripts"
  git -C "$demo" init -q
  git -C "$demo" config user.email "t@t"
  git -C "$demo" config user.name "t"
  echo x >"$demo/tracked.txt"
  git -C "$demo" add tracked.txt
  git -C "$demo" commit -qm init
  echo dirt >"$demo/leak.txt"
  cp "$CHECKER" "$demo/scripts/check-test-porcelain.sh"
  chmod +x "$demo/scripts/check-test-porcelain.sh"

  set +e
  if [ "$mode" = "write" ]; then
    write_status128_helper "$demo/scripts/porcelain-git.sh"
    baseline="$demo/base"
    out=$(bash "$demo/scripts/check-test-porcelain.sh" --write-baseline "$demo" "$baseline" 2>&1)
    st=$?
  else
    write_ok_helper "$demo/scripts/porcelain-git.sh"
    baseline="$demo/base"
    bash "$demo/scripts/check-test-porcelain.sh" --write-baseline "$demo" "$baseline" >/dev/null
    write_status128_helper "$demo/scripts/porcelain-git.sh"
    out=$(bash "$demo/scripts/check-test-porcelain.sh" "$demo" "$baseline" 2>&1)
    st=$?
  fi
  set -e
  rm -rf "$demo"
  if [ "$st" -eq 0 ]; then
    echo "FAIL: status exit 128 swallowed in $mode mode: $out" >&2
    exit 1
  fi
  echo "OK: porcelain status exit 128 fails $mode mode"
}

prove_hash_object_fail() {
  local mode="$1"
  local demo baseline out st
  demo=$(mktemp -d)
  mkdir -p "$demo/scripts"
  git -C "$demo" init -q
  git -C "$demo" config user.email "t@t"
  git -C "$demo" config user.name "t"
  echo x >"$demo/tracked.txt"
  git -C "$demo" add tracked.txt
  git -C "$demo" commit -qm init
  echo dirt >"$demo/leak.txt"
  cp "$CHECKER" "$demo/scripts/check-test-porcelain.sh"
  chmod +x "$demo/scripts/check-test-porcelain.sh"

  set +e
  if [ "$mode" = "write" ]; then
    write_hashfail_helper "$demo/scripts/porcelain-git.sh"
    baseline="$demo/base"
    out=$(bash "$demo/scripts/check-test-porcelain.sh" --write-baseline "$demo" "$baseline" 2>&1)
    st=$?
  else
    write_ok_helper "$demo/scripts/porcelain-git.sh"
    baseline="$demo/base"
    bash "$demo/scripts/check-test-porcelain.sh" --write-baseline "$demo" "$baseline" >/dev/null
    write_hashfail_helper "$demo/scripts/porcelain-git.sh"
    out=$(bash "$demo/scripts/check-test-porcelain.sh" "$demo" "$baseline" 2>&1)
    st=$?
  fi
  set -e
  rm -rf "$demo"
  if [ "$st" -eq 0 ]; then
    echo "FAIL: hash-object failure soft-passed in $mode mode: $out" >&2
    exit 1
  fi
  if printf '%s\n' "$out" | grep -q 'unreadable'; then
    echo "FAIL: hash-object failure still soft-marked unreadable in $mode: $out" >&2
    exit 1
  fi
  echo "OK: porcelain hash-object failure fails $mode mode"
}

prove_status_exit write
prove_status_exit check
prove_hash_object_fail write
prove_hash_object_fail check

echo "OK: porcelain fail-closed selftest"
