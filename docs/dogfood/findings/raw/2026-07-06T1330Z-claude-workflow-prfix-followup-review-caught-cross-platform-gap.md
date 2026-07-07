---
date: 2026-07-06
agent: claude
area: workflow
task: PR #71 (feat/EXECEV)
tags: [prfix, code-review, multi-agent]
---

# /prfix's second-pass broad review caught a bug the first-pass fix introduced/missed

## User Goal

Run `/prfix` on PR #71 with an explicit multi-stage pipeline: haiku-medium fixes the
two flagged review comments via TDD, then a broader review agent audits the fixes
plus the whole branch, then a sonnet-medium agent implements any follow-up fixes,
commits, pushes, and resolves the review threads.

## Observed

The first fix (haiku-medium) correctly installed PostToolUse hooks for all three
harness platforms (Claude, Codex, Devin) to address the reviewer's literal complaint.
But it left `cmd/armature/harness_hook.go:337` gating capture on `event.Tool == "Bash"`,
which only matches Claude's tool name — Codex uses `shell`/`local_shell`, Devin uses
`exec`. So the fix looked complete (hooks install, tests pass, build/lint/test green)
but silently reproduced the exact "capture branch never fires" failure mode the
original review comment was about, just moved one layer deeper and now specific to
2 of 3 platforms. Neither `make test` nor the haiku agent's own new tests caught this,
because the new tests only asserted hook *installation*, not that a non-Bash event
actually reaches `AppendActivity`.

The second-stage broad-review agent (fable/opus) caught it by reading
`internal/harnesshook/binding.go`'s `isShellTool`/`SupportedShellTools` pattern already
used a few lines above the broken gate, and noticing the inconsistency.

## Impact

Validates the pipeline's structure (dedicated review-broadening stage after the
narrow fix stage) — a single-pass "fix what the reviewer said" loop would have shipped
a fix that passed all checks yet still failed for 2 of 3 target platforms. The
narrow-fix agent's own tests gave false confidence because they tested config shape,
not runtime behavior end-to-end.

## Evidence

- `cmd/armature/harness_hook.go:337` before second fix: `if event.Kind == harnesshook.EventPostToolUse && event.Tool == "Bash" && ...`
- `internal/harnesshook/platform_codex.go` shell tool name ~line 44; `platform_devin.go` ~line 27 (`exec`)
- Existing correct pattern already in the same file at ~line 246: `isShellTool(tool, adapter.Capabilities().SupportedShellTools)`
- Final fix commit: `0ba01a05` on feat/EXECEV

## Suggested Follow-Up

When a subagent's task is "fix reviewer comment X," prompt it to also add a
behavioral/integration-level test (not just a config-shape test) whenever the fix
touches a dispatch/gating condition — the failure mode here was exactly "hook installed
but gate never matches," which only an end-to-end test would surface.
