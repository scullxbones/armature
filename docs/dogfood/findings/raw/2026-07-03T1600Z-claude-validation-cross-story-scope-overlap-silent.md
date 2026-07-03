---
date: 2026-07-03
agent: claude
area: validation
task: Planning EXECEV story (ADR-0008 execution evidence)
tags: [validate, scope-overlap, cross-story]
---

# Cross-story scope overlaps do not warn while same-story overlaps do

## User Goal

Release EXECEV (5 tasks) alongside the already-released HOOKBIND story, with
overlapping scopes across the two stories.

## Observed

EXECEV-T1's scope includes cmd/armature/harness_hook.go and
cmd/armature/harness_hook_test.go, which are also in scope for HOOKBIND-T1,
T2, and T3. EXECEV-T1 has direct blocked_by edges to HOOKBIND-T2 and T3 but
none to HOOKBIND-T1, yet `arm validate` emitted no scope-overlap WARNING.
Similarly EXECEV-T5 and HOOKBIND-T1/T5 both scope docs/harness-hook.md with
no edge and no warning. Within a single story earlier the same day, identical
overlap situations produced WARNINGs (see
2026-07-03T1500Z-claude-validation-scope-overlap-ignores-transitive-deps.md).

## Impact

Inconsistent protection: the overlap check appears scoped to siblings under
one parent, so two stories dispatched concurrently by a coordinator could
collide on the same files with no validation signal. Reduced confidence that
a clean `arm validate` means no file-level collision risk.

## Evidence

`arm validate --ci` exited 0 with only phantom-scope INFOs immediately after
EXECEV creation, despite the cross-story overlaps described above.

## Suggested Follow-Up

Confirm whether the overlap check is intentionally parent-scoped; if so,
extend it to all non-terminal issues (or at least all ready/claimable ones),
combined with the transitive-closure fix from the earlier finding.
