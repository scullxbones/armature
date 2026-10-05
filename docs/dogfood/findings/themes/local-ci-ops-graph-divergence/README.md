---
curated_by: cursor
date: 2026-10-05
---

# Theme: Local Validate Green, CI Sees a Different Ops Graph

## Summary

`arm validate --ci` is the same flag locally and in GitHub Actions, but the two runs often do not share an ops tip. CI `validate-graph` attaches `origin/_armature` first. A docs or planner checkout with no `.armature` worktree, a stale `origin/_armature` ref, or a push that rebased only after a failed first publish can be locally green and CI-red (or the inverse for environment-dependent delivery-gate tests). Agents treat the local green as shippable.

This is not the [sandbox-environment-vs-gates](../sandbox-environment-vs-gates/README.md) shape (wrong artifacts in a clean command). It is two honest graphs that disagree.

## Evidence

- [`CI validate-graph sees origin/_armature; a docs checkout often does not`](../../raw/2026-09-27T2320Z-cursor-validation-ci-validate-graph-uses-remote-ops.md) — PR 284: local `arm validate --ci` exit 0; CI failed on 11 W1 overlaps that only appeared after `git worktree add .armature origin/_armature`.
- [`Planner publish used a local graph that CI would reject`](../../raw/2026-09-27T2345Z-cursor-validation-planner-push-missed-remote-w1.md) — LNGHZN-S11 landed on origin with those overlaps; `arm push-ops` had not required the CI contract against the remote tip.
- [`git fetch origin _armature` left `origin/_armature` stale after CA publish`](../../raw/2026-09-21T1651Z-loops-workflow-stale-origin-armature-ref.md) — `ls-remote` moved; `git rev-parse origin/_armature` did not until a refspec fetch. Related tooling note also lives under [tooling-integration-gaps](../tooling-integration-gaps/README.md).
- [`make check` fails 5 delivery-gate tests locally on a clean tree while CI passes the same commit`](../../raw/2026-09-02T1045Z-claude-validation-delivery-gate-tests-fail-locally-pass-in-ci.md) — Inverse polarity (local red / CI green) from ambient git config, same consequence: agents stop trusting one of the two greens. Cross-listed under [sandbox-environment-vs-gates](../sandbox-environment-vs-gates/README.md).

## Candidate Follow-Ups

- Every `_armature` publish path should fetch/rebase onto the current remote ops tip, then run the same `arm validate --ci` contract CI uses.
- Document `git fetch origin refs/heads/_armature:refs/remotes/origin/_armature` (branch-only fetch can leave the remote-tracking ref stale).
- Pin test git config so local and CI delivery-gate suites cannot disagree on a clean tree.
