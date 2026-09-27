# Claim order, published ownership, and cattle workers

**Status:** Design only. Human checkpoint (architect Phase C). No production code in this package.  
**Audience:** Brian.  
**Date:** 2026-09-27  
**Tree:** `main` at `e0298837155c4e765f3a18398a288d8e9cfe87af`.  
**Prior design:** `claim-ttl-architecture.md` (Candidate B: one `Accept` / `Owner` / `LeaseLive` in `internal/claim`). That PR is assumed to land. This package does not reopen TTL dual-law, `ResolveClaim` deletion, or Arena B / D11.

Hard constraints: I1–I7, T1–T3; append-only `_armature` JSONL; no rollback of a local ops commit on publish failure; low-stakes publish stays best-effort except where this design **raises heartbeat publish when it is the liveness signal**; clone flock stays clone-local.

---

## Brief vs `main` (every claim checked)

HEAD matches the brief’s `e0298837`. Verified against the tree, not the brief’s wording.

| Brief claim | Verdict | Evidence |
| --- | --- | --- |
| `arm claim` appends, commits, `publishArmatureSequence` (Push, on rejection FetchAndRebase + one more Push), then reload + `HeldByExactWorkerAndClaimToken` → `lost_claim_race` | **Correct** | `cmd/armature/claim.go:1061-1110`; `helpers.go:555-564, 487-507`. Win check is **local store after publish returns**, not origin-only. |
| Second push failure → `localArmatureTipPublishError`, claim stays local | **Correct** | `helpers.go:517-570`; I2; #244. Reload / win check is skipped because `appendHighStakesOp` returned error. |
| Replay orders by timestamp (`sortOpsByTimestamp`); `time.Now().Unix()` whole seconds; same-second ties → stable load order | **Mostly correct; tie-break is sharper than “load order”** | `nowEpoch` is `Unix()` (`helpers.go:348-349`). Sort: timestamp, then `opSortKey` (`pipeline.go:246-263`). Equal timestamp **and** equal type key: `SortStableFunc` keeps prior order. Prior order is `LoadFromDirValidated` concatenating `ListLogFiles` (`opstream.go:117-135`). `os.ReadDir` returns names **sorted by filename**, so same-second two claims tie-break by **log filename** (`<worker-id>.log` / `<worker-id>~<slot>.log`), not an unspecified scramble. |
| If B (later timestamp) publishes and verifies first, then A (earlier timestamp) rebases, both believe they won; clock skew widens it | **Correct mechanism** | B’s win check sees only logs already in B’s ops worktree (`architecture.md:330-340` reads do not fetch). A’s `FetchAndRebase` then fold: earlier timestamp applied first, B’s live-lease claim is a no-op (`engine.go:147-157`). B never re-checks. This is split-brain, not “two Accept winners in one fold.” |
| `arm heartbeat` does not verify ownership; non-owner heartbeat ignored via `ClaimantHeartbeatClocks` | **Correct** | `cmd/armature/heartbeat.go:28-34` always `appendLowStakesOp` and prints success. `engine.go:172-178` + `race.go:36-38`. |
| `arm transition` has no ownership check beyond delivery-gate branch binding; a loser can mark `done` | **Correct, and slightly understated** | `transition.go` binds `WorkerID` from `resolveWorkerAndLog` and runs the delivery gate using `gateIssue.ClaimedBy` as **gate input**, not “caller must equal owner” (`transition.go:164-184, 351-352`). `applyTransition` applies any worker unless `IfClaimToken` is set (`engine.go:182-221`). Token gate is compensation-only. |
| Heartbeats publish only every `LowStakesPushThreshold` (`DefaultPendingOpCount` = 5); remote clones can see a live owner as stale; TTLs 30–3600 | **Threshold correct; “real data 30–3600” not in this tree** | `config.go:46,53,135`; `helpers.go:584-615`; `docs/configuration.md:16`. In-tree claim TTLs: flag/config default **60**; dogfood **120 / 240**; architecture prose **15 / 1440**. No `30` or `3600` on `main`. Treat 30–3600 as an operational range, not a repo fact. |
| Worker id is random UUID in local git config; `ARM_LOG_SLOT` appends `~slot` with charset check only | **Correct** | `internal/worker/identity.go:11-20` `uuid.New()` + `armature.worker-id`. `helpers.go:325-340` `^[A-Za-z0-9_-]+$`; invalid slot **warns and ignores** (falls back to unslotted id — two agents can then share one file). |
| `docs/concepts.md` wrongly says id is a hash of git name/email | **Correct (doc is wrong)** | `docs/concepts.md:40-44` vs `identity.go`. Same page also says claim races tie-break lexicographically by worker ID (`ResolveClaim`); materialize does not. |
| `internal/snapshot` bounds replay | **False as stated** | `snapshot.Store.Load` always `ops.LoadFromDirValidated` which reads every log from offset 0 (`opstream.go:77`). `materialize.Run` still `sortOpsByTimestamp(allOps)` and applies **all** ops (`pipeline.go:147-150`). `FullReplay=false` only means issue JSON was loaded first; `OpsProcessed` is still `len(allOps)` (`pipeline_test.go:573-574`). `MaterializeIncremental` **resets** then full-replays (`api.go:18-28`). `Checkpoint.LastCommitSHA` is never written by `Run`. `architecture.md:375-388` and `docs/concepts.md:16-17` describe O(new ops) via `git log last_sha..HEAD` + byte seeks; **that algorithm is not implemented.** |
| Claim-ttl already in tree (`Accept`, `Owner`, `LeaseLive`, `ResolveClaim` gone, TTL≥1, legacy 0→60) | **False on this SHA** | `ResolveClaim` still live (`claim.go:17-31`). `ForeignLiveLeaseBlocksChallenger` + 60-minute zero-TTL steal fallback still in `race.go`. Design below **assumes the claim-ttl PR lands** and grafts onto `Accept`/`Owner`/`LeaseLive`. |

Stale docs that would mislead this work (not brief errors): `architecture.md:487-489` “first claim by timestamp wins” / WorkerID tie-break; `:515` “CLI does **not** pull and re-materialize” (claim **does** reload locally after publish); `:204` heartbeat⇒`in-progress`; overlap never blocks.

---

## 1. Grounding model (call paths)

### 1.1 Why the current shape exists

I3: each worker writes only `ops/<id>.log`. Cross-clone exclusion is git merge of different files, not a lock. Publish is Push; on rejection, one rebase+Push (`helpers.go:555-564`). There is no compare-and-swap on a shared claim file because that file would be I3-illegal. Ownership was therefore defined as a **pure fold of JSONL** (`applyClaim`). Timestamps were the only total order available **without walking git**. That is why Law M uses `op.Timestamp` as both “when the worker thought it happened” and “`now` for steal.” Claim-ttl names that fold `Accept`/`Owner`; it does not change the order key.

Reads do not fetch so a laptop without network still lists tasks (I4, offline). The cost is: **this clone’s Owner is “logs I already have,” not “origin.”** #244 made high-stakes publish fail-loud but kept the local commit (I2). So “I appended a claim” and “origin has my claim” are different facts. The CLI win check conflates them: after a **successful** push, B can still miss A’s not-yet-fetched log.

### 1.2 Claim write path (traced)

```
arm claim
  tryAcquirePessimisticCloneClaimFlock          claim.go:890   // this git dir only
  PlanClaim                                     plan.go
  newClaimToken + nowEpoch()                    claim.go:1049-1056
  appendHighStakesOp
    AppendAndCommitIf                           helpers.go:502
    pushOpsBranch → publishArmatureSequence     helpers.go:506,555-564
  store.Load + HeldByExactWorkerAndClaimToken   claim.go:1077-1085
```

`HeldByExactWorkerAndClaimToken` requires `Status == claimed` (not `in-progress`) (`state.go:20-28`). After a later transition, the same token looks like a loss. Claim-ttl keeps that as compensation’s gate; owner-only verbs in **this** design must use `Owner`/`Lease.Token`, not that status-gated helper.

### 1.3 Heartbeat / transition / render-context

| Verb | Publish | Ownership at CLI | Fold |
| --- | --- | --- | --- |
| `heartbeat` | low-stakes, every 5 ops | none; always “sent” | clocks iff `ClaimantHeartbeatClocks` |
| `transition` | high-stakes | delivery gate uses `ClaimedBy` as metadata; append always | applies unless `IfClaimToken` mismatches |
| `render-context` | none | none | snapshot or `MaterializeAtSHA` |

### 1.4 Identity and log layout (traced)

```
resolveWorkerAndLog
  worker.GetWorkerID(repo)           // git config armature.worker-id
  slottedWorkerID(id)                // + "~" + ARM_LOG_SLOT if charset OK
  opsLogPath(..., id+".log")         helpers.go:309-322
```

`LoadFromDirValidated` expected worker is the **filename stem including slot** (`opstream.go:133`). `WorkerIDFromFilename` **strips** `~slot` (`files.go:505-514`) for `arm workers` display. Ops `worker_id` field is the slotted string. Two cloud clones: two `uuid.New()` values, two log files, never compacted. Same AMI with baked `armature.worker-id`: **two writers, one file — I3 broken.** Charset check does not detect two processes with the same slot in one clone; they share a file.

Local state that **must not** decide cross-clone ownership: clone flock, git exclude lock, `state/<worker>/` JSON, `checkpoint.json`, pending-push tracker, unpublished ops, `ARM_LOG_SLOT` invalid-fallback.

### 1.5 What “linear `_armature`” actually is

`publishArmatureSequence` is fast-forward-only in effect: non-FF push → rebase onto `origin/_armature` → push. After both succeed, every clone that fetches sees the **same commit list**. That list is a total order of **commits**, each typically one log append. Materialize **does not use it**. It concatenates files and sorts by worker clocks.

Git committer dates are also worker clocks (`git commit` on the claiming machine). Commit **order** (parent→child) is not a clock; it is the CAS outcome of push. That is the only cross-clone agreement that does not trust NTP.

---

## 2. Problem restatement

**P1.** “Who owns this issue?” must be the same function on every clone that has fetched `origin/_armature`, must not award ownership to an unpublished append, and must not let a publish-loser heartbeat or `transition --to done`. Lease liveness for steal must not treat unpublished heartbeats as proof of life. Worker `Unix()` must not pick the race winner.

**P2.** Disposable cloud VMs are cattle: identity cannot be “UUID I wrote into this clone’s git config last month.” Slot collisions must fail loud. Replay must not grow unbounded **without a named bound**; today there is no implemented bound (`internal/snapshot` does not provide one).

P1 and P2 share the fold and the log filename. They do not share the identity assignment policy. See §6 for the workstream split.

---

## 3. Two whole-shape candidates

Usage is written from the caller first. Types live in `docs/design/claim-order-sketch.go` (`//go:build ignore`). Shared non-goals: daemon (T1), rewriting JSONL (T2), fetch-on-every-read for `arm list`/`arm ready` (stay local logs + D12 lag), Arena B, D11, rolling back local commits.

### Candidate A — Published commit-order fold (extends claim-ttl `Accept`)

**Where truth lives:** The same `Accept`/`Owner`/`LeaseLive` as claim-ttl, but the log they fold is the **published prefix** of `_armature`, ordered by **commit sequence** then line in that commit, not by `op.Timestamp`. Unpublished ops are visible to the writing clone as `Pending`, never as `Owner`. Heartbeat/transition/render-context ask `RequireOwner`. Identity is an opaque string on the op; cattle assignment is Candidate A’s identity module (env from dispatcher).

#### Usage

```text
dispatcher: export ARM_WORKER_ID=$CLOUD_RUN_ID   # unique per VM/run
arm worker-init --check                          # pets: git config UUID; cattle: env wins

arm claim T:
  flock (clone) → PlanClaim → append claim+token (local commit kept)
  publishArmatureSequence
  if publish fails: CLAIM-1, Owner(published)=not me, Pending=my token
  if publish ok: Owner(published prefix) must equal my token or lost_claim_race

arm heartbeat T / transition T / render-context T:
  RequireOwner(published ∪ {this clone's committed-and-pushed tip if it matches origin})
  mismatch → named error, no silent success
  heartbeat that renews liveness: publish immediately (still not a daemon;
  one Push per heartbeat, fail-loud like high-stakes). Notes stay low-stakes.

materialize:
  checkpoint.last_materialized_commit
  git log last..HEAD on the ops worktree, apply new lines in that order
  Accept(held, claimOp) with now = committer time of that commit for steal
  cold clone: walk from root (or from a snapshot ref if slice 3 lands)

steal:
  foreign claim Accepts only if !LeaseLive(held, challengerCommitTime)
  unpublished claimant activity does not extend held
```

#### What it deletes / adds

Deletes: CLI win check via `HeldByExactWorkerAndClaimToken` for “did I win the race”; silent heartbeat success for non-owners; timestamp as race order key **after cutover**.

Adds: `PublishedSeq` recovered from git (not written into JSONL); `RequireOwner`; `ResolveIdentity` (env > config); log-path flock; real `LastCommitSHA` incremental apply.

Migration: JSONL bytes unchanged. Cutover commit C0 on `_armature`: ops **first introduced** in commits before C0 keep **timestamp order among themselves** (claim-ttl behavior, including existing races); ops first introduced at/after C0 sort by commit seq. Document winner changes **only** for in-flight races that straddle C0.

### Candidate B — Compare-and-swap claim ref + cattle logs

**Where truth lives:** `refs/armature/claim/<issue-id>` (or `refs/armature/leases/<issue-id>`) is a git object `{holder, token, ttl, last_commit}`. `arm claim` writes the blob and `git push --force-with-lease` that ref. Winner is the CAS; JSONL claim is audit. Identity still env/UUID; each cattle run still has its own log for notes/transitions.

#### Usage

```text
arm claim T:
  fetch refs/armature/claim/T
  if lease live and holder≠me: refuse
  write blob, force-with-lease from observed SHA
  on reject: lost_claim_race (no fold involved)
  append claim op for audit (high-stakes, may lag the ref)

arm heartbeat T:
  CAS update last_commit / expiry on the same ref; fail if not holder

Owner(): read the ref, not the fold
materialize status still from JSONL; doctor if ref and fold disagree
```

#### What it deletes / adds

Deletes: fold as ownership law (claim-ttl `Owner` becomes a replica/audit).

Adds: one ref per in-flight issue; force-with-lease protocol; divergence doctor; still need JSONL for I2 history.

Migration: old issues have no ref. Boot: first `arm claim` or `arm doctor --fix` creates refs from current `Owner(timestamp fold)` without rewriting logs.

### Candidate A′ (screened, not a third shape) — timestamps + verify-after-sync only

Keep timestamp `Accept`. After every claim publish, fetch+reload; owner-only verbs fetch then `RequireOwner`. Unpublished still looks owned on the writer until they fetch a contradicting log.

This is a **cmd protocol** on claim-ttl, not a new truth. It closes the “loser marks done” hole if every owner verb fetches, but **race winner remains worker `Unix()`**, so the brief’s “B published first, A’s earlier clock wins after rebase” remains: after both are published, `Owner` is A. B loses at next fetch. Better than today, worse than A for “linear history is the total order.” **Not scored as a whole shape**; pieces (RequireOwner, fetch on owner verbs) are reused in A.

---

## 4. Red-flag screen

| Flag | A Commit-order fold | B CAS refs |
| --- | --- | --- |
| **Shallow modules** | Deepens claim-ttl `internal/claim` with `PublishedPrefix` + `RequireOwner`. Identity is a second deep module (`internal/worker`). Cmd stays thin. Risk: git-walk leaking into `claim`. Mitigate: `internal/oporder` (or `materialize`) yields `[]LocatedOp`; `claim.Owner` stays pure. | `Owner` becomes “read a ref” — shallow unless CAS + live-lease + doctor reconciliation live behind one `LeaseStore`. Fold and ref are two laws until doctor is perfect. |
| **Information leakage** | Cmd must not parse commit seq. `LocatedOp` carries `Seq`. Identity callers get `Identity{ID, LogPath}` not env/git details. | Every command learns force-with-lease and ref names. Fold/ref mismatch leaks into doctor and agents (I4 noise). |
| **Temporal decomposition** | Protocol: append → publish → Owner(published). Time for TTL is committer date of **published** activity vs observer/challenger commit time. One pipeline, not claim-then-later-verify as a second program. | Claim ref update then JSONL append is a two-phase protocol; crash between them is a new state. |
| **Pass-through methods** | `RequireOwner` is a real gate (error). Do not wrap `HeldByExactWorkerAndClaimToken`. | `Owner` that only returns `git show ref` is a pass-through unless it includes live/stale. |

**Interface depth:** A hides rebase, file concat, and cutover behind `Owner(locatedOps, issue, now)` — the same sentence as claim-ttl plus “located ops are published commit order.” B’s public surface is smaller (`ClaimLease(issue)`) but the **system** surface is larger (refs + logs + reconciliation). Prefer A (hides more behind the existing claim module).

**Constitution:** A stays on per-worker files (I3). B’s CAS refs are git-native (I1) and not JSONL rewrites (I2), but they are **shared mutable refs** — not merge-conflict-free by construction; they are conflict-free by CAS retries. That is a different I3 reading. This design treats I3 as **one writer per ops log file**. Extra refs are allowed only if they are not a second ownership log that workers merge. B still trips “we need a lock” (the ref **is** the lock). Scrap signal.

**T1:** neither candidate is a daemon. Heartbeat-every-push is a CLI invocation.

**T2:** neither rewrites JSONL. B’s `--force-with-lease` on a lease ref is ref motion, not history rewrite of `_armature`. Still feels like T2-adjacent to reviewers; disclose it.

**Clocks:** A’s race winner is push order (no clock). TTL still needs a clock: **git committer date of the published activity commit** vs challenger’s committer date (deterministic replay) or vs observer `now` at read (ready/doctor). Do not use unpublished `op.Timestamp` for steal. B can store `expires_at` on the blob (writer clock again) or recompute from committer date.

---

## 5. Synthesis decision

**Build Candidate A** (published commit-order fold + dispatcher identity), grafting onto claim-ttl `Accept`/`Owner`/`LeaseLive`. Reject B as the ownership law. Do not ship A′ as the end state.

### Why A

- One sentence: *You own an issue iff your claim token is `Owner` of the published `_armature` prefix, folded in commit order with claim-ttl steal-on-stale; unpublished appends are pending, not owned; owner verbs fail loud.*
- Reuses claim-ttl instead of replacing it with refs.
- I3 unchanged: still one writer per log file.
- Cattle replay bound is the **same** `LastCommitSHA` walk that recovers commit order — one mechanism, two payoffs.
- Unpublished claims already exist (#244); A names them instead of pretending the local fold is global.

### Why not B

- Second source of truth vs JSONL; crash windows; I3-as-CAS instead of I3-as-files; claim-ttl `Owner` becomes dead on arrival.
- Per-issue refs do not bound cattle log replay.

### What would prove A wrong

1. Walking `_armature` to assign seq on every large clone is slower than timestamp sort and cannot be checkpointed (then keep timestamp `Accept` among **published file contents**, still excluding unpublished — that is A degraded to A′ plus published-set, and the dual-winner “earlier clock after rebase” stays).
2. Brian wants first-clock-wins as the product law (claim-ttl / architecture sentence) — then only RequireOwner + published-set, not commit-order.
3. Cloud dispatch cannot inject a unique `ARM_WORKER_ID` (then cattle identity is still UUID-per-clone; collision detection still ships; log explosion remains).
4. Heartbeat-as-high-stakes ejects workers on flaky git (then keep low-stakes heartbeat but **Owner/steal ignore unpublished heartbeats**, so a live owner who cannot push will look stale — disclose that trade).

### Identity (part of A, not a third shape)

| Source | When | Log file |
| --- | --- | --- |
| `ARM_WORKER_ID` env (charset = slot charset) | Dispatcher / cloud run | `ops/<id>.log` |
| else `git config armature.worker-id` | Pets after `worker-init` | same |
| `ARM_LOG_SLOT` | Only extra writers **in this clone**; refuse if `ARM_WORKER_ID` already unique | `ops/<id>~<slot>.log` |
| Collision | Exclusive flock on log path for append; if `ARM_WORKER_ID` set and git config differs, env wins and config is not rewritten unless `worker-init` | Fail `WORKER-COLLISION` / `LOG-SLOT-COLLISION` |

Do not hash name/email. Do not treat `WorkerIDFromFilename` stripping as identity (fix that display helper in the cattle slice). Cloud images must **not** bake `armature.worker-id`. Local git config must never be the cross-clone uniqueness guarantee.

**Local state that must not matter for correctness:** checkpoint, issue JSON, clone flock, unpublished ops, env charset fallback, TUI. **Must matter:** published `_armature` commits + per-file JSONL bytes + identity string on those ops.

---

## 6. Workstream split

**Two workstreams, serial on the fold, parallel on identity-after-interface-freeze.**

They are not one PR: P1 changes who wins a race; P2 changes who is allowed to write a log. Combining them hides regressions (“was that the seq fold or the env id?”).

They are not fully parallel: P2’s replay-bound proof (`LastCommitSHA`) **is** P1’s order recovery. Shipping cattle UUIDs first without published `Owner` leaves split-brain and I3-breakage on baked config.

| Stream | Depends on | Parallel with |
| --- | --- | --- |
| **W1 Claim order** | claim-ttl `Accept`/`Owner` landed | docs that already assume Owner |
| **W2 Cattle identity** | W1 `RequireOwner` + opaque `WorkerID` | can start identity env + flock **tests** once W1 API sketch is agreed; merge after W1 Owner is the only law |

**PR slices (W1 then W2):**

1. **W1.1 Published prefix** — `Owner` folds only ops present at ops-worktree HEAD that have been pushed (detect: commit is ancestor of `origin/_armature` **or** this command’s just-succeeded push). Unpublished local tip: `Pending`, not holder. Tests: unpublished claim not owned on another clone; publish failure keeps JSONL but `Owner` empty for that token globally.
2. **W1.2 Commit-order locate + cutover** — `LocatedOp` seq from git; `Accept` steal `now` = committer time; pre-C0 timestamp grandfather. Tests: two clones skewed clocks, B publishes first, exactly one owner (B); same-second two publishes, filename/commit order stable; grandfather fixtures.
3. **W1.3 Owner-only verbs** — heartbeat, transition (on claimed/in-progress), render-context: `RequireOwner`; named errors; heartbeat that renews lease publishes immediately. Tests: loser heartbeat/transition fail; winner render-context OK.
4. **W1.4 Incremental by `LastCommitSHA`** — implement `architecture.md` § incremental algorithm for real. Tests: N one-run logs, second invoke `OpsProcessed == new ops only`; cold clone still correct vs full walk.
5. **W2.1 `ARM_WORKER_ID` + collision flock** — env wins; baked-config two-writer detected on same file via flock (same clone) and doctor (divergent commit authorship / mismatch) for cross-clone. Tests: slot collision detected; env identity used on fresh clone without `worker-init` UUID.

Doctor `--fix` and `arm unassign` stay privileged (not owner-only). Notes stay annotative.

---

## 7. Constitution

| ID | How A honors it |
| --- | --- |
| **I1 Git-native** | Ownership = published `_armature` commits + JSONL. No DB. Identity env is an **input to the filename**, persisted in git as that file’s name and `worker_id` field. |
| **I2 Append-only** | No JSONL rewrite; local claim on publish fail kept; seq recovered from git, not patched into old lines. |
| **I3 Merge-conflict-free** | Still one writer per log. CAS refs rejected. Collision flock is clone-local (same as today’s claim flock). Cross-clone duplicate IDs are a **detect-and-fail**, not a merge. |
| **I4 Agents first** | Named errors (`lost_claim_race`, `CLAIM-NOT-PUBLISHED`, `NOT-CLAIM-OWNER`, `LOG-SLOT-COLLISION`); cattle one env var. |
| **I5 Deterministic gates** | `Owner` is pure over located ops. Delivery gates unchanged. |
| **I6 `done` ≠ `merged`** | Unchanged; loser cannot assert `done` without owning. |
| **I7 Humans accountable** | Doctor/unassign still human/coordinator overrides. |
| **T1** | No daemon. |
| **T2** | No history rewrite; cutover is new commits only. |
| **T3** | Still one-way publish of ops; no bidirectional mutable remote. |

---

## 8. Migration of existing `_armature` logs

1. Do not edit historical JSONL.
2. Pick cutover SHA C0 (the merge of W1.2). `LocateOps`: if an op’s **introducing commit** `< C0`, its sort key is `(0, timestamp, opSortKey, filename)`; if `>= C0`, `(1, commitSeq, lineInCommit)`.
3. In-flight leases spanning C0: **may change winner** if a later-clock claim had published first. List that in the W1.2 PR. Prefer cutting over when few claims are live, or grandfather **all currently live `Owner(timestamp)` tokens** by recording them as the held lease until they go stale (optional compatibility op: **not** required if dogfood can drain claims).
4. Zero-TTL / `ResolveClaim` behavior follows **claim-ttl**, not this note.
5. Checkpoints: first binary after W1.4 ignores old checkpoints missing `last_materialized_commit` (cold replay once). `state/` stays gitignored.

---

## 9. Proof bar (named tests per slice)

`_REQ_` suffix per `docs/conventions.md` once issues exist. Names below are the bar.

**W1.1**

- `TestUnpublishedClaimNotOwnedOnRemoteClone_REQ_*` — A appends+commit, push fails; B’s `Owner` is empty; A’s `Pending` is A’s token.
- `TestPublishedClaimOwnedAfterPush_REQ_*` — A push succeeds; B fetch; `Owner` is A.

**W1.2**

- `TestTwoClonesSkewedClocksExactlyOneOwner_REQ_*` — B clock +1h, A clock true; both claim live TTL; B push first; A rebase+push; `Owner==B` everywhere after fetch. Both ops remain in JSONL.
- `TestSameSecondPublishTie_REQ_*` — equal `op.Timestamp`; commit order decides; rerun stable.
- `TestGrandfatherPreCutoverTimestampOrder_REQ_*` — fixture logs introduced before C0 still match claim-ttl `Owner`.

**W1.3**

- `TestLoserHeartbeatFailsNamedError_REQ_*` — `NOT-CLAIM-OWNER` (or `ErrNotClaimOwner`); no clock bump; JSONL may contain the failed attempt **or** CLI refuses before append (prefer **refuse before append** so loser does not pollute; if we append, fold must ignore — today’s silent ignore is **not** enough because CLI said success).
- `TestLoserTransitionFailsNamedError_REQ_*` — cannot `--to done`.
- `TestRemoteDoesNotStealLivePublishedOwner_REQ_*` — A published claim+heartbeat; B sees logs; `Accept` no-op while `LeaseLive`.

**W1.4**

- `TestReplayCostBoundedWithNOneRunLogs_REQ_*` — N cattle logs already checkpointed; one new op; `OpsProcessed==1` (or `==len(new lines)`), not `==N`.
- Cold vs incremental byte-equal issues.

**W2.1**

- `TestARMWorkerIDAssignsLogAndOpWorkerID_REQ_*`
- `TestLogSlotCollisionDetected_REQ_*` — two processes same clone same id/slot; second append errors `LOG-SLOT-COLLISION`.
- `TestBakedGitConfigDoesNotOverrideEnv_REQ_*`

Claim-ttl tests (two live TTLs, boundary, zero TTL, publish-keeps-local) remain in force; W1.2 **changes** the two-live-TTL winner when publish order ≠ timestamp order — update that claim-ttl test explicitly, do not hide it.

---

## 10. Teach section

Today each agent writes a diary. Git publishes diaries on `_armature`. The program sorts diary lines by the **clock in the line**. If your clock is behind and you publish second, you can still “win,” and the person who already published thinks they won too because they never reread.

We should sort by **who got into git first**, ignore diaries that never left the laptop, and tell you loudly if you are not the owner when you heartbeat or mark done.

Cloud agents should not invent a random name from an empty laptop config. The dispatcher already has a run id; that string **is** the diary filename. Two agents with the same name must stop, not share a pen. Old diaries stay on the shelf; we do not rewrite them. We only change how we read from a marked commit onward. Fresh VMs still reread the whole shelf unless we finally implement the bookmark the architecture doc already described (`last_materialized_commit`). That bookmark is the same walk we use to know commit order — one bookmark, two jobs.

---

## 11. Diagram (what to draw)

1. Actors: clone A, clone B, `origin/_armature` (two log files + **commit spine**).
2. Layers: unpublished local commit (dashed) vs published prefix (solid).
3. `Accept` on the spine, not on wall clocks.
4. Owner verbs boxed: heartbeat / transition / render-context.
5. Cattle: `ARM_WORKER_ID` → filename; flock on that file; git config footnote for pets.
6. Cutover C0 on the spine; left = timestamp grandfather, right = commit seq.

---

## Appendix: file:line index (`e0298837`)

| Topic | Location |
| --- | --- |
| Claim + win check | `cmd/armature/claim.go:1049-1110` |
| Publish sequence | `cmd/armature/helpers.go:555-575` |
| Slot / nowEpoch / resolveWorker | `cmd/armature/helpers.go:309-349` |
| Heartbeat CLI | `cmd/armature/heartbeat.go:8-35` |
| Transition + gate | `cmd/armature/transition.go:151-184, 351-352` |
| Render-context | `cmd/armature/render_context.go:17-87` |
| Identity UUID | `internal/worker/identity.go:11-28` |
| Sort | `internal/materialize/pipeline.go:246-263, 113-150` |
| Checkpoint unused SHA | `internal/materialize/checkpoint.go:21-27` |
| Incremental = full replay | `internal/materialize/api.go:18-28` |
| Snapshot loads all | `internal/snapshot/snapshot.go:35-46` |
| Opstream offset 0 | `internal/ops/opstream.go:77, 117-135` |
| Apply claim / heartbeat / transition | `internal/materialize/engine.go:147-221` |
| Steal / heartbeat clocks | `internal/claim/race.go` |
| ResolveClaim leftover | `internal/claim/claim.go:17-31` |
| Filename strip slot | `internal/adapters/files.go:505-514` |
| FetchAndRebase | `internal/adapters/git.go:671-680` |
| Wrong concepts identity | `docs/concepts.md:40-44` |
| Aspirational incremental | `docs/design/architecture.md:375-388` |
