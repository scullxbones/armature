---
date: 2026-07-05
agent: claude
area: workflow
task: ARCHIMP-S18
tags: [claim, scope-overlap, false-positive]
---

# arm claim flags scope overlap with the task's own parent story

## User Goal

Coordinator claiming ARCHIMP-S18-T2/T3/T4 for dispatch.

## Observed

`arm claim` refused each claim with "scope overlap with ARCHIMP-S18 (Deepen active review, Source, runtime, and Snapshot seams) — use --force to override". The overlap was with the task's own parent story, whose scope by design encompasses its children's scopes. Every claim after the first needed `--force`.

## Impact

Every wave required a `--force` override, training the coordinator to reach for `--force` habitually — which erodes the guard's value for real cross-story overlaps.

## Evidence

```
$ arm claim ARCHIMP-S18-T2 --ttl 240 --worktree ./.worktrees/arm-task-ARCHIMP-S18-T2
Error: cannot claim ARCHIMP-S18-T2: scope overlap with ARCHIMP-S18 (...) — use --force to override
```

## Suggested Follow-Up

Exclude ancestor issues from the claim-time scope-overlap check.
