---
date: 2026-08-17
agent: 5207ee28
writer: 5207ee28
area: coordination
task: LNGHZN-S10-T4
story: LNGHZN-S10
tags: [i6, arm-merged, arm-sync, branch-metadata, agent-reliability]
---

# I6 promotion is agent-owned: claim never records Branch, so sync cannot see a landed PR

## User Goal

Start the next LNGHZN-S10 wave. T4 (PR #109) and T10 (PR #108) were already
on `origin/main`. Confirm whether armature had them as `merged`, and if not,
whether something automated should have flipped them.

## Observed

GitHub and git said shipped. Armature said `done`. `branch` and `pr` were
null on both issues. `arm sync --into main --dry-run` printed `No merged
branches detected`. The s10-loop paused to ask permission for `arm merged`.

This is not a T4/T10-specific miss. Every S10 task has null `branch`. `pr`
appears only when someone typed `arm merged --pr N` (T2/T3/T6/T7). T1 and
T8 are `merged` with no PR field either.

RCA (5 whys):

1. The materializer copies `issue.Branch` / `issue.PR` only from a
   transition payload that already has them.
2. The commands that ran never supplied those fields. `arm claim --worktree`
   created `task/LNGHZN-S10-T4` and wrote `worktree_path`, but the claim
   payload is `{ttl, worktree_path, claim_token}`. Worker `done` used
   `--outcome` only. `arm merged` never ran, so `pr` had no writer.
3. Default agent recipes omit the flags. Worker skill:
   `arm transition ID --to done --outcome "..."`. Coordinator step d:
   `arm merged --issue TASK_ID` (`--pr` optional). T4's own outcome text
   says "worktree is on `task/LNGHZN-S10-T4`" and still did not set the field.
4. Git reality and issue metadata are disconnected. Claim already knows
   `DeriveBranchName` → `task/<id>`. `arm sync` then skips every `done`
   issue with empty `Branch`. A GitHub merge never runs the local
   post-merge hook.
5. Two-phase completion left the join as an optional flag. We cannot rely
   on agents to run commands, even if we ask them to. `--branch` / `--pr`
   are therefore not a fix.

Even a populated `Branch` would not have been enough here: `BranchMergedInto`
is ancestry of the branch tip, and T4's review head `e65bedc6` is not on
`origin/main` after the GitHub merge (squash/rebase; dogfood L37).

## Impact

The next wave cannot start until a human or coordinator types `arm merged`.
That is I6 as ceremony: GitHub already confirmed-on-main; the graph does
not. README / use-case P2 / the post-merge hook promise auto-detect; the
data the detector needs is never written. Session tax: survey, 5-whys,
and a pause instead of claiming T5/T12.

Teaching agents to pass `--branch` would not close this. The system must
record the branch it just created and decide `merged` from git/GitHub
evidence without a hoped-for agent step.

## Evidence

- `arm show LNGHZN-S10-T4 --format json`: `status: done`, `branch: null`,
  `pr: null`. Same shape for T10.
- GitHub PR #109 MERGED at `2026-08-17T02:13:11Z` (`64d98421` on
  `origin/main`). PR #108 MERGED at `2026-08-16T20:50:12Z` (`658528d1`).
- `arm sync --into main --dry-run`: `No merged branches detected`.
- `internal/sync/sync.go`: skip when `issue.Branch == ""`.
- `cmd/armature/claim.go` claim payload: no `Branch`.
- `cmd/armature/merged.go` merge payload: `{To: merged, PR: pr}` — never
  writes `Branch`; writes `PR` only if `--pr` is passed.
- `arm log --issue LNGHZN-S10-T4 --json`: claim → done → reopen → claim
  → done; no `branch`, no `pr`, no `to: merged`.
- S10 merged transitions: T2/T3/T6/T7 have `pr`; T1/T8 do not; **zero**
  S10 ops have `branch`.
- `git merge-base --is-ancestor e65bedc6 origin/main` → false.
- Related: `2026-08-15T2036Z-5207ee28-coordination-manual-merge-protocol-stalls-session-restart.md`
  (I6 debt after human merge); archive L37 (`arm sync` misses squash).

## Suggested Follow-Up

- Persist the derived branch on the claim op (the name claim already
  computed). Do not ask agents for `--branch`.
- Make `arm sync` decide `merged` from git/GitHub evidence, including
  squash (issue-ID on `main`, or `gh pr view --state merged`). Keep
  `arm merged` as the explicit override, not the happy path.
- `pr` is audit sugar; do not block I6 on it. If recorded, take it from
  the merge detector, not from `--pr`.
- Do not "fix" this by adding another skill sentence. Agents will not
  reliably run it.
