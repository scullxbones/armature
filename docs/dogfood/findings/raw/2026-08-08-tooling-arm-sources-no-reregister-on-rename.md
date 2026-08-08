---
date: 2026-08-08
agent: claude
area: tooling
task: LNGHZN-S5 (coordinator, auditor gate)
tags: [arm-sources, sources-verify, auditor, repo-hygiene]
---

# `arm sources` has no way to re-point a source after its file is moved/archived

## User Goal

Pass the armature-auditor's source-freshness gate (`arm sources verify` clean) before story sign-off.

## Observed

`arm sources verify` reported STALE/MISSING sources pointing at `docs/design/dogfooding-learnings.md`, which a *different* story (TOPTIER-S10-T1, commit `60db35fe`) had `git mv`'d to `docs/archive/dogfooding-learnings.md` without updating the source registration. `arm sources sync` can't help (it fetches from the now-wrong path and fails); `arm sources` exposes only `add`, `link`, `stale-review`, `sync`, `verify` — there is no "update URL / re-point" command. `add` would create a duplicate; `stale-review` targets *changed* content, not a *moved* file.

## Impact

- A cross-story doc archival silently orphaned a source and now fails the global freshness gate for *every* subsequent story's auditor run, with no clean remediation path.
- I could not resolve it within LNGHZN-S5's blast radius and had to document an explicit exception (LNGHZN-S5's own citations are 711/711 clean) rather than block sign-off on unrelated debt.

## Evidence

- `arm sources verify` → `9d392cac ... STALE (cached content exists but last sync failed)`; sync error: `read "docs/design/dogfooding-learnings.md": no such file or directory`.
- `git show 60db35fe --find-renames` → `docs/{design => archive}/dogfooding-learnings.md`.
- `arm sources --help` → no update/re-point subcommand.

## Suggested Follow-Up

Add `arm sources update <id> --url <newpath>` (or `sources move`) to re-point a source and re-fingerprint. Consider a `doctor`/`validate` check that flags source URLs whose filesystem target no longer exists, and a note in any "archive/move docs" workflow to re-point sources. Related: sandbox-vs-real doctor discrepancy in [[2026-08-08-tooling-sandbox-devnull-false-d8-artifacts]].
