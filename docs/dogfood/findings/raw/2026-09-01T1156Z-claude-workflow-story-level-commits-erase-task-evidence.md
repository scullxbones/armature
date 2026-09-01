---
date: 2026-09-01
agent: claude
area: workflow
task: Promote stale done-under-merged tasks across the whole DAG
tags: [commit-convention, i6, merged, evidence, per-task-commit, umbrella-commit, audit]
---

# Story-level umbrella commits erase the only per-task evidence that `merged` is true

## User Goal

Clear 79 issues stuck at `done` across the DAG by promoting each to `merged`
only where "this work is confirmed on `origin/main`" (I6) could actually be
asserted, rather than assumed from the parent story's status.

## Observed

The cheapest sound evidence available is a conventional commit naming the task
in its scope — `type(<ID>): …` — reachable from `origin/main`. That criterion
settled 57 of 79 items outright. It failed on 22, and the failures were not
random: they cluster on stories whose worker committed **once, at story level**,
instead of once per task.

`ORCRUN-S1` is the clearest case. Its four children `ORCRUN-T01..T04` have zero
ID-tagged commits between them. All four were delivered by a single commit:

```
9e73eda7 feat(ORCRUN-S1): deepen orchestration run contract
 internal/orchestrate/run.go        | 467 +++++++++++++++++++++
 internal/orchestrate/run_test.go   | 275 ++++++++++++
 internal/orchestrate/engine.go     |  31 +-
 internal/orchestrate/preflight.go  |   9 +-
 internal/orchestrate/types_test.go |  55 +++
 cmd/armature/orchestrate.go        | 119 +-----
 cmd/armature/worker_run.go         |  60 +--
 … 12 files, 1166 insertions
```

`run.go` is listed `(new)` in `ORCRUN-T01`'s scope and is created by this
commit. The work plainly landed. But nothing in git attributes any part of that
diff to T01 rather than T02, T03 or T04, so no per-task assertion is
recoverable — only a story-level one.

`docs/conventions.md` and `AGENTS.md` both require per-task commits. This is
that convention being violated, and the cost only becomes visible months later,
at audit time.

## Impact

The convention is usually justified as reviewability. Its load-bearing purpose
is narrower and more important: **a per-task commit is the only durable,
machine-checkable evidence that a specific task's work reached main.** Armature
records no `branch`, no `pr`, and no merge commit for these issues (see Recurs),
so the commit message is the last remaining link between an issue ID and a diff.
Collapse four tasks into one story commit and that link is gone permanently —
I2 means it cannot be backfilled, only annotated.

The practical consequence at audit time is a forced downgrade of evidence. Those
four tasks could not be promoted on commit evidence and instead required a
weaker, hand-verified argument recorded as a note ("delivered by umbrella commit
9e73eda7; scope files confirmed present in `origin/main` history"). That
argument is sound but it is not reproducible by a tool, and it took a human-
supervised investigation per task to construct.

This also silently degrades any future automated check. A doctor check for
"task is `done` but its parent is `merged`" can suggest which issues to look at;
it can never confirm one is safe to promote, because for umbrella-committed
stories the evidence does not exist in any form a check can read.

## Evidence

- `git log origin/main --grep='ORCRUN-T01'` … `ORCRUN-T04` → 0 results each
- `git log origin/main --grep='ORCRUN-S1'` → `9e73eda7`, the sole delivery commit
- `git show --stat 9e73eda7` → creates `internal/orchestrate/run.go` (467 lines)
- `ORCRUN-T01` scope: `internal/orchestrate/run.go (new)`, `preflight.go`,
  `types.go`, `types_test.go` — all touched by `9e73eda7`, none individually attributed
- Same shape, different cause, on the ARM rename story: `ARM-S2-T2`'s commit is
  `c075ec38 fix(ARM-S2-T2+): complete armature rename — config keys, branch,
  worktree, state dir`, where the `+` suffix silently absorbs `ARM-S2-T3`'s and
  `ARM-S2-T4`'s work; both children have no commit of their own
- 22 of 79 stale issues had no ID-tagged commit; after investigation every one
  was found to have genuinely landed, i.e. the evidence gap was 100% false
  negatives caused by commit practice, not by undelivered work
- Recurs: [`arm sync` skips every done issue because `branch` is never recorded](2026-08-31T1142Z-claude-workflow-sync-skips-every-issue-branch-never-recorded.md)

## Suggested Follow-Up

Make the convention enforceable rather than aspirational. `arm transition --to
done` already refuses to run on `main`/`master` without `--force`; it could
equally warn when no commit reachable from `HEAD` names the issue being
transitioned. That is the moment the evidence still exists and the cost of
fixing it is one `git commit --amend`.

Failing that, record the delivery SHA on the transition op. The worker knows it
at `done` time, and one recorded SHA would make every question in this audit a
lookup instead of an investigation.

Worth noting for whoever writes that check: the `+` convention seen in
`ARM-S2-T2+` is an agent inventing a way to say "and its siblings". That it was
invented at all suggests workers feel the pressure to batch and have no
sanctioned way to express it.
