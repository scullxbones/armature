---
writer: 5207ee28-cdd8-48e6-98dc-7da179d4a40d
area: tooling
date: 2026-06-22
---

## Finding

`gh pr view --json headSha` fails with "Unknown JSON field: headSha". The field doesn't exist in `gh`'s PR JSON schema. The available SHA-related fields are `mergeCommit`, `potentialMergeCommit`, and `headRepository`, none of which give the head commit SHA directly. Obtaining the PR head SHA requires `git rev-parse HEAD` after checking out the branch, or parsing `gh api repos/<owner>/<repo>/pulls/<num>` directly.

## Context

Encountered while running prfix on PR #57 to verify the head SHA before finalizing changes. The prfix skill instructs verifying "head SHA" before finalize, but the obvious `gh pr view --json headSha` path fails with a hard error.

## Impact

Low — workaround is `git rev-parse HEAD` — but any skill/workflow that tries to gate on PR head SHA via `gh pr view` will hit this unexpectedly.
