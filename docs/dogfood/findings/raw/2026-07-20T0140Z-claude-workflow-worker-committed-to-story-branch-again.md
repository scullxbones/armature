---
date: 2026-07-20
agent: claude
area: workflow
task: TOPTIER-S4-T1 (coordinator dispatch)
tags: [worktree, isolation, scope-leak, recurrence]
---

# Worker committed directly to the story branch again, bypassing its isolated task worktree

## User Goal

Coordinating TOPTIER-S4: dispatch a haiku subagent as the implementer for TOPTIER-S4-T1
(`arm claim --worktree /tmp/arm-task-TOPTIER-S4-T1`, task branch `task/TOPTIER-S4-T1`), then
prepare a task-scoped review bundle (`arm review prepare --base <wave-base> --head
task/TOPTIER-S4-T1`) before merging into `feat/TOPTIER-S4`.

## Observed

The worker's own report claimed it created `docs/design/recovery-state-machine.md`, updated
`docs/agents/workflow.md`, committed, and ran `arm transition ... --to done`. `arm transition`
succeeded and recorded a concrete outcome. But `arm review prepare --base bf433d7e --head
task/TOPTIER-S4-T1` failed with "delivery contains no changed files" — `git log
bf433d7e..task/TOPTIER-S4-T1` was empty and the branch still pointed at the wave-base commit.
The worktree at `/tmp/arm-task-TOPTIER-S4-T1` had no uncommitted changes and no new file.
`git show HEAD:docs/design/recovery-state-machine.md` in the *main repo* (branch
`feat/TOPTIER-S4`, the coordinator's own checked-out branch) showed the commit
`feat(TOPTIER-S4-T1): Document recovery state machine for issue status x claim liveness`
sitting directly on `feat/TOPTIER-S4` at `a2311168` — the worker had committed straight to the
coordinator's branch in the main worktree instead of its assigned isolated worktree/branch,
despite an explicit "Working directory: /tmp/arm-task-TOPTIER-S4-T1" / "do NOT run `git
checkout feat/STORY-ID`" instruction in the dispatch prompt.

## Impact

This is the same failure mode as
`docs/dogfood/findings/raw/2026-07-19T1620Z-claude-workflow-worker-leaked-main-worktree.md`,
but this time the leak is total (100% of the delivery landed on the main branch, not a partial
stale-file leak) and `arm transition --to done` gave no signal that anything was wrong — the
issue's status and outcome looked completely normal. The only thing that caught it was the
task-scoped review-bundle step failing with "no changed files" against the expected branch,
which forced a manual `git log`/`git show` cross-check. Had the coordinator skipped straight to
`arm merged` (as the "no changed files" error is easy to misread as "reviewer found nothing to
flag" rather than "this branch has zero commits"), the story-branch content would already be in
the right place by accident this time, but a worker that also touched out-of-scope files or
diverged from the task's actual scope would have polluted `feat/TOPTIER-S4` with no worktree
isolation to contain it, and no doctor/validate signal would have caught it.

## Evidence

- Dispatch prompt included `Working directory: /tmp/arm-task-TOPTIER-S4-T1` and `Working
  branch: task/TOPTIER-S4-T1 ... do not run \`git checkout feat/STORY-ID\``.
- `git log bf433d7e..task/TOPTIER-S4-T1 --oneline` → empty.
- `git diff bf433d7e task/TOPTIER-S4-T1 --stat` → empty.
- `cd /tmp/arm-task-TOPTIER-S4-T1 && git log --oneline -5` → still at `bf433d7e`, `ls
  docs/design/` shows no `recovery-state-machine.md`.
- `git show HEAD:docs/design/recovery-state-machine.md` on the coordinator's own `feat/TOPTIER-S4`
  checkout → file present, committed as `a2311168 feat(TOPTIER-S4-T1): ...`.
- `arm review prepare --issue TOPTIER-S4-T1 --base bf433d7e --head task/TOPTIER-S4-T1` →
  `Error: prepare review bundle: delivery contains no changed files`.

## Suggested Follow-Up

Two occurrences now (2026-07-19, 2026-07-20) of a subagent worker writing to the coordinator's
checked-out branch instead of its isolated worktree, despite explicit instructions. Consider:
(1) the coordinator skill's wave-verification step should assert `git log
$WAVE_BASE_SHA..task/$TASK_ID` is non-empty (or run `arm review prepare` as a hard pre-merge
gate, not an optional step) before trusting a worker's self-report or `arm transition`; a
"delivery contains no changed files" error from `arm review prepare` should be treated as a
correctness gate failure, not skipped past; (2) since this keeps recurring specifically with
worker agents whose *session* cwd is the main repo even when told a different `Working
directory:`, dispatch workers via a true separate process/session rooted at the worktree path
(not just a prompt instruction) wherever the harness supports it; (3) `arm doctor` could gain a
check for "coordinator's checked-out branch has commits attributed to a task ID whose claimed
worktree branch has no matching commits" as a structural detector, complementing the existing D1
divergence check.
