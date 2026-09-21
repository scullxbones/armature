---
date: 2026-09-21
agent: loops
area: setup
task: MK-1..11 reshape via planner then coordinator
tags: [arm, PATH, dogfood]
---

# arm binary exists but is not on PATH

## User Goal

Run `arm` for planner/coordinator loops on scullxbones/armature (dogfood armature-on-armature).

## Observed

`which arm` / bare `arm` failed with "command not found" even though a built binary was at `/workspace/armature/bin/arm`. Default PATH had `/home/box/go/bin` but not the repo `bin/`.

## Impact

Blocked planner step zero until PATH was extended. Easy to misread as "arm not installed" and waste time rebuilding or searching.

## Evidence

```
type -a arm  → not found
ls /workspace/armature/bin/arm → present
PATH=/home/box/go/bin:/home/box/.grok/bin:/usr/local/bin:...
```

## Suggested Follow-Up

Document in AGENTS.md / verify-armature / planner prerequisites: `export PATH=\"$(git rev-parse --show-toplevel)/bin:$PATH\"` or install to `GOPATH/bin` as part of bootstrap.
