---
writer: 5207ee28-cdd8-48e6-98dc-7da179d4a40d
area: documentation
date: 2026-06-29T18:23Z
---

# coordinator skill: grep type-prefix assumption caused unreachable guard

## What the agent was trying to do

Running `/prfix` on PR #64 to address review bot comments about the task-scoped
review bundle logic in `armature-coordinator/SKILL.md`. Three Haiku subagents
fixed the reported issues, then an Opus review agent checked their work.

## What happened

Haiku fix #2 (unanchored `--grep="$TASK_ID"`) correctly replaced the pattern
with `grep -q "feat($TASK_ID):"` to prevent prefix collision. But the Opus
review caught a secondary problem: `feat` was hardcoded, silently missing
commits made with `fix(TASK-ID):`, `refactor(TASK-ID):`, `test(TASK-ID):`, or
`docs(TASK-ID):` — all types explicitly permitted by the armature-worker skill.

This same gap made the HEAD-fallback guard (fix #3) permanently unreachable:
the guard lived inside a loop over `TASK_ORDER_BY_APPEARANCE`, which only
contains tasks already found via grep. A task committed with the wrong type
never entered that array, so it was silently dropped from review with no
warning, no error, and no fallback.

## How it changed behavior / confidence

- The Haiku agents each verified with `make build && make lint && make test` and
  all passed — yet both fixes were subtly wrong. Tests pass because the skill is
  documentation (shell pseudocode), not executed code.
- The Opus review caught what unit tests cannot: semantic correctness of
  shell pseudocode in skill markdown.
- A Sonnet agent then fixed the type-prefix regex and moved the guard out of the
  found-only loop into an explicit reconciliation pass over `$WAVE_TASK_IDS`.

## Evidence

- PR #64 thread PRRT_kwDORnVQE86NDgXQ (grep anchoring) — Opus verdict: NEEDS_REVISION
- PR #64 thread PRRT_kwDORnVQE86NDotd (HEAD fallback) — Opus verdict: NEEDS_REVISION / largely unreachable
- Sonnet commit 817f0a33 — final fix with `grep -qE "^[a-z]+\($TASK_ID\):"` and
  reconciliation pass

## Observation

For shell pseudocode embedded in skill markdown, passing `make test` is
necessary but not sufficient to validate correctness. A second-pass semantic
review (human or high-capability model) is needed to catch logic gaps that tests
cannot reach. The multi-agent prfix pipeline (Haiku fix → Opus review → Sonnet
revise) caught this; a single-pass fix would have shipped a broken guard.
