---
curated_by: cursor
date: 2026-10-05
---

# Theme: Source Sync Re-fingerprints the Entire Manifest

## Summary

Adding or verifying one source runs a full-manifest `arm sources sync`. Agents wait minutes, then hunt for the one new OK line among dozens of `synced` / `STALE` rows (often historical paths that no longer exist on this machine). The same capture-volume class shows up on default `arm list` JSON.

## Evidence

- [`Source sync resyncs the entire manifest when adding one planning document`](../../raw/2026-07-05T0400Z-codex-workflow-source-sync-resyncs-entire-manifest.md) — One new Source re-fetched ~60 registered sources; the new source's own result was buried.
- [`arm sources sync` re-fingerprints the entire manifest`](../../raw/2026-09-27T1200Z-cursor-tooling-sources-sync-refingerprints-entire-manifest.md) — After `arm sources add` of the claim-order spec, sync took ~150s; unreachable `/home/brian/...` filesystem sources went STALE.
- [`arm list` default JSON floods agent tool capture`](../../raw/2026-09-21T1646Z-loops-tooling-arm-list-json-flood.md) — Same agent-capture class (~100KB vs 2.8KB `arm ready`). Also listed under [tooling-integration-gaps](../tooling-integration-gaps/README.md).

## Candidate Follow-Ups

- Default sync to newly added or changed sources; keep a `--all` / full-manifest mode for freshness audits.
- Skip unreachable historical filesystem entries without blocking the new OK source.
- Compact `arm list` rows for agent capture (or pagination) without changing the non-TTY JSON envelope.
