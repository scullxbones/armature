# ADR 0014: Two-Tier Gates and Evidence-Based Acceptance

## Status

Proposed

## Principles touched

I3, I5, I6, I7

## Context

Coordinating `LNGHZN-S9` showed the full deterministic gate (~10 minutes:
mutation, duplicated test/coverage runs, cross-compilation) being rerun for
every small remediation, and worker prose reports of green gates that were not
reproducible at integration (see
`docs/dogfood/findings/raw/2026-08-14T2130Z-5207ee28-coordination-worker-green-gate-not-reproducible.md`).
Reviewers marked gate criteria indeterminate because no citable execution
record existed, forcing expensive reruns. The design and ratified decisions are
in `docs/design/gate-efficiency.md`.

Alternatives considered: keeping a single gate with caching (complex,
non-deterministic invalidation); self-reported gate results in a structured
schema (re-opens the reproducibility hole); hardcoding repo build knowledge
into `arm` (couples the product to this repository's toolchain); a waiver
ledger for validation warnings (rejected as overkill — warnings become errors
outright).

## Decision

1. Gates split into two profiles. A **fast gate** (diff-routed, deterministic)
   is the iteration gate; the **full gate** is unchanged in content and is
   mandatory exactly twice per task lifecycle: final task head and story
   integration. Only a full gate confers delivery (I5).
2. Gate commands are **repository configuration**, not product knowledge: a
   `gates` map in `.armature/config.json`, with the profile name `full`
   reserved as the publish profile. Repos without configuration keep today's
   behavior.
3. `arm gate run <profile>` executes the configured command and appends an
   evidence op — profile, command, head SHA, start/end, exit status — to the
   invoking worker's own log (I3). Dirty-tree runs are recorded as uncitable.
   The process that observed the exit status writes the record; self-report
   never counts as evidence.
4. Acceptance keys on evidence: a gate criterion is satisfied iff an evidence
   op shows `exit=0`, `profile=full`, and a SHA equal to the review-bundle
   head. Anything less requires a rerun; "indeterminate" is not an outcome.
   This extends I6 with a sibling distinction: **reported ≠ evidenced**.
5. Review cycles are bounded: comprehensive initial review, consolidated
   remediation, hard-scoped confirmation, at most 3 remediation cycles, then
   mandatory human escalation (I7).

## Consequences

- Small remediations cost seconds, not ~10 minutes; mutation and
  cross-compilation failures can now surface only at the two full-gate
  boundaries, occasionally costing one late remediation cycle.
- Reviewers stop rerunning gates whose outcome is already proven for the exact
  SHA under review, and stop emitting indeterminate verdicts for known passes.
- Gate evidence becomes an append-only, git-native audit trail (I1, I2).
- Follow-up work implied: `make check-fast` routing, the `gates` config schema
  and `arm gate run` command, ReviewBundle evidence inclusion, and skill-text
  updates making the two-tier workflow normative — tracked as the
  gate-efficiency story.
