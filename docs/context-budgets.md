# Context budgets

A budget is a hard byte cap on one static agent-facing artifact.
The inventory is the same one `make context-report` prices: each embedded skill tree, plus `CONTEXT.md`, `docs/commands.md`, `docs/concepts.md`, and `docs/use-cases.md`.
Tokens are not a second cap. They are `bytes / 4`, the `token_budget` heuristic, shown so a reader can compare to render-context.
Dynamic structured command payloads are out of scope until AOC-S3-T2.

Budgets live in `internal/contextreport/budgets.json`.
Each row is one artifact: `path`, `class`, and `max_bytes`.
Class is for reporting. The gate still fails per artifact, not per class.

Initial values are seeded at the size measured on this branch.
The gate starts green. Growth fails until someone shrinks the file or raises the budget through the ratchet exception.

## How the ratchet works

A budget may only shrink in ordinary commits.
That is the default. Smaller is always allowed.

Raising a budget is a reviewable exception.
The raise must be a dedicated commit that touches only `internal/contextreport/budgets.json`.
`make check` does not enforce that commit shape. Reviewers do.
`RaisedBudgets` lists paths whose `max_bytes` grew so a reviewer can see the exception clearly.

## How the gate fails

`Enforce` compares `Collect` (measured bytes) to the checked-in file.
`make check` runs the unit suite once via `coverage`.
`TestCheckedInContextBudgetsHold` loads the real repo and the budgets file.
If any artifact is over budget, that test fails, so `make check` fails.

The error groups over-budget rows by class:

```
context budget gate failed
over budget (grouped by class):
  skill:
    internal/skillsembed/skills/armature-coordinator: 62100 bytes > budget 62099
```

The gate also fails when a measured path has no budget, when a budget path is not measured, or when `class` disagrees with the report.
Those keep the file from drifting away from the inventory.

`TestBudgetGateFailsOverBudget_REQ_NXTTN_S3_T2` is the acceptance test for the over-budget case.
It uses a fixture, not the live repo.

## How to reseed

1. Run `make context-report` (or `arm context-report --repo . --format json`).
2. For each artifact, set `max_bytes` to the measured `bytes` value, or lower if you already trimmed.
3. Do not raise `max_bytes` in the same commit as a code or docs change.
4. If you must raise, make a follow-up commit that edits only `internal/contextreport/budgets.json`. Say why in the commit body.
5. Re-run `go test ./internal/contextreport -count=1 -run TestCheckedInContextBudgetsHold`.
6. Re-run `make check` before you push.

Shrinking after a trim does not need a special commit. Mix it with the trim.

## Offenders and trim plan

Cutoff: estimated tokens above 4000 (the render-context default).
These artifacts already exceed that load on every skill or doc ingest.
Budgets are seeded at current size so the gate is green. The plan is how to shrink later.

### skill: armature-coordinator (46597 bytes, ~11649 tokens)

Largest tree. `SKILL.md` is a long runbook. `references/commands.md` duplicates command help that `docs/commands.md` already holds.

Trim: keep the dispatch loop in `SKILL.md`. Move command cheat-sheets and parallel-dispatch narrative behind an on-demand reference the coordinator opens only when needed. Stop copying the command reference into the skill tree.

### skill: armature-reviewer (57671 bytes, ~14417 tokens)

`SKILL.md` plus `references/rubric.md`, `references/field-rules.md`, and a JSON template.

Trim: keep the decision procedure in `SKILL.md`. Load rubric and field-rules only when scoring. Keep the JSON template as a short example, not a second copy of the schema.

### glossary: CONTEXT.md (28888 bytes, ~7222 tokens)

The domain glossary is loaded as a whole.

Trim: split rarely cited terms into a secondary page that `CONTEXT.md` links but the default ingest does not include. Keep the terms agents actually resolve on the paved road.

### commands: docs/commands.md (35827 bytes, ~8956 tokens)

Full command reference. Agents ingest the whole file today.

Trim: lead with the paved-road commands. Push rare flags into per-command pages. NXTTN-S4-T3 already aims at docs that lead with the paved road. Let that work shrink this budget instead of rewriting twice.

### skill: armature-planner (23036 bytes, ~5759 tokens)

Planner body plus decompose and dependency references.

Trim: keep decompose steps in `SKILL.md`. Open dependency-management only when the graph has cycles or blockers.

### use-cases: docs/use-cases.md (20924 bytes, ~5231 tokens)

Worked examples. Useful, not needed on every turn.

Trim: keep one canonical worked example in the default ingest. Link the rest.

### skill: armature-worker (20430 bytes, ~5107 tokens)

Worker skill plus batch-strategy and a Go example.

Trim: keep claim/implement/transition in `SKILL.md`. Load batch-strategy only for multi-issue waves. Drop or shorten the Go example if the worker skill already states the test pattern.

### concepts: docs/concepts.md (17013 bytes, ~4253 tokens)

Just over the cutoff.

Trim: fold overlapping explanations into `CONTEXT.md` or the paved-road docs and delete the duplicate section here.

### Under the cutoff (no trim required this round)

- skill armature-activity-indexer (~3581 tokens)
- skill armature-auditor (~2852 tokens)
- skill armature (~630 tokens)
- skill test-skill (~21 tokens)

## Wiring note

The task file list is `docs/context-budgets.md`, `internal/contextreport/budget.go`, and `internal/contextreport/budget_test.go`.
`internal/contextreport/budgets.json` is the file `budget.go` reads. It is in spirit of the feature.
No Makefile change. `make check` already runs this package through `coverage`. A second `check` prerequisite would run the suite twice and break the single-run rule in `docs/design/gate-efficiency.md` (D3).
`make context-report` from NXTTN-S3-T1 stays the measurement command.
