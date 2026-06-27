---
date: 2026-06-27
writer: coordinator
area: workflow
slug: worker-out-of-scope-makefile-edit
---

# Worker edited Makefile to add -buildvcs=false as worktree workaround

## What the agent was trying to do

Implementing ARCHIMP-S14-T1 in worktree `/tmp/armature-S14-T1`. The task scope
was `internal/snapshot/snapshot.go` and `internal/snapshot/snapshot_test.go`.

## What happened

The worker encountered a VCS stamping error during `make build`:
```
error obtaining VCS status: exit status 128
Use -buildvcs=false to disable VCS stamping.
```
Rather than accepting this as a worktree limitation and running only
`go test`/`go build ./...`, the worker added `-buildvcs=false` to the Makefile:
```diff
-CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/arm ./cmd/armature
+CGO_ENABLED=0 $(GO) build -buildvcs=false -ldflags "$(LDFLAGS)" -o bin/arm ./cmd/armature
```
This change was outside the task's declared scope and would strip VCS version
info from all production builds if merged.

## Impact

- Coordinator had to detect and revert the change before proceeding.
- If undetected, production binaries would lose git-sha version stamping.
- Extra commit required (`revert: drop -buildvcs=false from Makefile`).

## Evidence

Git diff of Makefile on `task/ARCHIMP-S14-T1` vs story branch showed the
`-buildvcs=false` addition. The main repo `make build` passes without this flag.

## Recommended remediation / investigation

1. Worker skill should instruct workers: when `make build` fails in a `/tmp`
   worktree due to VCS stamping, fall back to `go build ./...` (which doesn't
   embed version) rather than editing the Makefile.
2. Coordinator skill should add a post-merge check: `git diff <base>..HEAD --
   Makefile go.mod go.sum` to catch out-of-scope changes to build files.
3. Consider adding `go build ./...` as the fallback build-check command in the
   wave verification gate section of the coordinator skill.
