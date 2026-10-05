---
curated_by: cursor
date: 2026-10-05
---

# Theme: Timestamp Domains Mix in Tests and Lease Logic

## Summary

Ops replay, lease TTL, and rematerialization tests mix wall-clock, git committer time, synthetic `LastActivity` values, and filesystem mtime. A single backdated op sorts before its own `create`. A 2026 committer time compared to `LastActivity=100` inverts publish-order races. Mtime assertions fail because hooks write as a side effect.

This is the dogfood face of quality-control C6 (clock purity in domain code) and of C2's hermeticity gap: tests that depend on ambient time or the real git config are not deterministic.

## Evidence

- [`LeaseLive compared 2026 committer time to LastActivity=100`](../../raw/2026-09-27T1210Z-cursor-validation-leaselive-timestamp-domain.md) — First-published B lost to A's earlier `op.Timestamp`. CLAIMORD-W12 stored `AcceptAt` LastActivity in the steal/committer `now` domain.
- [`Simulating a stale claim requires backdating the whole ops log, not one op`](../../raw/2026-07-23T2215Z-claude-tooling-backdating-ops-log-for-stale-claim-test.md) — Global timestamp sort before replay; one backdated claim sorts before `create` ("issue not found"). Also listed under [tooling-integration-gaps](../tooling-integration-gaps/README.md).
- [`Mtime-based "no rematerialization" assertion fails due to system arm binary side effects`](../../raw/2026-06-28T1700Z-claude-workflow-test-strengthening-mtime-unreliable.md) — Integration tests that invoke real git inherit hook writes. Also listed under [tooling-integration-gaps](../tooling-integration-gaps/README.md).

## Candidate Follow-Ups

- Keep an injected clock in domain packages (forbidigo already bans bare `time.Now()` outside allowed paths; see the [quality-gap crosswalk](../quality-gap-crosswalk.md)).
- Provide a test helper that shifts a whole ops corpus by a uniform delta instead of rewriting one timestamp.
- Ban mtime as the sole rematerialization assertion; assert on content or explicit clock ticks.
