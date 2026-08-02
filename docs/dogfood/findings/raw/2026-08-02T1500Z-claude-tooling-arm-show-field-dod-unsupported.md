---
date: 2026-08-02
agent: claude
area: tooling
task: LNGHZN-S9 planning / fixing arm validate DoD-length errors
tags: [arm-show, extract-field, dod]
---

# `arm show --field dod` silently returns empty instead of erroring or printing the DoD

## User Goal

`arm validate --ci` reported `definition_of_done exceeds 500 chars` on 7 pre-existing
issues (LNGHZN-S8, NXTTN-S3-T1, LNGHZN-S7-T3, TOPTIER-S13, NXTTN-S3, LNGHZN-S7-T1,
LNGHZN-S7). To trim each DoD down under the limit, I needed to see the current full
text first.

## Observed

Ran `arm show <ID> --field dod` for each of the 7 issues. Every invocation returned
empty output — no error, no text, just nothing printed.

## Impact

Wasted a round trip discovering the flag doesn't do what its name implies. Reading
`cmd/armature/helpers.go`'s `extractFieldsFromIssue` (read earlier in this session for
unrelated reasons) explains why: the field-name switch only recognizes `id, title,
type, status, parent, outcome, scope, priority, assigned_worker, claimed_by,
blocked_by, blocks` — `dod`/`definition_of_done` isn't one of the cases, so it falls
through to the `default: value = ""` branch and prints an empty string silently rather
than erring on an unknown field name. A worker or planner hitting this without having
just read that source file would have no way to know whether the issue's DoD is
genuinely empty, whether `--field` needs a different name, or whether the flag is
broken — the empty output looks identical in all three cases.

This is exactly the kind of validate-driven remediation loop (`arm validate` reports a
DoD problem → agent needs to read and edit the DoD → `arm amend --dod`) that should be
a two-command round trip; instead it required a source-code read to even discover the
right way to view a DoD (there doesn't appear to be one via `--field`; `arm show <ID>`
without `--field` presumably renders it as part of full output, unconfirmed at time of
writing).

## Evidence

```
$ arm show LNGHZN-S8 --field dod
$ arm show NXTTN-S3-T1 --field dod
(both: no output, exit 0)
```

`cmd/armature/helpers.go`, `extractFieldsFromIssue`, switch statement lines ~301-328:
no `case "dod":` / `case "definition_of_done":` branch exists; falls through to
`default: value = ""`.

## Suggested Follow-Up

Either add `dod`/`definition_of_done` as a recognized `--field` value in
`extractFieldsFromIssue`, or have unknown `--field` names return a usage error instead
of silently printing empty string — the latter would have surfaced this immediately
instead of looking like "this issue has no DoD."
