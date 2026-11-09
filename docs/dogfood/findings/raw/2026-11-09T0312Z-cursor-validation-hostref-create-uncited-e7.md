---
date: 2026-11-09
agent: cursor
area: validation
task: "https://github.com/scullxbones/armature/issues/435"
tags: [hostref, E7, accept-citation, validate-graph, dogfood]
---

# Host-ref dogfood create without --source trips CI validate-graph E7

## User Goal

Bind GitHub issue #435 as an external-ref while delivering the ParallelMovingTip flake fix, then open a Ready PR whose validate-graph check stays green.

## Observed

`arm create` (no `--source`) then `arm ref` produced `bug-1794190919` on ops tip `7a46391e`. Local/CI `arm validate --ci` / `validate-graph` failed with `E7 uncited node: bug-1794190919`. Remediatied with `arm sources accept-citation bug-1794190919 --ci --rationale "…"` and `arm push-ops` → ops tip `6394667f`. Same class as the #463 PJ concern.

## Impact

Side-channel host-ref dogfood on a Ready PR can red a required CI check even when the code fix is sound. Agents must cite or accept-risk before STACK-READY, not after merge pressure.

## Evidence

- Create + external-ref commits on `_armature` at `7a46391e`
- CI job `validate-graph` failed on PR #464 tip `467f7a6e`
- Citation-accepted push: `6394667f`

## Suggested Follow-Up

Host-ref dogfood brief: require `arm create --source …` or immediate `accept-citation` before `push-ops` / Ready PR.
