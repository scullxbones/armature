---
writer: claude
area: workflow
slug: json-string-int-mismatch-hidden-by-go-struct-tests
date: 2026-06-28T22:00Z
---

# Critical JSON marshaling bug in review package hidden by Go-struct test construction

## What I was trying to do

Completing SMTC-S1 — full semantic conformance review protocol. All tasks implemented, `make check` green, story audited and transitioned. The opus code reviewer was dispatched to review all changes.

## What happened

The opus reviewer identified that `CriterionStatus` and `Rating` are `int` enums that marshal as integers (0, 1, 2), but:
1. The `armature-reviewer` skill instructs AI reviewers to emit string values: `"status": "satisfied"`
2. `arm review record` reads the assessment JSON and unmarshal fails with type mismatch
3. All tests passed because every test constructed assessments as Go structs and `json.Marshal`'d them — they never read a JSON file with string statuses

The end-to-end flow the story was meant to deliver (reviewer agent submits JSON → `arm review record` → persisted) was broken, but all tests were green.

A second critical bug was also hidden: `arm review record` validated citations against `BuildDiffIndex("")` (empty diff), so any assessment with file:line citations was rejected. Tests submitted citation-free assessments, masking this.

Both were fixed by the sonnet remediation agent.

## Why it matters

- Haiku workers implemented the skill docs and the Go code independently; the type contract between them was never tested end-to-end with a JSON fixture authored the way a real reviewer would author it.
- The coordinator dispatches workers wave by wave with no integration test spanning the full user journey until T10 (the final task). By then, the skill docs and types had diverged.
- The green test suite created false confidence; the opus review was the first time a human (or agent) read the code with the actual usage contract in mind.

## Evidence

- `internal/review/types.go`: `CriterionStatus` and `Rating` defined as `type CriterionStatus int`
- `armature-reviewer/SKILL.md`: instructs `"status": "satisfied"` (string)
- All `review_test.go` and `pipeline_test.go` tests construct `review.CriterionResult{Status: review.Satisfied}` (Go struct) — never parse a `.json` file
- Opus reviewer: "the prepare→reviewer→record loop is non-functional with a real reviewer agent"
- Fix: added `MarshalJSON`/`UnmarshalJSON` to both types using existing `String()`/`Parse*` helpers

## Potential mitigations

- When a task adds a new type that skill docs describe in JSON format, the worker should add at least one test that round-trips via a JSON fixture file (not only via Go struct construction).
- The coordinator could require a "skill-contract test" for any task that both touches Go types AND updates a skill doc.
- The armature-reviewer skill (or the DoD for T6/T1) should have required a test that feeds skill-format JSON to `arm review record`.
