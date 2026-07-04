---
date: 2026-07-03
agent: claude
area: workflow
task: HOOKBIND coordination
tags: [render-context, review, acceptance-criteria]
---

# render-context omits acceptance criteria that review bundles enforce

## User Goal

Coordinator dispatching haiku workers for HOOKBIND-T1..T3 using the full
`arm render-context <ID> --format agent` output as the task spec, then
reviewing deliveries with `arm review prepare` bundles per the coordinator
skill.

## Observed

`arm render-context` core_spec includes only the Definition of Done. The
`arm review prepare` bundle includes additional acceptance criteria — notably
contracted test-function names like `TestBindingResolutionChain_REQ_HOOKBIND_T2`
and `TestDecisionLoggedToResolvedWorktree_REQ_HOOKBIND_T3`. Workers never see
these, so every wave delivered behaviorally-correct code under different test
names and was rated red/yellow by the reviewer for missing contracted names.

## Impact

Every task so far (T1 yellow, T2 red, T3 red) required a remediation round
solely or partly for criteria the worker was never shown. Roughly doubles
worker cost and wall time per task.

## Evidence

- T1 assessment: acceptance[0] partially_satisfied — "No test named
  TestResolveIssueBinding_REQ_HOOKBIND_T1 exists" while behavior was covered.
- T2 assessment red: contracted names TestBindingResolutionChain_REQ_HOOKBIND_T2
  etc. absent; remediation commit e1d38e7e added them.
- T3 assessment red/yellow: same pattern (98263b5b, remediation follow-up).
- `arm render-context HOOKBIND-T2 --format agent` core_spec contains no
  acceptance-criteria layer.

## Suggested Follow-Up

Include the acceptance criteria (or the full issue contract used by
`arm review prepare`) as a layer in `arm render-context --format agent`.
