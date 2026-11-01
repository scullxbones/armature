# Ready queue and claim

`arm ready` lists issues that can be claimed now (open, blockers merged, parent not terminal, confidence not draft). `arm claim --worktree` binds the current worker to one of those issues and provisions a managed git worktree. This is the coordinator dispatch path.

## Sub-features

- Ready gate: type is ready-eligible (task/story/feature/bug), status `open`, no unmet blockers, not draft
- `--explain` diagnoses why open tasks are absent from the queue
- `--waves` partitions ready items by disjoint scope (json/agent only)
- `claim --worktree` required; omit the value for `.worktrees/<issue-id>` on branch `task/<id>` (or `fix/` / `feat/` by type)
- `--ttl` minutes (default 60); `--force` overrides scope-overlap warning
- `--from <parent-worktree-path>` creates the new task worktree from that parent worktree's current branch and tip. `--from` must already be a worktree of this repository (`is not an existing worktree of this repository`) and must be on a branch. It requires an explicit `--worktree <new-path>` destination, which must not already exist (`new worktree path … must not exist`).
- Claim stdout: `{"claimed_by":"<worker-uuid>","issue":"<id>","ttl":60}`

## How to get to it (user POV)

Issue must be **verified** and cited, then:

```bash
arm --repo "$TARGET" --format agent --non-interactive ready
arm --repo "$TARGET" --format agent --non-interactive claim --issue TASK-VERIFY-READY --worktree
```

On a TTY without `--non-interactive`, `arm ready` is a TUI. Always pass `--format agent --non-interactive` from this skill.

## Driving it with arm-verify.sh

Preconditions: launch + doctor passed. Helper bootstraps, ensures worker id, commits `README.md` + `ready.go`, `sources add`/`sync`, creates `TASK-VERIFY-READY` with `--source`, `dag transition --issue TASK-VERIFY-READY`, then adds a local bare `origin.git` and pushes `_armature`. `arm claim` publishes that branch; without a reachable `origin` it exits `CLAIM-1` (`publish _armature class=other`) and does not create the worktree.

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh drive ready-claim
```

Proof:

1. `ready` envelope `issues[]` contains `{"id":"TASK-VERIFY-READY",...}` (not a bare array, and the row key is `id`)
2. `claim --worktree` exit 0 with `claimed_by` set
3. `arm show --format json TASK-VERIFY-READY` has `status: claimed`
4. `git worktree list` includes `$TARGET/.worktrees/TASK-VERIFY-READY` on `task/TASK-VERIFY-READY`
5. `arm worktree list` envelope has `bound` containing `TASK-VERIFY-READY` (payload key `worktrees`, plus `count` and `help`)
6. ops log has a `claim` op (also visible via `arm log --json`)

Evidence: `evidence/<run-id>/drive/05-ready/` through `11-ops/`. Proof 6 is asserted twice: on the raw `.armature/ops/*.log` positional arrays (`11-ops/`) for durability, and on the materialized `arm log --json` objects (`10-log/`).

## Gotchas

- Fresh `create` is draft → `arm ready` prints `{count:0, issues:[], help:[...]}` (exit 0). Promote with `arm dag transition --issue <id>` after `arm validate` is green.
- `dag transition` fails on uncited nodes and other Graph Findings. Attach `--source` at create (or `sources link`) first.
- `claim` without `--worktree` is usage failure.
- `arm claim` publishes `_armature` before it returns. No `origin`, or an origin that is not a git repo, is `CLAIM-1` with `cause` containing `publish _armature class=other`. Low-stakes `create` and `dag transition` still succeed with no remote. The drive's bare `origin.git` lives inside the temp repo so cleanup removes it.
- `arm ready` always includes an `expired_claims` array on the envelope. It is `[]` when nothing has expired. Those rows are not also in `issues`.
- Claim also refuses on worktree-integrity grounds the happy path never shows: `--from` that `is not an existing worktree of this repository` (or is detached HEAD); an explicit `--worktree` destination that already exists; a worktree `already bound to` a **different** issue; an **unbound** detached HEAD; or an issue at `confidence=inferred` (which routes you to `arm confirm`). Re-claim of a worktree already bound to the **same** issue succeeds, including a detached HEAD mid-rebase (`checkExistingWorktreeBinding` in `cmd/armature/claim.go`). Do not pre-create the `--worktree` destination for `--from`.
- A **lost claim race** does not fail: it returns a different, wider object (`{issue, claimed:false, claimed_by, reason:"lost_claim_race", superseded_by_same_worker}`). Success has no `claimed` field (`{issue, claimed_by, ttl}`). Reject the explicit `claimed:false` / `reason:"lost_claim_race"` shape; do not treat a missing `claimed` as failure.
- The target repo's git hooks fire on the seed commit this drive makes; see [bootstrap.md](bootstrap.md). The helper puts the just-built `$SOURCE_ROOT/bin` first on `PATH` for that commit so hooks invoke this checkout's `arm`. With no active claim, `post-commit` is silent: the subject is unchanged and stderr does not contain `push-ops: push failed` or `not found`.
- Interactive `arm ready` can claim from the TUI; that is not the agent path.
- Expired claims are **not** in `issues`. Under `--format agent` or `json` they are an `expired_claims` adjunct on the same stdout envelope.
- Scope overlap with another claimed/in-progress task warns; `--force` to proceed.
- Cleanup must `git worktree remove` the claim worktree (the helper does this on `cleanup`).
