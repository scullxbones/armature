---
date: 2026-06-27
writer: coordinator
area: tooling
slug: worktree-lsp-bleed
---

# LSP diagnostics from /tmp worktree bleed into coordinator session

## What the agent was trying to do

Dispatched a haiku worker to implement ARCHIMP-S13-T1 in a git worktree at
`/tmp/armature-S13-T1`. The worktree was created by `arm claim --worktree`.

## What happened

Immediately after the worker began editing files in `/tmp/armature-S13-T1`,
`<new-diagnostics>` system-reminder messages appeared in the coordinator's
conversation context reporting LSP errors for those files:

- "This file is within module ../../../../tmp/armature-S13-T1, which is not
  included in your workspace."
- Multiple `undefined: State`, `undefined: Issue` etc. — likely mid-edit
  snapshot before imports were wired up.

## Impact

- Noise in coordinator context: diagnostic blocks pushed unrelated error content
  into the coordinator's attention stream.
- Risk of false alarm: coordinator could misinterpret mid-edit compiler errors
  as a worker failure and intervene prematurely.
- The diagnostics showed "internal package not allowed" errors — this is the
  LSP workspace issue (`go.work` not covering `/tmp`), not a real build error.

## Evidence

Verbatim system-reminder block injected mid-turn:

```
pipeline.go:
  ✘ [Line 11:2] use of internal package github.com/scullxbones/armature/internal/adapters not allowed (go list)
  ✘ [Line 31:44] undefined: Issue [UndeclaredName] (compiler)
  ...
```

## Recommended remediation / investigation

1. Check whether the LSP workspace config can be scoped to exclude `/tmp`.
2. Consider creating worktrees inside the repo directory (e.g., `.worktrees/`)
   rather than `/tmp` so the go.work file covers them automatically.
3. The coordinator skill should note: ignore LSP diagnostic system-reminders
   during worker dispatch — wait for the worker's completion notification.
