# Orchestrate Runtime Phase 1 Boundary Note Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the approved Phase 1 runtime boundary decisions as a concise append-only note in the canonical direction doc, without broadening scope into runtime control-model design, audit schema design, or final CLI naming.

**Architecture:** This is a documentation-only implementation slice. The detailed rationale remains in `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`, while `docs/design/orchestrate-runtime-direction.md` becomes the short boundary reference future design work must obey. The work should be append-only and should not restructure the existing runtime direction document.

**Tech Stack:** Markdown, git, ripgrep, sed

**Spec:** `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`

---

## File Map

### Modified Files
- `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md` — update status from `Draft` to `Approved` now that the design has been reviewed and accepted
- `docs/design/orchestrate-runtime-direction.md` — append the short Phase 1 boundary decision note that freezes `v1` posture, deterministic core, exception lane guardrails, enforcement envelope, non-goals, and deferred items

### No-Change Files
- `docs/design/orchestrate-runtime-index.md` — leave unchanged for this slice; the spec explicitly keeps the deliverable small and centered on the direction document
- `docs/design/orchestrate-runtime-roadmap.md` — leave unchanged; this plan does not attempt to mark later phases complete

---

## Chunk 1: Approve The Spec Artifact

### Task 1: Mark The Reviewed Spec As Approved

**Files:**
- Modify: `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`

- [ ] **Step 1: Change the spec status line from `Draft` to `Approved`**

Replace this line near the top of the file:

```markdown
**Status:** Draft
```

with:

```markdown
**Status:** Approved
```

- [ ] **Step 2: Verify the status change**

Run:

```bash
rg -n "^\*\*Status:\*\* " docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md
```

Expected:

```text
3:**Status:** Approved
```

- [ ] **Step 3: Commit the spec status update**

```bash
git add docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md
git commit -m "docs(spec): mark runtime phase 1 boundary design approved"
```

---

## Chunk 2: Append The Canonical Boundary Note

### Task 2: Add The Phase 1 Boundary Decision Note To The Runtime Direction Doc

**Files:**
- Modify: `docs/design/orchestrate-runtime-direction.md`

- [ ] **Step 1: Append a new section at the end of `docs/design/orchestrate-runtime-direction.md`**

Add this exact Markdown block after the existing `Future Research Note` section:

```markdown
## Phase 1 Boundary Decision Note (2026-05-09)

Phase 1 locks runtime semantics first and intentionally leaves the final CLI
surface undecided. Future CLI work may explore `arm worker run`,
`arm orchestrate --loop`, or another shape, but must preserve the boundary
below.

### V1 Runtime Posture

- `v1` is a value-first bounded hybrid runtime.
- Its job is to continuously identify clearly executable work, claim it
  deterministically, invoke the existing single-task orchestrator, and drain
  ready work without a human-operated loop.
- The runtime wraps the existing single-task orchestrator. It does not replace
  it.
- The runtime may make one bounded recovery attempt for selected
  `policy_evaluable` failures and must escalate unresolved or overly ambiguous
  cases with audit traceability.

### Deterministic Core

The following remain strictly deterministic and outside exception-agent
discretion in `v1`:

- queue polling
- claim attempts and claim win/loss handling
- retry and backoff behavior
- worker routing
- provenance refresh
- Definition of Executability fallback choice when policy already defines a safe
  option

### Exception-Agent Lane

The exception-agent lane is allowed in `v1` only as a narrow recovery
mechanism:

- entry only from explicitly enumerated recovery cases
- at most one bounded recovery attempt per triggering condition before
  escalation or cooldown
- recovery actions stay within a policy-approved action envelope
- the agent may choose among permitted recovery actions and explain its choice
- the agent may not broaden scope, rewrite task intent, or decompose work
  autonomously

Exception-agent safety is enforced by deterministic controls outside the agent,
not by prompt-only behavioral guidance. The real boundary is a runtime-enforced
permission profile, allowed-action contract, deterministic entry conditions, and
deterministic validation of any proposed recovery action. If the bounded
permission profile is insufficient, the runtime escalates rather than widening
permissions.

### Explicit V1 Non-Goals

- final CLI naming
- persistent agent supervisors
- autonomous decomposition or replanning
- broad semantic rewriting of tasks
- distributed workflow infrastructure

### Deferred By Design

This note does not settle the full worker state machine, complete audit schema,
detailed policy configuration surface, or full list of exception-agent recovery
cases.
```

- [ ] **Step 2: Verify the note was appended with the expected headings**

Run:

```bash
rg -n "Phase 1 Boundary Decision Note|V1 Runtime Posture|Deterministic Core|Exception-Agent Lane|Explicit V1 Non-Goals|Deferred By Design" docs/design/orchestrate-runtime-direction.md
```

Expected:

```text
<line>:## Phase 1 Boundary Decision Note (2026-05-09)
<line>:### V1 Runtime Posture
<line>:### Deterministic Core
<line>:### Exception-Agent Lane
<line>:### Explicit V1 Non-Goals
<line>:### Deferred By Design
```

- [ ] **Step 3: Manually read the appended note and confirm the four acceptance questions are answerable from this document alone**

Run:

```bash
sed -n '1,260p' docs/design/orchestrate-runtime-direction.md
```

Confirm the note makes these answers obvious without cross-referencing other
docs:

- what is the runtime allowed to do in `v1`?
- what must remain deterministic?
- when may an exception agent run?
- what is definitely out of scope?

- [ ] **Step 4: Commit the appended boundary note**

```bash
git add docs/design/orchestrate-runtime-direction.md
git commit -m "docs(design): append orchestrate runtime phase 1 boundary note"
```

---

## Chunk 3: Final Consistency Pass

### Task 3: Verify The Implementation Stayed Inside The Approved Scope

**Files:**
- Modify: none
- Verify: `docs/design/orchestrate-runtime-direction.md`
- Verify: `docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md`

- [ ] **Step 1: Check the final diff only touches the approved spec and direction doc**

Run:

```bash
git diff --name-only HEAD~2..HEAD
```

Expected:

```text
docs/design/orchestrate-runtime-direction.md
docs/superpowers/specs/2026-05-09-orchestrate-runtime-phase1-boundaries-design.md
```

- [ ] **Step 2: Verify we did not accidentally broaden scope into the roadmap or index**

Run:

```bash
git diff --name-only HEAD~2..HEAD | rg "orchestrate-runtime-(index|roadmap)\.md"
```

Expected: no output

- [ ] **Step 3: Run the lightweight doc sanity check**

Run:

```bash
git diff --check HEAD~2..HEAD
```

Expected: no output

- [ ] **Step 4: Summarize the completed Phase 1 deliverable in the commit log**

Run:

```bash
git log --oneline -2
```

Expected:

```text
<sha> docs(design): append orchestrate runtime phase 1 boundary note
<sha> docs(spec): mark runtime phase 1 boundary design approved
```

---

## Spec Coverage Check

- `CLI surface remains undecided` is implemented by the opening paragraph of the appended boundary note.
- `V1 is a value-first bounded hybrid runtime` is implemented by the `V1 Runtime Posture` section.
- `Deterministic core stays outside exception-agent discretion` is implemented by the `Deterministic Core` section.
- `Exception lane is narrow and runtime-enforced` is implemented by the `Exception-Agent Lane` prose and guardrail bullets.
- `Prompts are not the policy boundary` is implemented by the deterministic-controls paragraph in `Exception-Agent Lane`.
- `Explicit v1 non-goals and deferred items stay visible` is implemented by `Explicit V1 Non-Goals` and `Deferred By Design`.

## Placeholder Scan

This plan intentionally contains:

- exact file paths
- exact Markdown to append
- exact commands to run
- explicit expected outputs for each verification step

It intentionally does not contain:

- runtime implementation tasks
- decision-record extraction
- DoR/DoE matrix work
- CLI finalization work

Those are outside the approved spec for this slice.
