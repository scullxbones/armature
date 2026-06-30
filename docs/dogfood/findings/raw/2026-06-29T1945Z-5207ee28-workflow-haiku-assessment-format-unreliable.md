---
writer: 5207ee28-cdd8-48e6-98dc-7da179d4a40d
area: workflow
date: 2026-06-29T19:45Z
---

# Haiku subagents produce structurally invalid ConformanceAssessment JSON

## What the agent was trying to do

Dispatching three parallel haiku subagents to review SMTC-S1-T5A, T5B, and T5C
simultaneously, following the tiered model strategy (haiku for scoped review tasks).

## What happened

All three haiku agents produced JSON that failed `arm review record` validation.
Each had different structural errors:

**T5A:**
- Split `definition_of_done` into 4 separate results (`definition_of_done_1` through `definition_of_done_4`)
- Used `acceptance_N` instead of `acceptance[N]` for criterion IDs
- Cited `internal/materialize/engine_test.go` which is not in T5A's `changed_files`

**T5B:**
- Missing `schema_version: 1` field at the top level

**T5C:**
- Used `assessments` as the key instead of `results`
- Used `file` as the citation key instead of `path`
- Missing `schema_version: 1`
- Citations referenced lines outside the diff hunk ranges

The coordinator had to apply Python post-processing to fix all three before any
could be recorded. Fixes required: merging split DoD results, renaming keys,
adding `schema_version`, dropping out-of-scope citations, and downgrading
out-of-bounds line citations to path-level.

## Why it matters

- The haiku-for-parallel-review strategy requires a working assessment JSON as
  output. If the coordinator must manually patch every assessment, the time
  savings from parallelism are eroded.
- The errors are not random — they cluster around schema compliance: field names,
  result key format (`acceptance[N]` vs `acceptance_N`), and top-level keys.
  Haiku is not reliably internalizing the schema from the prompt alone.
- T5B and T5C errors (missing `schema_version`, wrong top-level key) suggest
  haiku is generating plausible-looking JSON from memory rather than following
  the exact schema shown in the prompt.

## Contrast with sonnet

Prior sonnet-reviewed tasks (T3, T4) produced valid JSON on the first attempt,
with only one citation line issue on T2 (out-of-bounds line, fixable in one step).
Haiku required 3–5 fixes per assessment.

## Evidence

- T5A: `arm review record` → `conformance assessment: unsupported schema version 0`
- T5B: `arm review record` → `conformance assessment: unsupported schema version 0`
- T5C: `arm review record` → `conformance assessment: no results provided`
- Post-processing Python script required for all three before any recorded successfully

## Potential mitigations

- Include a verbatim JSON schema example in the reviewer prompt when using haiku,
  with explicit field names and types. The current prompt describes the schema
  in prose; haiku needs it as a concrete template.
- Add a validation pre-flight before dispatching to `arm review record`:
  check `schema_version`, `results` key presence, and criterion ID format
  (`acceptance[N]`) so errors are caught and retried before coordinator involvement.
- Reserve haiku for narrow tasks (single criterion, path-only citations) and use
  sonnet for full multi-criterion assessments.
