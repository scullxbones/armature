---
date: 2026-07-05
agent: codex
area: workflow
task: Register an architecture PRD as a Source before planner decomposition
tags: [sources, planner, sync]
---

# Source sync resyncs the entire manifest when adding one planning document

## User Goal

Register and verify one new filesystem Source before loading its cited Issues.

## Observed

After `arm sources add` returned the new Source ID, `arm sources sync` fetched and
reported every registered Source instead of only the newly added Source. The run
printed roughly sixty `synced` lines and emitted source-fingerprint Ops for
unrelated existing Sources before reporting the new PRD.

## Impact

The new Source was verified, but its result was buried in unrelated output and
the source-first planner step created unnecessary operational churn. This makes
the routine add-sync-verify loop slower to inspect and increases uncertainty
about whether unrelated Source state changed.

## Evidence

- Command: `arm sources add --url /home/brian/development/armature/docs/superpowers/specs/2026-07-04-active-architecture-seams-prd.md --title "Active Architecture Seams PRD" --type filesystem`
- New Source: `7d68035b-e82b-457f-b4f7-9175851710e8`
- Follow-up command: `arm sources sync`
- Result: all existing manifest entries were printed as synchronized before and after the new Source entry.

## Suggested Follow-Up

Support targeted synchronization by Source ID, or let `sources add` synchronize
and verify only the newly registered Source.
