# Theme: Agent-Facing Views Omit Materialized State

## Summary

Writes succeed. The commands agents use to *confirm* the write look like
they answered, and the answer looks like "not set." `jq .field` on a
missing JSON key is `null`. `--field` on an unsupported name prints
empty with no error. Human `arm show` skips the same fields.

The materializer has the data (`Issue.Branch`, `Issue.PR`, provenance,
source links). The `IssueJSON` DTO and the human template do not copy
it. Agents then re-run the write, escalate, or read the raw op log.

This is the read-side twin of [missing-remediation-verbs](../missing-remediation-verbs/README.md)
(no verb to correct) and of [i6-promotion-agent-owned](../i6-promotion-agent-owned/README.md)
(no automatic promotion): I6 metadata can be recorded and still be
invisible.

## Evidence

- [`arm show` JSON prints `pr: null` after `arm merged --pr`](../../raw/2026-08-17T0236Z-5207ee28-commands-show-json-omits-pr-and-branch.md) — Ops had `pr: "109"` / `"108"`. `IssueJSON` has no `pr` or `branch`. `has("pr")` is false.
- [`arm show --format json` reports null confidence, source_links, context_files right after they were written](../../raw/2026-08-17T0130Z-5207ee28-commands-show-json-omits-provenance.md) — Create used `--confidence draft`, `--source`, `--context-file`; immediate JSON show still null. Human show omitted confidence too.
- [`arm show --field dod` silently returns empty](../../raw/2026-08-02T1500Z-claude-tooling-arm-show-field-dod-unsupported.md) — Cross-listed under [missing-remediation-verbs](../missing-remediation-verbs/README.md). Same "looks like it answered" tax, seven times.

## Candidate Follow-Ups

- Put every field the materializer stores that an agent needs to verify a write onto `IssueJSON` and the human show template. Prefer omitted-when-empty over `null` so `has("pr")` means "was recorded."
- `--field` should error on unknown names, not print nothing.
- Do not treat a skill note as the fix; agents verify by reading `arm show`.
