# Finding: Planner dismissed context_files warnings as noise; they are real scope signals

**Date:** 2026-06-28T20:43Z  
**Writer:** claude  
**Area:** workflow  
**Task:** Loading `docs/superpowers/plans/2026-06-27-semantic-conformance-review.md` into armature as SMTC epic

## What the agent was trying to do

Run `arm validate --ci` after loading the SMTC plan (12 issues: 1 epic, 1 story, 10 tasks) to confirm the plan was ready for workers. The initial finding treated 6 `context_files` WARNINGs as presentation noise. This finding corrects that characterization after reviewing CONTEXT.md and the actual task scopes.

## What happened

`arm validate --ci` exited 0 but emitted 6 `missing context_files on <ID> with broad scope` WARNINGs:

- `SMTC-S1` (story): scope spans 15+ files across review/, adapters/, ops/, materialize/, output/, cmd/, skillsembed/, docs/ — effectively the whole feature
- `SMTC-S1-T5`: 9 files across 3 unrelated packages (ops, materialize, output)
- `SMTC-S1-T7`: 4 files (new Skill tree + plugin.json + bootstrap_test.go)
- `SMTC-S1-T8`: 5 new files (testdata + scripts) — cohesive, dev-only
- `SMTC-S1-T9`: 5 files spanning 3 different Skills + 2 docs files
- `SMTC-S1-T10`: 2 test files + 1 spec doc

## Scope assessment per warned task

Per CONTEXT.md: `Scope` = **write boundaries**; `Context files` = background reading material. They are distinct. A broad scope on a Task means a worker must touch multiple modules or concerns, increasing collision risk and making the work harder to review atomically.

| Issue | Scope (files) | Assessment |
|---|---|---|
| SMTC-S1 (story) | 15+ files, all packages | Expected for a story; scope summary is fine |
| T5 | 9 files, ops + materialize + output | **Real problem.** Three separate domain modules in one task. Should split: (a) add op + schema; (b) materialize it; (c) surface in output. |
| T7 | 4 files, new Skill files + test | Cohesive — all in one functional area. Warning may be a heuristic false positive on count. |
| T8 | 5 files, scripts + testdata only | Cohesive dev-only group. Warning likely heuristic. |
| T9 | 5 files, 3 Skills + 2 docs | **Worth scrutiny.** Coordinator + Auditor + armature Skill + commands.md + concepts.md could legitimately be one "documentation pass" task, but it mixes skill update work with customer-facing docs. |
| T10 | 3 files, 2 test + 1 spec | Cohesive closeout. Warning likely heuristic. |

## How it changed behavior, confidence, or time spent

- **Initial misread cost ~0 time** — the planner passed validation and released the plan without addressing any warnings.
- **Real risk**: T5 scope (ops + materialize + output) means one worker must hold context across 3 packages simultaneously. This increases partial-delivery risk and reduces the signal value of per-task test runs.
- The planner (this agent) rationalized the warnings as "look alarming to a first-time planner" rather than inspecting whether the scope was actually too wide. This is the failure mode the warning is designed to catch.

## Evidence

```
WARNING: missing context_files on SMTC-S1-T5 with broad scope — split the task into smaller pieces or narrow scope
WARNING: missing context_files on SMTC-S1-T7 with broad scope ...
WARNING: missing context_files on SMTC-S1-T8 with broad scope ...
WARNING: missing context_files on SMTC-S1-T9 with broad scope ...
WARNING: missing context_files on SMTC-S1-T10 with broad scope ...
WARNING: missing context_files on SMTC-S1 with broad scope ...
COVERAGE: 527/527 cited (468 source-linked, 59 accepted-risk)
OK: no issues found
```

T5 scope for reference:
```
internal/ops/types.go, internal/ops/schema.go, internal/ops/types_test.go,
internal/ops/ops_test.go, internal/materialize/state.go,
internal/materialize/engine.go, internal/materialize/engine_test.go,
internal/output/output.go, internal/output/output_test.go
```

## What the planner should have done

1. Read the warning message literally: "split the task into smaller pieces or narrow scope".
2. Open each warned task and check whether the scope spans multiple domain modules.
3. For T5: split into three tasks — (a) declare op + schema, (b) implement materialization handler, (c) surface in output.
4. For T9: decide if 5 files of mixed documentation is one coherent task or should split Skills from customer docs.
5. For T7, T8, T10: confirm each is a single cohesive concern; if yes, these warnings are heuristic over-firing on file count rather than domain breadth.

## Observation about planner skill

The `armature-planner` skill's "Common Failure Modes" table does not mention `context_files` warnings as a breakdown signal. A planner who reads the skill documentation carefully has no explicit cue to treat this warning as a scope decomposition prompt. The warning text itself ("split the task") is actionable, but a planner who is confident in their plan will rationalize it away.
