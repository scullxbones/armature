# Ready queue and claim

`arm ready` lists issues that can be claimed now (open, blockers merged, parent not terminal, confidence not draft). `arm claim --worktree` binds the current worker to one of those issues and provisions a managed git worktree. This is the coordinator dispatch path.

## Sub-features

- Ready gate: type is ready-eligible (task/story/feature/bug), status `open`, no unmet blockers, not draft
- `--explain` diagnoses why open tasks are absent from the queue
- `--waves` partitions ready items by disjoint scope (json/agent only)
- `claim --worktree` required; omit the value for `.worktrees/<issue-id>` on branch `task/<id>` (or `fix/` / `feat/` by type)
- `--ttl` minutes (default 60); `--force` overrides scope-overlap warning
- Claim stdout: `{"claimed_by":"<worker-uuid>","issue":"<id>","ttl":60}`

## How to get to it (user POV)

Issue must be **verified** and cited, then:

```bash
arm --repo "$TARGET" --format agent --non-interactive ready
arm --repo "$TARGET" --format agent --non-interactive claim --issue TASK-VERIFY-READY --worktree
```

On a TTY without `--non-interactive`, `arm ready` is a TUI. Always pass `--format agent --non-interactive` from this skill.

## Driving it with arm-verify.sh

Preconditions: launch + doctor passed. Helper bootstraps, ensures worker id, commits `README.md` + `ready.go`, `sources add`/`sync`, creates `TASK-VERIFY-READY` with `--source`, `dag transition --issue TASK-VERIFY-READY`.

```bash
.cursor/skills/verify-armature/scripts/arm-verify.sh drive ready-claim
```

Proof:

1. `ready` JSON array contains `{"issue":"TASK-VERIFY-READY",...}`
2. `claim --worktree` exit 0 with `claimed_by` set
3. `arm show --format json TASK-VERIFY-READY` has `status: claimed`
4. `git worktree list` includes `$TARGET/.worktrees/TASK-VERIFY-READY` on `task/TASK-VERIFY-READY`
5. `arm worktree list` has `bound: ["TASK-VERIFY-READY"]`
6. ops log has a `claim` op (also visible via `arm log --json`)

Evidence: `evidence/<run-id>/drive/05-ready/` through `09-git-worktree/`.

## Gotchas

- Fresh `create` is draft → `arm ready` prints `null` (empty). Promote with `arm dag transition --issue <id>` after `arm validate` is green.
- `dag transition` fails on uncited nodes and other Graph Findings. Attach `--source` at create (or `sources link`) first.
- `claim` without `--worktree` is usage failure.
- Interactive `arm ready` can claim from the TUI; that is not the agent path.
- Expired claims are **not** in the ready array; they print as a JSON array on **stderr** under agent/json.
- Scope overlap with another claimed/in-progress task warns; `--force` to proceed.
- Cleanup must `git worktree remove` the claim worktree (the helper does this on `cleanup`).
