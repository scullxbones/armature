---
date: 2026-08-23
agent: codex
area: validation
task: Stable whole-DAG merged-status audit after ARCHIMP-S15 correction
tags: [i6, merged, integrity, audit, historical-delivery]
---

# Stable merged-status audit finds no remaining provably false record

## User Goal

Re-run the provisional whole-DAG audit against immutable Git inputs, distinguish work that
was never delivered from work that landed and was later removed or renamed, and preserve
`NOT_DETERMINABLE` rather than treating missing evidence as success or failure.

## Snapshot

- Code input: `origin/main` at `80dee97ccdf8cc12dc543882da8360aeab47d2f2`.
- Ops input: `origin/_armature` / local `_armature` at
  `07df325d794ae8f1569827cd4e06718005c2ea1c`.
- Population: 569 Issues materialized as `merged` after the append-only correction of
  `ARCHIMP-S15`, `ARCHIMP-S15-T1`, and `ARCHIMP-S15-T2` to `cancelled` and the resulting
  demotion of `ARCHIMP` to `open`.
- Stability check: both hashes were identical before and after classification.

The category name is `CONFIRMED_DELIVERY`, not literally `CONFIRMED_ON_MAIN`, because seven
legitimate non-code records are proved by the authoritative Git Ops branch or by a committed
no-op/review artifact. Code and documentation deliveries still require reachable
`origin/main` evidence.

| Category | Count |
|---|---:|
| `CONFIRMED_DELIVERY` | 562 |
| `FALSE_MERGED` | 0 |
| `NOT_DETERMINABLE` | 7 |

## Method

1. Refresh `origin`, pin the two hashes above, materialize, and enumerate
   `arm list --status merged --format json`.
2. For every Issue ID, search the subject and body of commits reachable from the pinned
   `origin/main`. A commit-message match counts only when that commit also contains a
   non-state patch. This confirmed 466 records; `.armature/`, `.issues/`, `.trellis/`, and
   `.arm/.armature/`-only commits were excluded from this bucket.
3. Review the remaining 103 records individually:
   - 45 Stories/Epics were confirmed from reachable child or story-level delivery patches;
   - 44 Tasks were confirmed through scoped path history, symbols, tests, renames, deletions,
     or squash-merge bodies;
   - 7 deliberate no-op, review, or Ops-only Tasks were confirmed from committed census,
     decision, follow-up, or authoritative Ops-branch evidence;
   - 7 transient verification claims lacked a durable attestation and remain indeterminate.
4. Treat later deletion or renaming as historical evidence, not as proof that the earlier
   merge was false. File existence alone was never used as sufficient evidence.

## Provisional mismatch queue disposition

Every non-S15 item provisionally called `FALSE_MERGED` was delivered historically:

- `ARCHIMP-S4-T1`: reachable implementation commits `b717ccc6` and `716accd2`; runner later
  removed deliberately by `9865c922`.
- `E7-S1` and its queued Tasks: individual reachable `E7-S1-T*` commits; managed execution
  later removed by `70c0d3b5` and `45d05dfe`.
- The queued `ORCH-RUNTIME-V1-*`, anonymous runtime Tasks, and parent Story: squash commit
  `357f4ed1` names the records in its body and contains their runtime implementation; the
  surfaces were later removed by the ORCRMV commits.
- `ORCRUN-S2-T6`: `6c215b90` places the dry-run return before auth resolution and adds
  `TestRepoRunner_DryRunSkipsAuthAndHarnessResolution`; the package was later removed.
- `SKOPT-1B` and `SKOPT-3`: reachable delivery commits `32137b1e` and `a57acd0d`.
- `LGSLT-2`: `1c939d25` introduced `TRLS_LOG_SLOT` handling and tests; the name later became
  `ARM_LOG_SLOT` during runtime hardening.

These records need no correction.

The provisional indeterminate queue also yielded durable evidence for 17 records. Examples:

- `E2-001-T1` is confirmed by the mode-aware `ResolveContext` implementation beginning at
  `95a7d7f5`, despite its own ID appearing only on the preceding design and plan commits.
- `NXTTN-S2-T3` and `NXTTN-S2-T4` are legitimate no-ops: the reachable T1 census at
  `2e0d675b` records zero parked surfaces at that point.
- `task-1775311522` has a committed decision on `_armature` and its three named UX gaps have
  reachable delivery commits `41eeee7f`, `fd4995aa`, and `8be077ce`.
- `ORCRMV-T01`, `ORCRMV-T02`, and `ORCRMV-T08` have reachable authoritative Ops commits that
  contain the cancellations/amendments, managed-execution record removal, and source URL
  changes they claim.
- `story-1773804400` is confirmed by reachable child commits `f4276ccb` and `069c05e6` plus
  the later file-renaming history.

## Not determinable

These seven records claim that transient commands or deployment checks passed. Their
underlying code or documentation is present historically, but that does not prove the
specific gate run. No retained gate attestation, check log, or equivalent generated evidence
was found.

| Issue | Why it remains indeterminate |
|---|---|
| `CRDWVVRFY-T4` | Source skill patches are reachable, but `make skill`, deployed-copy greps, and the reported `make check` run were not durably attested. |
| `E5-S5-T12` | `32e8e2b1` contains only Ops/materialized-state self-reporting for the final `make check` and validation run. |
| `MIGH-T7` | T1-T6 delivery commits are reachable; no retained evidence proves the separate cache-safe build/test/lint/validate/Doctor gate. |
| `ORCRMV-T09` | Deletions and static absence are provable, but the combined `make check`, materialize, sources, validate, and Doctor run is not. |
| `S7-T2` | The three seams are visible in history; the reported five-command integration run has no retained attestation. |
| `SFT-S1-T11` | No non-state delivery or gate artifact proves the final `make check` and validation run. |
| `TC-010` | No retained coverage or mutation report proves the historical percentages; rerunning today's different tree would not prove the old claim. |

No mutation is justified for these records. Indeterminate is evidence about the audit limit,
not evidence that the work failed.

## Impact

The S15 failure was real and has been corrected, but current-file mismatch was a poor proxy
for false merge across the rest of the DAG. Twenty-six alarming mismatches were historical
deliveries later removed, superseded, or renamed. Conversely, the stable pass found four
additional transient-verification records that the provisional pass had treated as confirmed.
The practical rule is: audit reachability and patches first, preserve no-op/Ops semantics, and
require durable gate attestations before calling an old verification claim confirmed.

## Evidence

- `git rev-parse origin/main`
- `git -C .armature rev-parse HEAD`
- `arm list --status merged --format json`
- exact-ID `git log` searches plus `git diff-tree --name-only -r <commit>`
- scoped `git log --follow`, `git log -S<symbol>`, commit-body, rename, and deletion history
- `git -C .armature log origin/_armature` and exact Ops commit diffs for non-code records
- `arm show --format json <issue...>` for DoD, scope, outcome, hierarchy, decisions, and notes

## Suggested Follow-Up

Persist deterministic gate attestations keyed by invoking checkout, commit, command, and
result. Until then, a historical verification-only `merged` record can honestly be no better
than `NOT_DETERMINABLE` during a later audit.
