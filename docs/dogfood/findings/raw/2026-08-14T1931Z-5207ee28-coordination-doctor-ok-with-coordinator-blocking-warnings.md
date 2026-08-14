---
area: coordination
writer: 5207ee28-cdd8-48e6-98dc-7da179d4a40d
date: 2026-08-14T19:31Z
story: LNGHZN-S9
---

# Coordinator health preflight cannot tell whether an `arm doctor` warning is blocking

## What the agent-user was trying to do

Start the documented coordinator loop for `LNGHZN-S9`. The coordinator skill
requires `arm validate` and `arm doctor` to be clean before dispatch, says to fix
warnings from other stories, and also describes the hard gate as zero errors.

## What happened

`arm doctor` exited successfully and printed `OK: no issues found`, but the strict
health section in the same output reported D1 and D9 warnings:

```text
OK: no issues found
⚠ D1: Git commits reference issues not in done/merged state
    - LNGHZN-S9 (open)
⚠ D9: Managed worktrees with no issue binding
    - /home/brian/development/armature/.worktrees/LNGHZN-S5-T9-test
```

D9 was actionable stale checkout metadata and could be cleaned. D1 was the story
being coordinated: `main` already contains the planning commit that introduced
`LNGHZN-S9`, so it cannot become green until the coordinator completes the story.
Treating every warning as blocking would make the requested story impossible to
start; treating the successful exit and `OK` as authoritative would ignore a real
D9 cleanup item.

## How it changed behavior, confidence, or time spent

The agent had to inspect the referenced commit and worktree manually, distinguish
an expected lifecycle warning from actionable residue, and invent a policy the
skill does not state. This adds delay to every preflight and makes coordinator
behavior depend on agent judgment even though the gate is intended to be
deterministic.

## Evidence

- `arm validate` completed with `COVERAGE: 716/716 cited`.
- `arm doctor` exited zero while printing both `OK: no issues found` and the D1/D9
  warnings above.
- `git log --all --grep=LNGHZN-S9` showed main commit `6040177c`, which documents
  the follow-on story while its issue status is necessarily still open.
- The D9 path was a clean checkout of `salvage/LNGHZN-S5-T9-test`; after its stale
  checkout was removed, D9 became green while D1 remained.

## What would have helped

The coordinator skill should define a warning policy by diagnostic: for example,
block on unrelated actionable D2/D9 residue, allow D1 for the story currently
being coordinated when its introducing commit is already on the base branch, and
state whether the command's exit status or strict diagnostic section owns the
preflight verdict.
