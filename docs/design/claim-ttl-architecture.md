# Claim and TTL architecture

**Status:** Design only. Human checkpoint. No production code in this package.  
**Audience:** Brian.  
**Date:** 2026-09-27  
**Method:** Traced from the current tree, not from memory. pstack `how` / `why` / `architect` skills are **not** in this checkout or plugin cache; screening uses the red-flag list named in the request (shallow modules, information leakage, temporal decomposition, pass-through methods) plus Brian's three principles as stated in the goal.

Hard constraints (not proposed to change): append-only `_armature` ops log; materialize is a pure fold of ops; Arena B (`Issue.Asserted` / `Derived`, `CurrentStateVersion = 2`) stays parked; doctor D11 / `TOPTIER-S12` stays reserved; low-stakes publish stays best-effort; do not roll back a local ops commit on publish failure (I2).

Related work that deliberately left this surface alone: no-comments waves; PR #244 fail-loud high-stakes publish; PR #272 publish failure classes. `docs/design/mk-reshape-a-primary.md` non-goals: unifying `ResolveClaim` with lose-race; fail-loud publish; changing TTL / race winner; Asserted/Derived.

---

## 1. Traced current-state model

There is not one claim law. There are **four** independently coded answers to "who holds this issue, and is the lease still live?" plus a clone-local flock that does not participate in that answer.

### 1.1 What a claim is

A claim is an append-only `claim` op on the claiming worker's JSONL log (`ops.OpClaim`). Payload fields used for lease identity: `ttl`, optional `worktree_path`, optional `claim_token` (`internal/ops/schema.go`).

Materialized fields on `materialize.Issue`: `Status`, `ClaimedBy`, `ClaimedAt`, `ClaimToken`, `ClaimTTL`, `LastHeartbeat`, `LastClaimingWorkerActivity`, `WorktreePath` (`internal/materialize/state.go:67-86`).

Status after a successful apply of a claim op is always `claimed` (`engine.go:159`). A heartbeat does **not** move status to `in-progress` (`engine.go:172-179`). Architecture's table that maps `in-progress` to "heartbeat op (implicit)" (`docs/design/architecture.md:204`) is not the implementation. `in-progress` is an asserted `transition` (or a derived parent promotion from a child's claim: `engine.go:524-534`).

### 1.2 Replay order (the fold)

All ops are sorted then applied:

```
timestamp ascending, then type key: create=0, other=1, note-delete=2
```

(`internal/materialize/pipeline.go:246-273`). Same-timestamp claims from two workers are **not** ordered by worker ID. `SortStableFunc` keeps original relative order among equal timestamps and equal type keys. That is a different tie-break than `ResolveClaim` (lexicographic `WorkerID`).

### 1.3 Who holds a claim — Law M (materialize)

**Owner of this law:** `State.applyClaim` + `ForeignLiveLeaseBlocksChallenger`.

On each `claim` op (`engine.go:147-169`):

1. If `ForeignLiveLeaseBlocksChallenger(held, challenger, op.Timestamp)` is true, the op is a **no-op** (return nil). The held lease is unchanged.
2. Otherwise the op **takes** the lease: status `claimed`, `ClaimedBy` = op worker, clocks set to op timestamp, TTL and token copied from payload, parent open → in-progress.

`ForeignLiveLeaseBlocksChallenger` (`internal/claim/race.go:17-32`):

- Blocks only if held status is `claimed` or `in-progress`.
- Does not block if `ClaimedBy` is empty or equals the challenger (**same worker always overwrites**, even while live).
- Staleness: `FoldLastActivity(claimedAt, lastHeartbeat, lastClaimingWorkerActivity)` then `IsClaimStale`.
- **Special case:** if held `TTLMinutes <= 0`, substitute `60` (`zeroTTLHeldLeaseReplayFallbackMinutes`) **before** `IsClaimStale`. So a recorded `--ttl 0` lease **does expire at 60 minutes for steal purposes**, even though `IsClaimStale(..., 0, now)` itself never expires.

This is **steal-on-stale, last successful apply wins**, not "first claim in the log wins." A later foreign claim wins only when the held lease is already stale **at the later op's timestamp**. Two live foreign claims: the earlier (in sorted apply order) keeps the lease; the later is a no-op.

The mk-reshape name `claimLostRace` does not exist in the tree. The function is `ForeignLiveLeaseBlocksChallenger`. Tests explicitly refuse unification with `ResolveClaim` (`internal/claim/race_test.go:91-100`).

### 1.4 Who holds a claim — Law R (ResolveClaim, unused by materialize)

`ResolveClaim` (`internal/claim/claim.go:17-31`): among a slice of claim ops, **earliest timestamp wins**; equal timestamps → lexicographically smaller `WorkerID`.

Callers:

| Site | What it does with the winner |
| --- | --- |
| `internal/audit/audit.go:90-114` | Marks every non-winner worker's claim as `[lost race]` in `arm log` |
| `cmd/armature/workers.go:305-306` | **Fallback** after a third, sequential simulation fails to name an `activeWorker` |

`workers.go` `claimWinnersByIssue` (`234-308`) is a **third** law:

- Replay per issue, sorted by timestamp, then worker ID, then type (not `opSortKey`).
- On `claim`, steal if `staleAt(activeWorker, op.Timestamp)`.
- Zero TTL here becomes **hardcoded 60** (`workers.go:264-267`), not config `default_ttl`, and not "never expire."
- Heartbeats/transitions only update clocks for that worker's struct.
- Terminal transition clears `activeWorker`.
- If nobody is active at the end, **fall back to `ResolveClaim` over every claim ever**, including historical claims on a finished issue.

`cmd/armature/hook.go:141-191` `hookFindActiveClaimID` is a **fourth** fold: only the **local worker log**, last claim per issue, `ttl <= 0` replaced by **config `default_ttl` (or 60)**, first non-stale non-transitioned issue in map iteration order.

### 1.5 CLI "did I win?" after write — Law C

`arm claim` (`cmd/armature/claim.go`) does **not** call `ResolveClaim`. After `appendHighStakesOp` of the claim, it reloads the snapshot and asks `HeldByExactWorkerAndClaimToken` (`claim.go:1081-1110`, `state.go:20-28`).

That predicate is true only when **all** of:

- issue is non-nil
- `claimToken != ""`
- `Status == claimed` (not `in-progress`, not `done`)
- `ClaimedBy == workerID`
- `ClaimToken == token`

So:

- A claim without a token (ready TUI, old logs) can never "win" this check.
- After any transition off `claimed`, the CLI treats the worker as not holding the claim, even if `ClaimedBy` and token still match (`claim_test.go:901-904` encodes this).

If the reload says lost: print `lost_claim_race` (or same-worker superseded), clean git excludes, **do not compensate**, return nil (success exit, claimed=false). The losing claim **op remains in the log** (I2).

### 1.6 Locking (mutual exclusion)

Two flocks, both under the clone's git common dir (`cmd/armature/claim_lock.go`):

| Lock | Acquire | Scope | Purpose |
| --- | --- | --- | --- |
| `armature-claim-<issue>.lock` | `TryLock` (non-blocking) | **This clone**, one issue | Fail immediately if another `arm claim` for the same issue is running here |
| `armature-git-exclude.lock` | blocking `Lock` | **This clone**, all claims | Serialize `.git/info/exclude` edits |

They do **not** serialize two clones, two machines, or two worktrees that do not share a git dir. Cross-clone races are I3-legal: each worker writes only its own log file; git merge of `_armature` is per-file.

`PlanClaim` overlap (`internal/claim/plan.go`) is **not** a lock. It is a pre-append advisory/block on **scope glob overlap** with other tasks already `claimed`/`in-progress` in the **local** snapshot. Foreign overlap blocks unless `--force`; same-worker overlap is dismissed with a note. `docs/design/architecture.md:505-509` still says overlap never blocks — that text is stale versus `PlanClaim` (`claim-overlap-plan.md` is the citable spec).

`docs/design/claim-overlap-plan.md:120-122` and `345-346` explicitly forbade changing lock ordering and splitting staleness/heartbeat/race-winner out of `internal/claim` **for that story**. That is why this surface is still tangled: later waves were told not to touch it.

### 1.7 TTL semantics — the dual law, clocks, expiry

#### Recorded claim TTL vs config `default_ttl`

| Input | Meaning | Sites |
| --- | --- | --- |
| `arm claim --ttl N` (`N > 0`) | Written on the claim op; drives `Issue.ClaimTTL` | `claim.go:1057-1059` |
| `arm claim` without `--ttl`, config `default_ttl > 0` | Copied into the flag before append | `claim.go:857-859` |
| `arm claim` without `--ttl`, config field omitted / zero at runtime | Flag default **60** (`claim.go:1167`). Test: absent/zero config → 60 (`claim_test.go:2341`) |
| `arm claim --ttl 0` | **Accepted.** Payload TTL 0. `IsClaimStale` never expires (`claim.go:56-60`) | Documented `docs/configuration.md:14,112` |
| Config `"default_ttl": 0` **present** | **D10-invalid** (`internal/config/strict.go:112-115`) | Intentional dual law |
| Ready TUI claim | Hardcoded `TTL: 60`, no token, no worktree, no flock | `ready.go:215-224` |

`IsClaimStale` (`internal/claim/claim.go:56-64`): `ttlMinutes <= 0` → never stale; else stale iff `now > last + ttl*60` (**strict after** the exact boundary is still live).

`FoldLastActivity` (`claim.go:47-54`): `max(claimedAt, lastHeartbeat, lastClaimingWorkerActivity)`.

Who bumps which clock:

| Clock | Bumped by | Guard |
| --- | --- | --- |
| `ClaimedAt` | `applyClaim` when the claim is accepted | n/a |
| `LastHeartbeat` | `applyClaim` (set to claim ts); `applyHeartbeat` | heartbeat only if `ClaimantHeartbeatClocks` (`workerID == claimedBy`) (`race.go:36-38`, `engine.go:175-177`) |
| `LastClaimingWorkerActivity` | `applyClaim`; claimant heartbeat; `applyTransition` if `op.WorkerID == ClaimedBy` | Notes, links, foreign ops bump `Updated` only (`state.go:78-84`) |
| Harness debounce | `HeartbeatDebounceInterval = 5m`, not a function of TTL (`claim.go:12-15`) | `ShouldHeartbeat` |

Expiry **does not write an op**. Status stays `claimed` or `in-progress` with stale clocks. Detection is read-time against `now`.

Surfaces that call `Issue.ClaimStale` → `IsClaimStale` with the **recorded** TTL (so `--ttl 0` never expires here):

- `ready.StaleClaims` — **status `claimed` only**, IDs only (`stale.go:11-28`)
- `ready.ExpiredClaims` — `claimed` **or** `in-progress` (`stale.go:45-77`); comments cite `recovery-state-machine.md`
- `ready.ComputeReady` — extra gate: even `status==open`, skip if `ClaimedBy != ""` and not stale (`compute.go:61-62`)
- `doctor.PlanFixes` — `StaleClaims` → release to `open`; leftover stale `in-progress` → `blocked` (`fix.go:35-48`)
- `worktree.Reconcile` — live vs orphan vs ghost (`reconcile.go:118`)
- `harness_hook.go:88` — uses recorded `issue.ClaimTTL` (never-expire for 0)

Surfaces that **do not** honor never-expire for TTL 0:

- `ForeignLiveLeaseBlocksChallenger` → 60 (`race.go:26-28`)
- `workers.go` winner sim → 60 (`264-267`)
- `workers.go` / `hook.go` activity classification → config default (`workers.go:197-200`, `hook.go:183-186`)

`arm ready` does **not** put expired `claimed` tasks into the ready queue. They stay `claimed` until doctor/transition/steal. They are listed separately (`ExpiredClaims`). Architecture ready-rule 4 ("not claimed, or current claim is expired") (`architecture.md:458`) is therefore **not** what `ComputeReady` does; recovery + `stale.go` comments are the live spec.

Exact boundary: at `now == last + ttlSeconds`, `IsClaimStale` is **false**. First reclaimable instant is `+ 1` second.

### 1.8 Compensation vs expiry vs materialize

If worktree setup fails **after** a won claim, `compensateClaimIfHeldByToken` (`claim.go:471-522`):

1. Reload; if not `HeldByExactWorkerAndClaimToken`, skip rollback (someone else already owns it).
2. `PlanCompensation` (`internal/claim/compensate.go:34-66`): if **prior** lease was live same-worker, restore prior status/clocks/token; else transition to `open`. Always `RestoreClaim=true`, `IfClaimToken` = the **new** token.
3. Append compensating `transition` via `appendHighStakesOp` (high-stakes publish).

Replay of that transition (`engine.go:182-214`, `216-221`, `229-238`):

- If `IfClaimToken` set, apply only when the issue is still held by that worker+token **and status is still `claimed`**. Otherwise no-op.
- Then status assert, worktree restore, then `restoreLeaseIfMarked` overwrites lease fields if `RestoreClaim`.

Expiry does not emit compensation. Doctor `--fix` emits **new** transitions (release/block), also high-stakes.

`PlanCompensation` uses `IsClaimStale` on the **prior** TTL as recorded (never-expire for 0). That can disagree with Law M's 60-minute steal window for the same zero-TTL lease.

### 1.9 Publish to `origin/_armature`

After a successful local append+commit:

| Path | Git | On remaining failure | Local commit |
| --- | --- | --- | --- |
| High-stakes (`appendHighStakesOpIf`, `helpers.go:492-507`) | `Push`; on error `FetchAndRebase` then `Push` (`555-564`) | `localArmatureTipPublishError` → command failure (`CLAIM-1`, …). #244. Classes (auth / non-ff / other) are #272 and stay out of this design's proposed changes. | **Kept** (I2) |
| Low-stakes (notes, heartbeats, decisions) | Same sequence only at `low_stakes_push_threshold` | Swallowed (`pushOpsBranchAlwaysResetTracker`) | Kept |
| Bare `appendOp` | None | n/a | Local until some other publish (parent auto-advance after claim is this path: `claim.go:1156`) |
| `arm push-ops` | Push only, no rebase retry | `PUSH-OPS-1` | n/a |

Reads do **not** fetch (`architecture.md:330-340`). Race resolution on this clone is whatever logs are already in the ops worktree. After a loud publish failure, **this clone has the claim op; origin may not.** Another clone can claim and publish first. When this clone later rebases, Law M decides. That is the intended I2 + fail-loud combination from #244; claim/TTL/race were explicitly not changed there.

`ops-branch-shared-sync-policy.md` describes an older silent high-stakes path. Treat `architecture.md` § Ops publish (updated for #244) as the live publish spec, not that sketch's "errors ignored" table.

### 1.10 Duplication map (same rule re-derived)

| Rule | Canonical (if any) | Copies |
| --- | --- | --- |
| Staleness predicate | `IsClaimStale` + `FoldLastActivity` | `Issue.ClaimStale`; doctor; ready; worktree; harness_hook; PlanCompensation; ForeignLiveLease (after TTL rewrite); workers; hook |
| Zero TTL | **split** — see §1.11 | `IsClaimStale` never; config D10; claim CLI 60 fallback; ForeignLiveLease 60; workers winner 60; workers/hook status default_ttl |
| Race winner | **split** | Law M steal-on-stale; Law R earliest; workers sequential+fallback; CLI token match |
| Claimant-only heartbeat clocks | `ClaimantHeartbeatClocks` | `applyHeartbeat`; workers `recordHeartbeat` only if that worker's struct exists (not the same predicate) |
| "Do I still own this lease?" | `HeldByExactWorkerAndClaimToken` | CLI win check; compensation skip; `compensationApplies` |
| Parent open → in-progress on claim | `promoteParentToInProgress` | CLI also appends a **second** transition if parent still open in the **index** (`claim.go:1147-1157`), best-effort `appendOp` |

### 1.11 Inconsistencies: intentional vs accident

| Observation | Verdict | Evidence |
| --- | --- | --- |
| `--ttl 0` never expires **and** present `"default_ttl": 0` is D10 | **Intentional dual law** | `docs/configuration.md:14,112`; `docs/use-cases.md:253`; `strict.go:112-115`; `IsClaimStale` comment |
| Foreign live-lease steal treats TTL≤0 as 60, unlike `IsClaimStale` | **Intentional for MATENC-S1, accidental as a product law** | Test name and comments `race_test.go:66-75`; mk-reshape said do not change TTL/race. Result: `--ttl 0` is never-expire for ready/doctor/harness, stealable after 60 minutes at replay. A reader cannot hold one sentence. |
| Architecture "first claim by timestamp wins" | **Stale doc / accident vs code** | `architecture.md:487-489` vs `applyClaim` + `race_test.go:91-100`. mk-reshape: do not unify. |
| `ResolveClaim` still used by audit + workers fallback | **Accident leftover** of the architecture sentence. Materialize does not use it. |
| Same-timestamp tie: `ResolveClaim` uses WorkerID; `sortOpsByTimestamp` does **not** | **Accident** | `claim.go:25-27` vs `pipeline.go:257-263` |
| Heartbeat ⇒ `in-progress` | **Stale architecture** (`architecture.md:204`) vs `applyHeartbeat` |
| Overlap "never blocks" | **Stale architecture** vs `PlanClaim` + `claim-overlap-plan.md` |
| `HeldByExactWorkerAndClaimToken` requires `StatusClaimed` | **Intentional for LNGHZN-S5-T9** (compensate the exact claim, not a later one) **and** a sharp edge: after `in-progress`, token-gated compensation is always a no-op |
| Clone flock vs cross-clone race | **Intentional** | I3; flock comments; architecture post-claim flow does not rematerialize from origin (`architecture.md:515`) |
| Ready TUI writes a different claim | **Accident** | `ready.go:215-224` vs `newClaimCmd` |
| `StaleClaims` vs `ExpiredClaims` status filter | **Intentional split** | `stale.go:45-50`; doctor uses both (`fix.go:35-48`) |
| Low-stakes heartbeat may never reach origin | **Intentional** | #244 / configuration.md; heartbeats must not eject workers |
| Keep local claim on publish failure | **Intentional I2** | #244; constitution I2; `helpers.go` does not reset |
| Arena B / state v2 / D11 | **Parked by instruction** | `mk-reshape-a-primary.md`; `reservations.go:19-24`; `checkpoint.go:19` `CurrentStateVersion = 1` |

---

## 2. Three whole-shape candidates

Usage sketches are written from the caller side first. Go types for all three live in `/opt/cursor/artifacts/claim-ttl-sketch.go` (not compiled into the module).

Shared non-goals: new daemon (T1), history rewrite (T2), bidirectional sync with a mutable remote (T3), Arena B overlay, D11, rolling back local commits.

### Candidate A — Minimal: keep this shape, fix accidents

**Where truth lives:** Still in the incremental fold `applyClaim` / `ForeignLiveLeaseBlocksChallenger`, plus clone flock, plus CLI token reload. No new op types. No fetch-on-read.

#### Usage (caller first)

```text
arm claim T: flock (clone) → PlanClaim → append claim → push _armature (fail loud, keep local)
           → reload snapshot → HeldByExactWorkerAndClaimToken → provision or "lost race"
arm ready:  ComputeReady on fold; ExpiredClaims via Issue.ClaimStale (recorded TTL)
materialize: ApplyOpsSorted; applyClaim is still the lease mutator
arm doctor:  D2 from ClaimStale; --fix still writes transitions; D11 untouched
arm log:     lost-race marks use the SAME steal-on-stale function as applyClaim, not ResolveClaim
```

#### What it deletes / adds

Deletes (accidents):

- `ResolveClaim` as a live winner (keep a test that it is unused, or delete).
- `zeroTTLHeldLeaseReplayFallbackMinutes` — steal uses `IsClaimStale` as-is (`--ttl 0` never stealable).
- Hardcoded `60` in `workers.go` winner sim; use recorded TTL + `IsClaimStale`.
- Ready TUI private claim append; call the same command path or refuse interactive claim without `--worktree`.
- Architecture sentences that contradict code (first-timestamp-wins; heartbeat⇒in-progress; overlap never blocks; ready-rule 4).

Adds: almost nothing. One exported `LeaseLive(held, now) bool` if workers/hook should stop copying clocks. Optional: include `in-progress` in `HeldByExactWorkerAndClaimToken` **or** document why compensation is claimed-only (prefer document; changing it changes LNGHZN tests).

Migration: **existing ops logs replay identically** except the listed accident fixes, which **must** be called out:

1. Zero-TTL leases become non-stealable (were stealable at 60 min under Law M).
2. Audit `[lost race]` marks will follow steal-on-stale (a later claim after expiry is a win, not a loss).
3. Same-timestamp two-claim order remains sort-stable, not WorkerID — document, do not silently add WorkerID to `sortOpsByTimestamp` (that would change replay of equal-timestamp pairs).

Failure: two clones race — same as today (both append; publish rebase; fold steals only if first is stale at second timestamp). Publish fail — same as #244 (local kept, CLI fails, `arm push-ops`).

### Candidate B — Single ownership function at read time

**Where truth lives:** One pure function `Owner(history, issueID, now) Lease` over the sorted op list. Materialize's `Issue` claim fields are a **cache of Owner at last applied op timestamp**, not a second law. Writes only append. Flock remains a clone mutex for exclude/worktree, not a winner.

This is the architecture.md *intent* (read-time resolution) actually implemented once, using the **steal-on-stale** rule that the engine already runs, not the earliest-wins sentence.

#### Usage (caller first)

```text
arm claim T: PlanClaim (overlap only) → append claim+token → publish
           → winner := Owner(allLogs, T, now)
           → if winner.Token != myToken { report lost; do not provision }
arm ready:  ready iff status open AND Owner(id, now).Holder == ""
            expired iff Owner shows a holder and !LeaseLive
materialize: applyClaim writes fields := Owner after this op
             (or applyClaim calls Accept(held, claimOp) which IS Owner's step)
arm doctor:  ClaimStale := !Owner(id, now).Live
arm workers / arm log / hook: Owner or LeaseLive only; no private folds
```

#### Signatures (see sketch)

`type Lease struct { Holder, Token string; Since, LastActivity int64; TTLMinutes int; StatusAtHold string }`  
`func Owner(log []ops.Op, issueID string, now int64) Lease`  
`func Accept(held Lease, claim ops.Op) (next Lease, took bool)` — the one step `applyClaim` must call.  
`func LeaseLive(l Lease, now int64) bool` — only `IsClaimStale` polarity.  
`ResolveClaim` deleted from production.

Module map:

- `internal/claim` **owns** Accept / Owner / LeaseLive / PlanClaim / PlanCompensation / heartbeat debounce. Deep module: callers pass ops or a Lease, not Issue guts.
- `internal/materialize` **imports claim**, applies Accept, does not re-derive steal.
- `cmd/armature` **does not** fold clocks. Workers/hook/ready TUI go through snapshot + Owner.
- `internal/audit` annotates lost claims via Accept over the prefix, not earliest-wins.
- `internal/ops` unchanged schema. No expiry op.

Deletes: `ResolveClaim`, `claimWinnersByIssue`, `hookFindActiveClaimID` clock maps, `zeroTTLHeldLeaseReplayFallbackMinutes`, duplicate TTL fallbacks except the **documented** config vs `--ttl 0` dual law (config default remains "how we fill the flag"; recorded 0 remains never-expire **everywhere**, including steal).

Adds: `Owner` / `Accept` as the module surface (~one file replacing `race.go` + the workers fold).

Migration: replay identical to **post-accident** Law M (Candidate A's intended end state). Golden: fold existing logs; Issue claim fields byte-equal except the zero-TTL steal cases listed in A.

Failure: two clones — Owner after both logs are present; no write-time CAS. Publish fail — local Owner says you hold it; other clones disagree until `push-ops`. That disagreement is I1 (git is the bus) plus I2 (do not unwrite). Doctor D12 already flags lag; do not invent D11.

### Candidate C — Lease model with explicit expiry ops

**Where truth lives:** Liveness is **written**. A claim op records `expires_at` (or TTL still, plus a later `expire` / `renew` op). Materialize does not consult wall clock except when a worker **emits** expire/renew. Ready/doctor see status `open` because an expire op (or doctor) wrote it.

#### Usage (caller first)

```text
arm claim T: append claim{expires_at: now+ttl or never}
arm heartbeat T: append renew{expires_at: now+ttl} (still low-stakes, best-effort publish)
arm ready:   no now-based ClaimStale; expired issues are already open
arm doctor:  may append expire ops for leases whose expires_at < now (this is the only now-fold, and it is a write)
materialize: applyExpire → clear lease; applyRenew → extend expires_at if claimant
```

#### Signatures (see sketch)

`OpExpire`, `OpRenew` **or** reuse `heartbeat` with `expires_at` on payload and a doctor-emitted `transition to open`. Prefer **no new op type** if C is chosen: heartbeat carries `expires_at`; doctor `--fix` remains the expire writer. A true lease model still needs a deterministic "someone must write expire" — that someone is doctor or the next challenger (`claim` still steals only after an expire op exists).

Structurally distinct version (the one scored below): **challenger cannot steal until an `expire` op is in the log.** `applyClaim` blocks whenever `ClaimedBy != ""` regardless of wall clock. TTL is a doctor/worker concern, not a fold concern.

Deletes: `IsClaimStale` from materialize/ready/compute (moves to doctor planner). `ForeignLiveLeaseBlocksChallenger` time argument.

Adds: expire op or mandatory doctor pass; agents must run doctor or an expire verb before reclaim. New concept: "lease record" vs "lease liveness event."

Migration: **old logs have no expire ops.** Replay must either (1) keep a compatibility now-fold forever (then C is not actually a different truth), or (2) one-time doctor rewrite — forbidden (I2) unless expire ops are **appended**, not substitutions. Bootstrapping: first `arm doctor --fix` after upgrade emits expire transitions for currently stale leases. Until that runs, ready queue stays empty of those tasks (same as today). Cross-clone: two clones can both think they hold a never-expired claim until expire is published.

Failure: two clones race while both leases unexpired — first apply wins forever until expire is written. Publish fail of expire: local open, origin still claimed (symmetric to today's claim publish fail). Worse than B for coordination: reclaim is blocked on a second write.

Write-time lock + publish (the other structural cousin) is **rejected** as a fourth candidate: a flock cannot span clones without a shared lock file on `_armature` (conflicts with I3) or a server (T1). "Publish then fetch then rematerialize then compensate if lost" is a **cmd adapter** on top of B, not a different truth. It can be a later CLI nicety; it is not a shape.

---

## 3. Screening

### 3.1 pstack / Ousterhout red flags

| Flag | A Minimal | B Owner | C Expire-ops |
| --- | --- | --- | --- |
| **Shallow modules** | `internal/claim` already mixes overlap, compensation, stale math, and a unused ResolveClaim. A leaves that pile, slightly smaller. | Deepens `internal/claim`: one Accept/Owner interface; cmd/workers/audit become thin. | Adds `expire` as another shallow verb unless doctor is the only writer — then doctor becomes the lease engine (wrong owner). |
| **Information leakage** | Issue fields, TTL 0, and token status leak across cmd, ready, workers, audit. A plugs the documented leaks, leaves Issue as the API. | Lease is the API; Issue claim fields are derived. Callers stop reading `ClaimTTL` to invent a 60. | `expires_at` on the log **is** the leaked clock; every reader must know doctor might not have run. |
| **Temporal decomposition** | Claim vs heartbeat vs doctor vs ready each implement "later." A does not merge time. | Owner(now) is the only time. Writes are not ordered as a protocol beyond append. | Protocol: claim → (renew)* → expire → claim. Temporal split is the product. |
| **Pass-through methods** | `Issue.ClaimStale` already wraps `IsClaimStale`. A may add another wrapper. | `Accept` is the law, not a wrapper around two laws. | `applyExpire` that only copies payload into fields is a pass-through unless it is the only mutator (then it is fine). |

### 3.2 Brian principles (scored with evidence, 1=worse, 5=better)

**Simplicity** — fewest concepts a reader must hold.

- **A: 3/5.** Reader still holds: flock (clone), steal-on-stale fold, token win check, dual zero-TTL (config vs flag), compensation token gate, separate ExpiredClaims list. Accident list shrinks but the shape is the current one. Evidence: §1 has four winner laws; A removes two of them (R and workers fallback) and one TTL rewrite.
- **B: 4/5.** Concepts: append a claim; Owner(now) is the holder; publish is orthogonal; flock is git-exclude only. Dual zero-TTL remains one documented config-vs-flag rule, not three implementations. Evidence: audit/workers/hook lose private folds (`audit.go:90-114`, `workers.go:234-308`, `hook.go:141-191`).
- **C: 2/5.** Adds expire/renew as first-class time, plus "doctor must have run," plus compatibility for logs without expire. Evidence: reclaim requires a second op; ready no longer has a now; two writers (claimant renew, doctor expire).

**Laziness** — least new code, prefer deleting, no speculative features.

- **A: 5/5.** Deletes dead winner + TTL rewrite + ready TUI fork; no new types. Evidence: mk-reshape already extracted `ForeignLiveLeaseBlocksChallenger`; A finishes the cleanup that story was forbidden to do.
- **B: 4/5.** Modest new surface (`Owner`/`Accept`) **in exchange for deleting** three folds. Speculative: none if Accept **is** today's applyClaim step. Risk: over-abstracting a snapshot API. Evidence: claim-overlap-plan forbade splitting race-winner **out** of `internal/claim` — B keeps it **in** claim, which matches that constraint.
- **C: 1/5.** New op or payload field, doctor migration appends, tests across old and new logs, heartbeat schema change. Speculative relative to actual bugs (duplicated folds, zero-TTL, stale docs). Evidence: no current bug requires expire-ops; D2 already writes transitions.

**Modularity** — one owner per rule, deep interface, small surface.

- **A: 2/5.** Law still lives in `applyClaim` **and** CLI token **and** (until deleted) other folds. `HeldByExactWorkerAndClaimToken` stays a second ownership predicate (status-gated). Evidence: `state.go:20-28` vs `race.go:17-32`.
- **B: 5/5.** One owner: `internal/claim.Accept`/`Owner`. Materialize applies; cmd reports; audit annotates. Overlap stays `PlanClaim`. Compensation stays `PlanCompensation` (token gate is "is this the lease Owner just named," not a fifth winner). Evidence: depguard already isolates `internal/claim` to ops+scopematch (`.golangci.yml`); B does not widen that.
- **C: 3/5.** Fold becomes simpler (no now) but **liveness ownership splits** to doctor + heartbeat + claim. Two modules own time.

### 3.3 Constraint fit

All three keep append-only logs, parked Arena B, reserved D11, best-effort low-stakes, no rollback. C pressures I4 (agents must expire before reclaim) more than A/B. Cross-clone lock is not in any viable candidate (I3).

---

## 4. Recommendation

**Build Candidate B** (single `Owner`/`Accept` at read time), with Candidate A's accident list as the **behavior spec** for that function.

### Why it beats the others

- **Simplicity:** One sentence a reader can hold: *Among claim ops, a foreign claim takes the lease only when the held lease is already stale at the new op's timestamp; same worker always replaces; `--ttl 0` never goes stale; config `default_ttl` 0 is not a claim TTL, it is an invalid config.* A still requires the reader to know which files secretly disagree. C requires a second verb.
- **Laziness:** B is A's deletes plus moving the existing `applyClaim` step behind a name everyone calls. It does not add expire ops, fetch-on-read, or a distributed lock. The sketch's `Owner` is a loop over `Accept` — the loop already exists in `ApplyOpsSorted`.
- **Modularity:** Race, TTL, and "lost" annotations gain one owner (`internal/claim`). That is the deepening `claim-overlap-plan.md:345-346` preserved (do not split race **out** of claim) and the mk-reshape graft (`claimLostRace` in claim, not in engine sermons) actually finished.

A is the right **incremental PR slicing** (delete `ResolveClaim` from audit/workers, delete 60-min steal fallback, fix docs, fix ready TUI) **inside** B, not a competing end state. Shipping A and stopping would leave `applyClaim` as an unnameable law and invite the next cmd to copy `workers.go` again.

C loses: more concepts, more code, worse reclaim, and it reintroduces write-time liveness that doctor already handles with transitions.

### Cost

- Refactor `applyClaim` to `Accept`; point workers, hook, audit, ready-explain at `Owner`/`LeaseLive`.
- Doc repair: architecture § claim race, heartbeat, overlap, ready rule 4; configuration.md already has the dual TTL law — keep it, make code match the never-expire side everywhere.
- Tests: see proof bar. Expect golden replay diffs **only** for zero-TTL steal and audit lost-race marks.
- Not in cost: Arena B, D11, opsync module, fetch-on-read, flock redesign.

### What would prove B wrong

1. **Equal-timestamp foreign claims** must be fair by WorkerID and today's sort-stable apply is producing real double-work in dogfood. Then Accept needs a documented tie-break (WorkerID) and that **is** a replay change — list it, do not hide it in a sort helper.
2. **`--ttl 0` steal-at-60** is load-bearing (someone relies on zero meaning "default 60 at replay"). Then the dual law is three-way and B's simplicity claim fails; keep the fallback and **name** it in configuration.md.
3. **Owner(now) on every ready** is too slow on 10k-op logs. Then keep Issue fields as cache (already the case) and only require cmd side-folds to die; Owner is for tests and audit.
4. Brian wants reclaim **impossible** until a human/doctor writes expire — that is C; B would be the wrong product.

### Proof bar for a later implementation PR

Do not merge an implementation without these concrete tests (names `_REQ_` per `docs/conventions.md`):

1. **Two clones race, both live TTLs:** clone A claim t=100 TTL=60, clone B claim t=110 TTL=60; after both logs present, `Owner` = A; B's CLI reports `lost_claim_race`; both ops remain in JSONL.
2. **Two clones race, first stale:** A at t=100 TTL=1; B at t=161; `Owner` = B (strict-after). At t=160 B still loses.
3. **Exact TTL boundary:** last activity + ttlSeconds is live; +1 second is stale (`TestIsClaimStale` already; must hold through Owner/Accept).
4. **Zero TTL:** `--ttl 0` never stale for ready, doctor, harness, **and** Accept (no 60-min steal). Present `"default_ttl": 0` still D10. Omitted config still fills claim flag with 60.
5. **Heartbeat renewal:** claimant heartbeat at t=150, TTL=1 minutes, now=209 live, now=211 stale; foreign heartbeat does not extend (`ClaimantHeartbeatClocks`).
6. **Publish failure keeps local claim:** existing `TestAppendHighStakesOp_PublishFailureKeepsLocalCommit` + claim mapping to `CLAIM-1`; Owner on that clone still shows the local worker; no JSONL line deleted.
7. **Replay of existing golden / fixture ops logs** produces identical materialized claim fields **unless** listed: (a) zero-TTL previously stolen at 60 minutes now retained; (b) audit lost-race marks follow Accept not ResolveClaim. No other field diffs.
8. **Ready TUI / any second claim writer** uses the same payload rules (token, TTL source, high-stakes publish) or is removed.
9. **`ResolveClaim` is dead** (compile-time unused or deleted). `claimWinnersByIssue` deleted.
10. **Compensation:** worktree failure still restores live same-worker prior lease; `IfClaimToken` mismatch still no-op; in-progress status behavior documented, not silently widened.

---

## 5. Teach section (plain language)

Armature's "who is working on this task" is not a row in a database. It is a **diary**. Each worker has their own diary file. Git publishes those files on a branch named `_armature`. The program **rereads every diary** and decides the current holder.

**Claim:** a diary line that says "I take this task for N minutes."  
**Heartbeat:** a diary line that says "I am still here," which pushes the deadline forward — but only if you are the person the diaries currently name as holder.  
**TTL:** the number of minutes of silence after which someone else may take the task. Silence is measured from the latest of: the claim, your heartbeat, or your own status change. Other people's notes do not count as you being alive.  
**`--ttl 0`:** you wrote "this one does not time out." That is allowed.  
**Config `default_ttl: 0`:** you tried to set the project's default timeout to zero. The doctor rejects that file. If you omit the field, the default is 60 minutes. Those are two different knobs.  
**Race:** two people wrote "I take it" before they saw each other. After both diaries are on the branch, the program keeps the first that was still inside its timeout when the second was written. The second line stays in the diary; it just does not count.  
**Lock file in `.git`:** only stops two claim commands in **this copy** of the repo from stepping on the same git exclude file. It does not stop a teammate's laptop.  
**Publish failure:** your diary line is saved locally. Git could not copy it to origin. The command fails and tells you to run `arm push-ops`. We do not erase your line. Until origin has it, a teammate can still take the task.  
**Expired vs ready:** an expired claim does not magically jump back onto the "ready" list. It stays "claimed but stale" until doctor or a new claim after timeout. That is so humans see the orphan instead of silently double-assigning.

What we should build: **one function** that reads the diaries and answers "who holds this, and are they still inside the timeout?" Everything else (ready list, doctor, log annotations, the claim command's "you lost") should ask that function. We should not add a second kind of "expired" line unless we are sure we want reclaim to wait on doctor.

---

## 6. What a diagram should show (3–6 things)

1. **Actors:** Worker A clone, Worker B clone, origin `_armature` (two log files, not one lock).
2. **States of an issue lease:** unheld / held-live / held-stale (status still claimed or in-progress) / released (status open) / terminal (done, merged, cancelled, blocked as asserted).
3. **Transitions:** claim Accept; heartbeat extends live; claimant transition; steal-on-stale Accept; doctor release/block; compensation restore vs open (token-gated).
4. **Publish overlay:** local append (always) vs origin (high-stakes fail-loud, low-stakes best-effort) — a dashed box, not a fifth state.
5. **What is not an actor:** the clone flock (footnote: same-clone only); D11; Arena B overlay.
6. **Zero-TTL callout:** flag 0 = no arrow out of held-live via time; config 0 = doctor D10, not on this diagram's state machine.

---

## Appendix: file:line index

| Topic | Location |
| --- | --- |
| Claim command, TTL flag, flock, append, win check, compensate, parent appendOp | `cmd/armature/claim.go:857-859, 890-899, 1049-1160, 450-522, 1167` |
| Flocks | `cmd/armature/claim_lock.go:24-54` |
| Ready TUI claim | `cmd/armature/ready.go:215-224` |
| Publish | `cmd/armature/helpers.go:487-582` |
| Workers folds | `cmd/armature/workers.go:151-308` |
| Hook active claim | `cmd/armature/hook.go:141-191` |
| Stale math / ResolveClaim / debounce | `internal/claim/claim.go:12-70` |
| Steal / zero-TTL 60 / heartbeat clocks | `internal/claim/race.go:1-38` |
| Compensation plan | `internal/claim/compensate.go:34-66` |
| Overlap | `internal/claim/plan.go:45-127` |
| applyClaim / heartbeat / transition / token gate | `internal/materialize/engine.go:147-238` |
| ClaimStale / HeldByExact | `internal/materialize/state.go:14-28` |
| Sort / fold | `internal/materialize/pipeline.go:246-273` |
| State version parked | `internal/materialize/checkpoint.go:19` |
| Stale vs expired lists | `internal/ready/stale.go` |
| Ready gate | `internal/ready/compute.go:31-62` |
| Doctor fix | `internal/doctor/fix.go:30-48` |
| D11 reserved | `internal/doctor/reservations.go:19-24` |
| D10 zero default_ttl | `internal/config/strict.go:112-115` |
| Config default 60 | `internal/config/config.go:44-51` |
| Audit ResolveClaim | `internal/audit/audit.go:90-114` |
| Architecture race/TTL/publish | `docs/design/architecture.md:353-365, 487-515` |
| Dual TTL law | `docs/configuration.md:14,102-112` |
| mk-reshape non-goals | `docs/design/mk-reshape-a-primary.md:31` |
| I2 / I3 | `CONSTITUTION.md` |
