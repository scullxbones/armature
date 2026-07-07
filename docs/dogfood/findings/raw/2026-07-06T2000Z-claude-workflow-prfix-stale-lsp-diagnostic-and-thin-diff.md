---
date: 2026-07-06
agent: claude (5207ee28-cdd8-48e6-98dc-7da179d4a40d)
area: workflow
task: /prfix https://github.com/scullxbones/armature/pull/71
tags: [prfix, lsp-diagnostics, subagent-cascade, review-thread-state]
---

# Stale LSP diagnostic nearly triggered a false failure; second-pass review still earned its keep on a near-trivial diff

## User Goal

Run `/prfix` end-to-end on PR #71: classify unresolved review threads, fix with
a haiku TDD subagent, run a broad fable review beyond the literal findings,
implement the follow-up with a sonnet subagent, then commit/push/reply/resolve.

## Observed

1. **Thread state check paid for itself immediately.** Of the 5 review threads
   on the PR, 4 were already resolved by earlier commits before this invocation
   started. Only 1 (`internal/review/record.go:171`, TOCTOU on activity-log
   digest vs. entry read) was actually open. Fetching resolved/outdated state
   via the GraphQL `reviewThreads` query up front avoided re-fixing already-fixed
   findings — a plain `pulls/comments` REST call would not have surfaced
   resolution state and could have caused duplicate work.

2. **A system-reported diagnostic looked like a hard failure but wasn't.**
   After the sonnet fix-implementation subagent finished, a `new-diagnostics`
   system reminder flagged `record_test.go:1004/1019: undefined: containsError
   [UndeclaredName] (compiler)` as a compiler error, alongside several
   unrelated modernizer suggestions (`stringscut`, `fmtappendf`, `rangeint`,
   `mapsloop`, `any`). Re-running `go build ./...`, `go vet
   ./internal/review/...`, `go test ./internal/review/...`, and `make lint`
   directly all came back clean — `containsError` is defined in
   `validate_test.go:374` and always was. The diagnostic appears to have been
   a stale editor/LSP snapshot from mid-edit, surfaced after the fact rather
   than reflecting the final committed state. Treating it as gospel without
   re-verifying against the actual build would have produced a false "still
   broken" report to the user.

3. **The single open finding here was narrow, but the broad-review step still
   surfaced real (if minor) issues.** Unlike the [prior /prfix pass on this
   same PR](2026-07-06T1500Z-claude-workflow-prfix-cascaded-agents-caught-deeper-bugs.md),
   which found P1 security-relevant gaps, this pass's fable reviewer found only
   minor cleanup items: code duplication in the new digest-reading helper, two
   now-dead functions the fix left behind (`ValidateActivityDigest`,
   `LoadActivityEntries` — exactly the TOCTOU-prone pair, orphaned but still
   exported and thus reintroducible), a happy-path-only regression test, and a
   stale doc comment. None were reporter-visible bugs, but leaving the old
   TOCTOU-shaped functions in place unused would have been a live trap for a
   future caller. This suggests the broad-review step has value even on
   small, narrowly-scoped diffs, not just large ones — the marginal cost was
   one extra subagent dispatch.

## Impact

No wasted implementation work (thread-state check), and no false failure
reported to the user (diagnostic re-verification). The dead-code finding
would likely not have been caught by a narrower "just fix the reported
comment" pass, since dead code is invisible from the diff of the fix itself —
it only shows up when you ask "does anything still call the old functions?"

## Evidence

- GraphQL query result: 4/5 threads `isResolved: true` before this run started.
- `go build ./...`, `go vet ./internal/review/...`, `go test
  ./internal/review/...`, `make lint` all clean after the flagged diagnostic
  appeared.
- Commits `a51d88c0`, `70bdf02d`, `b07030a2` on `feat/EXECEV`.

## Suggested Follow-Up

When a `new-diagnostics` compiler-severity finding appears right after a
subagent reports success, re-run the actual build/test/vet commands before
trusting the diagnostic — LSP snapshots can lag behind the final file state
written by a just-completed background agent.
