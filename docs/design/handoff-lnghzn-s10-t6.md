# Handoff — LNGHZN-S10-T6 (grill this)

**Use:** `/grilling` in a new session. This is the proposal to stress-test,
not a ratified spec. D7 in `docs/design/gate-efficiency.md` is already
amended on the T4 branch; T6 is the write-time follow-up that amendment
points at.

**Do not implement from this file.** Grill first. If the grill holds, add
T6 to `docs/design/plan-gate-efficiency.json` and implement on a new task
branch.

## How we got here

PR #109 (`LNGHZN-S10-T4`) implemented D7 as written: `arm validate` strict
by default, no `--scope`/`--parent`, plan-release gate on `dag transition`,
and `make check` ran `arm validate --ci`.

Review comment 3793083989: that last wire couples every worker's publish
gate to the **union** of every other worker's in-flight graph. A Go-only
worker goes red because someone else created a draft with a broad scope.
Ops writes stay I3-isolated; the gate that *reads* them is not. Agents
then route around `make check` onto `check-fast`. Fail-closed on paper,
fail-open in practice.

That comment was classified `NEEDS_HUMAN` because D7 and I3 were in
conflict. The human decision:

> Whole-graph validate is a property of **the repo at integration**, not
> of this worker's delivery.

But the original intent of T4 still stands: we are perpetually cleaning
the DAG long after the fact, when the *next* agent hits a failing
`validate`. Findings were supposed to die at introduction. They died at
the next reader instead.

## The spec miss

S10's story DoD was closer to right than T4's:

> validate is strict by default and **enforced at plan release**

T4 (and D7's last bullet) then added "inside the `full` gate." That
sentence put an integration question on every worker's `make check`.
That is the miss. The rest of T4 — audit defaults, burn-down, no scoping
flags, plan-release gate — is still the right audit command.

**Not a reboot.** Trim T4, amend D7, add T6 on this tree.

## What T4 is landing (do not undo)

On `task/LNGHZN-S10-T4` / PR #109, after the review pass:

| Door | Job | Mechanism |
|---|---|---|
| **Audit** | Is the graph green? | `arm validate` strict, whole-graph, no `--scope`. JSON keeps native `errors`/`warnings`/`infos`; `Strict` drives only `OK`/exit. Silent green is strict-only. `--ci --strict=false` is rejected. |
| **Plan release** | Can this planner add work to the union? | `dag transition --to verified` runs whole-graph strict validate. Recorded `--skip-validate-gate` for humans (I7). Happy-path errors name the finding, not the escape. |
| **Integration** | Is the **repo** green? | Story close + CI via `make validate-graph` / `arm validate --ci`. **Not** per-task `make check`. |
| **Write** | Can this mutation land? | **Missing. That is T6.** |

`make check` is a function of the tree the worker can change. `arm validate`
stays unscoped (D7 still rejects partial audit — that is how dirt hides).

T5 stays blocked on T4. It needs the plan-release enforcement point, not
`validate-graph` in the Makefile. T5 and T6 can proceed in parallel after
T4 merges.

## Why T6 belongs on S10, not a new story

- D7 lives in `docs/design/gate-efficiency.md`.
- The story DoD already promised "enforced at plan release."
- T5 already hangs off T4's release gate.
- T6 is "enforced at the write, so release is not the first time you see
  the mess."

## The problem T6 must close

Today the writes that *create* findings are fail-open:

- `dag apply --strict` defaults **false**, and `ValidatePlan` only warns
  about a missing DoD. It never runs W1–W11 / E-codes against the graph
  that will exist after the apply.
- `create` / `amend` / `link` append with no graph check.
- Nodes land as `draft`. The planner can walk away.
- The next session that runs `arm validate` inherits the dirt — including
  dirt it did not create.

`arm validate` is a reader. `make check` is a later reader's publish gate.
Neither is introduction.

**Rule that matches the intent:** a write may not land if it introduces a
finding on a node it touched.

That is not scoped `arm validate`. Scoped audit is how a dirty region
stays dirty forever (nobody's subtree owns it). Write-time attribution
is the opposite filter:

- Finding cites an ID this command created or amended → refuse. The
  creator is still here.
- Finding cites only pre-existing IDs → not this writer. Integration
  owns the backlog.

Pairwise findings (scope overlap) fall out correctly: if I create a task
that overlaps yours, the warning names my new ID. I `link` or I narrow
scope *now*.

Rule tightenings later are a different class. Old nodes failing a new
W-code are not "someone left a mess." They are integration burn-down.

## You cannot make an agent virtuous

There is no control that forces an agent not to pass `--strict=false`
except a harness-local hook or permission deny on that exact string.
Hooks are Claude-shaped. Codex, Gemini, a raw CLI, and a human keyboard
will not have them.

I5: the gate decides, not the prompt and not the hook.

The stack that actually works (same as `--skip-delivery-gate`):

1. **Default path is fail-closed.** `arm dag apply --plan p.json` runs
   the introduction check. No extra flag. Most agents do what the
   example shows.
2. **The error must not name the escape.** Delivery-gate dogfood:
   agents learned to bypass because the failure text offered
   `--skip-delivery-gate` as the remedy. The failure names the finding
   and the fix (`narrow --scope`, `add context_files`, `arm link`).
3. **Override is a recorded human act** (I7). Skills say do not use it.
   Hooks may deny it where the harness allows. Success-with-override is
   never "green."
4. **Integration is the backstop** for whatever still lands.

You will never get to "an agent cannot." You can get to "an agent
following the default path cannot, and a bypass is visible."

## Janitor workflow (not a cron)

A periodic sweep inside Armature is T1 (long-running process). Dies on
contact.

Use the moments that already exist:

| When | Who | What |
|---|---|---|
| Write (T6) | The creator, still in session | Mutation refused if it introduces findings on touched IDs |
| Plan release (T4) | The planner | `dag transition` refused while the union is dirty (they can still stop) |
| Story integration | Coordinator | S10 DoD already: `arm validate` strict-green at story close |
| PR / CI | CI | `make validate-graph` / `arm validate --ci`. N1: Armature is not a CI system; it can still be *run* by one. |
| Red at integration | Coordinator, new issue | A claimed cleanup task whose scope *is* the dirty nodes. The next `arm ready` feature task is the wrong owner. |

Historical dirt from rule changes uses the same queue.

## Proposed T6 scope

- Introduction check on `dag apply`, `create`, `amend`, `link`.
- Materialize current state, apply the proposed ops in memory, run
  `validate.Validate`, keep only findings whose cited IDs intersect the
  proposed targets. Non-empty → do not append.
- Default fail-closed. Recorded override. Happy-path errors do not
  advertise it.
- Optional hook/permission example that denies the override (defense in
  depth, not the primary control).
- Tests: a dirty plan cannot land; pre-existing foreign findings do not
  block an unrelated `create`.
- Flip or replace `dag apply --strict` (today opt-in, DoD-only).
  `--dry-run` is how a planner iterates.

## Open questions for the grill

1. Is "cited IDs intersect proposed targets" a precise enough
   attribution predicate? Coverage / uncited-node is repo-wide. Does a
   new uncited node fail apply (yes — it cites the new ID) while an old
   uncited node leave an unrelated create alone (yes — finding does not
   cite the new ID)?
2. Does `dag apply` refuse the whole plan if one issue is dirty, or
   apply the clean prefix? (Fail the whole plan. Partial apply is a
   different mess.)
3. Should write-time use the same strict suite as `arm validate`, or a
   subset (structural E-codes only, warnings later)? Intent says the
   same suite, or birth defects that are "only warnings" still land.
4. Recorded override shape: reuse `--strict=false` on apply, or a new
   `--skip-validate-gate` consistent with dag transition? Default must
   not require a flag to do the right thing.
5. Do `reparent` / `unlink` / `amend --scope` need the same check? T6
   names create/amend/link/apply. `reparent` can create illegal
   hierarchy (already checked at create). `amend --scope` *is* amend.
6. Keep whole-graph on `dag transition` once writes are fail-closed, or
   narrow it to the subtree? Current T4 decision: keep whole-graph at
   release (planner is integration-adjacent). Grill whether T6 makes
   that redundant.
7. Hook deny list: which harness files, and is that T6 or a note in the
   worker skill?

## What not to reopen

- Partial `arm validate --scope` as an audit escape. D7 rejected it.
- Putting `validate --ci` back on per-task `make check`.
- An Armature cron / daemon janitor (T1).
- Making the hook the primary control.
- Rebooting T4. The audit command and the plan-release gate stay.

## Pointers

- Spec: `docs/design/gate-efficiency.md` D7 (amended on this branch)
- Plan: `docs/design/plan-gate-efficiency.json` (T4 DoD last sentence
  amended; T6 not yet added)
- Current fail-open apply: `internal/decompose/apply.go` `ValidatePlan`
- Plan-release gate: `cmd/armature/dag_transition.go`
- Audit command: `cmd/armature/validate.go`, `internal/validate/validate.go`
- PR: https://github.com/scullxbones/armature/pull/109
