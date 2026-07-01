---
date: 2026-06-30
agent: claude
area: documentation
task: PR #65 review remediation (armature repo)
tags: [skillsembed, prfix, doc-drift]
---

# Embedded skill docs (armature-coordinator/auditor) drifted from actual CLI behavior in two independent spots

## User Goal

Run the `/prfix` skill against PR #65 to remediate two bot-reviewer findings: fix, verify via TDD, expand-scope review, implement, commit/push, and reply/resolve threads — all via a chain of subagents (haiku fix -> opus review -> sonnet finalize).

## Observed

Both confirmed-valid PR review findings were the same root-cause class: prose in `internal/skillsembed/skills/*/SKILL.md` (which is shipped as literal instructions read by coordinator/auditor agents) had drifted from the actual Go implementation:

1. `armature-coordinator/SKILL.md` told agents to run `git worktree add --detach <path>` before `arm claim --worktree <path>`. But `claim.go`'s `checkExistingWorktreeBinding` rejects a pre-existing worktree with an unbound detached HEAD — so following the doc as written makes `arm claim` fail every time.
2. `armature-auditor/SKILL.md` described the W5 "Context Files" validate warning as firing when `context_files` references files outside `scope`. The actual check (`checkW5MissingContextFiles`) fires on the opposite condition: an *empty* `context_files` array combined with scope spanning 3+ directories. The doc's suggested remedy (`--clear-context-files`) could never fix the real warning, since clearing an already-empty list is a no-op.

The opus review pass (dispatched specifically to expand scope beyond the literal two comments) caught two more stale references to the same wrong "mismatch" wording still living in the *same file*, left behind by the first fix pass — the haiku fixer corrected the primary description but didn't grep the rest of the file for now-inconsistent callbacks to the old (wrong) semantics.

## Impact

- Both bugs would have caused any agent literally following the coordinator/auditor skill docs to hit a hard failure (worktree claim rejection) or give guidance that structurally cannot resolve the warning it's addressing (clearing an empty list).
- This is doc-as-code with no test coverage: there's no automated check that `SKILL.md` prose matches the CLI/validator behavior it describes, so drift is only caught by attentive human/bot PR review, not CI.
- The multi-stage subagent pipeline (fix -> independent review -> finalize) worked well here specifically because the second pass looked at the *whole file* rather than just the diff — a single fix-and-ship pass would have shipped a self-inconsistent doc.

## Evidence

- `cmd/armature/claim.go` lines ~116-153 (`checkExistingWorktreeBinding`), error message "worktree at %s has a detached HEAD with no existing binding for %s"
- `internal/validate/validate.go` lines ~365-381 (`checkW5MissingContextFiles`)
- PR #65 review comments 3502811288, 3502811290 (chatgpt-codex-connector bot)
- Fix commit d3fc1ed4 on branch feat/DF-S5

## Suggested Follow-Up

Consider a lightweight consistency check (even a grep-based CI lint) that flags SKILL.md prose describing specific CLI flag/warning semantics without a corresponding test or doc-generation step, so drift between `internal/skillsembed/skills/**` and the Go implementation it documents surfaces before a bot reviewer catches it in a PR.
