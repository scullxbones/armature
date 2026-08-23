# ADR 0016: Three-Door Validation

Validation has three jobs. Putting any two on the same command either
conscripts the next worker as janitor or hides dirt in a corner nobody owns.

## Status

Accepted

## Principles touched

I3, I4, I5, I7

## Context

T4 put whole-graph `arm validate --ci` on every worker's `make check`. A
Go-only worker went red because someone else's in-flight draft had a broad
scope. Ops writes stayed I3-isolated; the gate that *read* them was not.
Agents routed around `make check` onto `check-fast`. Fail-closed on paper,
fail-open in practice. Review comment 3793083989; human ruling: whole-graph
validate is a property of the repo at integration, not of this worker's
delivery.

The other extreme — scoped `arm validate --scope` — was already rejected
(D7): a dirty region that sits in nobody's subtree stays dirty forever.

`dag apply --strict` was opt-in and only checked missing DoD. Creates,
amends, and links landed fail-open. Findings died at the next reader, not
at introduction.

## Decision

Three doors, three jobs.

1. **Introduction** (write). A command that appends an op `validate` can
   see may not land if it *introduces* a Graph Finding on an issue it
   created or targeted. Pre-existing findings on foreign IDs do not block.
   This is not scoped audit. Default fail-closed. `--dry-run` is how a
   planner iterates. Apply `--strict` is deleted (alpha; command BC is not
   a constraint). Ops-log BC is: `skipped_validate_gate` stays an optional
   payload field; absent means false.
2. **Plan Release** (`dag transition` / `confirm` to verified). Whole-graph
   strict validate. The planner is about to staff the union and can still
   stop. Drafts stay draft, or `dag revert` / cancel withdraws them.
   Birth is always draft; `arm create` does not emit verified.
3. **Integration** (story close and CI via `make validate-graph` /
   `arm validate --ci`). The per-task publish gate does not run it.

Attribution is *introduced* (before/after), not "any finding that cites a
touched ID." Findings carry structured cited IDs so a cycle that happens
to name an old node still counts as introduced.

Every `RegisteredOpTypes()` entry is classified `AffectsValidity` or not.
Writers of `true` types go through one append wrapper. An unclassified new
type fails CI.

Audit (`arm validate`) stays unscoped and strict by default. It is a
reader, not a write door.

## Consequences

- A planner following the default path cannot land a birth defect, and
  cannot release a dirty union. A feature worker's `make check` no longer
  depends on anyone else's in-flight nodes.
- `dag apply` must emit source-link ops in the same batch as creates
  (plan schema grows per-issue `source`). Existing plans without `source`
  fail apply until they gain it.
- Decision and status-transition ops are validity-visible (W8, W11).
- Break-glass is a **Release Override**, not a flag on the agent verb.
  `dag transition` and `confirm` do not accept a skip. The human command
  (`arm dag override-release`) requires a controlling terminal (`/dev/tty`),
  an interactive type-the-id, and a recorded reason. The op still sets
  `skipped_validate_gate` plus the reason; success is never "green." Skills
  and happy-path errors never name this command (they name revert/cancel).
  Residual: an agent with a PTY that reads `commands.md` can still invoke
  it. That is accepted; a proof of humanity is not in scope. Analytics on
  how often Introduction, Plan Release, and Release Override fire is a
  later measurement pass, not a fourth door.
- Story spec remains D7 in `docs/design/gate-efficiency.md`. Implementation
  is T12.

## Considered and rejected

- Scoped `arm validate --scope` as an audit or write filter (hides dirt).
- Whole-graph validate on per-task `make check` (I3 in spirit; trains
  agents onto `check-fast`).
- An Armature cron / daemon janitor (T1).
- A harness hook as the primary control (Claude-shaped; I5).
- Cites-touched attribution (an `amend --title` conscripts you as janitor
  for someone else's W4).
- Partial apply of a dirty plan (a new mess).
