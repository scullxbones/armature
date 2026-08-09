---
area: validation
writer: claude
date: 2026-08-09T15:10Z
pr: 89
story: LNGHZN-S5
---

# Deterministic gates judge files git does not treat as working-tree content, training workers to skip them

## What the agent-user was trying to do

Transition `LNGHZN-S5-T6` to `done` after committing the work and running
`make check` to green.

## What happened

The delivery gate refused:

```
Error: delivery gate check failed:
  1. CleanTree: Working tree is not clean. Commit or discard changes to:
     bin/, coverage.out, mutesting-report/, scripts/__pycache__/

Use --skip-delivery-gate to override (audit trail will record the override)
```

The working tree was clean. `git status --porcelain` printed nothing. All four
paths are build artifacts produced by `make check` itself, and all four are
gitignored. The gate is counting **ignored** files as uncommitted changes.

So the sequence is: run the required quality gate, and the act of running it puts
the tree into a state the delivery gate rejects. Deleting the artifacts and
re-running the transition worked.

## How it changed behavior / confidence / time spent

- This is not new. `LNGHZN-S5-T1`'s recorded outcome ends with: *"Skipped
  delivery gate: bin/ and coverage.out are build artifacts, not part of task
  scope."* The same obstacle, the same artifacts, resolved the other way.
- That is the real cost. The error message names `--skip-delivery-gate` as the
  remedy, so the path of least resistance is to bypass an I5 deterministic gate
  because of files git already ignores. A worker who does this once learns that
  gate failures are usually noise. The next genuine failure gets the same reflex.
- Every worker on this repo hits it, because `make check` is mandatory and
  produces these artifacts every time.

## The same shape in `arm doctor` D8

The delivery gate is not the only check with this problem. Immediately after the
task above completed, `arm doctor` reported:

```
✗ D8: Out-of-scope artifacts detected for active or recently-completed tasks
    - LNGHZN-S5-T6: .bashrc
    - LNGHZN-S5-T6: .bash_profile
    - LNGHZN-S5-T6: .gitconfig
    - LNGHZN-S5-T6: .gitmodules
    - LNGHZN-S5-T6: .claude/agents
    - LNGHZN-S5-T6: .claude/hooks
    - LNGHZN-S5-T6: .claude/launch.json
    ...
```

None of these came from the task. They are agent-harness files injected into the
repository root by the Claude Code sandbox — `findmnt` shows `.bashrc` as a
`devtmpfs` bind mount of `/dev/null`, the same mechanism that makes
`.git/config.lock` unwritable and so makes `git config set` (and therefore
`arm claim --worktree`) fail outright inside the sandbox.

The distinction from `CleanTree` is worth keeping, because the two failures are
not identical:

- `CleanTree` flags files that **are** gitignored. It should ignore them.
- D8 flags files that are **neither tracked nor gitignored**. Git reports them as
  untracked, so D8 is not strictly wrong — but they are not the task's artifacts
  either, and no worker can make them go away.

The common failure is the same: a deterministic gate renders a verdict on files
that are not the task's work product, and the worker cannot fix the cause. D8
will fail for every task run under this harness, in every repository, forever.

## How it changed behavior / confidence / time spent

See above for `CleanTree`. For D8 the cost is different and worse: `CleanTree`
blocks and can be cleared by deleting artifacts, so the worker at least gets an
answer. D8 does not block — it simply reports a permanent red mark on
`arm doctor` output that no action can clear. A check that is always red is a
check nobody reads. `arm doctor` is where the D1–D8 diagnostics live, so the cost
is borne by every other diagnostic in that command.

## What would have helped

For `CleanTree`: honor `.gitignore` — matching `git status --porcelain`, which is
what "clean working tree" means everywhere else in the toolchain. Failing that,
name the specific ignored paths the gate will tolerate, so the remedy is never
"turn the gate off".

For D8: judge only paths the repository could plausibly own. A configurable
ignore list for harness-injected paths would work; so would restricting the scan
to files git tracks or that the task's diff touched. The principle behind both is
the same — a deterministic gate should only rule on artifacts a worker can
actually control, or workers learn that its verdicts are noise.
