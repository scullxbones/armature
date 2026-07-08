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
- [`ARM_LOG_SLOT` env var required by parallel-dispatch protocol breaks unrelated tests](../../raw/2026-06-30T2205Z-claude-tooling-arm-log-slot-breaks-worker-identity-tests.md) — Setting `ARM_LOG_SLOT` per the coordinator's documented parallel-dispatch requirement broke four pre-existing worker-identity tests, which computed expected paths without accounting for the slot suffix `workerIdentityWithSlot()` appends when the var is set.
- [A fresh haiku subagent's TDD fix left test-artifact files in the source tree](../../raw/2026-07-01T1330Z-claude-workflow-fresh-agent-fix-left-stray-state-files.md) — After a subagent reported success (build/lint/test all green), five untracked files (`checkpoint.json`, `index.json`, `ready.json`, `traceability.json`, an `issues/` directory) were left sitting directly in `cmd/armature/` — never part of the intended diff, and not caught by the green gates.
- [Auto-mode classifier blocked an inferred top-level PR comment step](../../raw/2026-07-01T1630Z-claude-workflow-prfix-classifier-blocked-topcomment.md) — The orchestrator folded an unrequested "post a summary PR comment" step into a subagent's task list by inference; the auto-mode classifier correctly blocked the dispatch since posting a top-level comment wasn't part of what was actually asked.
- [Source sync resyncs the entire manifest when adding one planning document](../../raw/2026-07-05T0400Z-codex-workflow-source-sync-resyncs-entire-manifest.md) — `arm sources sync` after adding a single new Source re-fetched and reported every registered Source (~60 `synced` lines) instead of only the new one, making it hard to isolate the new Source's own verification result.
- [Ops branch upstream still tracks stale pre-rename remote after Trellis→Armature rename](../../raw/2026-07-07T1225Z-claude-setup-ops-branch-upstream-stale-after-rename.md) — `git status -sb` in the ops worktree reported the local `_armature` branch tracking `origin/_trellis`; a bare `git push` would have targeted the wrong remote branch.

## Candidate Follow-Ups

- Document in the `prfix` or review skills to use `in_reply_to` (not `in_reply_to_id`) when replying to review comments via `gh api`.
- Instruct workers/agents in their skills to use `go build ./...` or `go build -o /dev/null` for build checks to avoid leaving stray binaries.
- Add `armature` to the root `.gitignore` to prevent stray binaries from being committed accidentally.
- Update documentation or `CLAUDE.md` to note that `go build` / `go test` is the authoritative compiler check when LSP diagnostics report stale cache errors.
- Document hook side-effects on mtime in integration tests, and implement/inject a flag or env var to disable hook triggers under test.
- Update the worker-identity tests to compute expected paths through `workerIdentityWithSlot()` itself rather than duplicating the slot-suffix logic inline, so they can't drift from the parallel-dispatch protocol's `ARM_LOG_SLOT` requirement.
- Add a repo-root `.gitignore` entry (or a worker-side post-flight `git status --short` check against an expected file allowlist) to catch stray test-artifact files before a subagent reports success.
- Make `arm sources sync` accept a source-ID filter (or default to syncing only newly-added/changed sources) so verifying one new Source doesn't require reading through ~60 unrelated `synced` lines.
- Audit remaining local/remote branch upstream references for the old `_trellis` name post-rename, not just the ops branch — repoint all discovered stale upstreams explicitly rather than relying on bare `git push`.
