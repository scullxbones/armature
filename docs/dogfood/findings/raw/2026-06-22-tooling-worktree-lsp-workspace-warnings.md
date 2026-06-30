---
writer: claude
area: tooling
slug: worktree-lsp-workspace-warnings
date: 2026-06-22
---

# Worktree directories cause spurious LSP compiler errors in host IDE

## What I was trying to do

Coordinating ARCHIMP-S12 using the armature-coordinator skill. Workers were dispatched into git worktrees at sibling paths (`../armature-s12-t1`, `../armature-s12-t2`). The coordinator's IDE session remained in the primary repo at `/home/brian/development/armature`.

## What happened

After each worker completed and merged back, the IDE (via gopls) surfaced a wave of diagnostic errors for files inside the worktree paths:

```
harness_hook.go:
  ✘ use of internal package github.com/scullxbones/armature/internal/harnesshook not allowed
  ✘ undefined: adapterExitError
  ✘ undefined: currentCtx
  ⚠ This file is within module "../armature-s12-t2", which is not included in your workspace.
```

The same pattern appeared after T1 (`../armature-s12-t1/cmd/armature/claim.go`).

## Why it matters

- The errors look like real build failures and create false urgency. In both cases `go build ./...` in the primary repo succeeded immediately, and `make check` passed.
- The coordinator's instinct is to stop and diagnose — wasting time on ghost errors.
- A future coordinator agent (or human) unfamiliar with this pattern may abandon a valid implementation.

## Evidence

- `go build ./... && echo "BUILD OK"` → `BUILD OK` (both waves)
- `make check` → all green after T1 wave
- Errors only appeared for `../armature-s12-t{1,2}/...` paths, never for `./...`

## Root cause

gopls workspace discovery picks up `go.mod` files in sibling paths when those paths are on the same filesystem. The worktree shares the module path but lives outside the IDE's root, so gopls treats `internal/` imports as cross-module violations — which they would be if the packages were truly separate, but they're not.

## Potential mitigations

- Add a `go.work` file at the repo root that excludes sibling worktrees, or add worktree paths to a `.gopls-ignore` config.
- Coordinator skill could note: "LSP errors from `../armature-s12-*` paths are expected — verify with `go build ./...` in the primary repo before reacting."
- Tear down worktrees promptly after merge to limit the IDE exposure window.
