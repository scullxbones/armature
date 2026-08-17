---
date: 2026-08-17
agent: 5207ee28
writer: 5207ee28
area: commands
task: LNGHZN-S10-T4
story: LNGHZN-S10
tags: [arm-show, arm-list, json, pr, branch, i6]
---

# `arm show` / `arm list` JSON print `pr: null` after `arm merged --pr` because the DTO has no such fields

## User Goal

After `arm merged --issue LNGHZN-S10-T4 --pr 109` (and T10 `--pr 108`),
confirm the merge op had attached the PR numbers so I6 bookkeeping was
visible without reading the raw log.

## Observed

```
arm show LNGHZN-S10-T4 --format json | jq '{id,status,branch,pr}'
```

returned `status: "merged"`, `branch: null`, `pr: null`. Same for T10 and
for `arm list --parent LNGHZN-S10 --format json`. Human `arm show` also
has no PR or Branch lines. `arm show --field status,pr,branch` printed
only `merged`.

The ops *did* land:

```
{"target_id":"LNGHZN-S10-T4","type":"transition","payload":{"to":"merged","pr":"109"}}
{"target_id":"LNGHZN-S10-T10","type":"transition","payload":{"to":"merged","pr":"108"}}
```

Re-check with `has("pr")` / `has("branch")`: both false. jq's `.pr` on a
missing key is `null`. The command looked like it answered "not set."

`IssueJSON` (`internal/output/output.go`) has no `Branch` or `PR`.
`MarshalIssue` does not copy `issue.Branch` / `issue.PR`.
`renderIssueHuman` never prints them. The materializer *does* store them
(`internal/materialize/state.go` `json:"branch,omitempty"` / `json:"pr,omitempty"`).

Same class as `2026-08-17T0130Z-5207ee28-commands-show-json-omits-provenance.md`
(confidence / source_links / context_files). Distinct fields, same hole:
write succeeds, the agent-facing view cannot see it.

## Impact

Spent a round deciding whether `arm merged --pr` was a no-op. It was not.
That is the same "the command looks like it answered" tax as `--field dod`
returning empty. Combined with claim never persisting `Branch`, the only
way to verify I6 metadata is `arm log --json` filtered by `to==merged`.

Null vs omitted vs empty is indistinguishable from "the write never
landed." An agent will treat `pr: null` as evidence the flag was ignored
and may re-run `arm merged` or escalate.

## Evidence

- CLI: `arm merged --issue LNGHZN-S10-T4 --pr 109` printed
  `Marked LNGHZN-S10-T4 as merged (PR #109)`.
- `arm show LNGHZN-S10-T4 --format json` keys: id, title, type, status,
  parent, priority, definition_of_done, scope, outcome, claimed_by,
  blocked_by, blocks, acceptance, notes, assessment_attestations.
  No `pr`, no `branch`.
- `jq '{pr, has_pr:(has("pr"))}'` → `pr: null`, `has_pr: false`.
- `internal/output/output.go` `IssueJSON` / `MarshalIssue` / `renderIssueHuman`.
- Sibling: `docs/dogfood/findings/raw/2026-08-17T0130Z-5207ee28-commands-show-json-omits-provenance.md`.
- Related I6 metadata: `2026-08-17T0233Z-5207ee28-coordination-i6-promotion-agent-owned-metadata.md`.

## Suggested Follow-Up

Add `branch` and `pr` to `IssueJSON` and the human show template, copied
from the materialized issue. Prefer omitted-when-empty over `null` so jq
`has("pr")` matches "was recorded." Do not treat a skill note as the fix.
