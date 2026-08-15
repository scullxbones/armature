---
area: tooling
writer: 5207ee28
date: 2026-08-15T00:18Z
story: LNGHZN-S10
---

# arm claim blocks on an in-progress story's aggregate scope, though no worker holds it

## What the agent-user was trying to do

Claim `LNGHZN-S10-T3` (scope includes `internal/config/config.go`) after its
blocker `LNGHZN-S7-T1` was merged to main.

## What happened

```text
Error: cannot claim LNGHZN-S10-T3: scope overlap with LNGHZN-S7
       (Make configuration honest — wire or delete every dead knob (LH D1))
       — use --force to override
```

`LNGHZN-S7` is a **story**, not a task. Its children at the time:

```text
LNGHZN-S7-T1  merged
LNGHZN-S7-T2  open
LNGHZN-S7-T3  open
```

No S7 task was claimed or in progress. The only thing "holding" the scope was
the story container itself, which sits at `in-progress` because a child was
worked earlier. A story's scope is by design the union of its children's
scopes, so an in-progress story blocks every overlapping task in every other
story, for the entire remaining life of that story.

This is distinct from the directory-prefix false positive recorded in
`2026-08-14T2352Z-…-scope-overlap-matches-on-directory-not-file.md`, and it
survives that fix: the overlap here is genuine at the file level
(`internal/config/config.go` really is in both scopes). What is wrong is *who*
is being treated as a competing claimant.

The claim-time loop in `cmd/armature/claim.go` filters only on status:

```go
if id == issueID || (entry.Status != ops.StatusClaimed && entry.Status != ops.StatusInProgress) {
	continue
}
if claimPkg.ScopesOverlapEx(issue.Scope, entry.Scope, graph, issueID, id) { ... }
```

Nothing excludes non-task issues. `ScopesOverlapEx` does exclude
ancestor/descendant pairs — which is why a story never blocks *its own*
children — but a story in a different subtree is compared like a peer worker.

Meanwhile `internal/validate`'s equivalent W1 check deliberately skips non-task
issues, and has a test saying so by name:

```text
TestW1ScopeOverlap_SkipsNonTaskIssues
  "story-level aggregate scopes should not trigger worker collision warnings"
```

So the two implementations of "do these scopes conflict" disagree on *which
issues to compare*, on top of already disagreeing on *how to match paths*.
That is the third divergence found between them in one session.

## How it changed behavior, confidence, or time spent

It blocked the story's critical-path task behind an unrelated story that has no
active worker and may not have one for weeks. The offered remedy is again
`--force`, the override that is supposed to mean "I reviewed a real conflict and
accept it" — spending it on a case the system's own validate rules say is not a
worker collision.

The deeper cost is trust: the claim guard is the mechanism that makes parallel
agent work safe. Every false block trains the operator to reach past it, and the
guard is only worth having if `--force` stays rare and meaningful.

## What would have helped

- **Compare against tasks only.** Apply the same non-task filter `validate`
  already applies. A story is a container; it does not write files. Claim-time
  collision detection should consider only issues that a worker can hold — those
  of type `task` in `claimed`/`in-progress`.
- **One implementation.** These checks answering differently is the real defect;
  the specific disagreements are symptoms. Both the path-matching rule and the
  which-issues-to-compare rule belong in one place that claim and validate both
  call. (`LNGHZN-S10-T7` unifies the matcher; the issue-filter half needs the
  same treatment.)
- **Name the claimant in the error.** "scope overlap with LNGHZN-S7" reads as
  though someone is working it. Reporting the *type* and *holder* — "story
  LNGHZN-S7, no active claim" — would have made this diagnosable in seconds
  rather than requiring a read of the claim loop.
