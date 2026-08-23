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

## Evidence

- [I6 promotion is agent-owned: claim never records Branch](../../raw/2026-08-17T0233Z-5207ee28-coordination-i6-promotion-agent-owned-metadata.md) — T4 PR #109 and T10 PR #108 on `origin/main`; issues still `done`; `arm sync --into main --dry-run` printed `No merged branches detected`. Zero S10 ops ever carried `branch`. `pr` only appears when someone typed `--pr`.
- [Human merge protocol leaves arm behind GitHub](../../raw/2026-08-15T2036Z-5207ee28-coordination-manual-merge-protocol-stalls-session-restart.md) — Next session spent the whole turn reconciling handoff vs `arm list` vs GitHub, then `arm merged --force` ×4 because worktrees (and hook logs) were already gone.
- [`arm merged` gates on the stale snapshot `arm transition` refuses to trust](../../raw/2026-08-12T0204Z-claude-tooling-arm-merged-reads-stale-snapshot-after-transition.md) — Cross-listed under [missing-remediation-verbs](../missing-remediation-verbs/README.md). The closeout pair does not share a source of truth.

Related display hole (write lands, agent cannot see it): [agent-facing-views-omit-state](../agent-facing-views-omit-state/README.md).

The inverse failure — `merged` recorded for work never on main, and "No merged
branches detected" printed for a population that was never examined — is curated
under [unknown-recorded-as-answered](../unknown-recorded-as-answered/README.md).

## Candidate Follow-Ups

- Persist the derived branch on the claim op. Do not ask agents for `--branch`.
- Make `arm sync` decide `merged` from git/GitHub evidence, including squash (dogfood L37). Keep `arm merged` as the explicit override.
- Accept "PR/branch is on `origin/main`" as I6 evidence when no worktree remains, so `--force` is not required to tell the system what GitHub already knows.
- Do not treat a skill sentence as the close-the-loop step.
