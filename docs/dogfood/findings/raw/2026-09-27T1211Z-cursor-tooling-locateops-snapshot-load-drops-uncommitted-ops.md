---
date: 2026-09-27
agent: cursor
area: tooling
task: CLAIMORD-W14/W21 claim tests
tags: [df-12, locateops, snapshot, claim-order]
---

# DF-12 — git LocateOps on snapshot Load dropped uncommitted test ops

## User Goal

Run `go test ./cmd/armature/ -run TestClaimIgnoresNonTaskIssues` with uncommitted JSONL on disk.

## Observed

Auto-detecting `.git` on `filepath.Dir(opsDir)` walked the ops git history and skipped JSONL that was only on disk (`issue not found`). Restated on the W21 tip: walked git history and skipped JSONL that existed only on disk (`issue not found`).

## Impact

High if left on. Fixed by not auto-wiring snapshot.Load to LocateOps (reverted auto-wiring).

## Evidence

- Slice: CLAIMORD-W14/W21; CLAIMORD-W14 (fixed on W21 tip)
- Command: `go test ./cmd/armature/ -run TestClaimIgnoresNonTaskIssues`
- Happened: auto-detecting `.git` on `filepath.Dir(opsDir)` walked the ops git history and skipped JSONL that was only on disk (`issue not found`)
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc` (DF-12 appears twice; same finding)
- Fixed in CLAIMORD-W14/W21 by not auto-wiring snapshot.Load to LocateOps; reverted auto-wiring

## Suggested Follow-Up

Default snapshot load stays file concat; `Options.OpsWorktree` is the explicit incremental path (W14 test). Fixed in CLAIMORD-W14/W21.
