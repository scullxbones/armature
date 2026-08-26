# Architecture Decision Records

ADRs are append-only decision records: amending a past decision means adding a
new ADR that supersedes or amends the old one, not editing history. Use
`template.md` when writing a new one.

| ADR | Title | Status |
|---|---|---|
| [0001](0001-arm-init-bootstrap-boundary.md) | `arm init` Owns Repository Bootstrap | Accepted |
| [0002](0002-arm-bootstrap-unified-command.md) | `arm bootstrap` Owns Bootstrap And Reinstall | Accepted |
| [0003](0003-task-dispatch-requires-worktree.md) | Task Dispatch Always Requires a Worktree | Accepted (superseded in part by ADR-0013) |
| [0004](0004-deep-module-depguard-boundaries.md) | Depguard Boundaries for Deep Modules | Accepted |
| [0005](0005-semantic-conformance-review-boundary.md) | Keep Semantic Conformance Review Advisory and Skill-Driven | Amended by ADR-0008 |
| [0006](0006-eliminate-single-branch-mode.md) | Eliminate Single-Branch Mode | Accepted |
| [0007](0007-path-based-issue-binding-resolution.md) | Path-Based Issue Binding Resolution in the Harness Hook | Accepted |
| [0008](0008-execution-evidence-in-semantic-review.md) | Admit Harness-Recorded Execution Evidence Into Semantic Review, Upgrade-Only | Accepted |
| [0009](0009-ratify-the-armature-constitution.md) | Ratify the Armature Constitution | Accepted |
| [0010](0010-park-not-purge-subtractive-release.md) | Parked Surfaces Are Deleted Outright, Not Soft-Deprecated | Accepted |
| [0011](0011-cli-groups-mirror-deep-modules.md) | CLI Command Groups Are Discovered From Deep Module Boundaries | Accepted |
| [0012](0012-context-files-intent-lifecycle-and-cli-semantics.md) | Context Files Intent, Lifecycle, and CLI Semantics | Accepted |
| [0013](0013-managed-worktree-auto-provisioning.md) | Managed Worktree Auto-Provisioning with Boolean Flag | Accepted |
| [0014](0014-two-tier-gates-and-evidence-based-acceptance.md) | Two-Tier Gates and Evidence-Based Acceptance | Proposed (amended in part by ADR-0015) |
| [0015](0015-recalibrate-mutation-and-coverage-gates.md) | Recalibrate Mutation and Coverage Gates | Accepted |
| [0016](0016-three-door-validation.md) | Three-Door Validation | Accepted |
| 0017–0019 | Agent Output Contract cluster (ADR 0017 in flight as `AOC-S1-T1`; 0018 idempotency and 0019 TOON park planned). Numbering reserved so this table does not collide when those land. | — |
| [0020](0020-agent-error-contract.md) | Agent-grade error contract | Accepted |
