---
date: 2026-09-01
agent: claude
area: workflow
task: Close TOPTIER-B1 from its own claimed worktree
tags: [transition, delivery-gate, worktree, branch-discipline, clean-tree, gitignore, i5]
---

# Both `--to done` gates misjudge a claimed worktree, forcing `--force` on honest work

## User Goal

Complete `TOPTIER-B1` exactly as the workflow prescribes: `arm claim --worktree`,
TDD, `make check`, per-task commit, then `arm transition --to done` from that
worktree.

## Observed

Two independent gates rejected a transition that satisfied every condition they
exist to enforce.

**1 — Branch discipline reads the root repo, not the invoking checkout.**

```
$ cd .worktrees/TOPTIER-B1 && git rev-parse --abbrev-ref HEAD
fix/TOPTIER-B1
$ arm transition --issue TOPTIER-B1 --to done --branch fix/TOPTIER-B1
Error: cannot transition to done while on main branch: create a feature branch and open a PR
```

`cmd/armature/transition.go:80-85` resolves the branch from `appCtx.RepoPath`:

```go
repoPath := appCtx.RepoPath
gc := adapters.New(repoPath)
currentBranch, err := gc.CurrentBranch()
if currentBranch == "main" || currentBranch == "master" { ... }
```

`appCtx.RepoPath` is the *root* checkout — the one holding `.armature/` — which
is on `main` by definition while a worker is in a worktree. Passing
`--repo "$(pwd)"` does not help. So the check reports the branch of a checkout the
worker is not using, and is guaranteed to fire for every worker following the
prescribed worktree flow.

The only escape is `--force`, which disables the entire branch-discipline check.
An agent that hits this once learns to pass `--force` habitually, at which point
the check protects nothing — including in the case it was actually written for.

**2 — CleanTree counts gitignored build artifacts as a dirty tree.**

```
$ git status --short          # empty
$ arm transition ... --to done
delivery gate check failed:
  1. CleanTree: Working tree is not clean. Commit or discard changes to:
     bin/, coverage.html, coverage.out, mutesting-report/, scripts/__pycache__/
```

All five are gitignored (`git check-ignore -q` returns 0 for each) and git itself
reports the tree clean. They are the output of `make check` — which the task's own
DoD requires running. The gate therefore contradicts the DoD: satisfy the DoD and
you cannot transition; skip `make check` and you can.

`make clean` does not fully resolve it either — it misses `scripts/__pycache__/`,
so a second manual cleanup is still needed.

## Impact

I5 says deterministic gates decide. These two gates decide wrongly against work
that is genuinely compliant, and both push the worker toward a blanket override —
`--force` for the first, `--skip-delivery-gate` for the second. Overrides that
fire on correct work stop carrying information: the audit trail records an
override, but it cannot distinguish "the gate was wrong" from "the worker cut a
corner".

This also interacts badly with the evidence problem filed separately today. That
finding argues per-task commits are the only durable proof a task's work reached
main. Here the workflow that produces those commits is the one the gates obstruct,
and the path of least resistance — transition from the root checkout on `main`
with `--force`, skipping the worktree entirely — is exactly the practice that
destroys the evidence.

## Evidence

- `cmd/armature/transition.go:80-85` — branch read from `appCtx.RepoPath`
- Worktree `.worktrees/TOPTIER-B1` on `fix/TOPTIER-B1`, commit `639076a3`; root
  checkout on `main`; rejected with "cannot transition to done while on main branch"
- `--repo "$(pwd)"` pointed at the worktree: identical rejection
- `git status --short` empty while CleanTree listed five paths, every one of which
  `git check-ignore -q` confirms is ignored
- `make clean` (Makefile:147-149) removes `bin/ dist/ *.out coverage.html
  mutesting-report/` but not `scripts/__pycache__/`; `git clean -Xfd scripts/` finished the job
- Transition finally succeeded only with `--force` plus a manual two-step cleanup
- Related: [Story-level umbrella commits erase the only per-task evidence](2026-09-01T1156Z-claude-workflow-story-level-commits-erase-task-evidence.md)

## Suggested Follow-Up

For the branch check, read the branch from the invoking checkout rather than the
root — the delivery-gate code immediately below it already does exactly this via
`worktreeIssueBinding(invokingRepoPath)`, so the correct notion of "where the
worker actually is" is present in the same file. Better still, resolve the branch
from the worktree bound to the issue being transitioned.

For CleanTree, honour `.gitignore` — `git status --porcelain` already gives the
right answer, and any check that disagrees with git about whether a tree is clean
will keep producing false positives as build tooling changes.

Both are the same underlying mistake: asking a question about "the repo" when the
answer depends on which checkout is being used.
