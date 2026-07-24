---
date: 2026-07-23
agent: claude
area: tooling
task: LNGHZN-S3 coordination
tags: [lsp, gopls, worktree, false-positive]
---

# LSP (gopls) repeatedly reported false-positive diagnostics for code that built and tested cleanly

## User Goal

Verifying code correctness while implementing/remediating LNGHZN-S3-T1, both
inside a dispatched worker's `arm claim --worktree` location and later in the
main repo checkout.

## Observed

At least five separate times during this session, the automatic LSP diagnostic
surface reported "undefined" symbols, "unused" imports/functions, or
"unknown field" struct-literal errors for code that `go build ./...` and
`go test` confirmed compiled and passed correctly moments before or after.
Examples: `claimPkg.ShouldHeartbeat` reported undefined immediately after a
successful `go build ./...`; `Payload.Source` reported as a nonexistent field
on `ops.Payload` in three different test functions, when the field
demonstrably existed and the exact same field access passed in tests already
proven green. The diagnostics consistently included a workspace-boundary
warning ("This file is within module .../tmp/claude/arm-task-..., which is not
included in your workspace") when the affected file lived inside an
`arm claim --worktree` path outside the main repo's declared Go workspace,
suggesting gopls was analyzing worktree files against a stale or
mis-scoped module view rather than the real `go build` toolchain.

## Impact

Each occurrence required stopping to independently re-verify via a real
`go build`/`go vet`/`go test` run before trusting or dismissing the diagnostic,
costing a short but repeated tax across the session (5+ times) that would
otherwise have been spent proceeding directly. Some of these false positives
appeared for code in the *main* repo checkout as well, not only inside
worktrees, though they were more frequent for worktree-resident files.

## Evidence

- Diagnostic: `harness_hook.go: undefined: claimPkg.ShouldHeartbeat` and
  `unknown field Source in struct literal of type ops.Payload`, immediately
  following a `go build ./... ` run that exited 0 with no output.
- Diagnostic: `heartbeats[0].Payload.Source undefined (type ops.Payload has no
  field or method Source)` reported three separate times across different
  edits, while `go test -run ...` runs exercising that exact field access
  passed (`--- PASS`) each time.
- Workspace-boundary warning accompanying worktree-file diagnostics: "This file
  is within module '.../tmp/claude/arm-task-LNGHZN-S3-T1', which is not
  included in your workspace."

## Suggested Follow-Up

- No repo-side fix is likely possible for this (it's an editor/LSP tooling
  limitation, not an armature behavior), but worth noting explicitly in
  worker-facing or coordinator-facing guidance: when working inside an
  `arm claim --worktree` location (or any git worktree outside the main
  module's declared workspace), IDE diagnostics should not be trusted over a
  real `go build`/`go test` run — always re-verify via the actual toolchain
  before accepting or acting on an LSP-reported compile error in this setup.
