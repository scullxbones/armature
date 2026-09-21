---
date: 2026-09-21
agent: loops
area: workflow
task: MK reshape planner source registration
tags: [arm, sources, sync, worktree, _armature]
---

# arm sources sync from .armature worktree misses main-tree docs

## User Goal

Register a new filesystem design doc as a source while operating the ops worktree (`.armature` on `_armature`).

## Observed

`cd /workspace/armature/.armature && arm sources sync` tried to open paths like `docs/design/architecture.md` relative to the ops worktree cwd and failed with "no such file or directory" for many existing sources. Those files live on the main checkout, not on `_armature`.

## Impact

Planner "register sources first" step fails unless `--repo` points at the code checkout (or cwd is the main tree). Easy to thrash on MISSING sources and block `dag apply`.

## Evidence

```
cd .armature && arm sources sync --format json
# sync …: read "docs/design/architecture.md": no such file or directory
```

## Suggested Follow-Up

Planner skill: always `arm … --repo <code-root>` when ops live in `.armature` / dual-branch. Or make filesystem provider resolve against the linked code worktree, not ops cwd.
