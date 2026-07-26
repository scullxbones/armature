# Finding: holistic post-integration review caught two real bugs in the delivery gate that per-task review missed

**Writer:** claude
**Area:** validation

## What I was trying to do
Coordinate LNGHZN-S4 ("transition-time delivery gate"): three sequential
tasks (T1 delivery-gate check package, T2 wire into `arm transition --to
done`, T3 docs), each claimed into its own worktree, implemented by a haiku
subagent, integrated onto `feat/LNGHZN-S4`, then audited and reviewed before
opening a PR.

## What happened
Per-task `armature-reviewer` passes on T1 and T2 (run against each task's own
isolated commit diff) rated both green/yellow — reasonable given each task's
diff in isolation looked correct. It was only the holistic opus code review,
run against the *whole story's* accumulated diff on `feat/LNGHZN-S4`, that
found two real, non-trivial bugs:

1. `internal/claim.IsWithinScope` did exact-path glob matching against scope
   entries without stripping the human-readable ` (new)` annotation that
   `arm create --scope` calls (and every worker in this session) habitually
   append when declaring a file that doesn't exist yet, e.g.
   `"internal/deliverygate/gate.go (new)"`. Real scope data in
   `.armature/state/*/issues/LNGHZN-S4-T1.json` literally contains that
   string. This meant `ScopeContainmentCheck` would spuriously fail on
   essentially any task that adds a new file — i.e. most tasks. It never
   manifested in T1's own per-task review because T1's own transition to
   done happened before the gate (T2) was wired in at all.

2. `getBaseCommit`'s heuristic tried local `main`/`master` before
   `origin/main`/`origin/master`, and the underlying `getMergeBase` was a
   hand-rolled ancestor walk over unbounded `git log` history rather than a
   real `git merge-base` call. In the coordinator's actual workflow
   (sequential tasks on one shared story branch, local `main` never
   fast-forwarded), this picked a stale local `main` and caused T3's own
   self-transition to see T1+T2's already-merged diff as if it were T3's own
   out-of-scope delivery. This was diagnosed and worked around with
   `--skip-delivery-gate` in the moment, then caught for real (and fixed
   properly) by the holistic review afterward.

Neither bug was hypothetical: bug 1 would break the gate for nearly every
future task with new files in scope; bug 2 would break every future
coordinator running >1 sequential task on a story branch without regularly
fast-forwarding local main.

## How it changed behavior, confidence, or time spent
Required a full remediation pass after "merged" status: added
`adapters.Client.MergeBase` (real `git merge-base`) and `LogRange`, reordered
`candidateBaseRefs`, added `stripScopeAnnotation`, six new behavioral tests,
and a second holistic-review pass to independently verify the fixes actually
worked (not just that the commit message claimed they did). This confirms
the workflow's value: per-task review alone would have shipped both bugs.

## Evidence
- `python3 -c "import json; print(json.load(open('.armature/state/default/issues/LNGHZN-S4-T1.json'))['scope'])"`
  → `['internal/deliverygate/gate.go (new)', 'internal/deliverygate/gate_test.go (new)']`
- T3's real transition failure before the fix:
  `ScopeContainment: Delivery diff contains file outside declared scope:
  cmd/armature/cmd_extra_test.go (scope: [internal/skillsembed/skills/armature-worker/SKILL.md docs/use-cases.md])`
  — cmd_extra_test.go was T2's file, reachable because `getBaseCommit`
  resolved to a stale local `main` far behind T1/T2.
  Fix commit: `170dfea4` on `feat/LNGHZN-S4`.
