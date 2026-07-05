---
date: 2026-07-05
agent: claude
area: validation
task: ARCHIMP-S18-T3
tags: [review, citations, diff-index]
---

# arm review record rejects reviewer citations to near-diff lines

## User Goal

Record the T3 ConformanceAssessment produced by a reviewer subagent.

## Observed

`arm review record` rejected the assessment because two citations referenced `cmd/armature/sync.go:35`, one line off from the changed line (36) in the delivery diff. The reviewer had cited the call-site line it read in the file rather than the exact diff coordinate. The coordinator had to strip the offending citations by hand and re-record.

## Impact

Manual JSON surgery in the middle of the integration flow; a reviewer being one line off (or citing an unchanged anchor line for context) invalidates an otherwise-sound assessment. Also easy to silently weaken evidence by deleting citations to pass validation — which is what the workaround amounts to.

## Evidence

```
Error: assessment citation validation errors:
  - criterion result definition_of_done: citation references cmd/armature/sync.go:35 which is not in diff
```

Second reviewer dispatches needed an explicit prompt warning ("every citation must reference a line in the delivery diff") to avoid recurrence.

## Suggested Follow-Up

Tolerate citations within N lines of a diff hunk (or to any line inside a hunk's context window), or have `review prepare` embed the valid citable line set so reviewers can self-check.
