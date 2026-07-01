---
date: 2026-07-01
agent: claude
area: workflow
task: PR-66-review-remediation
tags: [subagent-handoff, test-hygiene, silent-side-effect]
---

# A fresh haiku subagent's TDD fix left test-artifact files in the source tree

## User Goal

Run `/prfix` on PR #66: dispatch a fresh (context-free) haiku subagent to fix
two reviewer findings using TDD, then hand its diff to an opus review pass
before implementing final fixes and pushing.

## Observed

After the haiku subagent reported success (build/lint/test all green), the
orchestrating agent's own `git status --short` showed five untracked paths
that were never part of the intended diff: `cmd/armature/checkpoint.json`,
`index.json`, `ready.json`, `traceability.json`, and an `issues/` directory,
all sitting directly in `cmd/armature/` (the package directory, not a temp
dir). These were materialize/state artifacts written by the subagent's own
new doctor test, which invoked the real `doctor` command against a repo whose
`appCtx.StateDir` was empty — so relative paths resolved into the test's (and
thus the repo's) working directory. The subagent's `make build`/`make lint`/
`make test` all passed anyway, because nothing asserted where those files
landed — only that the commands exited zero.

## Impact

The orchestrating agent had to notice and manually `rm -rf` the stray files
before committing, and separately flag it as a required investigation item
for the opus review pass. It turned out not to be a test-hygiene slip but a
symptom of a real product bug: `doctor`'s new `PersistentPreRunE` fallback
never set `StateDir`, so `arm doctor` would materialize state into the cwd on
every real invocation with an empty StateDir context, not just under test.
Had the orchestrator not diffed the working tree before handing off to
review, this leak would have been invisible until someone ran `arm doctor`
for real and found stray JSON files in their repo root.

## Evidence

- `git status --short` after the haiku agent's report showed:
  `?? cmd/armature/checkpoint.json`, `index.json`, `ready.json`,
  `traceability.json`, `issues/`.
- Opus review pass (finding #1, P0) traced this to
  `internal/doctor/doctor.go`'s `Materialize(stateDir, ...)` being called with
  `stateDir == ""` because doctor's new `PersistentPreRunE` set `appCtx`
  directly instead of delegating to root's `PersistentPreRunE` (which is what
  calls `stateDirFor` in `main.go`).
- Fix commit `be54f5fa` on `feat/SB-ELIM`.

## Suggested Follow-Up

When dispatching a subagent to make code changes, always diff the working
tree for untracked files before trusting a "build/lint/test all green"
report — a passing test suite does not guarantee no stray side effects, and
stray files in the source tree can be the first visible symptom of a
correctness bug (not just clutter). Consider adding this diff-check as a
standard step in `/prfix`'s apply phase rather than relying on the
orchestrator noticing it ad hoc.
