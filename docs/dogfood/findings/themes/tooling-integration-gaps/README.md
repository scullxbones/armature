# Theme: Tooling Integration Gaps

## Summary

External tool APIs, CLI schemas, caching issues, or system-wide hooks introduce behavior that deviates from agent expectations, causing noise, extra diagnostics, or test failures.

## Evidence

- [gh pr view JSON field unavailability](../../raw/2026-06-22T000000Z-5207ee28-tooling-gh-json-field-headsha.md) — `gh pr view --json headSha` fails because the field doesn't exist in gh's PR schema; workaround is `git rev-parse HEAD`.
- [Stale LSP after file deletion](../../raw/2026-06-27T1956Z-5207ee28-tooling-stale-lsp-after-file-deletion.md) — LSP emitted false duplicate-declaration diagnostics for deleted files that successfully compiled and tested.
- [GitHub PR comment reply requires in_reply_to](../../raw/2026-06-27T2107Z-claude-workflow-github-reply-field-name.md) — GitHub REST API accepts `in_reply_to` instead of `in_reply_to_id` (which is shown in many docs/examples) for posting replies via `gh api`.
- [Worker left stray compiled binary in repo root](../../raw/2026-06-27T0002Z-coordinator-workflow-worker-stray-binary.md) — Running bare `go build` inside a worktree writes the compiled binary directly to the repository root, polluting git status.
- [Mtime-based test assertions unreliable](../../raw/2026-06-28T1700Z-claude-workflow-test-strengthening-mtime-unreliable.md) — File modification time (mtime) assertions are unreliable in integration tests that invoke real git operations, as system-installed hooks trigger side-effect writes.
- [JSON string/int mismatch between skill docs and Go types hidden by struct-based tests](../../raw/2026-06-28T2200Z-claude-workflow-json-string-int-mismatch-hidden-by-tests.md) — `CriterionStatus`/`Rating` marshaled as integers but skill instructed string values; tests constructed Go structs so the mismatch was never exercised. The end-to-end reviewer→record flow was broken with all tests green until opus code review caught it.

## Candidate Follow-Ups

- Document in the `prfix` or review skills to use `in_reply_to` (not `in_reply_to_id`) when replying to review comments via `gh api`.
- Instruct workers/agents in their skills to use `go build ./...` or `go build -o /dev/null` for build checks to avoid leaving stray binaries.
- Add `armature` to the root `.gitignore` to prevent stray binaries from being committed accidentally.
- Update documentation or `CLAUDE.md` to note that `go build` / `go test` is the authoritative compiler check when LSP diagnostics report stale cache errors.
- Document hook side-effects on mtime in integration tests, and implement/inject a flag or env var to disable hook triggers under test.
