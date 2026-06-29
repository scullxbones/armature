# Theme: Effective Patterns — Approaches That Worked

## Summary

Not all findings describe friction. These findings document techniques or pipeline configurations that worked well and are worth repeating. Capturing confirmed patterns prevents revisiting solved problems and gives future coordinators a validated playbook.

## Evidence

- [prfix multi-agent pipeline: haiku→opus→sonnet worked well for PR review](../../raw/2026-06-29T0000Z-5207ee28-workflow-prfix-multiagent-pipeline.md) — Running two scoped haiku agents (one per Codex finding) with TDD, then an opus agent expanding scope to find real issues the Codex review missed, then a sonnet agent implementing all findings: the 3-tier pipeline caught a meaningful API change (NewAttestation signature, FilterDiff, wired fingerprint/citation validation) that a single-pass fix would have missed. Sequential execution on the same branch was correct; parallel would have conflicted on the shared build system.

## Pattern

**Tiered model selection for PR review**: Use haiku for fast, scoped TDD fixes on well-defined findings; opus for semantic scope expansion (finding issues the initial review missed); sonnet for multi-file implementation that requires reasoning across the whole change. Running agents sequentially on a shared branch avoids build-system conflicts.

This pipeline works best when:
- The initial review (Codex or equivalent) produces specific, scoped findings
- The opus pass is given the full diff and permitted to expand scope
- The sonnet implementation agent receives all opus findings at once rather than piecemeal

## Candidate Follow-Ups

- Encode the haiku→opus→sonnet pipeline as a recommended pattern in the `prfix` skill for PRs with more than 2 findings.
- Document the "sequential, not parallel" constraint for agents sharing a branch, alongside the existing guidance for parallel worktree-isolated agents.
