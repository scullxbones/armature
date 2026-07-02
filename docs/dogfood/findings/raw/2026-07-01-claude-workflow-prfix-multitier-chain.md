---
area: workflow
writer: claude
---

## What I was trying to do

Ran `/prfix` on PR #66 (armature) with an explicit user-specified multi-tier
subagent chain: haiku fixes findings via TDD → opus reviews and expands scope
→ sonnet implements remaining fixes, commits, pushes, replies, and resolves
threads.

## What happened

- The haiku subagent correctly implemented both review findings (dirty-tree
  guard on `CreateOrphanBranch`; legacy `config.json` migration) with real
  TDD tests, and all three gates (`make build`/`lint`/`test`) passed at that
  stage.
- To keep the working tree clean after migration (satisfying its own new
  dirty-check), the haiku agent added an *extra* untrack+commit step inside
  `migrateLegacySingleBranchOps` that wasn't asked for and wasn't covered by
  the original review comments. This was a reasonable inference but it
  introduced a new, non-obvious ordering/atomicity bug: the commit ran
  ungated and unscoped, before the dirty-check, so it could sweep unrelated
  staged changes into a "chore: migrate" commit or leave the repo
  half-migrated on a commit failure.
- The opus review agent caught this — a bug that green `make test` at the
  haiku stage did not surface, because no test exercised "dirty tree +
  legacy repo" at the same time as "unrelated staged changes present".
- The sonnet agent then fixed the ordering (dirty-check first, before any
  mutation) and scoped/hardened the commit, added a test for the sweep
  scenario, and finished the finalize steps (commit, push, reply, resolve
  threads) cleanly on the first pass.

## How it changed behavior, confidence, or time spent

The three-tier chain (cheap fixer → strong reviewer → mid-tier finisher)
worked as designed: it caught a real bug that the fixing tier introduced as
a side effect of solving its assigned problem, without that bug ever
reaching a human reviewer or being merged. Confidence in the final diff is
higher than it would have been from a single-pass haiku-only fix, and higher
than skipping straight to a fix without an independent review pass. Cost was
three sequential subagent dispatches (~700s + ~300s + ~250s) instead of one.

## Evidence

- Review agent's finding, verbatim: "the migration commit is not gated on
  tree cleanliness and sweeps unrelated staged changes" (bootstrap.go
  ~line 559-563), plus a second related finding about the ignored `Commit`
  error masking a half-migrated state.
- Final commit `516805f7` in scullxbones/armature fixed exactly this,
  confirmed by `make build`/`lint`/`test` all green afterward.
