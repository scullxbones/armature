---
area: workflow
writer: claude
---

## What I was trying to do

Ran `/prfix` on PR #66 (armature) with the user's explicit three-tier chain:
haiku fixes findings via TDD → opus reviews and expands scope if needed →
sonnet implements remaining fixes, commits, pushes, replies, and resolves
threads. This time PR #66 only had one unresolved review thread left (prior
runs had already cleared others — see [[project_armature_rename]] history
and earlier same-day findings on this same PR).

## What happened

- Dry-run classification found exactly one unresolved thread (P2, codex
  bot, bootstrap.go:868, about legacy config not being committed to the
  `_armature` branch during migration). Confirmed VALID by reading current
  code before dispatching any agent.
- Haiku implemented a real TDD fix (failing test first, then reorder
  config-load/write ahead of the migration commit, include config in the
  same scoped commit as ops data) and reported green `make build`/`lint`/
  `test`.
- Opus independently re-verified the gates itself (didn't trust the haiku
  report), read the full function for context, and checked the fix against
  the specific known failure modes from prior runs on this same PR
  (commit-sweep, error swallowing, precondition-gating regressions). This
  time it found **no blocking or should-fix issues** — only optional nits
  it explicitly said not to bother with. This is a contrast to the earlier
  same-PR run where opus caught a real ordering/atomicity bug the haiku
  tier introduced as a side effect.
- Sonnet finalized cleanly: independently sanity-checked the diff, reran
  gates, staged only the two relevant files (correctly leaving three
  unrelated untracked dogfood-finding files alone without being told which
  files existed), committed, pushed, replied in-thread with the commit SHA,
  and resolved the thread — verified via GraphQL that `isResolved: true`.

## How it changed behavior, confidence, or time spent

The chain isn't always going to catch a bug at the opus stage — sometimes
the haiku fix is just correct, and the value of the review stage on those
runs is confirmation rather than catch. That's still useful signal (an
independently-rerun green build plus a reviewer that goes looking for the
project's known failure classes and comes up empty is stronger evidence
than a single green CI run), but it means the chain's cost (three
sequential dispatches, ~400s + ~190s + ~120s here) isn't always "buying" a
caught bug — worth tracking the catch rate over time to judge whether the
fixed 3-tier chain is worth it for single-small-finding PRs versus
skipping straight to a stronger single-tier agent.

## Evidence

- Opus's independent verdict, verbatim: "the fix is solid... no blocking or
  should-fix issues... it re-ran make build/lint/test itself rather than
  trusting the haiku report."
- Sonnet correctly avoided staging `docs/dogfood/findings/raw/*.md`
  untracked files without being given an explicit file list — it derived
  the scope from `git status` and the task description alone.
- Final commit `ec4f2ea8` on `feat/SB-ELIM`, review reply at
  https://github.com/scullxbones/armature/pull/66#discussion_r3508678718,
  thread `PRRT_kwDORnVQE86Nqd_G` confirmed resolved via GraphQL.
