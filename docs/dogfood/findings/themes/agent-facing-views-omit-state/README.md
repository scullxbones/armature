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
- [`arm show` omitting `provenance.confidence` led a reviewing agent to demand an assertion on a field that does not exist](../../raw/2026-09-02T1045Z-claude-commands-show-omits-confidence-misleads-reviewer.md) — Third consumer of the same hole, and a new argument: the reviewer read `internal/materialize/state.go` and assumed the CLI surfaced what the model holds. `jq '.provenance.confidence == "draft"'` on an object with no `provenance` key is worthless in both directions. The Go model is the de-facto spec, so anything `show` drops becomes a source of confidently-wrong agent-to-agent instructions.
- [`arm list --format json` omits `blocked_by`, so any gating question costs N+1 calls](../../raw/2026-09-01T1156Z-claude-workflow-arm-list-json-omits-blocked-by.md) — The bulk-read surface emits `id, status, title, type, parent` and no edges, so a whole-DAG "what does this block" question is one `arm list` plus 765 `arm show` calls (~25 min serial). The expensive path then gets skipped: a prior audit checked four items by hand and recorded that it had not checked the rest. The pattern to watch for is any command whose JSON is a strict subset of `arm show`'s for the same issue.

## Candidate Follow-Ups

- Put every field the materializer stores that an agent needs to verify a write onto `IssueJSON` and the human show template. Prefer omitted-when-empty over `null` so `has("pr")` means "was recorded."
- `--field` should error on unknown names, not print nothing.
- Emit `blocked_by` / `blocks` in `arm list --format json` — both are already materialized, so this is a projection change, not new computation. A `--fields` flag serves better than a fixed minimal projection: only the caller knows whether it is asking a graph question or rendering a status table.
- Do not treat a skill note as the fix; agents verify by reading `arm show`.
