# Theme: Effective Patterns — Approaches That Worked

## Summary

Not all findings describe friction. These findings document techniques or pipeline configurations that worked well and are worth repeating. Capturing confirmed patterns prevents revisiting solved problems and gives future coordinators a validated playbook.

## Evidence

- [prfix multi-agent pipeline: haiku→opus→sonnet worked well for PR review](../../raw/2026-06-29T0000Z-5207ee28-workflow-prfix-multiagent-pipeline.md) — Running two scoped haiku agents (one per Codex finding) with TDD, then an opus agent expanding scope to find real issues the Codex review missed, then a sonnet agent implementing all findings: the 3-tier pipeline caught a meaningful API change (NewAttestation signature, FilterDiff, wired fingerprint/citation validation) that a single-pass fix would have missed. Sequential execution on the same branch was correct; parallel would have conflicted on the shared build system.
- [Sonnet reviewer with verbatim JSON template produced schema-valid assessments; haiku with prose prompt did not](../../raw/2026-06-29T1945Z-5207ee28-workflow-haiku-assessment-format-unreliable.md) — Sonnet reviewers given a concrete verbatim JSON template in the prompt (field names, bracket-notation criterion IDs, `schema_version: 1`) consistently produced valid ConformanceAssessment JSON on first attempt. Parallel haiku reviewers given the same task described in prose produced structurally invalid JSON requiring coordinator post-processing every time. The explicit template is the differentiating factor.
- [Parallel sonnet reviewers for independent tasks: no filesystem conflicts](../../raw/2026-06-29T1945Z-5207ee28-workflow-haiku-assessment-format-unreliable.md) — Three parallel review agents (T8/T9/T10) each writing to their own assessment file in the job tmp directory completed without interference. Read-only repo access + distinct output paths = safe parallelism for conformance review.

## Patterns

**Tiered model selection for PR review**: Use haiku for fast, scoped TDD fixes on well-defined findings; opus for semantic scope expansion (finding issues the initial review missed); sonnet for multi-file implementation that requires reasoning across the whole change. Running agents sequentially on a shared branch avoids build-system conflicts.

**Verbatim JSON template in reviewer prompts**: When dispatching a reviewer agent that must produce structured JSON, include the exact schema as a copy-paste template (not prose description). This is especially important for haiku but improves reliability for all models. The template should show field names, value format (e.g. `acceptance[0]` not `acceptance_0`), and required top-level fields (`schema_version: 1`).

**Parallel read-only reviewer dispatch**: Multiple reviewer agents can run in parallel safely when each writes to a distinct output file path and all repo access is read-only. Parallelism is safe; shared-branch write access is not.

## Candidate Follow-Ups

- Encode the haiku→opus→sonnet pipeline as a recommended pattern in the `prfix` skill for PRs with more than 2 findings.
- Document the "sequential, not parallel" constraint for agents sharing a branch, alongside the existing guidance for parallel worktree-isolated agents.
- Add the verbatim JSON schema template to the armature-reviewer SKILL.md so it's available to any model dispatched as a reviewer.
