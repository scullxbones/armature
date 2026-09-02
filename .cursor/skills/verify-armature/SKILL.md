---
name: verify-armature
description: Drive and verify the Armature `arm` CLI (git-native work orchestration, no server) from this checkout. Use when proving arm bootstrap, worker-init, create/list, doctor, ready/claim, or any isolated CLI behavior against a throwaway git repo.
---

# Verify Armature (`arm` CLI)

Armature is a git-native work orchestrator. The only user surface that matters here is the `arm` binary built from `./cmd/armature`. There is no server, daemon, port, or web UI. State lives in git (ops JSONL on the `_armature` branch, checked out at `.armature/`).

You are mid-task. Do not drive **this source checkout** as `--repo`. Do not rewrite `internal/skillsembed/skills/`. Do not merge, do not force-push, do not commit to `main`.

Read `features/README.md` for the mapped user paths. This file is the operating procedure.

## Launch

`arm` is a short-lived CLI. Launch means **build once**, then each drive uses its **own temp git repo**. Nothing stays listening.

From the source checkout (the armature git root):

```bash
make build
test -x ./bin/arm
./bin/arm version    # expect: arm version <git describe --tags --always --dirty>; exit 0
```

`make build` writes `./bin/arm` with `-ldflags "-X main.Version=$(git describe --tags --always --dirty)"`. Ready = the file is executable and `arm version` exits 0.

**Isolated target repo** (never the source tree):

```bash
.cursor/skills/verify-armature/scripts/arm-verify.sh launch
```

That helper:

1. Runs `make -C <source> build`
2. Creates `/tmp/arm-verify-target.XXXXXX` (`git init`, user.name/email, empty commit, `main`)
3. Writes a run env file and pointer `/tmp/arm-verify-current`
4. Records launch metadata under `.cursor/skills/verify-armature/evidence/<run-id>/launch/`

Manual equivalent if you cannot use the helper:

```bash
SOURCE=$(git rev-parse --show-toplevel)
make -C "$SOURCE" build
TARGET=$(mktemp -d /tmp/arm-verify-target.XXXXXX)
git init -q "$TARGET"
git -C "$TARGET" config user.email "arm-verify@example.com"
git -C "$TARGET" config user.name "arm-verify"
git -C "$TARGET" config commit.gpgsign false
git -C "$TARGET" commit --allow-empty -q -m "init"
git -C "$TARGET" branch -M main
ARM="$SOURCE/bin/arm"
```

Leave `ARM_LOG_SLOT` **unset** unless you are explicitly testing parallel writers on one clone. The in-tree e2e harness strips that env var because `arm dag apply` writes `$workerID.log` and ignores the slot (see `internal/e2eharness/harness.go`). Two verification instances = two temp repos, not two processes on this checkout.

Do **not** pass `--global` to bootstrap (it writes `~/.claude/`). Do **not** run `arm tui` / `arm dag summary` as the default drive — they are interactive TUIs. `arm ready` also opens a TUI on a TTY; always pass `--format agent --non-interactive`.

Teardown is **Cleanup**, not a server stop. There is no PID to keep alive.

## Doctor

This section is **not** the product command `arm doctor`. It answers: is *this verification instance* worth driving?

Run:

```bash
.cursor/skills/verify-armature/scripts/arm-verify.sh doctor
```

Checks (all read-only):

| Check | Pass |
| --- | --- |
| Binary we built | `ARM_BIN` is `<source>/bin/arm` and executable — not some other `arm` on `PATH` |
| Version | `$ARM_BIN version` equals `arm version $(git -C <source> describe --tags --always --dirty)` |
| Isolated `--repo` | realpath(target) ≠ realpath(source); target matches `/tmp/arm-verify-target.*` |
| Git | target is a git repo |

The helper also **invokes** product `arm doctor --repo "$TARGET" --format agent --non-interactive` and records stdout/exit. Interpret that separately:

- Unbootstrapped repo: exit 1, stdout `{"error":{"code":"GENERAL-1","cause":"armature.ops-worktree-path must be set: ...","next_actions":[],"exit_code":1}}`. Expected before the bootstrap feature.
- Bootstrapped empty repo: exit 0, JSON `{ "checks": [ { "check":"D1", "severity":"ok", ... }, ... D10 ] }`.
- Product `arm doctor --strict` promotes D* warnings to errors. A D6 warning on uncited issues still exits 0 without `--strict`.

Do not treat a green product doctor as proof that `--repo` is isolated. The table above is the isolation proof.

## Drive

Prefer the in-tree Go harness when you need the **full lifecycle** (bootstrap → worker-init → plan/create → claim → in-progress → done → merge/sync). From the **source** checkout only:

```bash
make test-e2eharness
```

That target builds `./bin/arm` and runs `ARM_BIN=$(pwd)/bin/arm go test -v -count=1 ./internal/e2eharness/...`. It already creates bare origins + clones. Do not invent a browser harness. Do not treat e2eharness as a way to drive the source working tree.

For a **single user path** mid-task, use the isolated repo + CLI. Global flags on every command:

```text
--repo <target> --format agent --non-interactive
```

`--format json` and `--format agent` are the same envelope **when the command implements structured output**. Exceptions you will hit:

- `arm show` is human unless `--format json` (agent does **not** switch `show` to JSON).
- `arm worker-init` always prints `Worker ID: <uuid>` (not JSON).
- Empty `arm ready` prints `null` (JSON null), not `[]` and not `{count, issues, help[]}`.
- Failures: one JSON object on stdout `{"error":{"code":"...","cause":"...","next_actions":[...],"exit_code":N}}` (see `docs/error-contract.md`). Graph Findings (`arm validate`) and product doctor reports are **not** that envelope.

Helper:

```bash
.cursor/skills/verify-armature/scripts/arm-verify.sh drive create-list
.cursor/skills/verify-armature/scripts/arm-verify.sh drive bootstrap
.cursor/skills/verify-armature/scripts/arm-verify.sh drive worker-init
.cursor/skills/verify-armature/scripts/arm-verify.sh drive doctor
.cursor/skills/verify-armature/scripts/arm-verify.sh drive ready-claim
```

One-shot (launch → doctor → drive → cleanup, evidence kept):

```bash
.cursor/skills/verify-armature/scripts/arm-verify.sh run create-list
```

### Command strings and observed shapes

Always prefix with `"$ARM" --repo "$TARGET" --format agent --non-interactive` except where noted.

**bootstrap** (works on an unbootstrapped repo; bypasses config resolve):

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive bootstrap
```

Stdout (pretty JSON): `{ "repo_setup": { "status": "initialized" | "already_initialized" }, "harness_setup": [ { "platform", "artifact", "status", "action", "note?" } ] }`. Default platform with verified artifacts is `claude` (local `.claude/`, not `--global`). Second run is idempotent (`already_initialized`). Refuses a dirty working tree. Side effects: git worktree at `$TARGET/.armature` on branch `_armature`; `git config armature.ops-worktree-path`; worker UUID in `armature.worker-id` if none was set; `.armature/ops/`, `config.json`, `state/`.

**worker-init**:

```bash
"$ARM" --repo "$TARGET" worker-init --check || "$ARM" --repo "$TARGET" worker-init
```

Stdout: `Worker ID: <uuid>`. Durable handle: `git -C "$TARGET" config --get armature.worker-id`. Bootstrap already calls `InitWorker` when unset. **Without `--check`, `worker-init` always writes a new UUID** (overwrites). Use `--check` unless you intend to rotate identity.

**create** (tasks must satisfy E6 or the write is refused as a Graph Finding):

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive create \
  --id TASK-VERIFY-CREATE \
  --title "Verification create+list" \
  --type task \
  --scope "verify-create.txt" \
  --dod "Issue is listed and showable" \
  --acceptance '[{"type":"test_passes"}]'
```

Success stdout: `{"id":"TASK-VERIFY-CREATE","status":"created"}`. Missing scope/acceptance/definition_of_done → `GENERAL-1` / `cannot introduce Graph Finding ... missing required field`. Overlapping scope with another non-terminal task is also refused. `--source <uuid-or-url>` is optional at create; without it the issue is uncited (`arm validate` errors, product doctor D6 warns, `arm dag transition` cannot promote).

**list**:

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive list
```

Stdout: a JSON **array** of `{id, type, status, title, claimed_by?}`. Not the ADR-0017 `{count, issues, help[]}` envelope (that contract is accepted; list has not fully migrated).

**show** (use json, not agent):

```bash
"$ARM" --repo "$TARGET" --format json --non-interactive show TASK-VERIFY-CREATE
"$ARM" --repo "$TARGET" show --issue TASK-VERIFY-CREATE --field status,title,type
```

JSON object includes `id`, `title`, `type`, `status`, `scope`, `definition_of_done`, `acceptance`, `claimed_by` when set. `--field` prints one value per line (human), useful as the second read.

**product doctor**:

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive doctor
```

Stdout: `{ "checks": [ { "check":"D1"|"D2"|...|"D10", "severity":"ok"|"warning"|"error", "message":"...", "items"? } ] }`. `--fix --dry-run` prints planned claim-liveness remediations without writing ops (observed `null` when there is nothing to fix; ops log byte size must not grow). `--fix` without dry-run **does** append ops.

**ready / claim**:

Draft issues (`confidence=draft` at birth) are excluded from the ready queue. Promote first:

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive dag transition --issue TASK-VERIFY-READY
# {"issue":"TASK-VERIFY-READY","promoted_to":"verified"}
"$ARM" --repo "$TARGET" --format agent --non-interactive ready
# [{"issue":"TASK-VERIFY-READY","type":"task","title":"...","scope":["ready.go"]}]
"$ARM" --repo "$TARGET" --format agent --non-interactive claim --issue TASK-VERIFY-READY --worktree
# {"claimed_by":"<uuid>","issue":"TASK-VERIFY-READY","ttl":60}
```

`--worktree` is required. Valueless `--worktree` provisions `$TARGET/.worktrees/<id>` on branch `task/<id>`. Confirm with `git -C "$TARGET" worktree list` and `"$ARM" --repo "$TARGET" --format agent --non-interactive worktree list` (`{"bound":["TASK-VERIFY-READY"], ...}`).

`dag transition` requires a strict-green `arm validate` of the graph (cited source, no E6, no scope overlap). Seed a filesystem source first:

```bash
"$ARM" --repo "$TARGET" sources add --url README.md --type filesystem --title "README"
# human: "added source <uuid> (README.md)"  (not JSON)
"$ARM" --repo "$TARGET" sources sync
"$ARM" --repo "$TARGET" sources verify
```

There is **no** `sources list` subcommand.

**log / ops files** (side-effect second read):

```bash
"$ARM" --repo "$TARGET" log --json     # JSONL ops
ls "$TARGET/.armature/ops/"*.log
```

Ops filename is `<worker-uuid>.log`, or `<worker-uuid>~<slot>.log` if `ARM_LOG_SLOT` is set.

## Evidence

Root (cleanup must not delete this tree):

```text
.cursor/skills/verify-armature/evidence/<run-id>/
```

The helper writes:

| Path | What |
| --- | --- |
| `launch/meta.txt` | source, binary, version, target path, run id |
| `launch/arm-version.txt` | `arm version` stdout |
| `doctor/verification.txt` | isolation/binary/version verdict |
| `doctor/product-doctor.stdout.txt` + `exit.txt` | product `arm doctor` |
| `drive/feature.txt` | which mapped feature ran |
| `drive/<step>/{cmd,exit,stdout,stderr,combined}.txt` | each `arm`/`git` invocation |
| `drive/06-ops/ops.log` | copied worker JSONL (create-list) |
| `drive/SUMMARY.txt` | one-line proof claim |
| `cleanup/meta.txt` | target removed; evidence kept |

Proof standard:

1. Exercise the real user path (`arm <command>`), not Go test helpers, as the **sole** proof. `make test-e2eharness` is allowed as additional lifecycle evidence, not a substitute for one mapped feature in this skill.
2. Capture the action **and** a second read: command + exit + stdout, then `arm list` / `arm show --format json` / `arm log --json` / git files under `.armature/` / `git worktree list`.
3. Verify side effects: ops log line for `create`/`claim`/`source-link`, `.armature/` worktree, `armature.worker-id`, `.worktrees/<id>` after claim.
4. No mocks. There is no network service to stub. Filesystem source URLs are real files in the temp repo.
5. If a dry-run flag exists, prove it skipped writes by comparing ops log size / `arm list` before and after — do not trust the flag name. Known dry-runs: `arm dag apply --plan P --dry-run`, `arm doctor --fix --dry-run`, `arm sync --dry-run`. `dag apply --dry-run` still **validates** the plan (unknown source → error) and must not create the issue.

## Cleanup

```bash
.cursor/skills/verify-armature/scripts/arm-verify.sh cleanup
```

The helper:

- Reads the current run env (`/tmp/arm-verify-current` → `/tmp/arm-verify-run.*.env`)
- Refuses to delete unless `realpath(target)` matches `/tmp/arm-verify-target.*` and is not the source checkout
- `git worktree remove --force` on extra worktrees (`.armature`, `.worktrees/*`), then `rm -rf` the temp repo
- Deletes only that run env / pointer
- Does **not** delete `.cursor/skills/verify-armature/evidence/`
- Does not `pkill arm` or match by process name (there is no daemon)
- Does not touch `main`, merge, or force-push

If you launched by hand, remove only the `$TARGET` you created, including its worktrees:

```bash
git -C "$TARGET" worktree list --porcelain
# remove every worktree path except $TARGET itself, then:
rm -rf "$TARGET"
```

Confirm evidence still exists: `test -d .cursor/skills/verify-armature/evidence/<run-id>` and `test -f .../launch/meta.txt`.

## Helpers

Executable helper (this is the harness named in the feature files):

```bash
.cursor/skills/verify-armature/scripts/arm-verify.sh launch
.cursor/skills/verify-armature/scripts/arm-verify.sh doctor
.cursor/skills/verify-armature/scripts/arm-verify.sh drive create-list
.cursor/skills/verify-armature/scripts/arm-verify.sh cleanup
```

Or `.../arm-verify.sh run create-list` for the full loop. Feature names: `bootstrap`, `worker-init`, `create-list`, `doctor`, `ready-claim`.

The script is bash, `chmod +x`. It records evidence itself. Do not reverse-engineer flags — they match this file.

## Interview corrections (vs the notes you were given)

- Runtime for the **binary** is Go-built `arm` + git. **Building** this checkout uses `make build` (GNU make + Go). `make install` writes `~/.local/bin/arm`; verification uses `./bin/arm`.
- `arm bootstrap` already registers `armature.worker-id` when missing. `arm worker-init` without `--check` **rotates** that id.
- `arm show --format agent` is human text; use `--format json` for the issue object.
- `arm list` / `arm ready` structured output is a raw JSON array (ready empty → `null`), not the `{count, payload, help[]}` agent envelope.
- Task `create` is gated on E6 fields (`scope`, `acceptance`, `definition_of_done`). Citation is required to **promote**, not to create.
- `claim --worktree` is mandatory; it provisions `.worktrees/<id>` on `task/<id>`.
- Product `arm doctor` is D1–D10 on a **bootstrapped** repo. Unbootstrapped → `GENERAL-1` ops-worktree-path. Do not conflate with this skill's Doctor section.
- No `.cursor/skills/` tree existed before this skill. Embedded workflow skills in `internal/skillsembed/skills/` are a different product.
- Secondary TUIs: `arm tui`, `arm dag summary`, and interactive `arm ready`. Drive with `--non-interactive`.
- Isolation is a disposable git repo + `--repo` (and `ARM_LOG_SLOT` only for parallel same-clone writers). No ports.
