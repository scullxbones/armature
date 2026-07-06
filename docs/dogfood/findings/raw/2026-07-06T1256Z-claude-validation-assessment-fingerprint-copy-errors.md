---
date: 2026-07-06
agent: claude
area: validation
task: EXECEV coordination
tags: [review-record, fingerprints, reviewer-subagent]
---

# Reviewer subagents copy stale fingerprints into assessments; record rejects them

## User Goal

Coordinator recording ConformanceAssessments from reviewer subagents via `arm review record` for EXECEV-T1..T5.

## Observed

On a re-review (same reviewer agent resumed with an updated bundle after remediation), the reviewer reused the previous bundle's `delivery_fingerprint` in its assessment JSON even though `bundle_id` was updated. `arm review record` correctly rejected it ("assessment delivery_fingerprint ... does not match bundle delivery_fingerprint ..."). The coordinator had to patch the assessment's fingerprints from the bundle file and retry. A later reviewer needed an explicit prompt instruction ("copy bundle_id and fingerprints.contract/fingerprints.delivery EXACTLY from the bundle file") to avoid this.

## Impact

Each occurrence costs a failed record + manual JSON surgery by the coordinator. The failure is silent from the reviewer's perspective — it returns a plausible assessment that fails integrity checks downstream. Patching fingerprints coordinator-side also weakens the attestation story (the coordinator is editing fields the reviewer was supposed to attest to).

## Evidence

- `arm review record --issue EXECEV-T1 ...` exit 1: `assessment delivery_fingerprint 1a0db372... does not match bundle delivery_fingerprint d0c1d75e...`
- Fixed by copying `fingerprints.contract`/`fingerprints.delivery` from `/tmp/tmp.c05LkfOsFU` into the assessment.

## Suggested Follow-Up

Have `arm review record` (or a `--from-bundle` flag) source the fingerprints from the bundle itself and require the reviewer to attest only to `bundle_id` — or make the reviewer skill state loudly that fingerprints must be copied from the bundle file verbatim on every (re-)review.
