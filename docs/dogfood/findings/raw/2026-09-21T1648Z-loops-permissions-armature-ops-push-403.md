---
date: 2026-09-21
agent: loops
area: permissions
task: MATENC-S1 planner release — push _armature
tags: [git, push, _armature, 403, dogfood]
---

# git push origin _armature denied 403 to scullxbones

## User Goal

Publish MATENC-S1 dag apply + transition ops so remote Cloud Agent workers can materialize the new story.

## Observed

```
git push origin _armature
remote: Permission to scullxbones/armature.git denied to scullxbones.
fatal: unable to access 'https://github.com/scullxbones/armature.git/': The requested URL returned error: 403
```

Local `_armature` has commits through `ops: dag-transition MATENC-S1`. Remote cannot see them until push succeeds.

## Impact

Coordinator cannot honestly dispatch Cloud Agents yet — workers on Cursor VMs read remote ops. Planner loop completes locally but release-to-workers is blocked on auth.

## Evidence

Push from `/workspace/armature/.armature` after successful local apply/transition/validate/doctor.

## Suggested Follow-Up

Document which credential (gh, HTTPS token, SSH) the box must use for dual-branch ops push; or use GitHub MCP / low-stakes push path already designed for D12.
