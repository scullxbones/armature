---
date: 2026-08-23
agent: claude
area: workflow
task: Close out LNGHZN-S10
tags: [review, conformance, worktree, teardown, evidence]
---

# A task can be fully reviewed on GitHub and still have zero conformance evidence in Armature

## User Goal

Verify `LNGHZN-S10-T5` was reviewed before promoting it to `merged`.

## Observed

`arm show LNGHZN-S10-T5` has no `Review:` line and there is no
`.armature/review/LNGHZN-S10-T5-*.json`; only T4, T10 and T12 carry assessments. Yet
PR #112 went through **three** review cycles (commits `c13bdce1`, `32e0d180`, `7cc683dc`,
`80dee97c`). The review happened; none of it entered Armature.

Compounding this: the activity log a retroactive `arm review prepare` would need lives in
the task worktree's private git dir, and `arm merged` tears that worktree down. So the
window to recover the evidence closes the moment the documented promotion command runs —
`.worktrees/LNGHZN-S10-T5` is still bound only by luck of the story not being closed yet.

## Impact

Conformance coverage silently under-reports the review that actually occurred, and the gap
is only discoverable by cross-checking GitHub by hand. The recovery ordering constraint
(review before teardown) is documented in the coordinator skill but is easy to violate,
and violating it is irreversible. Forced a human decision — reconstruct the bundle or
record an I7 override — during what should have been a mechanical closeout.

## Evidence

- `arm show LNGHZN-S10-T5`; `ls .armature/review/` (no T5 entry)
- `gh pr view 112` — three review cycles
- `.claude/skills/armature-coordinator/SKILL.md` §f — review must precede worktree teardown
- `cmd/armature/merged.go` — teardown on promotion

## Suggested Follow-Up

Have `arm merged` refuse (or loudly warn) when the task has no recorded conformance
assessment, the same way it already gates on `armature-hook.log` violations — the
information needed to catch this is already in hand at exactly the right moment.
