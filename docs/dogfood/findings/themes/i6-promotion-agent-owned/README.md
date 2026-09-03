# Theme: I6 Promotion Is Agent-Owned

## Summary

`done` ≠ `merged` (I6) is load-bearing: the next wave is not ready until
blockers are `merged`. GitHub and `origin/main` already know the work
landed. Armature does not, unless an agent types `arm merged`.

The designed auto path (`arm sync`, local post-merge hook) is dead on
arrival: `DetectMerges` skips every `done` issue with empty `Branch`,
`arm claim --worktree` creates `task/<id>` but never writes that name
onto the issue, and GitHub merge never runs the local hook. Optional
`--branch` / `--pr` on transition/merged are not a fix — agents will not
reliably pass flags even when skills ask them to.

Restart tax is reconstruct-and-`--force`, not implementation. A loop that
surveys while T4 is still `done` reports an empty wave and stops, even
after the PR is on main.

The 2026-08-28 → 2026-09-01 findings settle how deep this goes. `arm sync` is
not merely blind to squash and rebase merges — it is a **structural no-op for
every `done` issue in this repository and always has been**: 80 of 80 `done`
issues carry no `branch`, so `DetectMerges` `continue`s before any merge check
runs. Only 6 op lines in the entire `_armature` history carry `branch` at all.
Fixing merge *detection* would change nothing until `branch` is recorded where
it is already derived.

Downstream of that, the manual path is itself unsafe. `arm merged` writes to
whatever `armature.ops-worktree-path` points at (a leftover `/tmp` prfix
checkout, in one case), exits 1 after the transition has already succeeded, and
— even when everything works — cannot promote a parent whose children include a
`cancelled` one, because `RunRollup` counts `cancelled` as unmerged forever.

## Evidence

- [I6 promotion is agent-owned: claim never records Branch](../../raw/2026-08-17T0233Z-5207ee28-coordination-i6-promotion-agent-owned-metadata.md) — T4 PR #109 and T10 PR #108 on `origin/main`; issues still `done`; `arm sync --into main --dry-run` printed `No merged branches detected`. Zero S10 ops ever carried `branch`. `pr` only appears when someone typed `--pr`.
- [Human merge protocol leaves arm behind GitHub](../../raw/2026-08-15T2036Z-5207ee28-coordination-manual-merge-protocol-stalls-session-restart.md) — Next session spent the whole turn reconciling handoff vs `arm list` vs GitHub, then `arm merged --force` ×4 because worktrees (and hook logs) were already gone.
- [`arm merged` gates on the stale snapshot `arm transition` refuses to trust](../../raw/2026-08-12T0204Z-claude-tooling-arm-merged-reads-stale-snapshot-after-transition.md) — Cross-listed under [missing-remediation-verbs](../missing-remediation-verbs/README.md). The closeout pair does not share a source of truth.
- [`arm sync` skips every done issue because `branch` is never recorded](../../raw/2026-08-31T1142Z-claude-workflow-sync-skips-every-issue-branch-never-recorded.md) — The root cause, quantified. `internal/sync/sync.go:24` `continue`s on `issue.Branch == ""`; `arm claim --worktree` derives `task/<id>` via `DeriveBranchName`, creates it, and writes a claim op carrying `ttl`/`worktree_path`/`claim_token` — never the branch. 80/80 `done` issues affected. `arm worktree gc` is gated on terminal status, so worktrees accumulate and `arm worktree list` reports them as `orphan`s.
- [Post-merge hook runs but misses an asynchronous rebase merge](../../raw/2026-08-28T1247Z-codex-workflow-post-merge-hook-misses-async-rebase.md) — PRs #113/#114. The hook executed and printed `No merged branches detected.`; GitHub's stacked rebase merge had rewritten SHAs so the pushed tip was not an ancestor of `main`. Zero exit, no lifecycle action, worktrees retained.
- [A leftover `/tmp` PR-122 ops checkout stole `armature.ops-worktree-path`](../../raw/2026-08-31T0110Z-5207ee28-workflow-ops-worktree-hijack-drops-merged-ops.md) — Seven `--pr 116..122` transitions landed on a detached HEAD in `/tmp`; canonical `.armature` never moved. `arm show` read the configured path and reported `merged`; removing the `/tmp` tree would have dropped the only ref to those ops.
- [`arm merged` exits 1 after the transition already succeeded](../../raw/2026-08-31T1150Z-claude-workflow-arm-merged-reports-failure-after-succeeding.md) — The inverse shape: op appended, checkout removed, registration gone — only the `.git/worktrees/<id>` unlink failed with WSL `EBUSY`, and the command reported `general_error`. Eight stale admin dirs show this has been happening for a while, so `git worktree prune` never converges.
- [`RunRollup` counts a cancelled child as unmerged, permanently stranding the parent](../../raw/2026-09-01T1156Z-claude-materialize-rollup-strands-parents-with-cancelled-children.md) — `engine.go:607` tests `child.Status != ops.StatusMerged`, while `reconcile.go:250` already encodes the intended `merged || cancelled` predicate. Descoping one task silently prevents its parent from ever completing. `E6-S6` (21 merged / 2 cancelled) and `SMTC-S1` (12/1) had to be promoted by hand.

Related display hole (write lands, agent cannot see it): [agent-facing-views-omit-state](../agent-facing-views-omit-state/README.md).

The inverse failure — `merged` recorded for work never on main, and "No merged
branches detected" printed for a population that was never examined — is curated
under [unknown-recorded-as-answered](../unknown-recorded-as-answered/README.md).

What evidence survives once someone promotes by hand — and whether the available
statuses can express the truth at all — is curated under
[merge-evidence-not-durable](../merge-evidence-not-durable/README.md).

## Candidate Follow-Ups

- Persist the derived branch on the claim op. Do not ask agents for `--branch`. `DeriveBranchName` already computes it at claim time, so this costs nothing and removes the dependency on operator memory. Nothing else in this theme is worth fixing first — merge-strategy work is unreachable behind the empty-`Branch` guard.
- Fix `RunRollup` to treat `cancelled` as satisfying rollup (mirroring `reconcile.go:250`), with one guard: require at least one merged child, so a wholly-descoped story cannot claim delivery.
- Refuse to treat an unbound `/tmp` checkout as the ops worktree when `.armature` already has `_armature`; have `arm merged` / `arm doctor` warn when `armature.ops-worktree-path` is not the live `_armature` worktree.
- Make administrative-directory cleanup non-fatal: once the `merged` op is durable and the checkout is gone, a failed `.git/worktrees/<id>` unlink is a warning naming the leftover path, not `general_error`.
- Make `arm sync` decide `merged` from git/GitHub evidence, including squash (dogfood L37). Keep `arm merged` as the explicit override.
- Accept "PR/branch is on `origin/main`" as I6 evidence when no worktree remains, so `--force` is not required to tell the system what GitHub already knows.
- Do not treat a skill sentence as the close-the-loop step.
