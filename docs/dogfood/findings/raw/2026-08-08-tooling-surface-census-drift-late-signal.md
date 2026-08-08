---
date: 2026-08-08
agent: claude
area: tooling
task: LNGHZN-S5 (coordinator)
tags: [surface-census, census-drift, make-check, discoverability]
---

# New Issue field / CLI command / flag fail census-drift-check, but only at `make check` time

## User Goal

Land code that adds a new materialized Issue field (`worktree_path`, T1) and a new CLI command tree (`arm worktree list|gc` + `--dry-run`, T2), and integrate cleanly.

## Observed

Both additions compiled, linted, and tested green but broke `census-drift-check` (the last step of `make check`):

- T1: `FAIL: Issue field 'worktree_path' in code but not in census`.
- T2: `FAIL: CLI command 'worktree' / 'worktree list' / 'worktree gc' in code but not in census` and `FAIL: Command flag ownership 'worktree gc|--dry-run' in code but not in census`.

Neither the worker nor I anticipated the census requirement; both surfaced only after a full `make check` and required hand-editing `docs/design/surface-census.md` (add rows, update flag ownership, bump counts, keep `state.go:NN` anchors correct — an insert even shifted the `pr`/`assigned_worker`/`updated` anchors).

## Impact

- Extra remediation round per task that touched a tracked surface; the anchor-shifting made the census edits fiddly and error-prone (a later review flagged a stale `pr` anchor).
- The requirement is invisible until the slow terminal gate; easy to miss when a worker only runs `go build`/`go test`.

## Evidence

- `Makefile:123: census-drift-check ... Error 1` twice in this story (once for the field, once for the command+flag).
- Fix commits: census rows for `worktree_path` and the `worktree` command tree + `--dry-run` ownership.

## Suggested Follow-Up

Mention surface-census maintenance in the armature-worker/coordinator skills whenever scope touches `internal/materialize/state.go`, `internal/ops/types.go`, or `cmd/armature/*` command registration. Consider a lighter/faster `arm`-native census lint the worker can run pre-commit, and/or census rows keyed by symbol rather than line-anchored (`state.go:NN`) so field inserts don't shift unrelated anchors.
