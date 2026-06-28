---
date: 2026-06-27
writer: coordinator
area: workflow
slug: worker-stray-binary
---

# Worker left stray compiled binary in repo root

## What the agent was trying to do

A worker running in `/tmp/armature-S14-T6` ran `go build` without specifying
an output path (`-o`). When run from a worktree, `go build ./...` or
`go build ./cmd/armature` without `-o` writes the binary to the current
working directory instead of `bin/arm`.

## What happened

A file named `armature` (ELF 64-bit executable) appeared as an untracked file
in the repo root (`/home/brian/development/armature/armature`). Git status
reported it as an uncommitted change, and `gh pr create` warned "3 uncommitted
changes."

## Impact

- Repo root pollution with a compiled binary.
- `gh pr create` warning about uncommitted changes.
- If accidentally staged and committed, a large binary would enter git history.

## Evidence

```
$ git status --short
?? armature
$ file armature
armature: ELF 64-bit LSB executable, x86-64, ...
```

## Recommended remediation / investigation

1. Worker skill should instruct workers to use `go build -o /dev/null ./cmd/armature`
   or `go build ./...` only when the build artifact is irrelevant (which it is
   in most task contexts). The standard fallback should be
   `go build -o /dev/null ./cmd/armature` rather than bare `go build ./cmd/armature`.
2. Add `armature` (the bare binary name) to `.gitignore` alongside `bin/arm`.
3. Workers running in /tmp worktrees should always use `go build ./...` (no
   output binary) or explicitly `-o /dev/null` for build-check purposes.
