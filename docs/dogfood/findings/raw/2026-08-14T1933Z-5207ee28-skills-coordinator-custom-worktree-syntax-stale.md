---
area: skills
writer: 5207ee28-cdd8-48e6-98dc-7da179d4a40d
date: 2026-08-14T19:33Z
story: LNGHZN-S9
---

# Coordinator skill dispatch syntax is incompatible with the current `arm claim`

## What the agent-user was trying to do

Follow the checked-in `armature-coordinator` dispatch protocol verbatim by
pre-claiming `LNGHZN-S9-T1` into a dedicated worker worktree:

```text
arm claim LNGHZN-S9-T1 --ttl 240 --worktree /tmp/arm-task-LNGHZN-S9-T1
```

## What happened

The command failed before claiming the task:

```text
Error: accepts at most 1 arg(s), received 2
```

The current CLI defines `--worktree` as a boolean and always provisions
`.worktrees/<issue-id>`. Its help shows:

```text
--worktree       provision a worktree at .worktrees/<issue-id> (required)
```

The coordinator skill instead repeatedly documents `--worktree <path>`, uses
`/tmp/arm-task-TASK-ID` in its required worker prompt, and says that exact form is
an invariant. Reinstalling with `make install` did not change the result: the
PATH-resolved binary, installed binary, and freshly built `bin/arm` were
byte-for-byte identical and reported `v0.0.2-159-g80d511f1` at checkout HEAD
`80d511f1`.

## How it changed behavior, confidence, or time spent

The first dispatch failed, forcing a state audit to prove no partial claim or
worktree had been created. A coordinator that retries the documented form cannot
make progress. The stale required prompt also sends workers to a path that does
not exist even if the coordinator corrects only the claim command.

## Evidence

- `arm list --parent LNGHZN-S9` still showed T1 as `open` after the failure.
- `git worktree list --porcelain` showed no T1 worktree or task branch.
- `arm claim --help` documents only boolean `--worktree` examples.
- SHA-256 for `/home/brian/bin/arm`, `/home/brian/.local/bin/arm`, and `bin/arm`
  was `3713c063339322df99dcb43101145b8ebfd7ad4b0c5b2cdad7ea10153230053d`.

## What would have helped

Update the coordinator skill to use `arm claim TASK-ID --ttl N --worktree` and
derive the worker directory from the claim result or the documented
`.worktrees/<issue-id>` convention. A CLI compatibility check in skill validation
could catch argument-shape drift before deployment.
