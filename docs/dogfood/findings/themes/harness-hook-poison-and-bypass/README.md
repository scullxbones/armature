---
curated_by: cursor
date: 2026-10-05
---

# Theme: Harness Hooks Poison Commits or Fail Open

## Summary

Harness and git hooks that are supposed to enforce commit discipline instead (1) write `arm` error JSON onto the commit subject when there is no active claim, (2) match only a literal first-token `git` so `sh -c 'git commit'` bypasses the block, or (3) sit in a loop that re-appends identical empty-outcome `done` transitions. A related shape is generated hook shell that silently stops matching after a config-field removal.

Agents respond by disabling `core.hooksPath` for the rest of the session, which also drops the protections the other hooks provided.

## Evidence

- [`prepare-commit-msg` prepends `arm`'s error JSON to the commit subject`](../../raw/2026-09-02T1045Z-claude-tooling-prepare-commit-msg-injects-error-json.md) — `arm show active-claim --field id` writes the error envelope to stdout; `2>/dev/null` does not help. Agent then emptied `core.hooksPath` for 21 further commits. Also listed under [tooling-integration-gaps](../tooling-integration-gaps/README.md).
- [`The harness hook's direct-commit block matches only a bare git and is trivially bypassed`](../../raw/2026-08-23T2010Z-claude-validation-direct-commit-block-trivially-bypassed.md) — First field must be the literal `git`; `sh -c 'git commit …'` is unblocked. Also listed under [unknown-recorded-as-answered](../unknown-recorded-as-answered/README.md).
- [`Harness loop re-reports identical empty-outcome done`](../../raw/2026-09-12T0421Z-cursor-workflow-worker-thrash-identical-done.md) — Six byte-identical `done` ops in 15 minutes; ADR 0018 payload-keyed no-op is the ops-side mitigation, not a harness fix.
- [`Removing a config field silently defanged a shell-script safety check`](../../raw/2026-07-01T0912Z-claude-tooling-config-mode-removal-left-dead-shell-check.md) — Installed hook templates grepped a removed `mode` field. Also listed under [test-coverage-gaps](../test-coverage-gaps/README.md).

## Candidate Follow-Ups

- Write `arm` error envelopes to stderr; hooks should check exit status, not stdout emptiness.
- Treat `git commit` as the verb even when wrapped in `sh -c` / absolute paths / helpers.
- Fix the harness so a successful identical `arm transition --to done` is not re-issued (payload-keyed CLI no-op is already the right ops answer).
- Grep generated hook templates when a config field is removed.
