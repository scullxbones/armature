---
date: 2026-09-27
agent: cursor
area: validation
task: CLAIMORD-W12 oporder tests
tags: [df-11, leaselive, acceptat, claim-order]
---

# DF-11 — LeaseLive compared 2026 committer time to LastActivity=100

## User Goal

Run `go test ./internal/oporder/ -run 'TestTwoClones|TestSameSecond'` so first-published B is not inverted by timestamp domain mix.

## Observed

First-published B lost to A's earlier `op.Timestamp` because `LeaseLive` compared 2026 committer time to LastActivity=100.

## Impact

High (would invert every post-C0 race). Fixed in CLAIMORD-W12: `AcceptAt` stores LastActivity as steal/committer `now` so TTL is in the same domain as the challenger.

## Evidence

- Slice: CLAIMORD-W12
- Command: `go test ./internal/oporder/ -run 'TestTwoClones|TestSameSecond'`
- Happened: first-published B lost to A's earlier `op.Timestamp` because `LeaseLive` compared 2026 committer time to LastActivity=100
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc` (unlabeled heading between DF-12 restatements; W12 LeaseLive/AcceptAt finding)
- Fixed in CLAIMORD-W12

## Suggested Follow-Up

`AcceptAt` stores LastActivity as steal/committer `now` so TTL is in the same domain as the challenger. Fixed in CLAIMORD-W12.
