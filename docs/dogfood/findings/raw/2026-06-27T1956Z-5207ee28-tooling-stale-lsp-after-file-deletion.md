# LSP Showed Duplicate-Declaration Errors After File Deletion That Didn't Exist

**Date:** 2026-06-27  
**Writer:** 5207ee28 (coordinator)  
**Area:** tooling  
**Task:** Verifying sonnet remediation of review findings

## What the agent was trying to do

Confirm that a sonnet subagent had correctly deleted `runner.go` and `runner_test.go` from `internal/harnesshook/` after moving their types into `hook.go` and `hook_test.go`.

## What happened

After the subagent reported completion, the LSP emitted diagnostics:

```
runner.go: PolicyResolver redeclared in this block [DuplicateDecl]
runner.go: RunResult redeclared in this block [DuplicateDecl]
hook.go: PolicyResolver redeclared in this block
hook.go: RunResult redeclared in this block
runner_test.go: mockResolver redeclared in this block
hook_test.go: mockResolver redeclared in this block
```

Running `ls internal/harnesshook/` confirmed `runner.go` and `runner_test.go` were gone. Running `go build ./internal/harnesshook/...` succeeded with no output. Running `go test ./internal/harnesshook/...` passed. The diagnostics were stale LSP cache artifacts.

## How it changed behavior

The diagnostics caused ~3 minutes of investigation (reading file lists, running go build/test) to confirm the code was actually clean. The coordinator nearly acted on false diagnostics to "fix" a problem that didn't exist.

## What would have helped

- The coordinator should default to `go build` / `go test` as ground truth over LSP diagnostics when they conflict
- Note to add to CLAUDE.md or coordination skill: "LSP diagnostics may be stale after file deletions — use `go build ./...` as the authoritative check"
