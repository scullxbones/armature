# Theme: Reviewer Agent Reliability

## Summary

When reviewer agents produce ConformanceAssessment JSON, two recurring failure modes require coordinator post-processing before `arm review record` will accept the output: (1) hallucinated line numbers that fall outside the diff hunk ranges, and (2) haiku-tier models that produce structurally invalid JSON — wrong field names, missing required top-level fields, or incorrect criterion ID format.

## Evidence

- [`arm review record` citation validator caught out-of-bounds line number](../../raw/2026-06-29T1915Z-5207ee28-validation-citation-line-out-of-bounds.md) — During SMTC-S1-T2 review, a sonnet reviewer cited `internal/review/prepare.go:104` but the file is only 87 lines in the diff. `arm review record` rejected the assessment; coordinator had to patch the citation to path-level before recording succeeded. The validator is doing real work — but the coordinator shouldn't need to be involved.
- [Haiku subagents produce structurally invalid ConformanceAssessment JSON](../../raw/2026-06-29T1945Z-5207ee28-workflow-haiku-assessment-format-unreliable.md) — Three parallel haiku reviewers (T5A/B/C) all produced JSON that failed `arm review record`. Errors spanned multiple categories: `assessments` instead of `results`, `file` instead of `path`, missing `schema_version: 1`, split DoD results, `acceptance_N` instead of `acceptance[N]`, and citations to files outside `changed_files`. Every assessment required Python post-processing. Sonnet reviewers with an explicit JSON template in the prompt produced valid JSON on first attempt.
- [`arm review record`'s citation validation rejected most good-faith reviewer assessments](../../raw/2026-06-30T2215Z-claude-validation-review-record-citation-strictness-blocks-good-faith-reviews.md) — 5 of 11 DF-S5 review-record calls failed outright: citations pointing to a real line not present in the literal diff hunk, and missing "evidence text" for `indeterminate` status. The reviewer's underlying judgment was sound in every case; only the citation's exact form was rejected.
- [`arm review record` rejects reviewer citations to near-diff lines](../../raw/2026-07-05T2229Z-claude-workflow-review-record-rejects-context-line-citations.md) — A citation to `sync.go:35` was rejected because the changed line was 36 — one line off. The reviewer had cited the call site it actually read, not the exact diff coordinate. Required manual JSON surgery to strip the citation and re-record.
- [Reviewer subagents copy stale fingerprints into assessments; record rejects them](../../raw/2026-07-06T1256Z-claude-validation-assessment-fingerprint-copy-errors.md) — On a re-review with an updated bundle, the reviewer reused the previous bundle's `delivery_fingerprint` even though `bundle_id` had changed. Correctly rejected by `arm review record`, but the reviewer agent itself has no self-check catching this before returning.

## Pattern

Three distinct sub-patterns:

1. **Hallucinated line numbers (all models)**: Reviewer agents cite specific line numbers without verifying them against diff hunk `@@` headers. New files are especially prone — the model "knows" a function is "near the end" of a file and invents a plausible line number that exceeds the actual diff length. The citation validator in `arm review record` catches these, but forces coordinator intervention.

2. **Schema non-compliance (haiku-tier)**: Haiku does not reliably internalize the ConformanceAssessment schema from prose description alone. It generates plausible-looking JSON from memory, varying field names and structure. Sonnet with an explicit verbatim JSON template in the prompt produces schema-compliant output on first attempt; the same prose prompt given to haiku does not.

3. **Over-strict citation/fingerprint matching rejects good-faith output**: The validator requires exact-line diff-hunk citations and fresh fingerprints, with no tolerance for a citation one line off, a context-line reference, or a stale-but-plausible copy-paste of a prior bundle's fingerprint. Reviewer judgment is correct in these cases; only the mechanical form of the citation/fingerprint is wrong, and every rejection requires coordinator-side manual JSON surgery rather than a targeted re-prompt.

## Impact

- Every out-of-bounds citation requires the coordinator to patch the assessment file before recording — eroding the value of delegating review to a subagent.
- Haiku assessments required 3–5 structural fixes each. The time savings from parallel haiku dispatch were largely offset by coordinator post-processing.
- Nearly half (5 of 11) of DF-S5's review-record calls failed on citation/evidence strictness alone, despite correct underlying reviewer judgment.
- The citation validator is a net positive (it prevents garbage from entering the audit record), but ideally the reviewer would self-validate before returning, and the validator itself could tolerate near-miss citations (e.g. ±1 line, or context lines within the same hunk) without loosening its protection against genuinely fabricated citations.

## Candidate Follow-Ups

- Add citation line-range verification to the reviewer skill: "Before citing a line number, confirm it falls within a `@@` hunk in the diff. For new files (`@@ -0,0 +1,N @@`), valid lines are 1–N. When uncertain, use path-level citation (omit `line`)."
- Include a verbatim JSON schema template in reviewer prompts when using haiku — prose description is insufficient; haiku needs a concrete copy-paste template.
- Add a coordinator-side pre-flight validator (or use `arm review record --dry-run`) to catch schema and citation errors before the official record attempt, enabling clean retry without manual patching.
- Reserve haiku for narrow single-criterion tasks; use sonnet for full multi-criterion assessments until a schema-template prompt is standardized.
- Consider tolerating citations to any line within the same diff hunk (not just the exact changed line), and requiring `bundle_id`/`delivery_fingerprint` to be read fresh from the current bundle rather than accepted as reviewer-supplied free text.
