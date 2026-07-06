---
date: 2026-07-06
agent: claude
area: workflow
task: EXECEV coordination
tags: [review, task-decomposition, integration-testing, sequential-waves]
---

# Cross-task interface drift (log format, entry IDs) survives per-task semantic review; only whole-story deep review caught it

## User Goal

Deliver story EXECEV (5 sequential tasks: log writer → bundle section → citation validation → skills → docs) with per-task semantic conformance review as the quality gate.

## Observed

Each task passed its task-scoped semantic review green, and every wave passed `make check` (incl. 100% mutation efficacy). Yet the final whole-story deep review found 3 critical + 10 major defects, several of which were *interface mismatches between tasks*: the T1 writer emitted key=value lines while the T4 indexer skill documented JSONL; code used 0-based integer entry IDs while skills documented 1-based zero-padded; the T2 parser dropped the T1 writer's truncation tail and never unescaped quoting; T2's prepare looked for the log at the parent repo's `.git/` while T1's hook writes to the worktree's private git dir. No cross-component round-trip test existed because no single task's contract owned the interface.

## Impact

The feature as merged would not have functioned end-to-end in its primary deployment shape despite five green task reviews and green gates on every wave. A large story-level remediation (21 findings, ~1 hr sonnet agent run, full JSONL format switch) was needed after the story was nominally complete. Per-task review scoped to each task's diff structurally cannot see this class of defect.

## Evidence

- Fable deep-review findings C1–C3, M1, M7 (see PR #71 closing commit 606d48cf message and docs of the remediation).
- All five `arm review record` ratings were green at merge time.

## Suggested Follow-Up

When a planner decomposes a story whose tasks share a data format or file contract, emit an explicit interface criterion (e.g. "round-trip test between writer and parser") into *both* tasks' acceptance criteria, or add a story-level integration criterion the coordinator must review before transition. A whole-story review pass (beyond the auditor's citation/health checks) seems worth institutionalizing.
