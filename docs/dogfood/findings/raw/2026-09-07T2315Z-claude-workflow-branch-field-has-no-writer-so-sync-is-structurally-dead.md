---
date: 2026-09-07
agent: claude
writer: claude
area: workflow
task: AOC-S1 closeout after PR #136 merged
tags: [promotion, arm-sync, detect-merges, branch-field, claim, rollup, i6, i4, agent-output-contract]
---

# `issue.Branch` has no writer, and `branch`/`pr` are absent from every rendered view

## User Goal

Merge PR #136 (the last task of `AOC-S1`) and have Armature promote
`AOC-S1-T3` from `done` to `merged`, then roll the story up.

## Observed

PR #136 merged at `2026-09-07T22:52:44Z`
(`fdf3f3b61babf0e6e21a8815cc67d7a72491d90e`, `MERGED`). Nothing in
Armature moved. The operator ran `arm sync --dry-run --into origin/main`
and `--into main`; both printed **"No merged branches detected."**

## Finding 1 — `Branch` has no automatic writer

`DetectMerges` (`internal/sync/sync.go:23`) `continue`s on
`issue.Branch == ""` before any ancestry check. Prior findings recorded
that no `branch` value appeared on the ops for affected issues and read
that as a data gap. It is not a gap. **Nothing populates `Branch`
automatically:**

- `cmd/armature/claim.go` sets **no** `Branch` in any op payload
  (`rg -n "Branch:" cmd/armature/claim.go` returns nothing) — though
  claim is the one operation that authoritatively knows the branch name,
  because it creates it. It does record `WorktreePath`.
- `issue.Branch` is assigned in exactly one place,
  `internal/materialize/engine.go:231`, from `op.Payload.Branch` in
  `applyTransition`.
- That payload comes from `cmd/armature/transition.go:208`, fed by an
  **optional `--branch` flag**.

Confirmed on this issue: the `done` op is
`{"to":"done","outcome":"..."}` — no branch. So `issue.Branch` was empty,
`DetectMerges` skipped it, and `arm sync` reported success.

The only writer of the field `arm sync` depends on is someone
remembering an optional flag. For any issue claimed through the normal
`arm claim --worktree` flow, `Branch` is empty and `arm sync` is a no-op.
This is the default, not an edge case. The squash-merge blindness in
`BranchMergedInto` is real but sits *behind* a gate nothing opens —
fixing patch-id detection without giving `Branch` a writer changes
nothing.

## Finding 2 — the evidence fields are invisible in every rendered view

`arm show <id> --format json` returns these keys and no others:

```
acceptance, assessment_attestations, blocked_by, blocks, claimed_by,
definition_of_done, id, notes, outcome, parent, priority, scope,
status, title, type
```

There is **no `branch` key and no `pr` key**. The human view omits them
too. The two fields that distinguish *confirmed on main* from *asserted*
are recorded in the ops log and rendered nowhere.

This has a sharper consequence than "a field is missing." It cost this
session two false diagnoses, in the direction nobody is watching for:

1. `arm show AOC-S1-T3 --format json | jq '{branch,pr}'` printed
   `null, null`. jq renders an **absent key** and an **empty value**
   identically. That was read as "the promotion recorded no evidence"
   and written up as a live instance of Failure C — the DAG asserting
   `merged` without evidence.
2. The raw op was `{"to":"merged","pr":"136"}`. The PR *was* recorded.
   The operator used the evidenced path correctly. There was no Failure C
   here at all.

The curated theme `unknown-recorded-as-answered` describes absent checks
rendering as definite answers. This is the mirror image: **present
evidence rendering as definitely absent.** An agent — the primary user
under I4 — cannot tell an evidenced `merged` from an unevidenced one
without reading `.armature/ops/*.log` directly. `outcome`, free text with
no provenance, is rendered prominently; `branch` and `pr`, structured and
provenance-bearing, are not rendered at all.

An earlier retroactive op written during this session records the wrong
conclusion in its `outcome`; per I2 it was corrected by an appended note
(`note-1788824501073857298`), not rewritten.

## Finding 3 — story-level `merged` has no op, so it cannot carry evidence

`AOC-S1` has no `→ merged` transition op. Its `merged` status came
entirely from `RunRollup` deriving it from children. A story promoted
this way has no payload on which a `branch` or `pr` could ever be
recorded — story-level evidence is not merely missing, it is
unrepresentable. (§4a Q2 of the promotion handoff.)

## Impact

Delivered work sits in `done` indefinitely with the detector reporting
success. The operator's recovery — promoting by hand — works and can
carry evidence, but nothing routes them to it and nothing afterwards
shows whether they did.

Note what this instance does **not** show. Review evidence exists
(`.armature/review/AOC-S1-T3-265785cc-wrap.json`,
`AOC-S1-T3-auditor-wrap.json`), so Failure B is absent; and the operator
did record the PR, so Failure C is absent. The detection failure recurs
at full strength anyway. These are independent defects, not three views
of one.

## Evidence

- `gh pr view 136` → `MERGED`, `mergedAt 2026-09-07T22:52:44Z`,
  `mergeCommit fdf3f3b6...`.
- Raw ops, `.armature/ops/`:
  - `["transition","AOC-S1-T3",1788818776,...,{"to":"done","outcome":"..."}]` — no branch
  - `["transition","AOC-S1-T3",1788822473,...,{"to":"merged","pr":"136"}]` — PR recorded
  - no `→ merged` op for `AOC-S1` at all
- `arm show AOC-S1-T3 --format json | jq 'keys'` — the key list above.
- `internal/sync/sync.go:23`, `internal/materialize/engine.go:231`,
  `cmd/armature/transition.go:208`, `cmd/armature/merged.go:419`, and the
  empty result of `rg -n "Branch:" cmd/armature/claim.go`.
- `.worktrees/AOC-S1-T3` absent from disk and `git worktree list`
  (present at `ec5506b8 [task/AOC-S1-T3]` earlier the same session).

## Suggested Follow-Up

For the pending promotion grill
(`~/development/HANDOFF-PROMOTION-DESIGN-GRILL.md`), not for immediate
implementation:

1. **Give `Branch` a writer.** `arm claim` creates the branch and should
   record it on the claim op. Smaller and more boring than patch-id
   parity, and it gates whether any detection redesign can work at all —
   test it *before* the §3 Q1/Q2 patch-id questions, which are downstream.
2. **Render `branch` and `pr`.** Adding them to `IssueJSON` is a
   prerequisite for any I6 enforcement: a gate on evidence an agent
   cannot read is a gate nobody can verify. This belongs with the Agent
   Output Contract work (`AOC`), not only with promotion.
3. Ask whether story-level `merged` should require its own op rather than
   being derived, given Finding 3.
4. Add to the theme: the reporting surface can manufacture a **false
   negative** about evidence, not only a false positive about checks. Two
   agents in two sessions have now mis-derived promotion state from
   rendered views instead of ops; the handoff's own §7 records the same
   class of error. Re-derive from `.armature/ops/` — the rendered view is
   an undeclared proxy.
