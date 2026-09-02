#!/usr/bin/env bash
# Isolated arm CLI verification helper for this source checkout.
# Never points --repo at the source tree. Never deletes evidence/.
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
SKILL_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
SOURCE_ROOT=$(git -C "$SKILL_DIR" rev-parse --show-toplevel)
# Not an override: launch always rebuilds and drives <source>/bin/arm, and the
# verification doctor fails if ARM_BIN is anything else. An inherited value is
# recorded here only so `doctor`/`drive` can report it before launch runs.
ARM_BIN_INHERITED=${ARM_BIN:-}
ARM_BIN=${ARM_BIN:-"$SOURCE_ROOT/bin/arm"}
EVIDENCE_ROOT="$SKILL_DIR/evidence"
# Per-checkout pointer: a bare /tmp/arm-verify-current is shared by every
# session on the host, so a second launch would silently retarget the first
# session's doctor/drive/cleanup and orphan its temp repo. cksum keeps this to
# POSIX tools. Two sessions in the SAME checkout are caught by the live-pointer
# guard in cmd_launch; pass ARM_VERIFY_CURRENT to run them in parallel.
CURRENT_RUN_FILE=${ARM_VERIFY_CURRENT:-/tmp/arm-verify-current-$(printf '%s' "$SOURCE_ROOT" | cksum | awk '{print $1}')}
export GIT_CONFIG_NOSYSTEM=1
export GIT_TERMINAL_PROMPT=0

die() {
  printf 'arm-verify: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
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
  ARM_VERIFY_CURRENT  Path to the current-run pointer file
                      (default: /tmp/arm-verify-current-<checkout hash>)
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
  local env_file live
  env_file=$(cat "$CURRENT_RUN_FILE")
  if [ -f "$env_file" ]; then
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
  printf '%s\n' "$RUN_ENV" >"$CURRENT_RUN_FILE"
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
  assert_no_live_run

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
  RUN_ENV=$(mktemp /tmp/arm-verify-run.XXXXXX.env)

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
  [ -d "$TARGET_REPO/.armature" ] || die ".armature missing"
  [ -d "$TARGET_REPO/.armature/ops" ] || die ".armature/ops missing"
  [ -f "$TARGET_REPO/.armature/config.json" ] || die ".armature/config.json missing"

  # Parse the captured git evidence rather than merely recording it: a plain
  # .armature directory, a wrong branch binding, or a missing ops-worktree
  # config must each fail the git-native bootstrap proof.
  local ops_line ops_cfg
  ops_line=$(awk '$1 ~ /\/\.armature$/ { print; exit }' "$EVIDENCE_DIR/drive/04-git-worktree/stdout.txt")
  [ -n "$ops_line" ] || die ".armature is not a registered git worktree (see drive/04-git-worktree)"
  case "$ops_line" in
    *"[_armature]"*) ;;
    *) die ".armature worktree is not bound to _armature: $ops_line" ;;
  esac
  ops_cfg=$(awk '$1 == "armature.ops-worktree-path" { print $2; exit }' "$EVIDENCE_DIR/drive/05-git-config/stdout.txt")
  [ -n "$ops_cfg" ] || die "git config armature.ops-worktree-path is unset (see drive/05-git-config)"
  case "$ops_cfg" in
    */.armature | .armature) ;;
    *) die "armature.ops-worktree-path does not point at .armature: $ops_cfg" ;;
  esac
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
  capture drive/04-check-after arm worker-init --check
  assert_exit_0 drive/04-check-after
  capture drive/05-git-config git -C "$TARGET_REPO" config --get armature.worker-id
  assert_exit_0 drive/05-git-config
  # Second read only: unflagged worker-init would rotate the UUID.
  capture drive/06-check-again arm worker-init --check
  assert_exit_0 drive/06-check-again
  local created after again cfg
  created=$(tr -d '\r' <"$EVIDENCE_DIR/drive/03-worker-init/stdout.txt")
  after=$(tr -d '\r' <"$EVIDENCE_DIR/drive/04-check-after/stdout.txt")
  again=$(tr -d '\r' <"$EVIDENCE_DIR/drive/06-check-again/stdout.txt")
  cfg=$(tr -d '\r' <"$EVIDENCE_DIR/drive/05-git-config/stdout.txt")
  [ -n "$cfg" ] || die "worker-init did not write armature.worker-id"
  case "$created" in
    *"$cfg"*) ;;
    *) die "worker-init output does not mention the id it wrote ($cfg): $created" ;;
  esac
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
  capture drive/05-log "$ARM_BIN" --repo "$TARGET_REPO" log --json
  mkdir -p "$EVIDENCE_DIR/drive/06-ops"
  if ls "$TARGET_REPO/.armature/ops/"*.log >/dev/null 2>&1; then
    cat "$TARGET_REPO/.armature/ops/"*.log >"$EVIDENCE_DIR/drive/06-ops/ops.log"
  fi
  cp -a "$TARGET_REPO/.armature/ops/SCHEMA" "$EVIDENCE_DIR/drive/06-ops/SCHEMA" 2>/dev/null || true
  capture drive/07-git-worktree git -C "$TARGET_REPO" worktree list
  jq_assert "$EVIDENCE_DIR/drive/01-create/stdout.txt" \
    '. == {"id": "TASK-VERIFY-CREATE", "status": "created"}' "create did not return the expected object"
  jq_assert "$EVIDENCE_DIR/drive/02-list/stdout.txt" \
    'any(.[]; .id == "TASK-VERIFY-CREATE")' "list does not contain TASK-VERIFY-CREATE"
  jq_assert "$EVIDENCE_DIR/drive/03-show/stdout.txt" \
    '.id == "TASK-VERIFY-CREATE" and .status == "open" and .title == "Verification create+list"' \
    "show returned unexpected id/status/title"
  assert_exit_0 drive/05-log
  [ -f "$EVIDENCE_DIR/drive/06-ops/ops.log" ] || die "no ops log captured under drive/06-ops"
  # On disk each op is the positional array [op_type, target_id, ...] documented
  # in .armature/ops/SCHEMA; `arm log --json` materializes the same ops as
  # objects. Assert both so a durable append and its read path are each proven.
  jq_assert_slurp "$EVIDENCE_DIR/drive/06-ops/ops.log" \
    'any(.[]; .[0] == "create" and .[1] == "TASK-VERIFY-CREATE")' \
    "ops log has no create op for TASK-VERIFY-CREATE"
  jq_assert_slurp "$EVIDENCE_DIR/drive/05-log/stdout.txt" \
    'any(.[]; .type == "create" and .target_id == "TASK-VERIFY-CREATE")' \
    "arm log --json shows no create op for TASK-VERIFY-CREATE"
  printf 'create-list proof ok\n'
  printf 'proof: create TASK-VERIFY-CREATE then list/show/ops log\n' >"$EVIDENCE_DIR/drive/SUMMARY.txt"
}

drive_doctor() {
  require_bootstrapped
  capture drive/01-doctor arm doctor
  assert_exit_0 drive/01-doctor
  capture drive/02-doctor-strict arm doctor --strict
  local code severities
  for code in D1 D2 D3 D4 D5 D6 D7 D8 D9 D10; do
    jq_assert "$EVIDENCE_DIR/drive/01-doctor/stdout.txt" \
      "any(.checks[]; .check == \"$code\")" "doctor report is missing check $code"
  done
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
  capture drive/06-claim arm claim --issue TASK-VERIFY-READY --worktree
  assert_exit_0 drive/06-claim
  capture drive/07-show arm_json show TASK-VERIFY-READY
  capture drive/08-worktree-list arm worktree list
  capture drive/09-git-worktree git -C "$TARGET_REPO" worktree list
  capture drive/10-log "$ARM_BIN" --repo "$TARGET_REPO" log --json
  assert_exit_0 drive/10-log
  mkdir -p "$EVIDENCE_DIR/drive/11-ops"
  if ls "$TARGET_REPO/.armature/ops/"*.log >/dev/null 2>&1; then
    cat "$TARGET_REPO/.armature/ops/"*.log >"$EVIDENCE_DIR/drive/11-ops/ops.log"
  fi
  assert_exit_0 drive/08-worktree-list
  assert_exit_0 drive/09-git-worktree
  jq_assert "$EVIDENCE_DIR/drive/05-ready/stdout.txt" \
    'any(.[]; .issue == "TASK-VERIFY-READY")' "ready queue omits TASK-VERIFY-READY"
  jq_assert "$EVIDENCE_DIR/drive/06-claim/stdout.txt" \
    '.issue == "TASK-VERIFY-READY" and has("claimed_by")' "claim did not report the issue and claimant"
  jq_assert "$EVIDENCE_DIR/drive/07-show/stdout.txt" \
    '.status == "claimed"' "issue status is not claimed after claim"
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
  jq_assert_slurp "$EVIDENCE_DIR/drive/11-ops/ops.log" \
    'any(.[]; .[0] == "claim" and .[1] == "TASK-VERIFY-READY")' \
    "ops log has no claim op for TASK-VERIFY-READY"
  jq_assert_slurp "$EVIDENCE_DIR/drive/10-log/stdout.txt" \
    'any(.[]; .type == "claim" and .target_id == "TASK-VERIFY-READY")' \
    "arm log --json shows no claim op for TASK-VERIFY-READY"
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

  if [ -n "${ARM_VERIFY_RUN_ENV:-}" ]; then
    :
  else
    if [ -f "$CURRENT_RUN_FILE" ]; then
      env_file=$(cat "$CURRENT_RUN_FILE")
      rm -f "$env_file" "$CURRENT_RUN_FILE"
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
  exit "$status"
}

cmd_run() {
  local feature=${1:-create-list}
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
