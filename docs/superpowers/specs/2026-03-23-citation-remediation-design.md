# Citation Remediation — Design Spec

**Date:** 2026-03-23
**Epic:** E5 — Polish, UX Hardening & User-Facing Docs
**Story:** E5-S0-ext (extension to E5-S0)
**Status:** Draft

---

## Context

E5-S0 introduced citation validation: every issue node must carry at least one `source-link` op pointing to a registered source entry in `.issues/sources/manifest.json`. This is a backward-incompatible change — all 106 pre-existing nodes have zero citations, producing `COVERAGE: 0.0% (0/106 nodes cited)` from `trls validate`.

Two things are required to resolve this:

1. **New tooling** — a `trls source-link` command (the everyday citation path) and a `trls accept-citation` command (the auditable fallback for nodes where no recoverable source exists).
2. **Retroactive remediation** — register existing planning docs as sources and emit the appropriate ops for all 106 nodes.

An audit report command (`trls audit`) is scoped to E6 and not part of this story.

---

## New Op Type: `citation-accepted`

Added alongside the existing `source-link` op. Stored in the ops log like all other ops.

Register in `internal/ops/types.go`:
- Add `OpCitationAccepted = "citation-accepted"` constant
- Add `OpCitationAccepted: true` to `ValidOpTypes`

**Payload fields** — `Payload.Rationale` (`json:"rationale,omitempty"`) already exists on the flat `Payload` struct and is shared with `OpDecision`; reuse it. Only `ConfirmedNoninteractively bool \`json:"confirmed_noninteractively,omitempty"\`` is a new field addition:

| Field | JSON key | Description |
|---|---|---|
| `Rationale` | `rationale` | Required. Why citation evidence cannot be provided. Minimum 3 words; CLI rejects shorter values before emitting. `len(strings.Fields(rationale)) >= 3`. |
| `ConfirmedNoninteractively` | `confirmed_noninteractively` | `bool`, `omitempty`. Set `true` when `--ci` flag bypasses the interactive prompt. |
| `worker_id` | — | Session identity — already present on every op via the top-level `Op` struct. Not duplicated in payload. |
| `timestamp` | — | Unix epoch — already present on every op via the top-level `Op` struct. Not duplicated in payload. |

No seal, no git identity fields in the payload. Git provides the rest:

- **Named approver**: git commit author (`user.name` + `user.email`) on the commit that delivers the `.issues/ops/` change
- **Content integrity**: git commit SHA covers the ops log file
- **Cryptographic attestation**: `commit.gpgsign=true` enforceable by regulated customers via pre-receive hook — trellis does not replicate this

The accountability chain for any `citation-accepted` op is therefore: rationale + worker_id (op payload) × named author + timestamp + SHA (git commit) × optional GPG (customer policy).

**Materialized state** — add to `materialize.Issue` in `internal/materialize/state.go`:

```go
CitationAcceptances []CitationAcceptance `json:"citation_acceptances,omitempty"`
```

Where `CitationAcceptance` is a new struct:

```go
type CitationAcceptance struct {
    Rationale                  string `json:"rationale"`
    WorkerID                   string `json:"worker_id"`
    Timestamp                  int64  `json:"timestamp"`
    ConfirmedNoninteractively  bool   `json:"confirmed_noninteractively,omitempty"`
}
```

Add `case ops.OpCitationAccepted` to `engine.go`'s apply switch, appending to `CitationAcceptances`.

---

## New Command: `trls source-link`

A new top-level command registered in `cmd/trellis/main.go`. The `source-link` op type already exists in `internal/ops/types.go` and is handled in `engine.go`; no materialization changes are needed. This command provides a standalone CLI path to emit that op — previously only `trls import` emitted `source-link` ops internally.

The workflow is: register a source doc first (`trls sources add`), then link issues to it with `trls source-link`.

```
trls source-link --issue ISSUE-ID --source-id SOURCE-UUID [--section "Heading"]
```

**Behaviour:**
- `--issue` and `--source-id` are both required
- `--source-id` must match an entry in `.issues/sources/manifest.json`; rejected with `"unknown source ID %s — run 'trls sources add' to register sources"` if not found
- `--section` is optional; maps to the existing `Payload.Section` field, pinning the citation to a heading within the source doc
- Non-interactive; suitable for scripting and remediation loops

**Success output:** `"Linked <ISSUE-ID> to source <SOURCE-UUID>\n"` printed to stdout.

**Idempotency:** Duplicate `source-link` ops for the same issue/source-id pair are silently appended (consistent with the append-only log model). The validator counts a node as cited if it has at least one `source-link`; duplicates are harmless. No deduplication check required.

**Test scenarios (TDD):**
- Happy path: emits `source-link` op; node becomes cited
- Unknown `--source-id`: error, no op emitted
- Missing `--issue`: cobra usage error
- Missing `--source-id`: cobra usage error

**Validation impact:** a node with at least one `source-link` op is considered cited.

---

## New Command: `trls accept-citation`

Auditable fallback for nodes where no source material is recoverable.

```
trls accept-citation --issue ISSUE-ID --rationale "..."
```

**Behaviour:**
1. Validates `--rationale` is at least 3 words (`len(strings.Fields(rationale)) >= 3`); rejects with a usage error if not — no op emitted
2. When stdin is a TTY and `--ci` is not set, prints the following prompt and requires the operator to type the issue ID exactly (illustrative; tests assert this exact format):
   ```
   Accept citation risk for E2-001?
   This decision will be recorded in the audit log under your git identity.
   Type 'E2-001' to confirm:
   ```
3. On exact match, emits a `citation-accepted` op
4. In `--ci` mode, or when stdin is not a TTY, emits the op without prompting and sets `ConfirmedNoninteractively: true` in the payload
5. Any other input at the confirmation prompt (mismatch or empty) aborts with a non-zero exit, no op emitted

**Success output:** `"Citation risk accepted for <ISSUE-ID>\n"` printed to stdout after op is emitted.

**Test scenarios (TDD):**
- Happy path interactive: correct issue ID typed → op emitted
- Rationale < 3 words: usage error, no op emitted
- Confirmation mismatch: abort, no op emitted
- `--ci` flag: op emitted, `confirmed_noninteractively: true` in payload
- Non-TTY stdin (piped): same as `--ci` behaviour
- Missing `--issue`: cobra usage error
- Missing `--rationale`: cobra usage error

**Validation impact:** a node with at least one `citation-accepted` op (and no `source-link`) is considered cited, but counted separately in the coverage summary.

---

## Validation Change

### Logic (`internal/validate/validate.go`)

`checkE7E8E12Citations` considers a node cited if `len(SourceLinks) > 0 OR len(CitationAcceptances) > 0`.

The existing manifest-membership check (`unknown source: %s in citation for %s`) applies **only** to `source-link` ops — `citation-accepted` ops do not reference a manifest entry and are never checked against it.

### Traceability package (`internal/traceability/traceability.go`)

Add `CitationAcceptanceCount int` to `IssueRef`. Update `Compute` to treat a node as cited if `SourceLinkCount > 0 OR CitationAcceptanceCount > 0`, and separately tally:

- `AcceptedRiskNodes int` — count of nodes cited only via `citation-accepted`

Add to `Coverage` struct:
```go
AcceptedRiskNodes int     `json:"accepted_risk_nodes"`
AcceptedRiskPct   float64 `json:"accepted_risk_pct"`
```

`AcceptedRiskPct = float64(AcceptedRiskNodes) / float64(TotalNodes) * 100` (denominator is total nodes, not cited nodes).

### Pipeline (`internal/materialize/pipeline.go`)

`toTraceabilityRefs()` must populate `CitationAcceptanceCount` from `issue.CitationAcceptances`. Note: `toTraceabilityRefs` is called from two separate functions — `Materialize` and `MaterializeAndReturn` — both call sites produce a `traceability.json` file and both must produce the extended `Coverage` struct. Missing either call site will silently emit zeroes for the new fields.

### Coverage output (`cmd/trellis/validate.go`)

Human format — coverage line changes from:
```
COVERAGE: 0.0% (0/106 nodes cited)
```
to:
```
COVERAGE: 106/106 cited (102 source-linked, 4 accepted-risk)
```
The `accepted-risk` bucket is shown only when non-zero. The `source-linked` count in the human format is derived as `cited_nodes - accepted_risk_nodes`; a node with both a `source-link` op and a `citation-accepted` op counts as source-linked. There is no `source_linked_nodes` field on `Coverage` — the derivation is presentation-only.

JSON format (`--format json`) — `Coverage` struct gains the two new fields; existing fields and the `uncited` array are unchanged:
```json
{
  "total_nodes": 106,
  "cited_nodes": 106,
  "coverage_pct": 100.0,
  "accepted_risk_nodes": 4,
  "accepted_risk_pct": 3.77,
  "uncited": []
}
```

---

## Retroactive Remediation

### Step 1 — Register source docs

Register all existing planning docs as `filesystem` sources. These are the actual documents the issues were derived from:

| Source doc | Covers |
|---|---|
| `docs/trellis-prd.md` | Top-level product intent (E1, early epics) |
| `docs/superpowers/plans/2026-03-15-e2-001-multi-branch-mode.md` | E2-001-T* |
| `docs/superpowers/plans/2026-03-15-e2-002-branch-isolation.md` | E2-002-T* |
| `docs/superpowers/plans/2026-03-15-e2-003-merge-detection.md` | E2-003-T* |
| `docs/superpowers/plans/2026-03-15-e2-004-pr-done-to-merged.md` | E2-004-T* |
| `docs/superpowers/plans/2026-03-16-e3-collaboration.md` | E3-*, E3-00*-* |
| `docs/superpowers/plans/2026-03-17-ci-validation-pipeline.md` | CI-00* |
| `docs/superpowers/plans/2026-03-17-test-coverage-hardening.md` | TC-00* |
| `docs/superpowers/plans/2026-03-18-e4-index.md` | E4-S* stories |
| `docs/superpowers/plans/2026-03-18-e4-s1-tui-foundation.md` | E4-S1-T* |
| `docs/superpowers/plans/2026-03-18-e4-s2-glamour-render.md` | E4-S2-T* |
| `docs/superpowers/plans/2026-03-18-e4-s3-full-validate.md` | E4-S3-T* |
| `docs/superpowers/plans/2026-03-18-e4-s4-sources.md` | E4-S4-T* |
| `docs/superpowers/plans/2026-03-18-e4-s5-decompose-context.md` | E4-S5-T* |
| `docs/superpowers/plans/2026-03-18-e4-s6-dag-summary.md` | E4-S6-T* |
| `docs/superpowers/plans/2026-03-18-e4-s7-traceability.md` | E4-S7-T* |
| `docs/superpowers/plans/2026-03-18-e4-s8-brownfield-import.md` | E4-S8-T* |
| `docs/superpowers/plans/2026-03-18-e4-s9-stale-review.md` | E4-S9-T* |
| `docs/superpowers/plans/2026-03-19-e5-ux-tui-forensics-docs.md` | E5-S* (already registered) |

### Step 2 — Emit `source-link` ops

For each issue, emit `trls source-link --issue X --source-id Y`. Parent nodes (stories, epics) cite the umbrella doc for their epic. Work in epic order: E2 → E3 → CI/TC → E4 → E5.

E5 nodes have no existing `source_links` (confirmed via state inspection) — they are cited using the already-registered source (`2026-03-19-e5-ux-tui-forensics-docs.md`) without registering a duplicate. Use its existing UUID from the manifest.

### Step 3 — `accept-citation` the stragglers

A small number of bare-timestamp IDs (`task-1773804403`, `task-1773804476`, `story-1773804400`) predate the planning doc convention and have no recoverable source. These receive `trls accept-citation` with rationale: *"Created before planning doc convention existed; no recoverable source material."*

### Step 4 — Verify

```
trls validate
```

Expected: `COVERAGE: 106/106 cited (N source-linked, M accepted-risk)`, no `uncited node` errors.

### Step 5 — Commit

The new commands must be implemented and `make check` must be green **before** this step. Remediation is a data-only commit (ops logs + manifest); it does not require a separate `make check` run for the data itself.

```
git add .issues/ docs/superpowers/
git commit -m "chore(E5-S0-ext): retroactive citation remediation"
```

---

## E6 Scope Item: Audit Report

`trls audit` (or `trls validate --audit-report`) will surface:

- All `citation-accepted` ops: issue ID, rationale, worker_id, timestamp, git commit author (resolved via `git log` on the op file)
- Coverage summary: total nodes, source-linked count, accepted-risk count

Designed for paste-able compliance reporting. Not part of this story.

---

## Definition of Done

- `trls source-link` command implemented and tested (happy path, unknown source-id, missing flags)
- `trls accept-citation` command implemented and tested (happy path interactive, rationale < 3 words rejected, confirmation mismatch aborts, `--ci` sets `confirmed_noninteractively`, non-TTY treated as `--ci`)
- `citation-accepted` op type registered in `OpCitationAccepted` constant and `ValidOpTypes`; `Payload` struct gains `Rationale` and `ConfirmedNoninteractively` fields; `materialize.Issue` gains `CitationAcceptances []CitationAcceptance`; engine applies the op
- `traceability.IssueRef` gains `CitationAcceptanceCount`; `traceability.Coverage` gains `AcceptedRiskNodes` and `AcceptedRiskPct`; `Compute` updated; `pipeline.go` populates new field
- Validator treats `citation-accepted` as satisfying citation requirement; manifest-membership check unchanged (source-link ops only); coverage output updated in both human and JSON formats
- All 106 existing nodes cited (`trls validate` shows 0 `uncited node` errors; coverage line shows accepted-risk count)
- `make check` green before remediation commit; remediation commit is data-only (`.issues/`, `docs/superpowers/`)
