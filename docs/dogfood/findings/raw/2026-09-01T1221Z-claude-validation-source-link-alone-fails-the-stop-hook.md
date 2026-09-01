---
date: 2026-09-01
agent: claude
area: validation
task: File TOPTIER-B1 and TOPTIER-S18-T4 with citations to newly written findings
tags: [citations, source-link, accept-citation, harness-hook, dag-apply, stop-hook]
---

# A properly source-linked issue still fails the stop hook as "uncited"

## User Goal

Create two DAG issues citing dogfood findings authored in the same session, then
finish the session cleanly.

## Observed

Both issues were created via `arm dag apply` with a `source` field naming a UUID
already registered in the manifest. `arm validate` accepted them:

```
OK: no issues found (COVERAGE: 767/767 cited (695 source-linked, 72 accepted-risk))
```

767 = 765 before + 2 new, and 695 = 693 + 2, so validate counted both as
**source-linked**. The stop hook disagreed:

```
uncited source(s): f177ab50-73e2-456e-b8ae-e01d9799598b
```

Two separate causes, both invisible from the command that reported success.

**1 — `dag apply` writes a partial source-link op.** The op it emitted carries
only `source_id`:

```json
["source-link","TOPTIER-B1",...,{"source_id":"f177ab50-..."}]
```

`arm sources link` for the same pair emits `source_id` *and* `source_url`.
Historical ops from the planner path carry both, e.g. the `TOPTIER-S3` link
records `source_url: docs/design/top-tier-gap-analysis.md`.

**2 — the harness policy requires an acceptance regardless.**
`internal/harnesspolicy/resolver.go:94-108` marks every linked source
`Accepted: globallyAccepted || accepted[link.SourceEntryID]`, in *both* branches —
whether or not the source is known to the manifest. `CheckCitations`
(`verification.go:86-100`) then fails any check that is not `Accepted`. So a real,
registered, synced, correctly-linked source is reported as uncited unless someone
also runs `arm sources accept-citation`.

Only one of the two issues was flagged, because the hook evaluates the currently
*claimed* issue — `TOPTIER-B1`. The identical gap on `TOPTIER-S18-T4` went
unreported and would have surfaced only when someone claimed it.

## Impact

`arm validate` and the stop hook implement two different definitions of "cited".
Validate treats source-linked and accepted-risk as alternatives and reports full
coverage; the hook treats acceptance as mandatory and blocks. An agent that runs
validate to confirm its work is clean gets an unambiguous green, then gets
stopped anyway — with a message naming a UUID and no indication of which issue it
belongs to or what would satisfy it.

The natural repair is also the wrong one. `accept-citation` exists to record
*accepted risk* — an issue proceeding without a proper source. Using it to quiet
the hook on an issue that genuinely has a source inverts its meaning, and those
72 accepted-risk entries stop being a reliable count of undersourced work.

Because the hook only inspects the claimed issue, the gap is discovered one issue
at a time, at whatever future moment someone claims it.

## Evidence

- `arm dag apply --plan …` created `TOPTIER-B1` (source `f177ab50-…`) and
  `TOPTIER-S18-T4` (source `719764dd-…`)
- `arm validate --quiet` → `767/767 cited (695 source-linked, 72 accepted-risk)`
- Stop hook (`arm harness-hook`) → `uncited source(s): f177ab50-…`
- Op written by `dag apply`: `{"source_id":"f177ab50-…"}` — no `source_url`
- Op written by `arm sources link`: `{"issue":"TOPTIER-B1","source_id":"f177ab50-…","source_url":"docs/dogfood/findings/raw/…"}`
- `internal/harnesspolicy/resolver.go:100-108` — `Accepted` requires an acceptance
  in both the known-source and unknown-source branches
- `internal/harnesspolicy/verification.go:86-100` — `CheckCitations` fails on any
  `!check.Accepted`
- Manifest `issues` array stayed `null` for both entries after `arm sources link`,
  suggesting that denormalized field is unmaintained
- Resolved by re-linking through `arm sources link` and recording an acceptance
  with a rationale explaining the source is real — a workaround, not a fix

## Suggested Follow-Up

Decide which definition of "cited" is authoritative and make both paths use it. If
a verified manifest source satisfies citation, `resolveCitationChecks` should treat
a known, synced source as accepted without a separate acceptance op; if acceptance
is genuinely required for all sources, `arm validate` should stop reporting
source-linked issues as covered.

Independently, `arm dag apply` should write the same source-link payload as
`arm sources link`, including `source_url`. Two commands creating the same
relationship should not produce different ops.

The hook message would also be far more actionable if it named the issue and the
remedy, not just a bare UUID.
