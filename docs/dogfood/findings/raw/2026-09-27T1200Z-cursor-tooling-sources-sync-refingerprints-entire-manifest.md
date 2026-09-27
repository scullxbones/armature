---
date: 2026-09-27
agent: cursor
area: tooling
task: CLAIMORD sources add then sync
tags: [df-1, sources, sync, claim-order]
---

# DF-1 — `arm sources sync` re-fingerprints the entire manifest

## User Goal

Sync after `arm sources add` of the claim-order spec so the new source is usable.

## Observed

`arm sources sync` after `arm sources add` of the claim-order spec took ~150s. Several historical filesystem sources were STALE (missing local paths, `/home/brian/...`).

## Impact

Medium. Known theme; did not block apply. Time spent waiting on a full-manifest fingerprint while unreachable historical entries also went STALE.

## Evidence

- Command: `arm sources sync` after `arm sources add` of the claim-order spec
- Happened: ~150s; several historical filesystem sources STALE (missing local paths, `/home/brian/...`)
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`

## Suggested Follow-Up

Sync the new source, or skip unreachable entries without blocking the new OK source.
