# Theme: The CLI Records State It Provides No Verb to Correct

## Summary

Armature is append-only by construction (I2), which makes *correcting* recorded state a first-class product requirement rather than an afterthought — you cannot edit a row back into shape. Repeatedly, an agent reaches a state the CLI can create but has no command to fix, and the only remaining paths are hand-editing materialized JSON, re-appending ops by hand, or documenting an exception and moving on.

The pattern is not "a command is missing" in the abstract. In each case below the *adjacent* commands exist and nearly fit — `arm sources` has `add`/`sync`/`verify` but no re-point; `arm reopen` takes `--issue` but no rationale; `arm show` accepts `--field` but silently ignores the one field the agent needed. The gap is narrow and load-bearing, which is what makes it cost a full diagnostic round each time.

Two consequences worth separating:

- **Blast-radius leakage.** A gap created by one story becomes every later story's problem. A doc archived under TOPTIER-S10-T1 orphaned a registered source, and because `arm sources verify` is a global freshness gate, *every subsequent story's auditor run* fails on debt no single story can remediate.
- **Provenance loss in a provenance system.** `arm reopen` writes a status change with no reason attached, in a system whose entire premise is that history explains itself. The append-only log records that the state changed and not why.

## Evidence

- [`arm sources` has no way to re-point a source after its file is moved/archived](../../raw/2026-08-08-tooling-arm-sources-no-reregister-on-rename.md) — `add` would duplicate, `sync` fetches the now-wrong path, `stale-review` targets changed content rather than moved files. Resolved only by documenting an explicit exception at story sign-off.
- [`arm reopen` records no rationale, in a system whose whole premise is provenance](../../raw/2026-08-09T1500Z-claude-workflow-reopen-has-no-rationale.md) — No `--reason`/`--rationale`/`--outcome`. The reopen of LNGHZN-S5 (PR still open, three new unmerged children, so `done` was no longer true) is recorded with none of that context.
- [`arm show --field dod` silently returns empty instead of erroring or printing the DoD](../../raw/2026-08-02T1500Z-claude-tooling-arm-show-field-dod-unsupported.md) — Needed to trim seven over-length DoDs flagged by `arm validate --ci`; the field selector returned nothing, with no error, for every one.
- [`arm dag apply` stores scope as one joined string, so every task it creates fails validation immediately](../../raw/2026-08-09T1430Z-claude-commands-dag-apply-scope-not-split.md) — The plan schema requires scope as a comma-separated string and rejects an array outright; the resulting issues then fail validation on the scope they were just created with.
- [`arm merged` gates on the stale materialized snapshot that `arm transition` deliberately refuses to trust](../../raw/2026-08-12T0204Z-claude-tooling-arm-merged-reads-stale-snapshot-after-transition.md) — The two halves of the documented closeout pair read different sources of truth; the error prescribes the action the agent just performed and never mentions `arm materialize`.
- [Collapse migration leaves unexplained `.arm.collapsed-<timestamp>` backup dir](../../raw/2026-07-16T1642Z-claude-workflow-collapse-backup-dir-unexplained.md) — Adjacent shape: state the tool *creates* and then provides no verb, and no output, to reason about. Establishing that a 75MB directory was safe to delete took a three-way diff and a support round-trip.
- [Scope normalizer still requires comma-space, so a comma-joined create payload stays one phantom path](../../raw/2026-08-15T2049Z-5207ee28-commands-scope-normalize-requires-comma-space.md) — `arm create --scope "a.go,b.go"` stores one path. `amend` does not split it. Same leftover as `dag apply` joining scope.
- [Handoff and plan JSON reused T6; the live DAG already had a merged T6](../../raw/2026-08-17T0129Z-5207ee28-workflow-plan-id-already-taken.md) — No "next free child ID" verb. Planner had to `arm show` the collision and invent T12 by hand.

## Candidate Follow-Ups

- Each of these is small individually; the value is in treating them as one class during the [agent-grade error contract](../../../design/long-horizon-proposals.md) work (LH C3 / `LNGHZN-S6`). Three of the six fail *silently or misleadingly* rather than loudly — `--field dod`, `arm merged`, and the migration backup — and a `next_actions` field would convert them from dead ends into self-clearing errors even before the underlying verbs exist.
- Add the narrow missing verbs: `arm sources repoint` (or accept a new URL on an existing source ID), `arm reopen --reason`, a `--field` that errors on unknown names rather than printing nothing, a scope splitter that does not require comma-space, and a "next free child ID" helper so plan JSON cannot collide with a merged sibling.
- Settle the read-source question once: which commands may gate on the materialized snapshot and which must replay ops. Today it is decided per command, and the only written record of the hazard is a comment in `cmd/armature/transition.go`.
- Worth checking against the surface census (`NXTTN-S2`): these are gaps where the census records a command family as present while the family is incomplete for its own workflow.
