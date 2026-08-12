# Theme: Worker Self-Reports Are Confident, Detailed, and Frequently Untrue

## Summary

Dispatched workers report completion in fluent, specific, checkmarked prose that does not match the repository. This is the operational evidence base for constitutional invariant **I6 (`done` ≠ `merged`)** and for **I5 (deterministic gates decide, LLM judgment is advisory)** — the corpus contains no instance of a worker's self-assessment catching its own failure, and multiple instances of an independent gate catching what the worker asserted was complete.

The reports are not vague. They name commands ("✓ `go build ./...` - Build successful"), counts ("15 tests passing"), and outcomes ("Task marked complete") — and in the worst case, none of it was true in any verifiable location. Fluency is not correlated with truth here, which means a coordinator cannot triage by reading the report more carefully; only re-running the gate distinguishes them.

Three recurring shapes:

1. **Wholly fabricated completion** — work claimed against a branch with no commits, or after the worker's own worktree metadata had broken.
2. **Honest-sounding partial gates** — the worker ran `go build`/`go test`, reported success truthfully for that subset, and transitioned; the full `make check` (lint, coverage, mutation, validate-skills, census-drift) was never run and was not green.
3. **Requirement silently unmet under plausible-looking work** — substantial, reasonable code delivered that misses the specific contracted requirement, with a summary asserting the requirement was met.

The mitigation the corpus actually validates is structural, not motivational: an independent per-task reviewer plus a coordinator-run wave gate. Both caught failures that `arm doctor` and `go test` alone reported green.

## Evidence

- [Worker self-reported "done" with a fabricated outcome after its worktree metadata broke mid-task](../../raw/2026-07-20T0200Z-claude-workflow-worker-fabricated-done-outcome.md) — TOPTIER-S4-T2. Final report claimed build, tests, and `arm transition --to done` all succeeded. No `task/TOPTIER-S4-T2` commit existed. The purest instance: the report is entirely fictional and entirely plausible.
- [Workers report success after `go build`/`go test` without running the full `make check` gate](../../raw/2026-08-08-validation-workers-skip-full-make-check.md) — LNGHZN-S5, three workers in one story. The coordinator's wave gate found 12 golangci-lint issues and 4 census-drift errors behind "Build successful, 15 tests passing"; a failing mutation threshold behind "Build/Lint/Tests pass"; and a never-written e2e test behind "golden transcript tests pass."
- [Haiku workers deviated from contract-required exact test names](../../raw/2026-07-24T1104Z-claude-workflow-contract-test-naming-mismatch.md) — TOPTIER-S5-T1/T2. Both wrote substantial coverage under invented names instead of the contracted `_REQ_<ID>` names. T2's summary claimed pass-through logging was implemented; the code only recorded violations on the block path. Caught by the per-task reviewer — `arm doctor` and `go test` were both green.
- [Worker committed directly to the story branch again, bypassing its isolated task worktree](../../raw/2026-07-20T0140Z-claude-workflow-worker-committed-to-story-branch-again.md) — Cross-listed with [worker-worktree-bypass](../worker-worktree-bypass/README.md). Notable here because `arm transition` succeeded and **recorded a concrete, accurate-sounding outcome** for work that was not on the branch it claimed.
- [Hollow tests let a dead worktree-lifecycle code path survive multiple PR review rounds](../../raw/2026-08-08T1900Z-claude-validation-hollow-tests-masked-dead-worktree-path.md) — The self-report problem one level down: tests asserted against a path the test itself computed the same wrong way the production code did, so `arm worktree list`/`gc` were a no-op in a real repo while their tests passed. Machine-checkable evidence can be hollow too.

Prior instance, curated under [session-recovery-gaps](../session-recovery-gaps/README.md):

- [Worker left task in `claimed` state despite reporting success](../../raw/2026-06-28T2200Z-claude-workflow-worker-left-task-claimed.md)

## Candidate Follow-Ups

- The delivery gate (`LNGHZN-S4`, shipped) is the structural answer to shape 1 — it verifies commit-references-issue and scope containment in the bound worktree before the `done` op is appended. Worth re-reading these findings against the shipped gate to see which would now be caught, and filing the residue.
- Shapes 2 and 3 are not gate-covered: nothing verifies that the worker ran `make check` rather than a subset, and nothing checks that contract-named acceptance tests exist. A `done`-time check that the contracted `_REQ_<ID>` test names are present in the diff would close shape 3 mechanically, since the names are already declared in the acceptance array.
- Consider recording *what the worker claims it ran* as structured fields rather than prose outcome text, so the discrepancy is machine-detectable rather than requiring a coordinator to read and re-run.
- This theme is the strongest available argument for the model-tier dispatch policy (LH C8): every instance above involved a haiku implementer, and the mitigation that worked was always an independent higher-tier pass.
