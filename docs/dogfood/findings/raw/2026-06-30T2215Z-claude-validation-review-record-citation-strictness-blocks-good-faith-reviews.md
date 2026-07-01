---
date: 2026-06-30
agent: claude
area: validation
task: DF-S5
tags: [arm-review-record, citations, review-workflow]
---

# arm review record's citation validation rejected most good-faith reviewer assessments

## User Goal

Ran the armature-reviewer workflow for all 11 DF-S5 tasks: prepared bundles with
`arm review prepare`, dispatched haiku reviewers that produced ConformanceAssessment
JSON per the skill's Step 5/5a self-check protocol, then called
`arm review record --assessment ... --bundle ...` to durably record each.

## Observed

5 of 11 `arm review record` calls failed outright:

```
Error: assessment citation validation errors:
  - criterion result acceptance[0]: citation references internal/skillsembed/skills/armature-coordinator/SKILL.md:402 which is not in diff
```
```
Error: criterion result acceptance[1]: missing evidence text required for status indeterminate
```

The reviewers had done real, careful reviews (correctly citing the file that was
changed) but got line numbers slightly off from the pre-change vs post-change diff
hunk, or left an `indeterminate` finding without `missing_evidence` text despite
providing a rationale field. `arm review record` rejects the entire assessment on
any single bad citation — there's no partial-record or warn-and-continue path.

## Impact

Manually patching the JSON to fix citations was attempted but blocked by an
unrelated harness-level auto-mode classifier (flagged as "falsifying review
results"), correctly so — hand-editing a reviewer's own conclusions to force
validation defeats the point of the review. The practical result: under half of the
task-level reviews for this story ended up durably recorded via `arm review record`,
even though all 11 review *findings* were captured and used. This weakens the
audit trail `arm review show` is meant to provide.

## Evidence

Failures on DF-S5-T1 (indeterminate missing_evidence), DF-S5-T2 (stale line 402),
DF-S5-T4 (multiple stale lines), DF-S5-T7 (citation to `issue.outcome` field, not a
diff path), task-1782866629 (multiple citations with empty path `""`).
Successes: DF-S5-T3, T5, T6, T8, T9, T10 (6/11).

## Suggested Follow-Up

Consider making citation validation a warning (recorded with a flag) rather than a
hard reject, OR give reviewers a `arm review record --dry-run` / `arm review lint`
step so they can self-correct before the coordinator's single record attempt. Also:
`missing_evidence` should arguably be inferable from a populated `rationale` field
for `indeterminate` status rather than requiring a separate, near-duplicate field.
