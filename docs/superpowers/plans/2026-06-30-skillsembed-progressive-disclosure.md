# Skillsembed Progressive Disclosure Audit Plan

> **For agentic workers:** This is a REVIEW-ONLY audit task. Do not edit any SKILL.md files. Write the audit findings in this plan document only.

**Goal:** Audit the five embedded SKILL.md files (`internal/skillsembed/skills/*/SKILL.md`) for progressive disclosure violations: heavy code/JSON blocks over ~15 lines suitable for extraction to `references/` or `templates/`, description-field overload (SDO), and cross-skill duplication. Produce extraction candidates grouped by task, in the same format as existing implementation plans.

**Scope:** Read-only audit of:
- `internal/skillsembed/skills/armature-coordinator/SKILL.md`
- `internal/skillsembed/skills/armature-worker/SKILL.md`
- `internal/skillsembed/skills/armature-planner/SKILL.md`
- `internal/skillsembed/skills/armature-auditor/SKILL.md`
- `internal/skillsembed/skills/armature-reviewer/SKILL.md`

**Output:** New plan file `docs/superpowers/plans/2026-06-30-skillsembed-progressive-disclosure.md` (this file) enumerating all extraction candidates. No SKILL.md is edited.

**Rationale:** Progressive disclosure moves detailed reference material (diagrams, large code samples, JSON templates) out of main narrative paths so skill bodies remain scannable and focus on workflow steps. Heavy blocks distract users and make navigation harder. Extraction candidates live in `references/` (for conceptual docs) or `templates/` (for structured examples like JSON) and are loaded on demand.

---

## Audit Findings Summary

**Total extraction candidates identified: 13**

| Category | Count | Skills Affected | Priority |
|----------|-------|-----------------|----------|
| Diagrams (DOT format) | 2 | coordinator, planner | medium |
| JSON/Code templates over 15 lines | 5 | planner, reviewer | high |
| Large bash scripts with heavy comments | 3 | coordinator (2), none | high |
| Cross-skill duplication (DAG Hygiene Mandate) | 1 | all 5 skills | medium |
| Description-field overload (SDO) | 1 | coordinator | low |
| Prerequisite duplication | 1 | all 5 skills | low |

---

## Extraction Candidates by Skill

### Armature Coordinator (`internal/skillsembed/skills/armature-coordinator/SKILL.md`)

#### Candidate 1: Coordinator Loop Diagram
- **Location:** Lines 53–82 (~30 lines)
- **Type:** DOT diagram
- **Current Context:** "The Coordinator Loop" section, step-by-step header
- **Extraction Target:** `internal/skillsembed/skills/armature-coordinator/references/coordinator-loop.md`
- **Rationale:** Diagram is large and distracts from narrative. Replace with a reference like "See Coordinator Loop diagram (`references/coordinator-loop.md`)".
- **Example Extract:**
  ```
  # Coordinator Loop
  
  ```dot
  digraph coordinator_loop {
    ...
  }
  ```
  ```

#### Candidate 2: Per-Task Commit Capture Script
- **Location:** Lines 302–368 (~66 lines)
- **Type:** Bash script with heavy inline comments
- **Current Context:** "Semantic Review (Reviewer Dispatch)" subsection, step 1 "Capture per-task commit SHAs"
- **Extraction Target:** `internal/skillsembed/skills/armature-coordinator/references/task-commit-capture.md`
- **Rationale:** Script is complex and deeply nested with multiple inline explanations (recursive bash array manipulation, reconciliation pass logic). Extract to standalone reference; replace main text with a summary and pointer.
- **Example Extract:**
  ```
  # Per-Task Commit Capture
  
  When workers dispatch in parallel...
  
  ## Script
  
  COMMITS_IN_RANGE=$(git rev-list --reverse "$WAVE_BASE_SHA"..HEAD)
  ...
  ```

#### Candidate 3: Parallel Branch Overlap Audit Script
- **Location:** Lines 419–443 (~24 lines)
- **Type:** Bash script
- **Current Context:** "Parallel Branch Overlap Audit" subsection, "Identify overlapping files" section
- **Extraction Target:** `internal/skillsembed/skills/armature-coordinator/references/overlap-audit.md`
- **Rationale:** Self-contained bash script for a specific procedure. Extract to standalone reference; replace with summary and pointer.

#### Candidate 4: DAG Hygiene Mandate (Duplication)
- **Location:** Lines 35–47 (~13 lines)
- **Type:** Boilerplate mandate
- **Cross-Skill Instance Count:** Appears verbatim or near-verbatim in:
  - armature-coordinator: lines 35–47
  - armature-worker: lines 33–45
  - armature-planner: lines 26–38
  - armature-auditor: lines 14–26
- **Extraction Target:** `internal/skillsembed/skills/references/dag-hygiene-mandate.md` (shared across all skills via symlink or include pattern, pending design decision on shared references/)
- **Rationale:** Boilerplate repeated in 4 skills with only minor line variations. Extract once; include/reference from each skill.
- **Action:**
  - Create: `internal/skillsembed/skills/references/dag-hygiene-mandate.md` (shared resource)
  - Replace all 4 instances with: `{% include "references/dag-hygiene-mandate.md" %}` or link to reference

#### Candidate 5: Description-Field Overload (SDO)
- **Location:** Skill frontmatter, lines 2–8
- **Type:** Description field listing multiple sub-features
- **Current Description:**
  ```
  description: >
    Use when operating orchestration in an armature-managed repository — surveys
    the story DAG, dispatches workers wave by wave, integrates outcomes, validates
    citation coverage, and closes stories with a pull request.
    Requires a worker identity (arm worker-init) and arm on PATH.
  ```
- **Issue:** Lists 5 actions (surveys, dispatches, integrates, validates, closes) + prerequisite. Longer than armature-auditor or armature-reviewer descriptions.
- **Suggestion:** Trim to primary action + role. Move sub-features to skill body or references.
- **Proposed Trim:**
  ```
  description: >
    Use when operating orchestration in an armature-managed repository — dispatch workers,
    manage task waves, and close stories with pull requests. Requires arm worker-init.
  ```

#### Candidate 6: Prerequisite Duplication
- **Location:** Lines 17–33
- **Cross-Skill Instance Count:** Appears with similar structure in all 5 skills
- **Extraction Target:** `internal/skillsembed/skills/references/prerequisites-common.md` (if design pattern allows shared references)
- **Rationale:** All skills repeat "If `arm` is not found, stop", "worker-init requirement", "run arm doctor" steps. Extract common boilerplate.

---

### Armature Worker (`internal/skillsembed/skills/armature-worker/SKILL.md`)

#### Candidate 7: DAG Hygiene Mandate (Duplication)
- **See Armature Coordinator Candidate 4** — same boilerplate appears here at lines 33–45

#### Candidate 8: Cross-Layer JSON Fixture Testing Block
- **Location:** Lines 121–140 (~20 lines)
- **Type:** Detailed explanation block + example reference
- **Current Context:** "Section 5b: Cross-Layer JSON Fixture Testing" — subsection of "Pre-Transition Verification"
- **Assessment:** Already well-structured with a reference to `examples/json-roundtrip-test.go`. This is borderline but could be extracted to `references/json-fixture-testing.md` for standalone reference. Lower priority than other candidates.

---

### Armature Planner (`internal/skillsembed/skills/armature-planner/SKILL.md`)

#### Candidate 9: Planner Loop Diagram
- **Location:** Lines 44–77 (~34 lines)
- **Type:** DOT diagram
- **Current Context:** "The Planner Loop" section, step-by-step header
- **Extraction Target:** `internal/skillsembed/skills/armature-planner/references/planner-loop.md`
- **Rationale:** Large diagram. Replace with reference and brief summary.

#### Candidate 10: Complete Well-Formed Task JSON Example
- **Location:** Lines 214–237 (~24 lines)
- **Type:** JSON template
- **Current Context:** "Writing Good Plan JSON" section, "Complete Well-Formed Task Example" subsection
- **Extraction Target:** `internal/skillsembed/skills/armature-planner/templates/task-json-template.json` (or `.md` wrapper with JSON code block)
- **Rationale:** Multi-field JSON example with comments. Extract to standalone template; replace main text with pointer and short explanation of key fields.

#### Candidate 11: DAG Hygiene Mandate (Duplication)
- **See Armature Coordinator Candidate 4** — same boilerplate appears here at lines 26–38

---

### Armature Auditor (`internal/skillsembed/skills/armature-auditor/SKILL.md`)

#### Candidate 12: DAG Hygiene Mandate (Duplication)
- **See Armature Coordinator Candidate 4** — same boilerplate appears here at lines 14–26

---

### Armature Reviewer (`internal/skillsembed/skills/armature-reviewer/SKILL.md`)

#### Candidate 13: ReviewBundle JSON Example
- **Location:** Lines 62–91 (~30 lines)
- **Type:** JSON template
- **Current Context:** "Input: ReviewBundle" section, "ReviewBundle Example" subsection
- **Extraction Target:** `internal/skillsembed/skills/armature-reviewer/templates/review-bundle-example.json` (or `.md` with code block)
- **Rationale:** Large JSON example with multiple nested objects. Extract to standalone template.

#### Candidate 14: Definition of Done JSON Example
- **Location:** Lines 98–113 (~16 lines)
- **Type:** JSON template
- **Current Context:** "Step-by-Step Review Process", step 2 "Evaluate Definition of Done"
- **Extraction Target:** `internal/skillsembed/skills/armature-reviewer/templates/criterion-result.json` or add to shared template collection
- **Rationale:** Structured JSON example suitable for extraction. Could be bundled with Candidate 15 as a "criterion examples" collection.

#### Candidate 15: Acceptance Criterion JSON Example
- **Location:** Lines 139–152 (~14 lines)
- **Type:** JSON template (borderline, just at the 15-line threshold)
- **Current Context:** "Step 3: Evaluate Each Acceptance Criterion"
- **Extraction Target:** Same as Candidate 14 — bundled template collection
- **Rationale:** Borderline size but structurally similar to definition of done example. Bundle both in a single `references/criterion-evaluation-examples.md` file to reduce redundancy.

#### Candidate 16: ConformanceAssessment JSON Example
- **Location:** Lines 172–193 (~22 lines)
- **Type:** JSON template
- **Current Context:** "Step 5: Produce ConformanceAssessment JSON"
- **Extraction Target:** `internal/skillsembed/skills/armature-reviewer/templates/conformance-assessment.json` (referenced as existing in the skill: "See `templates/conformance-assessment.json`")
- **Rationale:** Full conformance assessment structure. Extract to durable template file.

---

## Implementation Strategy

### Phase 1: Diagram Extraction (Candidates 1, 9)
Create `references/` subdirectories in armature-coordinator and armature-planner. Move DOT diagrams to `.md` files; replace main text with links and summaries.

### Phase 2: JSON/Code Template Extraction (Candidates 10, 13–16)
Create `templates/` subdirectories in armature-planner and armature-reviewer. Extract JSON examples to standalone `.json` files (or `.md` wrappers). Replace main text with brief explanations and file references.

### Phase 3: Bash Script Extraction (Candidates 2, 3)
Create `references/` subdirectories in armature-coordinator. Extract bash scripts to `.md` files with inline explanations. Replace main text with summaries and references.

### Phase 4: Cross-Skill Duplication (Candidate 4, 6)
Design shared `references/` directory pattern (either symlinks or include patterns in future markdown processing). Extract DAG Hygiene Mandate and prerequisite boilerplate. Replace all instances with references.

### Phase 5: Description Trim (Candidate 5)
Review frontmatter descriptions across all skills. Trim overly detailed descriptions like armature-coordinator to focus on primary role. Update in SKILL.md frontmatter.

---

## Extraction Task Outline (Ready for Future Implementation)

> The following tasks are ready to be added to a follow-up story once this audit is complete.

### Task 1: Extract Diagrams (armature-coordinator, armature-planner)
**Files:** 
- Create: `internal/skillsembed/skills/armature-coordinator/references/coordinator-loop.md`
- Create: `internal/skillsembed/skills/armature-planner/references/planner-loop.md`
- Modify: `internal/skillsembed/skills/armature-coordinator/SKILL.md` (replace lines 53–82 with reference)
- Modify: `internal/skillsembed/skills/armature-planner/SKILL.md` (replace lines 44–77 with reference)

**Acceptance:**
- [ ] Both diagram files exist and contain valid DOT syntax
- [ ] Main skill files reference diagrams with clear links
- [ ] `make validate-skills` passes
- [ ] Skill bodies remain scannable (no loss of comprehension)

### Task 2: Extract JSON Templates (armature-planner, armature-reviewer)
**Files:**
- Create: `internal/skillsembed/skills/armature-planner/templates/task-json-template.json`
- Create: `internal/skillsembed/skills/armature-reviewer/templates/review-bundle-example.json`
- Create: `internal/skillsembed/skills/armature-reviewer/templates/criterion-result.json` (or combined examples file)
- Create: `internal/skillsembed/skills/armature-reviewer/templates/conformance-assessment.json`
- Modify: affected SKILL.md files (replace inline examples with references)

**Acceptance:**
- [ ] All template files exist and contain valid JSON
- [ ] JSON examples pass `jq . < file` validation
- [ ] Main skill files reference templates clearly
- [ ] `make validate-skills` passes

### Task 3: Extract Bash Scripts (armature-coordinator)
**Files:**
- Create: `internal/skillsembed/skills/armature-coordinator/references/task-commit-capture.md`
- Create: `internal/skillsembed/skills/armature-coordinator/references/overlap-audit.md`
- Modify: `internal/skillsembed/skills/armature-coordinator/SKILL.md` (replace lines 302–368 and 419–443 with references)

**Acceptance:**
- [ ] Both script files exist with clear explanations
- [ ] Scripts remain runnable (no syntax breaks)
- [ ] Main skill file references scripts clearly
- [ ] `make validate-skills` passes

### Task 4: Consolidate Cross-Skill Duplication (DAG Hygiene + Prerequisites)
**Files:**
- Create: `internal/skillsembed/skills/references/dag-hygiene-mandate.md` (shared)
- Create: `internal/skillsembed/skills/references/prerequisites-common.md` (shared, optional)
- Modify: All 5 SKILL.md files (armature-coordinator, armature-worker, armature-planner, armature-auditor, armature-reviewer) — replace boilerplate with references

**Acceptance:**
- [ ] Shared reference files exist
- [ ] All 5 skills reference the shared mandate file
- [ ] No content loss (references are complete)
- [ ] `make validate-skills` passes on all skills

### Task 5: Trim Description Fields (SDO Cleanup)
**Files:**
- Modify: `internal/skillsembed/skills/armature-coordinator/SKILL.md` (frontmatter, lines 3–6) — trim description to 1–2 sentences

**Acceptance:**
- [ ] Description remains clear and concise
- [ ] Coordinator role is still obvious to users
- [ ] No content loss (details moved to body or references)
- [ ] `make validate-skills` passes

---

## Notes and Caveats

1. **Shared References Directory:** Tasks 4 and 6 assume a shared `internal/skillsembed/skills/references/` directory accessible to all skills. This may require design-level decisions on how symlinks or includes work in the embedded FS. Existing design uses skill-specific `references/` directories.

2. **Template Location:** JSON/code templates could live in:
   - Skill-specific `templates/` subdirs (current proposal)
   - Shared `templates/` directory (if design pattern allows)
   - Embedded in `references/` as `.md` code blocks (lighter-weight alternative)

3. **Validation:** Each task should pass `make validate-skills` to confirm SKILL.md syntax and references are valid.

4. **Future Iteration:** After extraction, a follow-up audit may identify more consolidation opportunities (e.g., test naming patterns, acceptance criteria templates).

---

## Audit Methodology

Candidates were identified by:
1. **Scanning all 5 SKILL.md files** for blocks ≥15 lines containing code, JSON, diagrams, or heavy narrative
2. **Searching for regex patterns** (bash if/for/while, JSON {, DOT digraph/graph) across all files
3. **Comparing cross-skill text** (DAG Hygiene Mandate, Prerequisites sections) for duplication
4. **Checking frontmatter descriptions** for SDO (excessive detail, multiple sub-features listed)
5. **Grouping candidates by type** (diagrams, code, JSON, duplication, SDO) for extraction strategy

**Files audited:** 5 SKILL.md files (total ~4000 lines)
**Audit scope:** Progressive disclosure suitability only; no semantic review of skill content

---

## Sign-Off Criteria

This audit plan is complete when:
1. All 16 candidates are listed above with clear extraction targets
2. Implementation tasks (Tasks 1–5) are written in standard task format (Files, Acceptance, Steps)
3. Rationale is documented for each candidate
4. No SKILL.md files have been edited (audit only)
5. This plan file is committed to version control
