---
date: 2026-08-02
agent: claude
area: workflow
task: PR #88 review triage
tags: [delivery-gate, story, transition, gate-bypass]
---

# Story delivery gate can be silently bypassed by transitioning from the wrong checkout

## User Goal

Triaging open automated review comments on PR #88 (LNGHZN-S4, delivery gate) before
closing it out, to decide which findings block merge vs. defer.

## Observed

An independent read of `cmd/armature/transition.go` (`isClaimedWorktreeForIssue`,
~line 180, and the `runGate` decision for `story`-type issues) confirmed a real gap:
`isClaimedWorktreeForIssue(gateRepoPath, issueID)` only inspects the `armature-issue-id`
marker of the checkout `arm transition` is actually invoked from. If a story is claimed
into worktree A (via `arm claim` in worktree mode) but `arm transition <story> --to done`
is run from a *different* checkout — e.g. the coordinator's own main repo — the probe
reads that other checkout's marker (absent, or for a different issue), returns `false`,
and the delivery gate is skipped entirely. The op is appended with
`SkippedDeliveryGate: false`, so the audit trail shows no override even happened.

This is worse than `--skip-delivery-gate`, which at least leaves an auditable record.
Here, a dirty tree, out-of-scope commits, or missing conventional-commit reference on
the actual claimed worktree (A) can pass `done` with zero record that the gate never
ran. Only affects `story`-type transitions — task/bug/feature always run the gate and
fail closed on `VerifyIssueWorktreeBinding`/`VerifyIssueBranchBinding` mismatches,
since those checks live *inside* `runDeliveryGateCheck`, which this bypass never
reaches.

## Impact

Deliberately deferred rather than fixed in PR #88 or filed as a new story: the PR had
already dragged on and the user wanted to close it out. Captured here instead so the
gap isn't lost. Concrete repro for whoever picks this up:

```
arm claim <story-id> --worktree <path>   # story claimed into worktree A
# ...leave worktree A dirty / commit out-of-scope files there...
cd <main-coordinator-checkout>            # NOT worktree A
arm transition <story-id> --to done --outcome "..."   # succeeds, gate never ran
```

## Evidence

- `cmd/armature/transition.go:328-338` (`isClaimedWorktreeForIssue`) — resolves the
  git dir of `worktreePath` (the invoking checkout, via `--repo`/`.`), not the actual
  worktree the story was claimed into.
- `cmd/armature/transition.go:178-181` — `runGate` for `story` depends entirely on
  that probe's boolean.
- `deliverygate.VerifyIssueWorktreeBinding` / `VerifyIssueBranchBinding` — the checks
  that would have caught a wrong-checkout mismatch — only run inside
  `runDeliveryGateCheck`, which is unreached when `runGate` is false.
- Independent review (Opus subagent, 2026-08-02) confirmed this as a real bug, real
  P2 (arguably worse in effect than the tag implies, since it's a silent bypass rather
  than a recorded override).
- PR #88 review thread `PRRT_kwDORnVQE86Vp38f` on `cmd/armature/transition.go:180`
  (chatgpt-codex-connector) — original report.

## Suggested Follow-Up

Fix direction (not yet filed as a story): derive "is this story actually claimed into
a worktree" from repo-global state (the claim op / materialized index / `git worktree
list` cross-checked against each worktree's own marker) rather than trusting the
invoking checkout's own marker file — and fail closed (require the gate) whenever a
claimed worktree exists for the issue, regardless of which checkout the transition
command happens to run from.
