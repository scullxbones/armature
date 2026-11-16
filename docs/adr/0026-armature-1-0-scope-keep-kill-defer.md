# ADR 0026: Armature 1.0 scope — keep, kill, and defer

The workstreams below are the 1.0 line. Earlier promotion fixes assumed a
local agent. The cloud-agent plus API-lander workflow has no such agent.
This ADR records the scope decision and the N1 design. It does not amend
ADR 0022.

## Status

Accepted

## Principles touched

N1, I1, I4, I5, I6, I7, N2, N3, N4, T1, T3 (unchanged)

## Context

On 15 November 2026 Brian re-groomed the backlog: a value-ranked roadmap,
178 dogfood findings reviewed, measured materialization times, and a skills
audit.

The N1 root cause is the same in every earlier promotion fix. Each fix
assumed a local agent that has the worker skill, a review attestation, and
`_armature` push rights, and that this agent runs `arm` after merge. In the
cloud-agent plus API-lander workflow, nobody does. `done` and `merged` stay
distinct (I6), and the host never records the delivery.

N1 still holds: armature is not a CI system. The design below uses the
host's CI as the place that runs `arm`. It does not turn `arm` into CI.

Measured materialization, from that re-groom: warm about 956 ms, cold about
5.2 s, `render-context` about 5.8 s. The cost is git subprocesses and blob
re-reads. Op count is not the cost. The target for workstream 4 is 10–100 ms.

## Decision

### Scope choices

1. The workstreams in the keep table define 1.0.
2. `arm mcp` is required in the Cursor setup, and it is the dogfood-finding
   collector.
3. Claude, Codex, and Devin stay in scope beside new Cursor and Grok Build
   adapters. Item 28 expands from the earlier harness set to these five.
4. Keep Living Diagrams (item 41).
5. External adoption is the end goal.
6. Code is the source of truth. Specs and issues are archaeology.
   HOSTREF-S2 is GitHub issues used as context and citation.
7. Item 32 becomes the landing page, built with armature (the second
   substrate).
8. Keep item 25 transition hooks, and build them with a rollout dogfood.

A GitHub Action may push `_armature` by running `arm`. ADR 0022 needs no
amendment. Its writers stay `arm sync` and `arm merged`; the Action is a
host that runs `arm`, not a second writer. The reviewer path attests
without a terminal: `arm review prepare`, `arm review validate`, and
`arm review record`. A 19-leaf backfill, and PR #464 reaching Green with
real gate evidence, are the proof. The rating stays unread, as ADR 0022
already requires. Tripwire T3 is unchanged: HOSTREF-S2 cites issues and
does not sync bidirectionally with them. Tripwire T1 is unchanged:
required `arm mcp` is the stdio session ADR 0024 already exempts.

Value rank and build order differ. Skills diet is rank 5 and Phase 0,
because it depends on N1. Item 45 is rank 11 and may land beside any phase.

### Keep (build for 1.0)

| Rank | Workstream | What 1.0 builds |
|---|---|---|
| 1 | N1 promotion from the host | The design in the N1 section below. Decided here. Not yet built. |
| 2 | Item 28 harness contract | One contract, five harnesses: Claude, Codex, Devin, Cursor, Grok Build. |
| 3 | Cursor setup | Required `arm mcp`, plus dogfood collection. Absorbs item 37. |
| 4 | Materialization speed | Replaces item 44 ops compaction. Target 10–100 ms. Today: warm ~956 ms, cold ~5.2 s, `render-context` ~5.8 s. |
| 5 | Skills diet | Remove coordinator, activity-indexer, and auditor from default bootstrap into an opt-in pack. Keep reviewer (N1's attestation engine). Rewrite worker for cloud agents. |
| 6 | HOSTREF-S2 | GitHub issues as context and citation. |
| 7 | Item 25 transition hooks | Build with a rollout dogfood. |
| 8 | N3 scope at claim | Scope at claim is the contract. |
| 9 | Item 41 Living Diagrams | Keep. |
| 10 | Item 32 landing page | The landing page, built with armature (second substrate). |
| 11 | Item 45 | A CI secret scan of `_armature`. |

Done, and not a 1.0 workstream: item 24 T4 (PR #444).

### Kill

| Item | Why it is out |
|---|---|
| 26 event stream | Item 25's `on:` filter gives hooks the event vocabulary. A stream is not required. |
| 29 model-tier dispatch policy | All five harnesses pick models themselves. There is nothing for `arm` to enforce. |
| 33 session handoff bundle | For a cloud agent the PR is the handoff. The MCP session log keeps context. |
| 42 time-travel state | Archaeology is about intent, which HOSTREF-S2 serves. State history is `arm log --since --issue` and `arm context-history`. |

### Defer

Each row stays out until its revive trigger fires. Item 46 is a hold, not a cancellation.

| Item | Revive trigger |
|---|---|
| 46 extensibility seam / `TOPTIER-S14` | First partner request. |
| 38 co-change scope suggestion | Scope gaps recur after N3. |
| Item 28 canary CI | A harness release breaks a fixture. |
| 43 flow analytics / latency | A month of N1's real done/merged times exists. |
| 44 ops compaction | Workstream 4 misses the 10–100 ms target. |
| Task-scope idea 3, post-Bash diff | A harness offers a post-shell hook. It then becomes a detect column. |
| Task-scope idea 4, protect hook config | A partner runs untrusted agents. Until then, document per-harness managed-config support. Do not build the hook. |

### N1 design (decided, not yet built)

Requirements:

| ID | Requirement |
|---|---|
| R1 | Fetch `refs/pull/N/head`. Fall back after a squash. |
| R2 | The PR's full `make check` result is gate evidence, so an honest review can be Green. |
| R3 | Assessments are keyed by delivery `(base, tip)`, so a shared diff is reviewed once. |
| R4 | Refuse an empty contract. |

Poka-yoke, before merge. A required pre-merge `promotion-ready` check on
each ID-scoped PR runs prepare, validate, and assessment while the head is
local. The check is bound to the current head SHA. Every review-fix push
reruns it. An older head's assessment never satisfies it.

After merge, the Action writes only the stored result plus the squash SHA.
A scheduled sweep is idempotent. One writer, via an Actions concurrency
group. Pushes are fast-forward only.

`make check` stays a side-effect-free local and CI check. Only CI records
gate evidence: `arm gate run full` at the pushed head, on a clean checkout.
A local run never counts. Optional `arm land --dry-run` warns early.

Jidoka, after merge. A post-merge failure or sweep drift fails
`promotion-ready` for the affected issue tree only, and opens one GitHub
issue as the andon signal. Clearing that failure requires a root-cause
note. A repeated failure kind is filed as a dogfood finding.

## Consequences

Build order is the phase list. Value rank stays the keep table. The living
sequence is `docs/design/next-work-sequencing.md`.

- **Phase 0, truth.** N1, then the skills diet. The diet depends on N1:
  reviewer is the attestation engine, and the worker rewrite targets the
  cloud-agent workflow this design assumes.
- **Phase 1, speed.** Workstream 4.
- **Phase 2, reach.** Item 28, the Cursor setup with `arm mcp`, and N3
  scope at claim.
- **Phase 3, leverage.** Item 25 with its rollout dogfood, then HOSTREF-S2.
- **Phase 4, trust.** Living Diagrams, then the landing page.
- **1.0 cut** after Phase 4. External adoption is the goal past the cut.
  This ADR does not name the release version.
- **Item 45** can land beside any phase.

Follow-ups:

- Implement the N1 design above. Do not reopen ADR 0022 to do it.
- Do not file stories for killed rows.
- Do not build a deferred row until its revive trigger fires. `TOPTIER-S14`
  may still appear in `arm ready`. Ready is not this line.
- Item 28's canary CI stays deferred with its own trigger. The five-harness
  contract does not include it.
- Document per-harness managed-config support with the harness contract.
  Build task-scope idea 4 only when a partner runs untrusted agents.
