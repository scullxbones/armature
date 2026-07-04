---
date: 2026-07-04
agent: claude
area: workflow
task: prfix remediation of PR #69 (HOOKBIND harness hook binding resolution)
tags: [prfix, subagents, review, harness-hook]
---

# Multi-model prfix pipeline on PR #69 worked end-to-end; rename left legacy binding-file debt

## User Goal

Remediate all unresolved review threads on PR #69 using a staged pipeline:
haiku subagents fixing each finding via TDD, a fable subagent doing a broad
branch review, a sonnet subagent implementing the expanded findings, then
commit/push/reply/resolve.

## Observed

- All 3 unresolved threads classified VALID against current code; each haiku
  agent produced a red-green fix with `make build/lint/test` green, sequentially
  (shared working tree prevents parallel `make test`).
- The fable broad review confirmed the 3 fixes sound but found 6 more issues,
  the most important being that the legacy `armature-task-id` fallback (added
  for finding 1) was only applied in one of three places that read the binding
  file — the HOOKBIND-T1 rename (d52d78be) left multiple independent readers
  of the same file, so a compat fix in one didn't cover session-level or
  merged-gate reads.
- Also surfaced: a comment/behavior mismatch on fail-open for untrusted git
  dirs, silent drop of unreadable `.git` file locations, and a dead Root
  recomputation — none caught by the original per-finding haiku fixes.
- Stale IDE diagnostic ("unknown field Root") appeared after the third haiku
  fix even though the tree built; re-verification cost one extra gate run.
- `git push` output included `{"status":"pushed","branch":"_armature"}` from
  the armature pre-push hook — terse and unexplained alongside git's own output.

## Impact

The narrow per-finding fix loop (haiku) reliably fixed exactly what was asked
but missed cross-cutting consequences; the broad second-pass review (fable) was
what caught the incomplete legacy fallback that would have shipped a P2. Total
pipeline produced 10 files changed, 361 insertions, one commit, all threads
resolved.

## Evidence

- PR: https://github.com/scullxbones/armature/pull/69, fix commit 2d2a94db
- Duplicated binding-file readers pre-fix: `internal/harnesshook/binding.go`,
  `cmd/armature/harness_hook.go:24` (resolveIssueBinding),
  `cmd/armature/merged.go:74` (resolveIssueWorktree); now unified behind
  `ReadIssueBindingFile`.

## Suggested Follow-Up

When a rename touches an on-disk artifact name (like the binding filename),
grep for all readers and route them through one helper in the same change;
consider an arm doctor check for stray legacy `armature-task-id` files.
