#!/usr/bin/env bash
# Isolated arm CLI verification helper for this source checkout.
# Never points --repo at the source tree. Never deletes evidence/.
set -euo pipefail

# Defined first: state_dir below runs at load time and calls die, so a later
# definition would leave every one of its guards a no-op.
die() {
  printf 'arm-verify: %s\n' "$*" >&2
  exit 1
}

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
SKILL_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
SOURCE_ROOT=$(git -C "$SKILL_DIR" rev-parse --show-toplevel)
# Not an override: launch always rebuilds and drives <source>/bin/arm, and the
# verification doctor fails if ARM_BIN is anything else. An inherited value is
# recorded here only so `doctor`/`drive` can report it before launch runs.
ARM_BIN_INHERITED=${ARM_BIN:-}
ARM_BIN=${ARM_BIN:-"$SOURCE_ROOT/bin/arm"}
EVIDENCE_ROOT="$SKILL_DIR/evidence"
path_owner_uid() {
  # Empty when stat is unavailable, which makes callers fail closed.
  stat -c %u "$1" 2>/dev/null || stat -f %u "$1" 2>/dev/null || printf ''
}

parent_is_safe() {
  # Safe means: a real directory (not a symlink) that either belongs to this
  # user or is sticky, so other users cannot rename or replace entries we own.
  local p=$1 perms
  if [ -L "$p" ]; then
    return 1
  fi
  [ -d "$p" ] || return 1
  # Sticky restricts renames to an entry's owner -- but NOT the directory's own
  # owner, who may still rename or delete anything inside. So a sticky parent is
  # only trustworthy when we own it or root does (as with /tmp).
  if [ -k "$p" ]; then
    if [ -O "$p" ]; then
      return 0
    fi
    [ "$(path_owner_uid "$p")" = "0" ]
    return
  fi
  # Otherwise ownership alone is not enough -- a parent we own but that others
  # can write is a parent in which others can rename our leaf away.
  [ -O "$p" ] || return 1
  perms=$(ls -ld "$p" | cut -c1-10)
  case "$perms" in
    ?????w???? | ????????w?) return 1 ;;
  esac
  return 0
}

# State (run pointer, run env, lock) lives in a private per-user directory, not
# a predictable world-writable /tmp path: launch SOURCES the run env, so another
# account able to pre-create that path could run arbitrary code as this user.
state_dir() {
  local base
  # XDG_RUNTIME_DIR gets the same parent test as the fallback: existing is not
  # the same as safe, and a shared non-sticky value here would leave the leaf
  # swappable exactly as an unsafe TMPDIR would.
  if [ -n "${XDG_RUNTIME_DIR:-}" ] && parent_is_safe "$XDG_RUNTIME_DIR"; then
    base="$XDG_RUNTIME_DIR/arm-verify"
  else
    # The leaf's own 0700 mode does not stop another user from RENAMING it when
    # the parent is a shared non-sticky directory, which would let them swap a
    # directory in around our later checks. Require a parent we own or one with
    # the sticky bit (as /tmp has), and fall back to /tmp when TMPDIR fails that.
    local parent="${TMPDIR:-/tmp}"
    if ! parent_is_safe "$parent"; then
      parent=/tmp
      parent_is_safe "$parent" ||
        die "no safe parent for verification state (checked '${TMPDIR:-/tmp}' and /tmp); set XDG_RUNTIME_DIR"
    fi
    base="$parent/arm-verify-$(id -u)"
  fi
  # mkdir -p, chmod and test -O all FOLLOW symlinks, so a symlink planted at the
  # predictable fallback path would pass every check while its owner kept the
  # directory entry and could retarget it. Reject the symlink itself.
  if [ -L "$base" ]; then
    die "state directory $base is a symlink; refusing to use it"
  fi
  mkdir "$base" 2>/dev/null || true
  if [ -L "$base" ]; then
    die "state directory $base is a symlink; refusing to use it"
  fi
  [ -d "$base" ] || die "cannot create state directory $base"
  chmod 700 "$base" 2>/dev/null || true
  [ -O "$base" ] || die "state directory $base is not owned by this user; refusing to use it"
  local perms
  perms=$(ls -ld "$base" | cut -c1-10)
  case "$perms" in
    ?????w???? | ????????w?) die "state directory $base is group- or world-writable ($perms); refusing to use it" ;;
  esac
  printf '%s\n' "$base"
}

STATE_DIR=$(state_dir)
# Per-checkout pointer: one shared pointer would let a second launch silently
# retarget the first session's doctor/drive/cleanup and orphan its temp repo.
# cksum keeps this to POSIX tools. Two sessions in the SAME checkout are caught
# by the reservation in cmd_launch; pass ARM_VERIFY_CURRENT to run in parallel.
CURRENT_RUN_FILE=${ARM_VERIFY_CURRENT:-$STATE_DIR/current-$(printf '%s' "$SOURCE_ROOT" | cksum | awk '{print $1}')}
export GIT_CONFIG_NOSYSTEM=1
export GIT_TERMINAL_PROMPT=0

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

assert_safe_to_source() {
  # This file is sourced, so a file we do not own, or one others can write, is
  # arbitrary code execution as this user. Refuse rather than run it.
  local f=$1 perms
  if [ -L "$f" ]; then
    die "run env is a symlink, refusing to source it: $f"
  fi
  [ -f "$f" ] || die "run env is not a regular file: $f"
  [ -O "$f" ] || die "run env is not owned by this user, refusing to source it: $f"
  # ls -ld columns: 1 type, 2-4 user, 5-7 group, 8-10 other. Group write is
  # column 6, other write is column 9.
  perms=$(ls -ld "$f" | cut -c1-10)
  case "$perms" in
    ?????w???? | ????????w?) die "run env is group- or world-writable, refusing to source it: $f ($perms)" ;;
  esac
}

abs_path() {
  # Resolve to an absolute, symlink-free path with shell builtins only.
  # AGENTS.md reserves Python for a last resort, and realpath/readlink -f are
  # not guaranteed on every supported checkout.
  local p=$1 dir base
  if [ -d "$p" ]; then
    (CDPATH= cd -P -- "$p" && pwd)
    return
  fi
  dir=$(dirname -- "$p")
  base=$(basename -- "$p")
  printf '%s/%s\n' "$(CDPATH= cd -P -- "$dir" && pwd)" "$base"
}

jq_assert() {
  # jq_assert <file> <filter> <message>  — single JSON document
  jq -e "$2" "$1" >/dev/null 2>&1 || die "$3 (see $1)"
}

jq_assert_slurp() {
  # jq_assert_slurp <file> <filter> <message>  — JSONL, slurped into an array
  jq -se "$2" "$1" >/dev/null 2>&1 || die "$3 (see $1)"
}

usage() {
  cat <<'EOF'
Usage: arm-verify.sh <command> [args]

  launch              Build ./bin/arm and create an isolated temp git repo
  doctor              Read-only: is this verification instance worth driving?
  drive <feature>     Drive one mapped feature (bootstrap|worker-init|create-list|doctor|ready-claim)
  cleanup             Tear down only the temp repo/worktrees this run created
  run <feature>       launch → doctor → drive <feature> → cleanup (evidence kept)

Environment:
  ARM_VERIFY_CURRENT  Path to the current-run pointer file (default:
                      <XDG_RUNTIME_DIR or $TMPDIR>/arm-verify.../current-<hash>)
  ARM_VERIFY_RUN_ENV  Skip pointer file and load this run env directly

Evidence is written under:
  .agents/skills/verify-armature/evidence/<run-id>/
Cleanup never deletes that tree.
EOF
}

load_run() {
  local env_file=${ARM_VERIFY_RUN_ENV:-}
  if [ -z "$env_file" ]; then
    [ -f "$CURRENT_RUN_FILE" ] || die "no current run (run launch first); missing $CURRENT_RUN_FILE"
    env_file=$(cat "$CURRENT_RUN_FILE")
  fi
  [ -f "$env_file" ] || die "run env not found: $env_file"
  assert_safe_to_source "$env_file"
  RUN_ENV_FILE=$env_file
  # shellcheck disable=SC1090
  . "$env_file"
  [ -n "${RUN_ID:-}" ] || die "run env missing RUN_ID"
  [ -n "${TARGET_REPO:-}" ] || die "run env missing TARGET_REPO"
  [ -n "${EVIDENCE_DIR:-}" ] || die "run env missing EVIDENCE_DIR"
}

assert_no_live_run() {
  # Refuse to overwrite a pointer whose target repo still exists: that run's
  # later doctor/drive/cleanup would otherwise follow this pointer instead.
  [ -f "$CURRENT_RUN_FILE" ] || return 0
  local env_file live marker_pid
  env_file=$(cat "$CURRENT_RUN_FILE")
  # A reservation marker is an ACTIVE launch that has not yet written its run
  # env, not a stale pointer: deleting it would let a second launch re-reserve
  # and reopen the race the reservation exists to close. Only reclaim it when
  # the reserving process is gone (a crashed launch). kill -0 on a pid owned by
  # another user reports failure, which would reclaim early; verification runs
  # single-user, and pid reuse is likewise accepted as the lesser risk against
  # never reclaiming.
  case "$env_file" in
    "reserved by pid "*)
      marker_pid=$(printf '%s\n' "$env_file" | awk '{print $4}')
      if [ -n "$marker_pid" ] && kill -0 "$marker_pid" 2>/dev/null; then
        die "another launch (pid $marker_pid) is reserving $CURRENT_RUN_FILE.
Wait for it to finish, or set ARM_VERIFY_CURRENT=<other path> for a parallel session."
      fi
      rm -f "$CURRENT_RUN_FILE"
      return 0
      ;;
  esac
  if [ -f "$env_file" ]; then
    assert_safe_to_source "$env_file"
    live=$(
      # shellcheck disable=SC1090
      . "$env_file" >/dev/null 2>&1
      printf '%s' "${TARGET_REPO:-}"
    )
    if [ -n "$live" ] && [ -d "$live" ]; then
      die "a verification run is already active for this checkout (target $live).
Run 'arm-verify.sh cleanup' first, or set ARM_VERIFY_CURRENT=<other path> to launch a parallel session."
    fi
  fi
  # Stale pointer (target already gone): safe to replace.
  rm -f "$CURRENT_RUN_FILE"
}

POINTER_LOCK="$CURRENT_RUN_FILE.lock"

POINTER_LOCK_DEPTH=0

pointer_force_release() {
  # Used by the failure paths: drop the lock if this process owns it, whatever
  # the nesting depth. Without this, a failure inside the critical section would
  # leave the lock behind for every later run.
  POINTER_LOCK_DEPTH=0
  local owner
  owner=$(readlink "$POINTER_LOCK" 2>/dev/null || printf '')
  if [ "$owner" = "$$" ]; then
    rm -f "$POINTER_LOCK"
  fi
}

pointer_lock_acquire() {
  # Re-entrant for this process: cleanup paths call release_pointer_if_ours,
  # which takes the same lock, and a non-reentrant lock would make the process
  # wait on its own live pid and then abandon the lock.
  if [ "$POINTER_LOCK_DEPTH" -gt 0 ]; then
    POINTER_LOCK_DEPTH=$((POINTER_LOCK_DEPTH + 1))
    return 0
  fi
  # Every pointer mutation -- inspect, reclaim, reserve, release -- runs inside
  # this lock, because each of those is a read followed by a write and no
  # sequence of atomic single-file operations makes that pair atomic.
  # The lock is a symlink whose target IS the owning pid, so taking it and
  # publishing ownership are one atomic operation.
  #
  # There is deliberately no automatic takeover of a lock whose owner is gone.
  # Reclaiming is inspect-then-replace -- a read followed by a write -- and no
  # arrangement of atomic single-file operations makes that pair atomic, so
  # every version of it (rm, compare-and-swap, rename-and-restore) left a window
  # where two processes could enter the critical section. Since the section is
  # three file operations long, a holder dying inside it is vanishingly rare;
  # when it happens we fail closed and name the one command that fixes it,
  # rather than racing to guess.
  local waited=0 owner
  # Checked BEFORE the first attempt, not inside the loop: `ln -s x dir` links
  # INSIDE the directory and reports success, so a pre-symlink lock left by an
  # older version would silently hand out a lock that excludes nobody.
  if [ -d "$POINTER_LOCK" ] && [ ! -L "$POINTER_LOCK" ]; then
    die "$POINTER_LOCK is a directory left by an older version.
Remove it (rm -rf '$POINTER_LOCK') and retry."
  fi
  while ! ln -s "$$" "$POINTER_LOCK" 2>/dev/null; do
    owner=$(readlink "$POINTER_LOCK" 2>/dev/null || printf '')
    if [ -z "$owner" ] || ! kill -0 "$owner" 2>/dev/null; then
      die "$POINTER_LOCK is held by pid ${owner:-unknown}, which is gone (killed mid-run?).
Remove it (rm -f '$POINTER_LOCK') and retry."
    fi
    waited=$((waited + 1))
    if [ "$waited" -gt 100 ]; then
      die "timed out waiting for $POINTER_LOCK (held by pid $owner)"
    fi
    sleep 0.1
  done
  POINTER_LOCK_DEPTH=1
}

pointer_lock_release() {
  [ "$POINTER_LOCK_DEPTH" -gt 0 ] || return 0
  POINTER_LOCK_DEPTH=$((POINTER_LOCK_DEPTH - 1))
  [ "$POINTER_LOCK_DEPTH" -eq 0 ] || return 0
  rm -f "$POINTER_LOCK"
}

pointer_is_ours() {
  # The pointer belongs to this process while it holds the reservation, or once
  # it names the run env this process wrote/loaded. Anything else is a newer
  # launch that has already taken over, and must not be disturbed.
  [ -f "$CURRENT_RUN_FILE" ] || return 1
  local cur
  cur=$(cat "$CURRENT_RUN_FILE")
  case "$cur" in
    "reserved by pid $$ "*) return 0 ;;
  esac
  [ -n "${1:-}" ] && [ "$cur" = "$1" ]
}

launch_partial_cleanup() {
  local status=$?
  trap - EXIT
  # cmd_run's trap is only installed after launch returns, so a launch that
  # fails part-way (build error, git init failure) would otherwise strand its
  # reservation and any temp repo it had already created. Evidence is kept, as
  # on every other path.
  if [ -n "${TARGET_REPO:-}" ] && [ -d "$TARGET_REPO" ]; then
    case "$TARGET_REPO" in
      /tmp/arm-verify-target.*) rm -rf "$TARGET_REPO" ;;
    esac
  fi
  if [ -n "${RUN_ENV:-}" ]; then
    rm -f "$RUN_ENV"
  fi
  release_pointer_if_ours "${RUN_ENV:-}"
  pointer_force_release
  exit "$status"
}

reserve_run_pointer() {
  # Inspect, reclaim and reserve as one critical section: assert_no_live_run
  # deletes a stale pointer, and without the lock another launch could reserve
  # in that gap and have its reservation removed.
  pointer_lock_acquire
  trap 'pointer_lock_release' EXIT
  assert_no_live_run
  # write_run_env only lands at the END of launch, so a liveness check alone
  # leaves a window in which two launches from this checkout both proceed and
  # the loser's target is orphaned. noclobber makes this create-or-fail, so the
  # pointer is reserved before anything is built or created.
  if ! (
    set -C
    printf 'reserved by pid %s at %s\n' "$$" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$CURRENT_RUN_FILE"
  ) 2>/dev/null; then
    die "another launch reserved $CURRENT_RUN_FILE first.
Wait for it to finish (or run 'arm-verify.sh cleanup'), or set ARM_VERIFY_CURRENT=<other path> for a parallel session."
  fi
  pointer_lock_release
  trap - EXIT
}

release_pointer_if_ours() {
  # release_pointer_if_ours <run-env-path>
  # Ownership check and deletion must not straddle the lock: a launch could
  # reclaim the pointer between them and lose its reservation to our rm.
  pointer_lock_acquire
  if pointer_is_ours "${1:-}"; then
    rm -f "$CURRENT_RUN_FILE"
  fi
  pointer_lock_release
}

write_run_env() {
  umask 077
  cat >"$RUN_ENV" <<EOF
RUN_ID=$(printf '%q' "$RUN_ID")
SOURCE_ROOT=$(printf '%q' "$SOURCE_ROOT")
ARM_BIN=$(printf '%q' "$ARM_BIN")
TARGET_REPO=$(printf '%q' "$TARGET_REPO")
EVIDENCE_DIR=$(printf '%q' "$EVIDENCE_DIR")
EXPECTED_VERSION=$(printf '%q' "$EXPECTED_VERSION")
STARTED_AT=$(printf '%q' "$STARTED_AT")
EOF
  # Hand the pointer from reservation to run env under the lock, and only while
  # it is still OUR reservation: a bare truncate+write here is a window in which
  # another launch sees an empty pointer, calls it stale, and installs its own.
  pointer_lock_acquire
  if ! pointer_is_ours ""; then
    pointer_lock_release
    die "our reservation of $CURRENT_RUN_FILE was taken over by another launch; aborting"
  fi
  printf '%s\n' "$RUN_ENV" >"$CURRENT_RUN_FILE"
  pointer_lock_release
}

capture() {
  # capture <label> <cmd...>  — records command, exit, stdout, stderr under EVIDENCE_DIR
  local label=$1
  shift
  local dir="$EVIDENCE_DIR/$label"
  mkdir -p "$dir"
  printf '%s\n' "$*" >"$dir/cmd.txt"
  # Remember the caller's errexit state instead of restoring -e unconditionally:
  # require_worker deliberately turns it off so it can branch on a nonzero
  # `worker-init --check`, and re-enabling -e here would exit the shell before
  # that documented `--check || worker-init` recovery could run.
  local errexit=off
  case $- in
    *e*) errexit=on ;;
  esac
  set +e
  "$@" >"$dir/stdout.txt" 2>"$dir/stderr.txt"
  local ec=$?
  if [ "$errexit" = on ]; then
    set -e
  fi
  printf '%s\n' "$ec" >"$dir/exit.txt"
  {
    printf 'cmd: %s\n' "$*"
    printf 'exit: %s\n' "$ec"
    printf '%s\n' '--- stdout ---'
    cat "$dir/stdout.txt"
    printf '\n%s\n' '--- stderr ---'
    cat "$dir/stderr.txt"
    printf '\n'
  } >"$dir/combined.txt"
  return "$ec"
}

arm() {
  "$ARM_BIN" --repo "$TARGET_REPO" --format agent --non-interactive "$@"
}

arm_json() {
  "$ARM_BIN" --repo "$TARGET_REPO" --format json --non-interactive "$@"
}

cmd_launch() {
  need_cmd git
  need_cmd jq
  need_cmd make
  unset ARM_LOG_SLOT || true
  reserve_run_pointer
  trap launch_partial_cleanup EXIT

  if [ -n "$ARM_BIN_INHERITED" ] && [ "$ARM_BIN_INHERITED" != "$SOURCE_ROOT/bin/arm" ]; then
    printf 'arm-verify: ignoring ARM_BIN=%s; verification always drives the binary it just built at %s/bin/arm\n' \
      "$ARM_BIN_INHERITED" "$SOURCE_ROOT" >&2
  fi

  EXPECTED_VERSION=$(git -C "$SOURCE_ROOT" describe --tags --always --dirty 2>/dev/null || printf 'dev')
  make -C "$SOURCE_ROOT" build
  [ -x "$SOURCE_ROOT/bin/arm" ] || die "make build did not produce $SOURCE_ROOT/bin/arm"
  ARM_BIN="$SOURCE_ROOT/bin/arm"

  local ver_out
  ver_out=$("$ARM_BIN" version)
  case "$ver_out" in
    "arm version "*) ;;
    *) die "arm version did not print expected prefix: $ver_out" ;;
  esac
  "$ARM_BIN" version >/dev/null

  RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)-$(od -An -N3 -tx1 /dev/urandom | tr -d ' \n')
  STARTED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  EVIDENCE_DIR="$EVIDENCE_ROOT/$RUN_ID"
  mkdir -p "$EVIDENCE_DIR/launch"
  TARGET_REPO=$(mktemp -d /tmp/arm-verify-target.XXXXXX)
  RUN_ENV=$(mktemp "$STATE_DIR/run.XXXXXX.env")

  git init -q "$TARGET_REPO"
  git -C "$TARGET_REPO" config user.email "arm-verify@example.com"
  git -C "$TARGET_REPO" config user.name "arm-verify"
  git -C "$TARGET_REPO" config commit.gpgsign false
  git -C "$TARGET_REPO" config init.defaultBranch main
  git -C "$TARGET_REPO" commit --allow-empty -q -m "init"
  git -C "$TARGET_REPO" branch -M main

  write_run_env

  {
    printf 'run_id=%s\n' "$RUN_ID"
    printf 'source_root=%s\n' "$(abs_path "$SOURCE_ROOT")"
    printf 'arm_bin=%s\n' "$(abs_path "$ARM_BIN")"
    printf 'expected_version=%s\n' "$EXPECTED_VERSION"
    printf 'arm_version=%s\n' "$ver_out"
    printf 'target_repo=%s\n' "$(abs_path "$TARGET_REPO")"
    printf 'evidence_dir=%s\n' "$EVIDENCE_DIR"
    printf 'run_env=%s\n' "$RUN_ENV"
    printf 'started_at=%s\n' "$STARTED_AT"
  } | tee "$EVIDENCE_DIR/launch/meta.txt"

  git -C "$TARGET_REPO" rev-parse --show-toplevel >"$EVIDENCE_DIR/launch/target-toplevel.txt"
  git -C "$TARGET_REPO" worktree list >"$EVIDENCE_DIR/launch/worktree-list.txt"
  printf '%s\n' "$ver_out" >"$EVIDENCE_DIR/launch/arm-version.txt"
  trap - EXIT
}

cmd_doctor() {
  load_run
  unset ARM_LOG_SLOT || true
  mkdir -p "$EVIDENCE_DIR/doctor"
  local src target bin expected actual
  src=$(abs_path "$SOURCE_ROOT")
  target=$(abs_path "$TARGET_REPO")
  bin=$(abs_path "$ARM_BIN")
  expected=$EXPECTED_VERSION
  actual=$("$ARM_BIN" version)
  local fail=0
  local report="$EVIDENCE_DIR/doctor/verification.txt"

  {
    printf 'verification doctor\n'
    printf 'source_root=%s\n' "$src"
    printf 'target_repo=%s\n' "$target"
    printf 'arm_bin=%s\n' "$bin"
    printf 'expected_version=%s\n' "$expected"
    printf 'arm_version=%s\n' "$actual"
  } >"$report"

  if [ ! -x "$bin" ]; then
    printf 'FAIL: binary not executable: %s\n' "$bin" | tee -a "$report" >&2
    fail=1
  fi
  if [ "$bin" != "$(abs_path "$src/bin/arm")" ]; then
    printf 'FAIL: ARM_BIN is not the binary we built at %s/bin/arm (got %s)\n' "$src" "$bin" | tee -a "$report" >&2
    fail=1
  fi
  case "$actual" in
    "arm version $expected") printf 'PASS: version matches build (%s)\n' "$expected" >>"$report" ;;
    *)
      printf 'FAIL: version mismatch: got %s want arm version %s\n' "$actual" "$expected" | tee -a "$report" >&2
      fail=1
      ;;
  esac
  if [ "$target" = "$src" ]; then
    printf 'FAIL: --repo points at the source checkout (%s). Drive a temp repo.\n' "$src" | tee -a "$report" >&2
    fail=1
  fi
  case "$target" in
    /tmp/arm-verify-target.*) printf 'PASS: target is an isolated temp repo\n' >>"$report" ;;
    *)
      printf 'FAIL: target %s is not under /tmp/arm-verify-target.*\n' "$target" | tee -a "$report" >&2
      fail=1
      ;;
  esac
  # Ask git, not the filesystem: a leftover but corrupt .git would otherwise
  # PASS here and let the product doctor's nonzero exit be excused as
  # "expected before bootstrap", certifying an undrivable instance.
  if ! git -C "$target" rev-parse --show-toplevel \
    >"$EVIDENCE_DIR/doctor/target-toplevel.txt" \
    2>"$EVIDENCE_DIR/doctor/target-toplevel.stderr.txt"; then
    printf 'FAIL: git does not recognize target as a repo: %s\n' "$target" | tee -a "$report" >&2
    fail=1
  else
    printf 'PASS: target is a git repo (toplevel=%s)\n' "$(cat "$EVIDENCE_DIR/doctor/target-toplevel.txt")" >>"$report"
  fi

  # Product health check: distinct from this verification doctor.
  set +e
  "$ARM_BIN" --repo "$TARGET_REPO" --format agent --non-interactive doctor \
    >"$EVIDENCE_DIR/doctor/product-doctor.stdout.txt" \
    2>"$EVIDENCE_DIR/doctor/product-doctor.stderr.txt"
  local pec=$?
  set -e
  printf '%s\n' "$pec" >"$EVIDENCE_DIR/doctor/product-doctor.exit.txt"
  if [ "$pec" -eq 0 ]; then
    printf 'product arm doctor: exit 0 (bootstrapped repo health)\n' >>"$report"
  else
    printf 'product arm doctor: exit %s (expected before bootstrap; see product-doctor.stdout.txt)\n' "$pec" >>"$report"
  fi

  if [ "$fail" -ne 0 ]; then
    printf 'FAIL: verification doctor\n' >>"$report"
    cat "$report" >&2
    exit 1
  fi
  printf 'PASS: verification instance is worth driving\n' | tee -a "$report"
}

require_bootstrapped() {
  if [ ! -d "$TARGET_REPO/.armature" ]; then
    capture drive/pre-bootstrap arm bootstrap
  fi
}

require_worker() {
  set +e
  capture drive/pre-worker-check arm worker-init --check
  local ec=$?
  set -e
  if [ "$ec" -ne 0 ]; then
    capture drive/pre-worker-init arm worker-init
    capture drive/pre-worker-check2 arm worker-init --check
  fi
}

assert_exit_0() {
  local label=$1
  local ec
  ec=$(cat "$EVIDENCE_DIR/$label/exit.txt")
  [ "$ec" = "0" ] || die "$label exited $ec (see $EVIDENCE_DIR/$label/combined.txt)"
}

assert_uuid() {
  # Canonical 8-4-4-4-12 hex grammar. An alphabet-plus-length check accepts
  # nonsense like 36 hyphens, which would let an identity regression pass.
  local v=$1 label=$2
  case "$v" in
    [0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]-[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]) ;;
    *) die "$label is not a canonical UUID: '$v'" ;;
  esac
}

assert_exit_nonzero() {
  local label=$1
  local ec
  ec=$(cat "$EVIDENCE_DIR/$label/exit.txt")
  [ "$ec" != "0" ] || die "$label unexpectedly exited 0 (see $EVIDENCE_DIR/$label/combined.txt)"
}

cmd_drive() {
  local feature=${1:-}
  [ -n "$feature" ] || die "drive requires a feature name"
  need_cmd jq
  load_run
  unset ARM_LOG_SLOT || true
  mkdir -p "$EVIDENCE_DIR/drive"
  printf '%s\n' "$feature" >"$EVIDENCE_DIR/drive/feature.txt"
  case "$feature" in
    bootstrap) drive_bootstrap ;;
    worker-init) drive_worker_init ;;
    create-list) drive_create_list ;;
    doctor) drive_doctor ;;
    ready-claim) drive_ready_claim ;;
    *) die "unknown feature: $feature (bootstrap|worker-init|create-list|doctor|ready-claim)" ;;
  esac
  printf 'drove %s; evidence at %s\n' "$feature" "$EVIDENCE_DIR"
}

drive_bootstrap() {
  # Launch left an unbootstrapped git repo. Drive bootstrap itself.
  capture drive/01-bootstrap-before-git-worktree git -C "$TARGET_REPO" worktree list || true
  capture drive/02-bootstrap arm bootstrap
  assert_exit_0 drive/02-bootstrap
  # Read the identity between the two calls: reading only at the end cannot tell
  # "first bootstrap created it" from "second bootstrap created it", nor catch
  # the idempotent run rotating it.
  capture drive/02b-worker-id git -C "$TARGET_REPO" config --get armature.worker-id
  assert_exit_0 drive/02b-worker-id
  capture drive/03-bootstrap-idempotent arm bootstrap
  assert_exit_0 drive/03-bootstrap-idempotent
  capture drive/04-git-worktree git -C "$TARGET_REPO" worktree list
  capture drive/05-git-config git -C "$TARGET_REPO" config --get-regexp '^armature\.'
  {
    printf 'proof: bootstrap\n'
    printf 'stdout (agent JSON) in drive/02-bootstrap/stdout.txt — expect repo_setup.status=initialized then already_initialized\n'
    printf 'side effects: .armature/ worktree on _armature; git config armature.ops-worktree-path;\n'
    printf '  .claude/skills + .claude/plugins/armature deployed (harness_hook_config skipped)\n'
  } >"$EVIDENCE_DIR/drive/SUMMARY.txt"
  jq_assert "$EVIDENCE_DIR/drive/02-bootstrap/stdout.txt" \
    '.repo_setup.status == "initialized"' "bootstrap did not report status=initialized"
  jq_assert "$EVIDENCE_DIR/drive/03-bootstrap-idempotent/stdout.txt" \
    '.repo_setup.status == "already_initialized"' "second bootstrap did not report status=already_initialized"

  # Local harness deploy is part of the mapped bootstrap feature, so an
  # all-skipped or all-unsupported harness_setup must not pass as a proof.
  local first="$EVIDENCE_DIR/drive/02-bootstrap/stdout.txt"
  jq_assert "$first" 'has("harness_setup") and (.harness_setup | length) > 0' \
    "bootstrap reported no harness_setup results"
  jq_assert "$first" 'all(.harness_setup[]; .status == "ok" or .status == "skipped")' \
    "bootstrap reported a failing harness_setup result"
  jq_assert "$first" \
    'any(.harness_setup[]; .platform == "claude" and .artifact == "skills" and .status == "ok" and .action == "install")' \
    "bootstrap did not install claude skills"
  jq_assert "$first" \
    'any(.harness_setup[]; .platform == "claude" and .artifact == "plugin_metadata" and .status == "ok" and .action == "install")' \
    "bootstrap did not install claude plugin metadata"
  # Hooks are documented as skipped without --with-hooks, which this drive omits.
  jq_assert "$first" \
    'any(.harness_setup[]; .artifact == "harness_hook_config" and .status == "skipped")' \
    "bootstrap did not report harness_hook_config as skipped"
  [ -d "$TARGET_REPO/.claude/skills" ] || die ".claude/skills was not deployed"
  ls "$TARGET_REPO/.claude/skills/" >/dev/null 2>&1 || die ".claude/skills is unreadable"
  [ -n "$(ls -A "$TARGET_REPO/.claude/skills")" ] || die ".claude/skills was deployed empty"
  [ -d "$TARGET_REPO/.claude/plugins/armature" ] || die ".claude/plugins/armature was not deployed"
  # An empty plugin directory would satisfy a directory-only check while leaving
  # the harness unusable, so require the metadata file itself to be valid JSON
  # naming the plugin.
  [ -f "$TARGET_REPO/.claude/plugins/armature/plugin.json" ] ||
    die ".claude/plugins/armature/plugin.json was not deployed"
  jq_assert "$TARGET_REPO/.claude/plugins/armature/plugin.json" \
    '.name == "armature"' "deployed plugin.json is not valid metadata for the armature plugin"
  [ -d "$TARGET_REPO/.armature" ] || die ".armature missing"
  [ -d "$TARGET_REPO/.armature/ops" ] || die ".armature/ops missing"
  [ -f "$TARGET_REPO/.armature/config.json" ] || die ".armature/config.json missing"
  # Present in the worktree is not the same as committed: coordination state that
  # was never committed to _armature vanishes in every other clone.
  capture drive/06-config-on-branch git -C "$TARGET_REPO" show _armature:config.json
  assert_exit_0 drive/06-config-on-branch
  jq_assert "$EVIDENCE_DIR/drive/06-config-on-branch/stdout.txt" \
    'type == "object" and (.default_ttl | type) == "number"' \
    "config.json committed on _armature is not a decodable config object"

  # Parse the captured git evidence rather than merely recording it: a plain
  # .armature directory, a wrong branch binding, or a missing ops-worktree
  # config must each fail the git-native bootstrap proof.
  local want_ops ops_line ops_cfg ops_cfg_abs
  want_ops=$(abs_path "$TARGET_REPO")/.armature
  ops_line=$(awk -v want="$want_ops" '$1 == want { print; exit }' "$EVIDENCE_DIR/drive/04-git-worktree/stdout.txt")
  [ -n "$ops_line" ] || die "$want_ops is not a registered git worktree (see drive/04-git-worktree)"
  case "$ops_line" in
    *"[_armature]"*) ;;
    *) die ".armature worktree is not bound to _armature: $ops_line" ;;
  esac
  ops_cfg=$(awk '$1 == "armature.ops-worktree-path" { print $2; exit }' "$EVIDENCE_DIR/drive/05-git-config/stdout.txt")
  [ -n "$ops_cfg" ] || die "git config armature.ops-worktree-path is unset (see drive/05-git-config)"
  # Resolve and require equality with the registered worktree: a suffix match
  # would accept any other path that merely ends in .armature.
  case "$ops_cfg" in
    /*) ops_cfg_abs=$(abs_path "$ops_cfg" 2>/dev/null || printf '%s' "$ops_cfg") ;;
    *) ops_cfg_abs=$(abs_path "$TARGET_REPO/$ops_cfg" 2>/dev/null || printf '%s' "$ops_cfg") ;;
  esac
  [ "$ops_cfg_abs" = "$want_ops" ] || die "armature.ops-worktree-path is $ops_cfg (resolved $ops_cfg_abs), expected $want_ops"
  # features/bootstrap.md maps worker-identity creation as a bootstrap side
  # effect, so a bootstrap that stopped writing it must fail here.
  local first_worker boot_worker
  first_worker=$(tr -d '\r' <"$EVIDENCE_DIR/drive/02b-worker-id/stdout.txt")
  assert_uuid "$first_worker" "the worker id written by the first bootstrap"
  boot_worker=$(awk '$1 == "armature.worker-id" { print $2; exit }' "$EVIDENCE_DIR/drive/05-git-config/stdout.txt")
  [ "$boot_worker" = "$first_worker" ] ||
    die "the idempotent bootstrap changed armature.worker-id: $first_worker -> ${boot_worker:-unset}"
  printf 'bootstrap proof ok\n'
}

drive_worker_init() {
  require_bootstrapped
  # Bootstrap already writes armature.worker-id, so driving --check alone would
  # certify a regressed unflagged worker-init. Clear the identity first and walk
  # the documented `worker-init --check || worker-init` session idiom.
  capture drive/01-config-unset git -C "$TARGET_REPO" config --unset armature.worker-id || true
  capture drive/02-check-unset arm worker-init --check || true
  assert_exit_nonzero drive/02-check-unset
  capture drive/03-worker-init arm worker-init
  assert_exit_0 drive/03-worker-init
  # Read the config BEFORE the first --check: reading only afterwards cannot
  # catch a --check that rewrites the identity it is supposed to report.
  capture drive/03b-git-config git -C "$TARGET_REPO" config --get armature.worker-id
  assert_exit_0 drive/03b-git-config
  capture drive/04-check-after arm worker-init --check
  assert_exit_0 drive/04-check-after
  capture drive/05-git-config git -C "$TARGET_REPO" config --get armature.worker-id
  assert_exit_0 drive/05-git-config
  # Second read only: unflagged worker-init would rotate the UUID.
  capture drive/06-check-again arm worker-init --check
  assert_exit_0 drive/06-check-again
  local created after again cfg cfg_before
  created=$(tr -d '\r' <"$EVIDENCE_DIR/drive/03-worker-init/stdout.txt")
  cfg_before=$(tr -d '\r' <"$EVIDENCE_DIR/drive/03b-git-config/stdout.txt")
  after=$(tr -d '\r' <"$EVIDENCE_DIR/drive/04-check-after/stdout.txt")
  again=$(tr -d '\r' <"$EVIDENCE_DIR/drive/06-check-again/stdout.txt")
  cfg=$(tr -d '\r' <"$EVIDENCE_DIR/drive/05-git-config/stdout.txt")
  assert_uuid "$cfg_before" "the id written by worker-init"
  [ "$cfg" = "$cfg_before" ] ||
    die "worker-init --check changed armature.worker-id: $cfg_before -> ${cfg:-unset}"
  [ -n "$cfg" ] || die "worker-init did not write armature.worker-id"
  # The documented contract is exactly `Worker ID: <uuid>`; a substring check
  # would accept malformed output such as `created <uuid>`, and the exact checks
  # below only cover the separate --check branch.
  [ "$created" = "Worker ID: $cfg_before" ] ||
    die "worker-init did not print 'Worker ID: $cfg_before': $created"
  case "$after" in
    "Worker ID: $cfg") ;;
    *) die "worker-init --check disagrees with git config ($cfg): $after" ;;
  esac
  [ "$after" = "$again" ] || die "worker-init --check is not stable: '$after' vs '$again'"
  printf 'worker-init proof ok: %s\n' "$after"
  printf 'proof: --check fails when unset, worker-init creates the id, --check is then stable and matches git config\n' \
    >"$EVIDENCE_DIR/drive/SUMMARY.txt"
}

drive_create_list() {
  require_bootstrapped
  require_worker
  # Read the identity BEFORE create: capturing it only afterwards would let a
  # create that rotated armature.worker-id pass, since the new log filename and
  # both worker fields would agree with the replacement.
  capture drive/00-worker-id-before git -C "$TARGET_REPO" config --get armature.worker-id
  assert_exit_0 drive/00-worker-id-before
  capture drive/01-create arm create \
    --id TASK-VERIFY-CREATE \
    --title "Verification create+list" \
    --type task \
    --scope "verify-create.txt" \
    --dod "Issue is listed and showable" \
    --acceptance '[{"type":"test_passes","description":"list and show return the created id"}]'
  assert_exit_0 drive/01-create
  capture drive/02-list arm list
  assert_exit_0 drive/02-list
  capture drive/03-show arm_json show TASK-VERIFY-CREATE
  assert_exit_0 drive/03-show
  capture drive/04-show-fields "$ARM_BIN" --repo "$TARGET_REPO" show --issue TASK-VERIFY-CREATE --field status,title,type
  assert_exit_0 drive/04-show-fields
  # Exit 0 alone would accept empty, reordered or wrong values; --field is its
  # own output path, so assert the projection itself.
  local fields_out want_fields
  fields_out=$(tr -d '\r' <"$EVIDENCE_DIR/drive/04-show-fields/stdout.txt")
  want_fields=$(printf 'open\nVerification create+list\ntask')
  [ "$fields_out" = "$want_fields" ] ||
    die "show --field status,title,type projected unexpected values (see $EVIDENCE_DIR/drive/04-show-fields/stdout.txt)"
  # Materialized proof of draft-at-birth. `arm show` does not project
  # provenance.confidence (its keys are id/type/status/title/scope/
  # definition_of_done/acceptance), so assert the consequence instead: a draft
  # issue must not reach the ready queue. A create corrupted to `verified` on
  # replay would surface here even though both log assertions passed.
  capture drive/04b-ready arm ready
  assert_exit_0 drive/04b-ready
  jq_assert "$EVIDENCE_DIR/drive/04b-ready/stdout.txt" \
    '(. // []) | map(select(.issue == "TASK-VERIFY-CREATE")) | length == 0' \
    "draft TASK-VERIFY-CREATE reached the ready queue; materialized confidence is not draft"
  capture drive/05-log "$ARM_BIN" --repo "$TARGET_REPO" log --json
  mkdir -p "$EVIDENCE_DIR/drive/06-ops"
  if ls "$TARGET_REPO/.armature/ops/"*.log >/dev/null 2>&1; then
    cat "$TARGET_REPO/.armature/ops/"*.log >"$EVIDENCE_DIR/drive/06-ops/ops.log"
  fi
  cp -a "$TARGET_REPO/.armature/ops/SCHEMA" "$EVIDENCE_DIR/drive/06-ops/SCHEMA" 2>/dev/null || true
  capture drive/07-git-worktree git -C "$TARGET_REPO" worktree list
  capture drive/08-worker-id git -C "$TARGET_REPO" config --get armature.worker-id
  jq_assert "$EVIDENCE_DIR/drive/01-create/stdout.txt" \
    '. == {"id": "TASK-VERIFY-CREATE", "status": "created"}' "create did not return the expected object"
  # Assert the list projection itself, not just presence of the id: list is its
  # own output path and the structured show assertion cannot certify it.
  jq_assert "$EVIDENCE_DIR/drive/02-list/stdout.txt" \
    'any(.[]; .id == "TASK-VERIFY-CREATE" and .status == "open" and .type == "task" and .title == "Verification create+list")' \
    "list has no TASK-VERIFY-CREATE entry with the expected status/type/title"
  # The E6 fields matter as much as the identity ones: a task materialized
  # without scope, definition_of_done or acceptance fails validation and cannot
  # enter the ready flow, and nothing else in this drive would notice.
  jq_assert "$EVIDENCE_DIR/drive/03-show/stdout.txt" \
    '.id == "TASK-VERIFY-CREATE" and .status == "open" and .title == "Verification create+list"
     and .type == "task" and .scope == ["verify-create.txt"]
     and .definition_of_done == "Issue is listed and showable"
     and (.acceptance | length) == 1
     and .acceptance[0].type == "test_passes"
     and .acceptance[0].description == "list and show return the created id"' \
    "show returned unexpected id/status/title/type/scope/definition_of_done/acceptance"
  assert_exit_0 drive/05-log
  [ -f "$EVIDENCE_DIR/drive/06-ops/ops.log" ] || die "no ops log captured under drive/06-ops"
  # On disk each op is the positional array [op_type, target_id, ...] documented
  # in .armature/ops/SCHEMA; `arm log --json` materializes the same ops as
  # objects. Assert both so a durable append and its read path are each proven.
  # I3 also means exclusive per-worker log files, so assert the create in the
  # configured worker's OWN log with the positional worker field matching, and
  # require the materialized read to agree on worker_id. Concatenating every log
  # would pass even if the op landed under another worker or filename.
  assert_exit_0 drive/08-worker-id
  local worker_id worker_id_before
  worker_id=$(tr -d '\r' <"$EVIDENCE_DIR/drive/08-worker-id/stdout.txt")
  worker_id_before=$(tr -d '\r' <"$EVIDENCE_DIR/drive/00-worker-id-before/stdout.txt")
  assert_uuid "$worker_id_before" "armature.worker-id before create"
  [ "$worker_id" = "$worker_id_before" ] ||
    die "create changed armature.worker-id: $worker_id_before -> ${worker_id:-unset}"
  assert_uuid "$worker_id" "armature.worker-id"
  local worker_log="$TARGET_REPO/.armature/ops/$worker_id.log"
  [ -f "$worker_log" ] || die "no ops log for the configured worker at $worker_log"
  cp "$worker_log" "$EVIDENCE_DIR/drive/06-ops/worker.log"
  jq_assert_slurp "$EVIDENCE_DIR/drive/06-ops/worker.log" \
    "any(.[]; .[0] == \"create\" and .[1] == \"TASK-VERIFY-CREATE\" and .[3] == \"$worker_id\" and .[4].confidence == \"draft\")" \
    "the configured worker's own log has no draft-confidence create op for TASK-VERIFY-CREATE attributed to it"
  jq_assert_slurp "$EVIDENCE_DIR/drive/05-log/stdout.txt" \
    "any(.[]; .type == \"create\" and .target_id == \"TASK-VERIFY-CREATE\" and .worker_id == \"$worker_id\" and .payload.confidence == \"draft\")" \
    "arm log --json shows no draft-confidence create op for TASK-VERIFY-CREATE by $worker_id"
  printf 'create-list proof ok\n'
  printf 'proof: create TASK-VERIFY-CREATE then list/show/ops log\n' >"$EVIDENCE_DIR/drive/SUMMARY.txt"
}

drive_doctor() {
  require_bootstrapped
  require_worker
  # Seed a deterministic warning: an uncited issue is D6, which is warning
  # severity. On a pristine repo there is nothing for --strict to promote, so a
  # regression that ignored the flag would still exit 0 and be certified here.
  capture drive/00-create-uncited arm create \
    --id TASK-VERIFY-DOCTOR \
    --title "Verification doctor warning" \
    --type task \
    --scope "verify-doctor.txt" \
    --dod "Doctor reports an uncited-issue warning" \
    --acceptance '[{"type":"test_passes"}]'
  assert_exit_0 drive/00-create-uncited
  capture drive/01-doctor arm doctor
  assert_exit_0 drive/01-doctor
  capture drive/02-doctor-strict arm doctor --strict || true
  local code severities
  for code in D1 D2 D3 D4 D5 D6 D7 D8 D9 D10; do
    jq_assert "$EVIDENCE_DIR/drive/01-doctor/stdout.txt" \
      "any(.checks[]; .check == \"$code\")" "doctor report is missing check $code"
  done
  # features/doctor.md maps --strict as "warnings become failing", so prove both
  # halves: the default run reports the warning and still exits 0, the strict run
  # over the same state exits nonzero.
  jq_assert "$EVIDENCE_DIR/drive/01-doctor/stdout.txt" \
    'any(.checks[]; .check == "D6" and .severity == "warning")' \
    "default doctor did not report the seeded uncited-issue warning as D6"
  # A bare nonzero exit would also be satisfied by an unknown flag or a crash,
  # so require the strict run to still be a full D1-D10 report carrying the
  # seeded warning, plus the documented promotion diagnostic.
  assert_exit_nonzero drive/02-doctor-strict
  for code in D1 D2 D3 D4 D5 D6 D7 D8 D9 D10; do
    jq_assert "$EVIDENCE_DIR/drive/02-doctor-strict/stdout.txt" \
      "any(.checks[]; .check == \"$code\")" "strict doctor report is missing check $code"
  done
  jq_assert "$EVIDENCE_DIR/drive/02-doctor-strict/stdout.txt" \
    'any(.checks[]; .check == "D6" and .severity == "warning")' \
    "strict doctor did not carry the seeded uncited-issue warning"
  local strict_err
  strict_err=$(cat "$EVIDENCE_DIR/drive/02-doctor-strict/stderr.txt")
  case "$strict_err" in
    *"promoted to errors"*) ;;
    *) die "strict doctor failed without the warning-promotion diagnostic: $strict_err" ;;
  esac
  severities=$(jq -r '[.checks[] | "\(.check)=\(.severity)"] | sort | join(" ")' \
    "$EVIDENCE_DIR/drive/01-doctor/stdout.txt")
  printf 'doctor proof ok; severities: %s\n' "$severities"
  printf 'proof: product arm doctor JSON report with D1-D10\n' >"$EVIDENCE_DIR/drive/SUMMARY.txt"
}

drive_ready_claim() {
  require_bootstrapped
  require_worker
  printf '# verify source\n' >"$TARGET_REPO/README.md"
  printf 'package ready\n' >"$TARGET_REPO/ready.go"
  git -C "$TARGET_REPO" add README.md ready.go
  git -C "$TARGET_REPO" commit -q -m "docs: seed files for ready-claim"
  # Absolute URL: the filesystem provider resolves a relative source URL
  # against the process cwd, not --repo, so a relative README.md would cite
  # the caller's checkout (or fail) instead of this isolated target repo.
  capture drive/01-sources-add arm sources add --url "$TARGET_REPO/README.md" --type filesystem --title "README"
  assert_exit_0 drive/01-sources-add
  capture drive/02-sources-sync arm sources sync
  assert_exit_0 drive/02-sources-sync
  local src_id
  # `sources add` prints "added source <uuid> (<path>)" as text, not JSON.
  src_id=$(awk '{ for (i = 1; i <= NF; i++) if (length($i) == 36 && $i ~ /^[0-9a-f-]+$/) { print $i; exit } }' \
    "$EVIDENCE_DIR/drive/01-sources-add/stdout.txt")
  [ -n "$src_id" ] || die "could not parse source UUID from sources add"
  printf '%s\n' "$src_id" >"$EVIDENCE_DIR/drive/source-id.txt"
  capture drive/03-create arm create \
    --id TASK-VERIFY-READY \
    --title "Verification ready+claim" \
    --type task \
    --scope "ready.go" \
    --dod "Claimed with a managed worktree" \
    --acceptance '[{"type":"test_passes"}]' \
    --source "$src_id"
  assert_exit_0 drive/03-create
  capture drive/04-dag-transition arm dag transition --issue TASK-VERIFY-READY
  assert_exit_0 drive/04-dag-transition
  capture drive/05-ready arm ready
  assert_exit_0 drive/05-ready
  # Same reasoning as create-list: read the identity before the command under
  # test, so a claim that rotated it cannot satisfy every attribution check
  # against its own replacement.
  capture drive/05b-worker-id-before git -C "$TARGET_REPO" config --get armature.worker-id
  assert_exit_0 drive/05b-worker-id-before
  capture drive/06-claim arm claim --issue TASK-VERIFY-READY --worktree
  assert_exit_0 drive/06-claim
  capture drive/07-show arm_json show TASK-VERIFY-READY
  capture drive/08-worktree-list arm worktree list
  capture drive/09-git-worktree git -C "$TARGET_REPO" worktree list
  capture drive/10-log "$ARM_BIN" --repo "$TARGET_REPO" log --json
  assert_exit_0 drive/10-log
  capture drive/12-worker-id git -C "$TARGET_REPO" config --get armature.worker-id
  mkdir -p "$EVIDENCE_DIR/drive/11-ops"
  if ls "$TARGET_REPO/.armature/ops/"*.log >/dev/null 2>&1; then
    cat "$TARGET_REPO/.armature/ops/"*.log >"$EVIDENCE_DIR/drive/11-ops/ops.log"
  fi
  ls "$TARGET_REPO/.armature/ops/" >"$EVIDENCE_DIR/drive/11-ops/listing.txt" 2>&1 || true
  assert_exit_0 drive/08-worktree-list
  assert_exit_0 drive/09-git-worktree
  # Assert the ready projection itself: claim reads materialized state, so a
  # corrupt agent-facing ready entry would otherwise go uncaught.
  jq_assert "$EVIDENCE_DIR/drive/05-ready/stdout.txt" \
    'any(.[]; .issue == "TASK-VERIFY-READY" and .type == "task" and .title == "Verification ready+claim" and .scope == ["ready.go"])' \
    "ready queue has no TASK-VERIFY-READY entry with the expected type/title/scope"
  # ttl too: an omitted or 0 ttl (never expires) is broken claim-expiry
  # semantics that every other assertion here would miss. 60 is the documented
  # default for a fresh bootstrap, which this drive uses.
  jq_assert "$EVIDENCE_DIR/drive/06-claim/stdout.txt" \
    '.issue == "TASK-VERIFY-READY" and has("claimed_by") and .ttl == 60' \
    "claim did not report the issue, claimant and ttl=60"
  # has("claimed_by") alone would accept an empty or wrong claimant, so tie the
  # attribution to this repo's registered worker.
  assert_exit_0 drive/12-worker-id
  local worker_id claimed_by worker_id_before
  worker_id=$(tr -d '\r' <"$EVIDENCE_DIR/drive/12-worker-id/stdout.txt")
  worker_id_before=$(tr -d '\r' <"$EVIDENCE_DIR/drive/05b-worker-id-before/stdout.txt")
  assert_uuid "$worker_id_before" "armature.worker-id before claim"
  [ "$worker_id" = "$worker_id_before" ] ||
    die "claim changed armature.worker-id: $worker_id_before -> ${worker_id:-unset}"
  [ -n "$worker_id" ] || die "target has no armature.worker-id after claim"
  claimed_by=$(jq -r '.claimed_by // ""' "$EVIDENCE_DIR/drive/06-claim/stdout.txt")
  [ -n "$claimed_by" ] || die "claim reported an empty claimed_by"
  [ "$claimed_by" = "$worker_id" ] || die "claim attributed to $claimed_by, expected $worker_id"
  jq_assert "$EVIDENCE_DIR/drive/07-show/stdout.txt" \
    '.status == "claimed"' "issue status is not claimed after claim"
  # The materialized snapshot is a different read path from the op stream, so
  # assert the claimant there too; a replay that drops or corrupts the claim
  # payload would otherwise pass on status alone. (The snapshot carries no ttl
  # field, so ttl is asserted on the response and both log reads instead.)
  jq_assert "$EVIDENCE_DIR/drive/07-show/stdout.txt" \
    ".claimed_by == \"$worker_id\"" \
    "materialized issue is not attributed to the configured worker $worker_id"
  [ -d "$TARGET_REPO/.worktrees/TASK-VERIFY-READY" ] || die "managed worktree directory missing"

  # Inspect the captured inventories: a worktree on the wrong branch or with
  # no armature binding would otherwise pass every assertion above.
  jq_assert "$EVIDENCE_DIR/drive/08-worktree-list/stdout.txt" \
    'any(.bound[]?; . == "TASK-VERIFY-READY")' "arm worktree list does not report TASK-VERIFY-READY as bound"
  local wt_line
  wt_line=$(awk '$1 ~ /\/\.worktrees\/TASK-VERIFY-READY$/ { print; exit }' \
    "$EVIDENCE_DIR/drive/09-git-worktree/stdout.txt")
  [ -n "$wt_line" ] || die "git worktree list has no entry for .worktrees/TASK-VERIFY-READY"
  case "$wt_line" in
    *"[task/TASK-VERIFY-READY]"*) ;;
    *) die "claimed worktree is not on task/TASK-VERIFY-READY: $wt_line" ;;
  esac

  # Proof 6 from features/ready-claim.md: the claim must be durably appended to
  # the ops log, not merely reflected in command output and materialized state.
  [ -f "$EVIDENCE_DIR/drive/11-ops/ops.log" ] || die "no ops log captured under drive/11-ops"
  # I3 -- each worker writes exclusively to its own log file -- so it is not
  # enough that SOME log carries the claim: assert it in the claiming worker's
  # own file, with the positional worker field matching, and require the
  # materialized read to agree on worker_id too.
  assert_uuid "$worker_id" "armature.worker-id"
  local worker_log="$TARGET_REPO/.armature/ops/$worker_id.log"
  [ -f "$worker_log" ] || die "no ops log for the claiming worker at $worker_log (see drive/11-ops/listing.txt)"
  cp "$worker_log" "$EVIDENCE_DIR/drive/11-ops/worker.log"
  jq_assert_slurp "$EVIDENCE_DIR/drive/11-ops/worker.log" \
    "any(.[]; .[0] == \"claim\" and .[1] == \"TASK-VERIFY-READY\" and .[3] == \"$worker_id\" and .[4].ttl == 60)" \
    "the claiming worker's own log has no claim op for TASK-VERIFY-READY attributed to it with ttl=60"
  jq_assert_slurp "$EVIDENCE_DIR/drive/10-log/stdout.txt" \
    "any(.[]; .type == \"claim\" and .target_id == \"TASK-VERIFY-READY\" and .worker_id == \"$worker_id\" and .payload.ttl == 60)" \
    "arm log --json shows no claim op for TASK-VERIFY-READY by $worker_id with ttl=60"
  printf 'ready-claim proof ok\n'
  printf 'proof: ready queue then claim --worktree; status=claimed; .worktrees/TASK-VERIFY-READY on task/; claim op in the ops log\n' \
    >"$EVIDENCE_DIR/drive/SUMMARY.txt"
}

cmd_cleanup() {
  load_run
  mkdir -p "$EVIDENCE_DIR/cleanup"
  local target
  target=$(abs_path "$TARGET_REPO")
  case "$target" in
    /tmp/arm-verify-target.*) ;;
    *) die "refusing to delete unexpected target path: $target" ;;
  esac
  src=$(abs_path "$SOURCE_ROOT")
  if [ "$target" = "$src" ]; then
    die "refusing to delete source checkout"
  fi

  if [ -d "$TARGET_REPO" ]; then
    # Remove extra worktrees first, then the repo directory (includes .armature).
    while IFS= read -r wt; do
      [ -n "$wt" ] || continue
      [ "$wt" = "$target" ] && continue
      git -C "$TARGET_REPO" worktree remove --force "$wt" >/dev/null 2>&1 || rm -rf "$wt"
    done < <(git -C "$TARGET_REPO" worktree list --porcelain 2>/dev/null | awk '/^worktree /{print substr($0,10)}')
    rm -rf "$TARGET_REPO"
  fi

  {
    printf 'removed_target=%s\n' "$target"
    printf 'evidence_kept=%s\n' "$EVIDENCE_DIR"
    printf 'evidence_exists=%s\n' "$( [ -d "$EVIDENCE_DIR" ] && printf yes || printf no )"
  } | tee "$EVIDENCE_DIR/cleanup/meta.txt"

  if [ -z "${ARM_VERIFY_RUN_ENV:-}" ]; then
    # Only release the pointer if it still refers to THIS run: a same-checkout
    # launch racing this cleanup may already have taken it, and deleting its
    # reservation would let a third launch in and orphan a target.
    release_pointer_if_ours "${RUN_ENV_FILE:-}"
    if [ -n "${RUN_ENV_FILE:-}" ]; then
      rm -f "$RUN_ENV_FILE"
    fi
  fi

  [ -d "$EVIDENCE_DIR" ] || die "cleanup deleted evidence (bug)"
  [ -f "$EVIDENCE_DIR/launch/meta.txt" ] || die "evidence launch/meta.txt missing after cleanup"
  printf 'cleanup complete; evidence remains at %s\n' "$EVIDENCE_DIR"
}

cleanup_on_exit() {
  local status=$?
  trap - EXIT
  # Run in a subshell: cmd_cleanup may die(), and an exit from inside an EXIT
  # trap would mask the drive's real status.
  (cmd_cleanup) || printf 'arm-verify: cleanup failed after exit %s\n' "$status" >&2
  pointer_force_release
  exit "$status"
}

cmd_run() {
  local feature=${1:-create-list}
  # load_run prefers ARM_VERIFY_RUN_ENV, so with it set the launch below would
  # create one target while doctor/drive/cleanup operated on another -- cleanup
  # tearing down the override's run and orphaning the one just created.
  if [ -n "${ARM_VERIFY_RUN_ENV:-}" ]; then
    die "ARM_VERIFY_RUN_ENV is set; 'run' creates its own run and would then operate on the override.
Use the separate launch/doctor/drive/cleanup commands with that variable, or unset it."
  fi
  cmd_launch
  # doctor/drive exit nonzero exactly when this verifier catches a regression —
  # the case where leaving the temp repo, its worktrees, the run env and the
  # current-run pointer behind would orphan the target on the next launch.
  trap cleanup_on_exit EXIT
  cmd_doctor
  cmd_drive "$feature"
  trap - EXIT
  cmd_cleanup
}

main() {
  local cmd=${1:-}
  shift || true
  case "$cmd" in
    launch) cmd_launch "$@" ;;
    doctor) cmd_doctor "$@" ;;
    drive) cmd_drive "$@" ;;
    cleanup) cmd_cleanup "$@" ;;
    run) cmd_run "$@" ;;
    -h|--help|help|"") usage ;;
    *) die "unknown command: $cmd" ;;
  esac
}

main "$@"
