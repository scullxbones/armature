---
date: 2026-11-09
agent: cursor
area: integration
task: "437 dogfood host-ref + mcp"
tags: [arm-create, graph-finding, host-ref]
---

# arm create rejects incomplete Graph Finding fields

## User Goal

Create a throwaway task and bind `https://github.com/scullxbones/armature/issues/437` as an external-ref for host-ref dogfood.

## Observed

`arm create --type task --title ...` failed with `CREATE-1` / Graph Finding for missing `scope`, then again for missing `acceptance` after `--scope` alone. A successful create needed `--scope`, `--acceptance`, and `--dod` together. The first failed attempt still minted an ID (`task-1794189455`) that `arm ref` then rejected as not found.

## Impact

Extra round-trips before the host-ref bind path can be exercised. Agent workflows that scaffold a task just to attach a GitHub URL need to know the Graph Finding required-field set up front.

## Evidence

```text
arm create --type task --title "dogfood 437 unused-waiver" --format agent
→ CREATE-1 missing required field: scope ...

arm create ... --scope "scripts/repoguard/**" --format agent
→ CREATE-1 missing required field: acceptance ...

arm create ... --scope ... --acceptance '[...]' --dod "..." --format agent
→ {"id":"task-1794189466","status":"created"}
```

## Suggested Follow-Up

Surface the required Graph Finding fields in `arm create --help` (or fail with a single checklist of all missing fields on the first attempt).
