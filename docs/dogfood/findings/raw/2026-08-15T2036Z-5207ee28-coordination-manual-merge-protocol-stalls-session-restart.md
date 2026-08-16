---
date: 2026-08-15
agent: 5207ee28
writer: 5207ee28
area: coordination
task: LNGHZN-S10
story: LNGHZN-S10
tags: [session-restart, arm-merged, handoff, worktree-teardown, i6]
---

# Human merge protocol leaves arm and the handoff behind GitHub, so the next session cannot start work from either

## User Goal

Resume LNGHZN-S10 coordination from a 02:41Z `/tmp` handoff after the human
had kept merging PRs. The question for this capture: did those manual
processes cause undue friction on getting started again?

## Observed

They did. The restart tax was a full reconstruct-and-reconcile pass before
any ready task could be trusted. Almost none of that tax was implementation
work; it was closing loops the protocol leaves open.

The operating protocol (human-owned, written into the handoff, not to be
changed without asking) is:

1. Worker commits to `task/<ID>`, opens its own PR.
2. Coordinator reviews; **human merges**.
3. Agent never pushes to `main`.
4. Only then `arm merged --issue <ID>`.
5. Pause after each PR. Rebase onto `origin/main`, no merge commit.
6. Session state that is "not in issue bodies" lives in `/tmp/handoff-*.md`.

What the next coordinator actually found (checkout `main` = `origin/main` =
`3daf005e`, ~20:31Z):

| Source of truth | What it said |
|---|---|
| Handoff 02:41Z | `#103` is the next merge. T3 must not start. `#102`/`#104`/`#105` CONFLICTING. Ready work is "wait, then T3". |
| `arm list` | T1/T8 `merged`. T2/T3/T6/T7 `done`. T4/T9 blocked. Ready: T11 only. |
| GitHub / `origin/main` | T1 #100, T8 #103, T6 #102, T2 #104, T7 #105, T3 #106 all MERGED. Also #107 (ADR 0015) and S9 #97/#98. |
| `arm ready --explain` | The honest signal: T4 blocked until `arm merged` T3/T6/T7; T9 until T7. |

So three records disagreed, and the document written *for* restart was the
wrongest of the three.

Closing the I6 gap (`done` ≠ `merged`) was not a one-liner. Bare
`arm merged` failed on every GitHub-merged task:

```text
Error: cannot verify hook log for LNGHZN-S10-T6: no worktree or hook-log
       target exists for done issue LNGHZN-S10-T6
Error: issue LNGHZN-S10-T6 cannot be merged: worktree inventory unreadable
       (use --force to override)
```

`.worktrees/LNGHZN-S10-T*` was already empty. The merge-time hook log lives
in the worktree git-dir; tearing that down before `arm merged` makes the
next session spend `--force` four times. That flag is the same one the
earlier T7 note told coordinators **not** to use to skip a scope violation.
The restart agent has to distinguish "force because the human already
merged and the worktree is gone" from "force past I5". The product does
not distinguish them.

A second manual artifact was stale in the same way: `feat/LNGHZN-S10`. The
conversation-start git status named that branch; the workspace was on
`main`. Practice has been per-task PRs to `main`, but the coordinator
skill and the handoff still talk as if the story branch is the integration
head. New work starting from the named story branch would have been
behind ADR 0015, T2/T3/T6/T7, and S9.

Two other manual steps left landmines on the only "ready" tasks:

- **T9** was filed when T2/T3 had not shipped. Its DoD still says
  `check-fast` and `arm gate run` are not-yet-enforceable. Both are on
  `main`. The title still mentions glob-vs-glob (moved to T6 rem). Prior
  HITL said leave title/deps alone. The ready queue surfaces T9 as high
  priority as if that contract were current.
- **T11** was filed by hand from the ADR 0015 write-up. Create payload:
  one comma-joined scope string (validate: phantom path), no
  `acceptance`, DoD > 500 chars, uncited, `confidence: draft`. It still
  appears in `arm ready`. Dispatching it would send a worker at a path
  that is not a file.

`arm validate` after the sync: 3 ERRORs, all T11; 25 overlap warnings
(T7's 208→26 claim still holds in spirit). `arm doctor` D8 flags
untracked `.rtk/filters.toml` against the just-merged S10 tasks — another
checkout-local leftover that is not S10 work.

## Impact

Yes — the manual processes caused **undue** restart friction, in the
specific sense that they are locally reasonable and globally
incoherent.

What is *not* undue, and should stay:

- Humans merge PRs (I7). Accountability does not transfer.
- `done` ≠ `merged` (I6). Self-report is not confirmed-on-main.
- Pause-after-PR and per-task PRs. Those reduced sneak-ons and stacking
  conflicts this story.

What *is* undue is that **none of those steps updates the records the
next session is told to trust**:

- The human merge does not run `arm merged`, and nothing watches
  `origin/main` to do it later. I6 becomes a coordinator archaeology
  problem instead of a one-command confirmation.
- Worktree teardown is treated as cleanup, but it is also destruction of
  the hook-log evidence `arm merged` requires. The next session must
  `--force` or it cannot unblock T4/T9.
- The `/tmp` handoff is written once, at session end, and then the human
  keeps merging. The next agent spends the first pass *disproving* a
  confident START HERE (`#103` next, do not start T3) rather than
  reading `arm ready --explain`.
- Manual `arm create` (T11) is not validated at create time, so the only
  ready-looking new work is unsafe to dispatch.
- Session-local traps ("don't `make install`, S9 is concurrent"; "rebase
  #104/#102/#105") age into false constraints.

Time: the entire restart session was reconstruct + `arm merged --force`
×4 + rewrite the handoff. Zero implementation. That is the wrong shape
for "get started on work again." Confidence: `arm ready` was correct
only *after* the reconstruct; before it, a naive coordinator would
either wait for #103 (already merged) or dispatch malformed T11.

The deeper cost is the same one recorded for false `--force` on T8
claim: every restart that requires `--force` to tell the system what
GitHub already knows trains the operator that I6 is ceremony.

## Evidence

- Handoff at 02:41Z: `/tmp/handoff-LNGHZN-S10.md` (replaced at 20:31Z).
  START HERE was `#103` MERGEABLE; T3 blocked on T8 merge; `#102`/`#104`/`#105`
  CONFLICTING.
- GitHub merge times (all 2026-08-15): #100 02:15Z, #103 02:42Z, #102
  03:30Z, #104 12:05Z, #105 12:05Z, #107 18:05Z, #106 19:05Z, S9
  #97/#98 19:40Z.
- `arm ready --explain --parent LNGHZN-S10` before sync named the
  missing `arm merged` commands explicitly.
- `arm merged --issue LNGHZN-S10-T{2,3,6,7}` refused with
  `no worktree or hook-log target exists for done issue`.
- `git worktree list` / `.worktrees/`: canonical S10 task worktrees gone.
- Notes recorded on T2/T3/T6/T7:
  `note-1786825831646368305` (T6) and siblings — `--force` after GitHub
  merge, not to skip a hook-log violation.
- T11 create op `_armature` `d6e0bdc1`:
  `"scope":["cmd/armature/ready.go,cmd/armature/stalereview.go,..." ]`.
- `arm validate` ERRORs: missing acceptance, DoD exceeds 500 chars,
  uncited node — all `LNGHZN-S10-T11`. Phantom-scope INFO on that same
  comma-joined string.
- Related earlier findings in this pile:
  `2026-06-27T1954Z-5207ee28-coordination-session-recovery-branch-divergence.md`
  (restart reconstruct),
  `2026-08-12T0204Z-claude-tooling-arm-merged-reads-stale-snapshot-after-transition.md`
  (`arm merged` source-of-truth),
  `2026-08-15T1300Z-5207ee28-tooling-prfix-default-cwd-is-story-tree.md`
  (story checkout vs task worktree).

## Suggested Follow-Up

Product, not process theater:

- **`arm merged` should accept "PR/branch is on `origin/main`" as the
  I6 evidence** when no worktree remains. Requiring a live hook log
  after the human's normal cleanup makes I6 un-recordable without
  `--force`. Detect via `gh` / `git merge-base --is-ancestor`, and keep
  `--force` for actual hook-log *violations*.
- **Do not delete the bound worktree until `arm merged` succeeds** — or
  persist `armature-hook.log` outside the worktree git-dir. Today's
  teardown is cleanup and evidence destruction at once.
- **Stop treating `/tmp` handoffs as a source of truth.** `arm ready
  --explain` already diagnosed this restart. A handoff should be
  "protocol + judgements + traps," and should say so in the first line
  (the 02:41Z file said "read the issues," then immediately gave a
  START HERE that went stale).
- **Validate `arm create` before it hits the ready queue.** T11's
  payload would have failed the same checks `arm validate` ran later.
  Draft + phantom scope + no acceptance should not be `arm ready`.
- Optional: run `arm merged` (or `arm sync`, which already has
  merge-detection tests) as a post-merge habit the human can trigger
  from the PR, so the next agent does not inherit a four-task I6 debt.

Do not weaken I6 or I7 to fix this. The defect is that the manual
protocol has no close-the-loop step the human actually runs, and the
product punishes the next session for that omission.
