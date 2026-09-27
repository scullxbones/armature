---
date: 2026-09-27
agent: cursor
area: validation
task: LNGHZN-S11 PR 284
tags: [validate-graph, W1, local-vs-ci]
---

# CI validate-graph sees origin/_armature; a docs checkout often does not

## User Goal

Make PR 284's `validate-graph` job match a local `arm validate --ci` run after
the planner reported the local command green.

## Observed

CI `validate-graph` attaches `origin/_armature` then runs `make validate-graph`
(`./bin/arm validate --ci`). That path is strict: 0 errors and 11 W1
scope-overlap warnings still fail the job.

A checkout of `docs/LNGHZN-S11-promotion` has no `.armature` worktree. The
same `arm validate --ci` against an unattached or stale ops tree does not
see CLAIMORD W11/W12/W13/W21 plus LNGHZN-S11 T1/T2/T3 together, so it can
exit 0. After `git worktree add .armature origin/_armature`, the local
command reproduced the 11 W1 findings.

The overlaps were real: LNGHZN-S11 tasks share `docs/commands.md`,
`cmd/armature/transition.go`, `cmd/armature/helpers.go`, and
`internal/materialize/engine.go` with in-flight CLAIMORD tasks and had no
ordering edge. One `arm link --source LNGHZN-S11-T1 --dep CLAIMORD-W21`
cleared them via the existing transitive `blocked_by` chain.

## Impact

A planner can release a story whose local validate is green and whose CI
integration door is red, because the two commands were not looking at the
same graph. That is the #284 failure mode.

## Evidence

- CI run: https://github.com/scullxbones/armature/actions/runs/36356237702
- Local after attaching `origin/_armature`: `arm validate --ci` →
  `validation failed with 0 error(s) and 11 warning(s)` (all W1)
- `.github/workflows/ci.yml` job `validate-graph` fetches
  `refs/heads/_armature` before `make validate-graph`

## Suggested Follow-Up

Publish-path validation must fetch/rebase onto the current remote ops tip
and run the same `arm validate --ci` contract before `git push origin
_armature`.
