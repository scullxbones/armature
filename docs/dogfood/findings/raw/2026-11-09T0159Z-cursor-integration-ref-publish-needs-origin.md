---
date: 2026-11-09
agent: cursor
area: integration
task: "437 dogfood host-ref + mcp"
tags: [arm-ref, publish, origin]
---

# arm ref publish fails closed without origin remote

## User Goal

Bind GitHub issue #437 onto a freshly bootstrapped throwaway repo task via `arm ref`.

## Observed

In a `git init` repo with no `origin`, `arm ref` returned `REF-1` citing `git fetch origin` / `'origin' does not appear to be a git repository`. The external-ref op still appeared on a later `arm show` (`host_refs` listed the URL as current), so local write and remote publish are easy to confuse. After adding a local bare `origin` and pushing `main`, a repeat `arm ref` returned `noop: true`.

## Impact

Dogfood / sandbox hosts without a remote look like a total ref failure even when the live name was recorded. Agents may stop before discovering `list --external-ref` already works.

## Evidence

```text
arm ref task-1794189466 https://github.com/scullxbones/armature/issues/437 --role engineering
→ REF-1 publish _armature ... fatal: 'origin' does not appear to be a git repository

arm show task-1794189466 --format agent
→ host_refs: [{url: .../issues/437, role: engineering, current: true}]
```

## Suggested Follow-Up

Distinguish local-op success from publish failure in the `arm ref` agent envelope (or degrade publish to a warning when origin is absent in non-shared clones).
