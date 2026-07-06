# Semantic Conformance Review Design

Date: 2026-06-27

## Summary

Armature will add an advisory semantic-review capability that asks a fresh LLM
reviewer whether a completed delivery is faithful to the issue's Definition of
Done and Acceptance criteria. This fills the gap that deterministic controls
cannot close: lint, tests, coverage, mutation testing, architectural rules, and
scope enforcement can establish mechanical facts, but they cannot establish
that the delivered behavior means what the task required.

The capability is an Armature reviewer protocol. Armature assembles a canonical
review bundle, validates the structured result, derives a reviewer-facing
rating, records a compact attestation, and renders PR evidence. A bundled
`armature-reviewer` skill performs the subjective LLM-as-judge evaluation in a
fresh model context. Armature does not run a model, configure or execute
customer checks, inspect implementation activity history, or block delivery on
the review result in phase one.

This is a material extension of Armature's product vision. Armature already
coordinates work from explicit issue contracts and preserves provenance in git;
semantic review makes those contracts useful at delivery time without
reintroducing managed execution.

## Product Boundary

### Goals

- Evaluate each assessable delivery for semantic fidelity to its Definition of
  Done and Acceptance criteria.
- Prescribe a stable, evidence-cited rubric to the reviewer model.
- Use a fresh reviewer context independent of the implementation worker.
- Keep the detailed report available to PR reviewers while retaining only a
  compact durable attestation in Armature history.
- Wire review into `armature-coordinator` alongside `armature-worker` and
  `armature-auditor`.
- Establish a repo-owned evaluator corpus so the bundled reviewer can be
  calibrated before any future enforcement is considered.
- Add no customer configuration for deterministic checks.

### Non-Goals

- Do not execute or evaluate customer lint, test, coverage, mutation, or
  architectural checks.
- Do not treat Armature's repository-development commands, including
  `make check`, as product behavior or a customer contract.
- ~~Do not collect general tool activity, command transcripts, reasoning traces,
  or harness activity logs.~~ **Amended by ADR-0008:** Execution evidence (harness-recorded
  command/output pairs) is now admissible as a third evidence class for behavioral
  criteria, upgrade-only. See ADR-0008 and docs/sensitive-environments.md for
  disclosure posture and how to disable capture.
- Do not use harness hooks to initiate semantic review. **Amended by ADR-0008:**
  Hooks record execution evidence but still neither initiate nor gate conformance
  review.
- Do not allow the reviewer to modify or remediate the delivery it assesses.
- Do not make a rating or missing assessment block completion, merge, or PR
  delivery in phase one.
- Do not make parent descriptions, rendered context, or implied expectations
  into unstated binding requirements.
- Do not retain full assessment reports in the Armature op log or repository.
- Do not implement automatic remediation, multi-judge consensus, or semantic
  gating in phase one.

## Domain Model

The canonical glossary in `CONTEXT.md` defines the terms introduced or sharpened
by this design:

- **Leaf Issue** is an issue with no children in the current DAG. It is a graph
  property, not a lifecycle status.
- **Task Contract** is the authoritative description of intended delivery.
  Phase-one semantic review judges its Definition of Done and Acceptance fields;
  deterministic policy remains responsible for mechanically enforceable fields
  such as Scope.
- **Assessable Delivery** is work recorded by an issue transition to `Done` with
  an Outcome and an associated repository diff.
- **Review Bundle** is Armature's canonical read-only input package for the
  Reviewer.
- **Reviewer** is a fresh LLM worker role using `armature-reviewer` with bounded
  read-only repository access.
- **Conformance Assessment** is the detailed advisory LLM-as-judge result.
- **Criterion Result** is the evidence-cited semantic judgment for one
  Definition-of-Done or Acceptance criterion.
- **Conformance Rating** is the green, yellow, or red summary derived by
  Armature from Criterion Results.
- **Assessment Attestation** is the compact durable record that an assessment
  occurred against specific contract and delivery fingerprints.

Although work should ordinarily be planned on leaf issues, review eligibility
is based on an assessable delivery rather than issue type or current topology.
Any issue type that records delivered work can therefore be reviewed. A parent
Story that records an Outcome is reviewed against its aggregate delivery range.

## Design Principles

- **Outcome over activity.** Judge the completed change, not the path the worker
  took to produce it.
- **Explicit requirements only.** Definition of Done and Acceptance are binding.
  Other context may explain them but cannot silently expand them.
- **Fresh judgment.** The Reviewer does not inherit the implementation
  conversation or the worker's conclusions.
- **Structured subjectivity.** The rubric and output schema constrain the model;
  Armature derives aggregate meaning deterministically.
- **Advisory first.** Review evidence informs PR reviewers but does not control
  lifecycle transitions in phase one.
- **Curation by promotion.** Full reports are ephemeral. Only compact
  attestations and findings deliberately converted into Issues, Notes,
  Decisions, or waivers become durable Armature knowledge.
- **No new customer check surface.** Customer-owned deterministic controls stay
  in their existing CI and development workflows.

## Architecture

Phase one adds a deep `review` module with two public operations: prepare a
review and record its result. Command handlers perform input/output adaptation;
the module owns the protocol semantics.

### Armature Owns

- assessable-delivery validation
- Review Bundle construction and versioning
- contract, delivery, and bundle fingerprints
- result-schema validation
- Criterion Result validation
- deterministic Conformance Rating derivation
- attestation idempotence and stale-assessment detection
- assessment materialization
- PR-summary and detailed Markdown rendering

### The `armature-reviewer` Skill Owns

- reading the Review Bundle
- inspecting the delivery diff
- bounded read-only exploration of surrounding repository code when needed to
  interpret the diff
- applying the prescribed criterion rubric
- citing concrete delivery evidence
- emitting schema-constrained output

### The `armature-coordinator` Skill Owns

- recording the delivery range
- dispatching a fresh Reviewer after delivery
- retrying one malformed or failed review in a new context
- recording valid results through Armature
- carrying unavailable assessments and rendered reports into PR preparation

### The `armature-auditor` Skill Owns

- deterministic repository-health and provenance gates before story sign-off
- Outcome concreteness checks
- assessment-coverage reporting

The Auditor no longer duplicates the Reviewer's semantic judgment that every
Acceptance criterion was implemented. In phase one it must not turn assessment
absence or rating into a gate.

### Harness Hooks

Harness hooks remain outside this workflow. A stop hook runs before a stable
delivery commit exists, cannot provide the required fresh reviewer context, and
would couple semantic review to provider-specific hook behavior. Hooks may
continue enforcing deterministic policies such as task Scope. **Amended by ADR-0008:**
Hooks now record execution evidence (harness-captured command/output pairs) to
a worktree-local activity log. Despite recording activity, hooks still neither
initiate nor gate conformance review. The activity log is a resource exclusively
consumed by `arm review record` during attestation. See ADR-0008 for trust model,
discovery posture, and capture mechanics.

## Command Surface

The intended command group is:

```text
arm review prepare ISSUE --base BASE_SHA --head HEAD_SHA [--format agent|json]
arm review record ISSUE --base BASE_SHA --head HEAD_SHA --input RESULT_PATH [--format human|json|markdown]
```

`prepare` verifies that the issue is an assessable delivery and that the Git
range exists. It emits the canonical bundle. A task normally uses the parent of
its delivery commit as `base` and the delivery commit as `head`. A Story uses
the feature-branch base and final feature head.

`record` reconstructs the bundle from the explicit delivery range, then
validates a Reviewer result against it, derives the rating, emits an
`assessment-attested` op for a valid non-duplicate result, materializes the
attestation, and renders the detailed report. `--input -` reads the result from
standard input. An idempotent duplicate returns the existing result without
appending another op. The command does not persist the detailed report itself.

Exact Cobra wiring and flag spelling may be refined during implementation, but
the prepare/record separation and explicit delivery range are design
requirements.

## Review Bundle

The versioned bundle contains:

```json
{
  "schema_version": 1,
  "bundle_id": "sha256:...",
  "issue": {
    "id": "FEATURE-S1-T1",
    "type": "task",
    "title": "...",
    "outcome": "..."
  },
  "contract": {
    "definition_of_done": "...",
    "acceptance": ["..."]
  },
  "delivery": {
    "base_sha": "...",
    "head_sha": "...",
    "changed_files": ["..."],
    "diff": "..."
  },
  "fingerprints": {
    "contract": "sha256:...",
    "delivery": "sha256:..."
  }
}
```

The raw Definition of Done is one criterion. Acceptance remains an ordered list
and receives deterministic positional identities such as `acceptance[0]`.
Phase one does not ask the LLM to invent a stable decomposition of compound
prose. Poorly decomposed criteria can therefore produce
`partially_satisfied` or `indeterminate`, which exposes a planning-quality
problem rather than hiding it.

The delivery diff is the assessment subject. The changed-file manifest and
base/head identities allow the Reviewer to retrieve and inspect large diffs in
bounded pieces when inlining the entire patch is impractical. Surrounding code
may be read to interpret a change, but unchanged code is not itself evidence
that the delivery implemented a requirement.

Armature-owned coordination paths, including `.armature/**` and `.arm/**`, are
excluded from the delivery diff and changed-file manifest. This prevents
single-branch op logs and generated coordination state from becoming reviewer
input. A range containing only excluded coordination paths is not an assessable
delivery.

The bundle excludes customer check commands and results, implementation
activity, worker-authored arguments, and hidden prompt context. The recorded
Outcome is descriptive context, not proof of conformance.

## Reviewer Rubric

The Reviewer emits one result for the Definition of Done and one for each
Acceptance item:

- `satisfied`: delivery evidence supports the complete criterion.
- `partially_satisfied`: evidence supports only part of the criterion, and the
  missing portion is identified.
- `not_satisfied`: evidence contradicts the criterion or the delivery does not
  implement it.
- `indeterminate`: the diff and bounded repository context cannot establish an
  answer, or the criterion is too ambiguous to judge without inventing a
  requirement.

Every result includes:

- criterion identity and verbatim criterion text
- status
- concise rationale
- zero or more structured diff citations containing path and hunk/line identity
- an explicit missing-evidence statement when citations are absent

The Reviewer does not emit an authoritative aggregate rating. Armature derives:

- **Green** when every criterion is `satisfied`.
- **Yellow** when at least one criterion is `partially_satisfied` or
  `indeterminate` and none is `not_satisfied`.
- **Red** when at least one criterion is `not_satisfied`.

This keeps semantic labels authoritative while providing fast reviewer triage.
It also avoids inaccessible color-only semantics and false precision from
numeric scores.

## Result Validation

`arm review record` rejects a result when:

- its schema version is unsupported
- its bundle ID or fingerprints do not match the prepared delivery
- a required criterion is missing or duplicated
- a criterion identity or verbatim text differs from the bundle
- its status is outside the prescribed enumeration
- a citation references a path or diff location outside the delivery range
- required rationale or missing-evidence text is absent

The result need not prove that its subjective conclusion is correct; that is
what calibration and human review address. Validation proves that the result is
well-formed, complete, attributable, and tied to the intended delivery.

## Assessment Attestation And Retention

A valid non-duplicate result emits a compact append-only
`assessment-attested` op. The payload contains at least:

- bundle ID and schema version
- contract and delivery fingerprints
- base and head SHAs
- Reviewer Skill version
- model/provider identity when the harness exposes it
- derived Conformance Rating
- counts by Criterion Result status
- result fingerprint

The full rationale and citations are not stored in the op. Materialized issue
state exposes the latest applicable attestation and enough history to identify
superseded or stale attestations.

Recording the same bundle, reviewer identity, and result fingerprint is
idempotent. A changed delivery or contract produces new fingerprints and makes
the prior attestation stale for current-delivery reporting. The tiny op history
remains append-only; detailed model prose does not accumulate in Git history.

The detailed JSON/Markdown result is ephemeral Armature output. A Coordinator
may publish it through the repository's PR mechanism. Forge or CI retention
then governs that published copy. Findings with lasting value are curated by
promotion into existing Armature domain records rather than automatically
retaining every report.

## Coordinator Workflow

For each completed Issue delivery:

1. The Worker transitions the Issue to `Done`, records a concrete Outcome, and
   creates the delivery commit.
2. The Coordinator identifies the delivery base/head range.
3. The Coordinator runs `arm review prepare`.
4. The Coordinator dispatches a fresh agent whose first instruction is to use
   `armature-reviewer`, passing the bundle and repository location.
5. The Reviewer evaluates with read-only access and returns structured JSON.
6. The Coordinator runs `arm review record` and retains the rendered report for
   PR assembly.
7. The Coordinator marks the Issue `Merged` and continues the normal wave.

Yellow, red, unavailable, and stale results do not alter this phase-one flow.
They are evidence for the later PR reviewer.

After leaf work completes, the existing Auditor runs its deterministic and
provenance checks. The parent Story then records its own Outcome and receives a
review against the aggregate feature-branch delivery range. The PR includes:

- a summary row per assessable delivery
- its green/yellow/red/unavailable state
- criterion-status counts
- detailed per-criterion reports or links to the publishing surface that holds
  them

## Failure Handling

- Missing or ambiguous Definition of Done or Acceptance text produces an
  `indeterminate` Criterion Result and identifies the planning defect. The
  Reviewer must not manufacture requirements.
- Insufficient evidence produces `indeterminate`, not a guessed success or
  failure.
- An invalid result gets one retry in another fresh Reviewer context.
- A fingerprint mismatch requires a newly prepared bundle; it is never
  overridden.
- A second evaluator or schema failure is represented to the Coordinator as
  `assessment_unavailable` with a concise reason.
- Missing or unavailable assessment output remains report-only and does not
  block delivery.
- Oversized diffs are inspected incrementally through their base/head range and
  changed-file manifest. The Reviewer must return `indeterminate` where it could
  not inspect enough evidence rather than silently claiming complete coverage.

## Evaluator Testing And Calibration

Phase one has two test layers.

### Deterministic Protocol Tests

Normal Go tests cover:

- bundle construction and canonical fingerprinting
- delivery-range and citation validation
- result-schema validation
- the complete rating-derivation truth table
- attestation creation, idempotence, staleness, replay, and materialization
- human, JSON, and Markdown rendering
- coordinator-facing unavailable-result behavior

These tests use Armature's repository-development workflow as appropriate, but
that workflow is not part of the product protocol and imposes nothing on
customers.

### Reviewer Eval Corpus

The Armature repository includes a development-only corpus of human-labeled
Task Contracts and delivery diffs for calibrating the bundled Skill. The corpus
is not deployed as a customer Skill artifact. Cases cover:

- complete implementations
- partial implementations
- clearly missing or contradictory behavior
- ambiguous or unobservable behavior
- misleading comments, Outcomes, or tests that assert unsupported behavior
- unrelated changes
- deliveries that require bounded surrounding-code inspection
- oversized or compound criteria that should produce `indeterminate`

The primary metrics are:

- false-green rate
- per-criterion classification agreement
- unsupported-citation rate
- correct use of `indeterminate`
- schema-compliance rate

Model execution is nondeterministic and provider-dependent, so these evals do
not become customer checks or ordinary deterministic build gates. Armature
records a baseline by Skill/model version. Real dogfood misses are reviewed by
humans and deliberately promoted into durable fixtures when they represent a
reusable failure mode.

## Security And Trust

The delivery diff and repository may contain sensitive customer code. The
Reviewer uses the customer's already-selected harness and model boundary;
Armature introduces no new network destination or hosted evaluation service.

Review Bundle and result content are untrusted inputs to the CLI. Paths and
citations must be validated against the repository and delivery range. Markdown
rendering must escape content rather than allowing model output to inject raw
HTML or forge-specific control syntax.

The model's status remains subjective evidence. Schema validity, citations, and
fingerprints improve auditability but do not convert the judgment into a
deterministic fact.

## Future Phases

Usage should determine later phases rather than this design precommitting to
enforcement.

Potential extensions include:

- reviewer disposition or override records for calibration
- requirement-reference criteria once requirement-level provenance is present
- selective second opinions for yellow or red assessments
- model-specific calibration thresholds
- optional remediation suggestions or explicit human-requested remediation
- opt-in advisory policies at Story or repository level
- eventual opt-in gates only after false-green and reviewer-agreement evidence
  demonstrate that a specific Skill/model version is reliable enough

Automatic blocking is not the default destination. Any enforcement proposal
requires a separate decision based on observed dogfood and customer evidence.

## Implementation Status

**Status:** Complete as of 2026-06-28

The semantic conformance review design has been fully implemented. The implementation includes:

- **Protocol Commands:** `arm review prepare` and `arm review record` commands with correct flag signatures
- **Bundle Construction:** Review Bundle generation with canonical fingerprinting for contract and delivery metadata
- **Result Validation:** Comprehensive schema and conformance validation for reviewer results
- **Rating Derivation:** Deterministic green/yellow/red rating calculation from criterion results
- **Assessment Attestation:** Compact durable op-based record with result fingerprint deduplication and idempotence
- **Materialization:** Assessment attestations properly materialized into issue state and accessible via `arm show`
- **End-to-End Testing:** Full single-branch lifecycle tests covering bootstrap→claim→deliver→prepare→record→rematerialize workflow

All deterministic protocol tests pass, including bundle construction, delivery-range validation, result-schema validation, rating-derivation truth table, attestation creation with idempotence and staleness handling, and materialization replay.

## Success Criteria

Phase one is successful when:

- every assessable delivery in the Coordinator workflow is offered to a fresh
  `armature-reviewer`
- Armature prepares a stable bundle and validates complete Criterion Results
- ratings are derived deterministically and never supplied authoritatively by
  the model
- detailed reports reach PR reviewers without accumulating in Git history
- compact attestations can be replayed and correlated to exact contracts and
  delivery ranges
- Reviewer and Auditor responsibilities no longer overlap
- evaluator fixtures expose false greens, unsupported citations, and misuse of
  `indeterminate`
- customers configure no additional deterministic checks for this capability
