---
date: 2026-06-30
agent: claude
area: tooling
task: DF-S5
tags: [arm-log-slot, worker-identity, testing]
---

# ARM_LOG_SLOT env var (required by parallel-dispatch protocol) breaks unrelated tests

## User Goal

Following the armature-coordinator skill's dispatch protocol, every parallel worker
was instructed to run `export ARM_LOG_SLOT=<slot>` as its first shell command, per
the documented "Parallel Dispatch" requirement for log attribution.

## Observed

With `ARM_LOG_SLOT` set, four pre-existing tests in `cmd/armature` failed:
`TestAppCtxStateDirSet`, `TestLogSlot_Empty_UsesPlainLog`,
`TestSync_DryRun_PrintsPlanWithoutWritingOps`, `TestSecondaryStatePaths`. These tests
computed expected worker-identity-derived paths (e.g. `.armature/state/<workerID>`)
without accounting for the slot suffix that `workerIdentityWithSlot()` appends when
`ARM_LOG_SLOT` is non-empty. One worker (task-1782866629) discovered and fixed this
mid-task, outside its declared scope, to get `make test` green.

## Impact

Any worker or coordinator following the documented parallel-dispatch protocol
(`export ARM_LOG_SLOT=N`) inherits a red `make test` before touching any of their own
files, unless they happen to already know about this interaction. This is a latent
trap: the fix worked here because one worker noticed and had scope-creep-tolerant
instructions, but a strictly-scoped worker would have hit an unexplained test
failure with no path to green.

## Evidence

Commit `dccd03b3` (task-1782866629) fixed the four tests by applying
`workerIdentityWithSlot()` to expected paths, plus ops-directory cleanup for
`TestLogSlot_Empty_UsesPlainLog`. Prod code (`main.go`, `hook.go`, `helpers.go`)
already applied the slot suffix correctly — only test expectations were stale.

## Suggested Follow-Up

Fix these four tests on `main` (not just per-branch) so `ARM_LOG_SLOT` is safe to set
globally as the coordinator skill instructs. Consider adding a test that explicitly
sets `ARM_LOG_SLOT` and asserts path derivation, so this doesn't regress silently.
