---
name: verify-armature
description: Drive and verify the Armature `arm` CLI (git-native work orchestration, no server) from this checkout. Use when proving arm bootstrap, worker-init, create/list, doctor, ready/claim, ops-backup, or any isolated CLI behavior against a throwaway git repo.
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
./bin/arm version --format human
# arm version <git describe --tags --always --dirty>
./bin/arm version
# non-TTY, or --format agent|json:
# {"count":1,"versions":[{"version":"<same describe>"}],"help":["arm version reports the build identity of this binary"]}
```

`make build` writes `./bin/arm` with `-ldflags "-X main.Version=$(git describe --tags --always --dirty)"`. Ready = the file is executable and both version shapes exit 0. A non-TTY `arm version` with the default format is the envelope (`writeVersionOutput` treats non-interactive as structured unless `--format human` was set explicitly). On a TTY, default format stays the `arm version …` line.

**Isolated target repo** (never the source tree):

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh launch
```

That helper:

1. Runs `make -C <source> build`
2. Creates `/tmp/arm-verify-target.XXXXXX` (`git init`, user.name/email, empty commit, `main`)
3. Writes a run env file and a per-checkout pointer inside a private per-user state directory — `$XDG_RUNTIME_DIR/arm-verify` when set, else `${TMPDIR:-/tmp}/arm-verify-<uid>`, created 0700 and refused unless this user owns it — as `<state dir>/current-<checkout hash>` and `<state dir>/run.XXXXXX.env`. The run env is sourced, so it is refused if it is a symlink, not owned by this user, or group/world-writable. Override the pointer with `ARM_VERIFY_CURRENT`; `launch` refuses to overwrite a pointer whose target repo still exists, and holds `<pointer>.lock` (a symlink naming the owning pid) while mutating it.
4. Records launch metadata under `.agents/skills/verify-armature/evidence/<run-id>/launch/`

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

Leave `ARM_LOG_SLOT` **unset** unless you are explicitly testing parallel writers on one clone. The in-tree e2e harness strips that env var because `arm dag apply` writes `$workerID.log` and ignores the slot (see `internal/e2e/harness/harness.go`). Two verification instances = two temp repos, not two processes on this checkout.

Do **not** pass `--global` to bootstrap (it writes `~/.claude/`). Do **not** run `arm tui` / `arm dag summary` as the default drive — they are interactive TUIs. `arm ready` also opens a TUI on a TTY; always pass `--format agent --non-interactive`.

Teardown is **Cleanup**, not a server stop. There is no PID to keep alive.

## Doctor

This section is **not** the product command `arm doctor`. It answers: is *this verification instance* worth driving?

Run:

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh doctor
```

Checks (all read-only):

| Check | Pass |
| --- | --- |
| Binary we built | `ARM_BIN` is `<source>/bin/arm` and executable — not some other `arm` on `PATH` |
| Version | non-TTY `$ARM_BIN version` envelope `versions[0].version` equals `git -C <source> describe --tags --always --dirty`; `$ARM_BIN version --format human` prints `arm version <that>` |
| Isolated `--repo` | realpath(target) ≠ realpath(source); target matches `/tmp/arm-verify-target.*` |
| Git | target is a git repo |

The helper also **invokes** product `arm doctor --repo "$TARGET" --format agent --non-interactive` and records stdout/exit. Interpret that separately:

- Unbootstrapped repo: exit 1, stdout one Command Failure `{"error":{"code":"DOCTOR-1","cause":"armature.ops-worktree-path must be set: ...","next_actions":["arm bootstrap","arm worker-init"],"exit_code":1}}`. `GENERAL-1` is retired. Expected before the bootstrap feature.
- Bootstrapped repo: exit 0 when no check is `error`. Stdout is one envelope `{count, checks, help}` whose `checks` are exactly D1–D10 then D12 (D11 is reserved for TOPTIER-S12-T2 and is not emitted).
- Product `arm doctor --strict` promotes warnings to a nonzero exit. A D6 warning on uncited issues still exits 0 without `--strict`. The report stays on stdout; stderr is `doctor --strict: N warning(s) promoted to errors`.

Do not treat a green product doctor as proof that `--repo` is isolated. The table above is the isolation proof.

## Drive

Prefer the in-tree Go harness when you need the **full lifecycle** (bootstrap → worker-init → plan/create → claim → in-progress → done → merge/sync). From the **source** checkout only:

```bash
make test-e2eharness
```

That target builds `./bin/arm` and runs `ARM_BIN=$(pwd)/bin/arm go test -v -count=1 ./internal/e2e/harness/...`. It already creates bare origins + clones. Do not invent a browser harness. Do not treat `make test-e2eharness` as a way to drive the source working tree.

For a **single user path** mid-task, use the isolated repo + CLI. Global flags on every command:

```text
--repo <target> --format agent --non-interactive
```

`--format json` and `--format agent` are the same structured shape **when the command implements it**. What you will hit:

- `arm show`, `arm list`, `arm ready`, `arm worktree list`, `arm doctor`, and `arm ops-backup` print one Agent Output Contract envelope `{count, <payload>, help, ...adjuncts}`. `show`/`list`/`ready` use payload key `issues`. `doctor` uses `checks`. `worktree list` uses `worktrees` plus a `bound` adjunct. `ops-backup` uses `snapshots`, `verifications`, or `plans`.
- `arm show` is human only at the default `--format human`. `--format agent` is the envelope, same as `--format json`. `--field` stays one value per line and is not an envelope.
- `arm worker-init` always prints `Worker ID: <uuid>` (not JSON). `arm sources add` prints `added source <uuid> (<url>)` (not JSON).
- Empty `arm ready` is `{count:0, issues:[], help:[...]}` (exit 0). It is not `null` and not a bare array. Expired claims are an `expired_claims` adjunct on that stdout object, not a second JSON value on stderr.
- `arm create`, `arm claim`, and `arm dag transition` still print their own small objects (not the envelope).
- Failures: one JSON object on stdout `{"error":{"code":"<COMMAND>-1","cause":"...","next_actions":[...],"exit_code":N}}`. The code is the top-level command prefix (`CREATE-1`, `DOCTOR-1`, `OPS-BACKUP-1`), not retired `GENERAL-1`. Graph Findings (`arm validate`) and a doctor report are **not** that envelope. `doctor --strict` and `ops-backup restore-plan` on diverge print their protocol payload and exit nonzero without a second error envelope.

Helper:

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh drive create-list
.agents/skills/verify-armature/scripts/arm-verify.sh drive bootstrap
.agents/skills/verify-armature/scripts/arm-verify.sh drive worker-init
.agents/skills/verify-armature/scripts/arm-verify.sh drive doctor
.agents/skills/verify-armature/scripts/arm-verify.sh drive ready-claim
.agents/skills/verify-armature/scripts/arm-verify.sh drive ops-backup
```

One-shot (launch → doctor → drive → cleanup, evidence kept):

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh run create-list
```

### Command strings and observed shapes

Always prefix with `"$ARM" --repo "$TARGET" --format agent --non-interactive` except where noted.

**bootstrap** (works on an unbootstrapped repo; bypasses config resolve):

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive bootstrap
```

Stdout (pretty JSON): `{ "repo_setup": { "status": "initialized" | "already_initialized" }, "harness_setup": [ { "platform", "artifact", "status", "action", "note?" } ] }`. Default platform with verified artifacts is `claude` (local `.claude/`, not `--global`). Second run is idempotent (`already_initialized`). Refuses a dirty working tree. Side effects: git worktree at `$TARGET/.armature` on branch `_armature`; `git -C "$TARGET" config armature.ops-worktree-path`; worker UUID in `armature.worker-id` if none was set; `.armature/ops/`, `config.json`, `state/`.

**worker-init**:

```bash
"$ARM" --repo "$TARGET" worker-init --check || "$ARM" --repo "$TARGET" worker-init
```

Stdout: `Worker ID: <uuid>`. Durable handle: `git -C "$TARGET" config --get armature.worker-id` (worktree file `.git/config.worktree` once `extensions.worktreeConfig` is on). Bootstrap already calls `Init` when unset. **Without `--check`, `worker-init` always writes a new UUID** (overwrites). Use `--check` unless you intend to rotate identity. To clear it inside a temp repo, `git -C "$TARGET" config --worktree --unset armature.worker-id`.

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

Success stdout: `{"id":"TASK-VERIFY-CREATE","status":"created"}`. Missing scope/acceptance/definition_of_done → `CREATE-1` whose `cause` contains `cannot introduce Graph Finding` and `missing required field`; nothing is written. Overlapping scope with another non-terminal task is also refused. `--source <uuid-or-url>` is optional at create; without it the issue is uncited (`arm validate` errors, product doctor D6 warns, `arm dag transition` cannot promote).

**list**:

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive list
```

Stdout: `{count, issues, help}`. Each `issues[]` row is `{id, type, status, title}` only (no `claimed_by`). `count` equals `len(issues)`.

**show** (`--format json` and `--format agent` are the same envelope):

```bash
"$ARM" --repo "$TARGET" --format json --non-interactive show TASK-VERIFY-CREATE
"$ARM" --repo "$TARGET" show --issue TASK-VERIFY-CREATE --field status,title,type
```

The issue object is `issues[0]` and includes `id`, `title`, `type`, `status`, `scope`, `definition_of_done`, `acceptance`, and `claimed_by` when set. `--field` prints one value per line (human), useful as the second read.

**product doctor**:

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive doctor
```

Stdout: `{count, checks, help}`. Each `checks[]` row is `{check, severity, message, items?, verbose_items?}`. Live `check` ids, in order, are D1, D2, D3, D4, D5, D6, D7, D8, D9, D10, D12. `--fix --dry-run` prints planned claim-liveness remediations without writing ops (JSON `null` when the plan slice is empty; ops log byte size must not grow). `--fix` without dry-run **does** append ops.

**ready / claim**:

Draft issues (`confidence=draft` at birth) are excluded from the ready queue. Promote first:

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive dag transition --issue TASK-VERIFY-READY
# {"issue":"TASK-VERIFY-READY","promoted_to":"verified"}
"$ARM" --repo "$TARGET" --format agent --non-interactive ready
# {"count":1,"issues":[{"id":"TASK-VERIFY-READY","type":"task","title":"...","scope":["ready.go"]}],"help":[...]}
"$ARM" --repo "$TARGET" --format agent --non-interactive claim --issue TASK-VERIFY-READY --worktree
# {"claimed_by":"<uuid>","issue":"TASK-VERIFY-READY","ttl":60}
```

`claim` publishes `_armature`. Point `origin` at a local bare repo and `git -C "$TARGET" push origin _armature` before claiming. With no origin the command exits `CLAIM-1` (`publish _armature class=other`) and does not provision a worktree.

`--worktree` is required. Valueless `--worktree` provisions `$TARGET/.worktrees/<id>` on branch `task/<id>`. Confirm with `git -C "$TARGET" worktree list` and `"$ARM" --repo "$TARGET" --format agent --non-interactive worktree list` (envelope `{count, worktrees, help, bound:[...], ...}`; `bound` includes `TASK-VERIFY-READY`).

**ops-backup** (published `_armature` tip; needs a local `origin` that has `refs/heads/_armature`; does not require a prior `arm doctor`):

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive ops-backup snapshot --output "$BUNDLE"
# {"count":1,"snapshots":[{"status":"ok","tip":"<sha>","bundle":"<path>"}],"help":[...]}
"$ARM" --repo "$TARGET" --format agent --non-interactive ops-backup verify "$BUNDLE"
# {"count":1,"verifications":[{"status":"ok","bundle":"<path>","advertises":"refs/heads/_armature"}],"help":[...]}
"$ARM" --repo "$TARGET" --format agent --non-interactive ops-backup restore-plan --from "$BUNDLE"
# {"count":1,"plans":[{"disposition":"fast-forward","recovered":"<sha>","bundle":"<path>"}],"help":[...]}
```

`restore-plan` does not publish. `disposition` `diverge→_armature-recovered` exits 1 with the plan envelope and no second error object. Refresh/verify failures are `OPS-BACKUP-1`.

`dag transition` requires a strict-green `arm validate` of the graph (cited source, no E6, no scope overlap). Seed a filesystem source first:

```bash
"$ARM" --repo "$TARGET" sources add --url "$TARGET/README.md" --type filesystem --title "README"
# human: "added source <uuid> ($TARGET/README.md)"  (not JSON)
# Absolute path required: the filesystem provider resolves a relative URL
# against the process cwd, not --repo, so `README.md` would cite whichever
# checkout you happen to be standing in (or fail to sync).
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
.agents/skills/verify-armature/evidence/<run-id>/
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
2. Capture the action **and** a second read: command + exit + stdout, then `arm --repo "$TARGET" list` / `arm --repo "$TARGET" show --format json` / `arm --repo "$TARGET" log --json` / git files under `.armature/` / `git -C "$TARGET" worktree list`.
3. Verify side effects: ops log line for `create`/`claim`/`source-link`, `.armature/` worktree, `armature.worker-id`, `.worktrees/<id>` after claim.
4. No mocks. There is no network service to stub. Filesystem source URLs are real files in the temp repo.
5. If a dry-run flag exists, prove it skipped writes by comparing ops log size / `arm list` before and after — do not trust the flag name. Known dry-runs: `arm dag apply --plan P --dry-run`, `arm doctor --fix --dry-run`, `arm sync --dry-run`. `dag apply --dry-run` still **validates** the plan (unknown source → error) and must not create the issue.

## Cleanup

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh cleanup
```

The helper:

- Reads the current run env (`<state dir>/current-<checkout hash>` → `<state dir>/run.*.env`, where `<state dir>` is `$XDG_RUNTIME_DIR/arm-verify` or `${TMPDIR:-/tmp}/arm-verify-<uid>`)
- Refuses to delete unless `realpath(target)` matches `/tmp/arm-verify-target.*` and is not the source checkout
- `git -C "$TARGET" worktree remove --force` on extra worktrees (`.armature`, `.worktrees/*`), then `rm -rf` the temp repo
- Deletes only that run env / pointer
- Does **not** delete `.agents/skills/verify-armature/evidence/`
- Does not `pkill arm` or match by process name (there is no daemon)
- Does not touch `main`, merge, or force-push

If you launched by hand, remove only the `$TARGET` you created, including its worktrees:

```bash
git -C "$TARGET" worktree list --porcelain
# remove every worktree path except $TARGET itself, then:
rm -rf "$TARGET"
```

Confirm evidence still exists: `test -d .agents/skills/verify-armature/evidence/<run-id>` and `test -f .../launch/meta.txt`.

## Helpers

Executable helper (this is the harness named in the feature files):

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh launch
.agents/skills/verify-armature/scripts/arm-verify.sh doctor
.agents/skills/verify-armature/scripts/arm-verify.sh drive create-list
.agents/skills/verify-armature/scripts/arm-verify.sh cleanup
```

Or `.../arm-verify.sh run create-list` for the full loop. Feature names: `bootstrap`, `worker-init`, `create-list`, `doctor`, `ready-claim`, `ops-backup`.

The script is bash, `chmod +x`. It records evidence itself. Do not reverse-engineer flags — they match this file.

## Interview corrections (vs the notes you were given)

- Runtime for the **binary** is Go-built `arm` + git. **Building** this checkout uses `make build` (GNU make + Go). Installing to PATH writes `~/.local/bin/arm`; verification uses `./bin/arm`.
- `arm bootstrap` already registers `armature.worker-id` when missing. `arm worker-init` without `--check` **rotates** that id.
- `arm show --format agent` is the `{count, issues, help}` envelope, same as `--format json`. Human text is the default format. The issue body is `issues[0]`.
- `arm list` / `arm ready` structured output is that same envelope (ready empty → `count: 0`, `issues: []`). List rows are `{id, type, status, title}`. Ready rows use `id`, not `issue`.
- Task `create` is gated on E6 fields (`scope`, `acceptance`, `definition_of_done`). A refused create is `CREATE-1`, not `GENERAL-1`. Citation is required to **promote**, not to create.
- `claim --worktree` is mandatory; it provisions `.worktrees/<id>` on `task/<id>`.
- Product `arm doctor` emits D1–D10 and D12 on a **bootstrapped** repo. D11 is reserved and not live. Unbootstrapped → `DOCTOR-1` ops-worktree-path. Do not conflate with this skill's Doctor section.
- `arm ops-backup snapshot|verify|restore-plan` is the published-tip backup lever. It is not a doctor check.
- Repo-local skills live under `.agents/skills/`. This skill is `.agents/skills/verify-armature/`. Embedded workflow skills in `internal/skillsembed/skills/` are a different product.
- Secondary TUIs: `arm tui`, `arm dag summary`, and interactive `arm ready`. Drive with `--non-interactive`.
- Isolation is a disposable git repo + `--repo` (and `ARM_LOG_SLOT` only for parallel same-clone writers). No ports.
