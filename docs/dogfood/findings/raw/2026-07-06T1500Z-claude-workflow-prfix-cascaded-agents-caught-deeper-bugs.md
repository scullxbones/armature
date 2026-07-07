---
date: 2026-07-06
agent: claude
area: workflow
task: /prfix PR #71
tags: [prfix, subagent-cascade, code-review]
---

# Three-stage agent cascade (fix -> broad review -> fix) surfaced bugs the narrow fix pass missed

## User Goal

Address PR #71's two remaining unresolved bot review comments, then use a second
independent reviewer to look beyond just those two fixes at the whole branch,
then implement whatever that turned up, commit/push, and close the loop with
replies + resolved threads.

## Observed

The haiku fix agent correctly fixed the two bot-reported findings (issue-gated
activity log attachment, HeadSHA-scoped citation validation) with TDD and green
build/lint/test. But it necessarily worked within the narrow frame of "fix these
two things." The subsequent fable broad-review agent, given full branch context
(not just the diff), found three P1s the narrow fix pass had no way to catch:
the fix for HeadSHA-mismatched citations could be defeated by a hand-edited
bundle since `Record` never recomputed `ComputeBundleID` to check bundle
self-consistency; a *different* code path (`decodeStructuredHookEvent` on the
Codex adapter) let model-authored `tool_input` masquerade as harness-verified
exit code/output, which undermines the entire evidence model the PR is meant to
build; and citations with a known nonzero exit code could still support a
"Satisfied" criterion. None of these were in scope for the original two bot
comments — they were adjacent design gaps the broad reviewer only found because
it was told to look at the whole feature, not just the diff.

## Impact

The cascade (narrow TDD fix -> independent broad review with full context ->
targeted fix) caught security-relevant gaps in an evidence-integrity feature
that a diff-scoped review would have missed entirely. This validates deliberately
widening reviewer scope beyond "review these specific findings" for
security/trust-boundary-sensitive PRs, at the cost of ~2 extra agent dispatches
and ~15 more minutes of wall time.

## Evidence

PR https://github.com/scullxbones/armature/pull/71, commits `8e948791`
(bundle-integrity + exit-code + status-scoped HeadSHA fixes) and `bbb50bd3`
(tool_input fallback removal + binding-source symmetry fix), landed after the
fable agent's broad-review report identified them as P1/P2 findings absent from
the original two GitHub review comments.

## Suggested Follow-Up

Consider making "expand scope beyond the literal findings" a default step in
/prfix for PRs touching security- or trust-boundary-sensitive code (evidence
validation, auth, permission checks), rather than something that has to be
explicitly requested per invocation.
