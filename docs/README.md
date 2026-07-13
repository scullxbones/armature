# Docs Index

This index tracks every document under `docs/`, its audience, and whether it's
current (actively maintained, safe to rely on) or historical (kept for
reference only).

## Top-level references

| Doc | Audience | Status |
|---|---|---|
| `commands.md` | Users, agents | Current — CLI reference |
| `concepts.md` | Users, agents | Current |
| `configuration.md` | Users, agents | Current |
| `getting-started.md` | New users | Current |
| `harness-hook.md` | Agents, integrators | Current |
| `provider-smoke-tests.md` | Contributors | Current |
| `sensitive-environments.md` | Operators | Current |
| `use-cases.md` | Users | Current |
| `validation-codes.md` | Users, agents | Current |

## Agents (`agents/`)

| Doc | Audience | Status |
|---|---|---|
| `dogfood-findings.md` | Contributors | Current |
| `quality-gates.md` | Contributors, agents | Current |
| `skills.md` | Contributors, agents | Current |
| `workflow.md` | Contributors, agents | Current |

## ADRs (`adr/`)

All ADRs are current (append-only decision records; superseding an ADR means
adding a new one, not editing the old).

| Doc | Audience | Status |
|---|---|---|
| `0001-arm-init-bootstrap-boundary.md` | Contributors | Current |
| `0002-arm-bootstrap-unified-command.md` | Contributors | Current |
| `0003-task-dispatch-requires-worktree.md` | Contributors | Current |
| `0004-deep-module-depguard-boundaries.md` | Contributors | Current |
| `0005-semantic-conformance-review-boundary.md` | Contributors | Current |
| `0006-eliminate-single-branch-mode.md` | Contributors | Current |
| `0007-path-based-issue-binding-resolution.md` | Contributors | Current |
| `0008-execution-evidence-in-semantic-review.md` | Contributors | Current |
| `0009-ratify-the-armature-constitution.md` | Contributors | Current |
| `0010-park-not-purge-subtractive-release.md` | Contributors | Current |
| `0011-cli-groups-mirror-deep-modules.md` | Contributors | Current |
| `0012-context-files-intent-lifecycle-and-cli-semantics.md` | Contributors | Current |
| `template.md` | Contributors | Current — template for new ADRs |
| `README.md` | Contributors | Current — ADR index |

## Design (`design/`)

Active proposals, specs, and planning docs for work not yet fully decomposed
or landed.

| Doc | Audience | Status |
|---|---|---|
| `architecture.md` | Contributors, agents | Current — canonical architecture reference |
| `roles.md` | Contributors | Current |
| `quality-controls.md` | Contributors | Current — Active |
| `next-work-sequencing.md` | Planners | Current — cross-document execution order |
| `the-next-ten.html` | Planners | Current — strategic proposal scoring |
| `top-tier-gap-analysis.md` | Planners | Current |
| `long-horizon-proposals.md` | Planners | Current |
| `narrow-gaps-addendum.md` | Planners | Current |
| `subtractive-release.md` | Planners | Current — resolved, awaiting `/to-issues` |
| `cli-grammar-contract.md` | Planners | Current — resolved, awaiting `/to-issues` |
| `dotdir-collapse.md` | Planners | Current — resolved, awaiting `/to-issues` |
| `bootstrap-agent-integration.md` | Planners | Current — draft proposal |
| `codebase-architecture-improvements.md` | Planners | Current — proposed |
| `dag-documentation-deep-dive.md` | Planners | Current — proposed |
| `graph-facts-refactor-follow-up.md` | Planners | Current — proposed |
| `harness-hooks-review-and-retooling.md` | Planners | Current — proposed |
| `deterministic-quality-guardrails.md` | Planners | Current — proposed standard |

## Archive (`archive/`)

Historical reference only — superseded by `design/architecture.md` or
otherwise closed out. Each file carries a `Superseded by` header.

| Doc | Audience | Status |
|---|---|---|
| `trellis-prd.md` | Historical | Superseded — early PRD, superseded by `architecture.md` |
| `gap-resolutions.md` | Historical | Superseded — decisions folded into `architecture.md` |
| `trellis-rename.md` | Historical | Superseded — rename decision record, status Complete |
| `dogfood-ceremony-e1.md` | Historical | Superseded — passed ceremony record |
| `dogfooding-learnings.md` | Historical | Superseded — retrospective notes |
| `ai-context-management-market-analysis.md` | Historical | Superseded — point-in-time market analysis |
| `orchestration/` | Historical | Superseded — `arm orchestrate` subsystem removed (see `orchestration/README.md`) |

## Superpowers plans and specs (`superpowers/plans/`, `superpowers/specs/`)

Dated, one-off implementation plans and design specs generated per story/task
during development. Audience: contributors and agents executing that specific
piece of work. Status: historical by nature — each is scoped to the date in
its filename and is not maintained after its story lands. Not individually
indexed here; consult the filename date and the linked story/ADR for context.

## Dogfood (`dogfood/`)

Audience: contributors. Status: current — captured dogfooding session notes,
see `agents/dogfood-findings.md` for the process.
