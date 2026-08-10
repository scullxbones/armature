# Recovery State Machine: Issue Status × Claim Liveness

**Author:** Armature Design  
**Status:** Design Document  
**Purpose:** Specify the correct reconciliation action for every combination of issue status and claim liveness, covering the three failure scenarios identified in dogfood testing.

---

## Executive Summary

This document enumerates every combination of:
- **Issue status:** `open`, `claimed`, `in-progress`, `done`, `merged`, `blocked`, `cancelled`
- **Claim liveness:** `unclaimed`, `claim-active` (TTL not expired), `claim-expired` (stale)

For each combination, it specifies:
1. Whether the state is valid or invalid
2. The correct reconciliation action for `arm doctor` (diagnostic) and the Coordinator (operational)
3. Coverage of three failure scenarios from dogfood findings:
   - **D1 — Branch Divergence:** Git commits reference an issue not yet in `done`/`merged` state
   - **D2 — Orphaned Claim:** Issue is `claimed`/`in-progress` with expired TTL and no worker activity
   - **Redispatch Starvation:** Coordinator does not re-dispatch a task after claim expiration, leaving it silently stuck

---

## State Machine Matrix

### Legend

| Term | Meaning |
|---|---|
| **unclaimed** | No `claim` op exists for this issue |
| **claim-active** | A `claim` op exists; no heartbeat/transition from claiming worker exists beyond TTL |
| **claim-expired** | A `claim` op exists; no heartbeat/transition from claiming worker within TTL minutes of claim timestamp or last heartbeat |
| **Valid** | State is coherent; no action required |
| **Warning** | State is valid but advisory; `arm doctor` emits a warning |
| **Invalid** | State violates invariants; `arm doctor` emits an error |
| **Doctor Action** | What `arm doctor --fix` should attempt to remediate; omitted if valid or warning only |
| **Coordinator Action** | What the Coordinator should do to unblock the DAG |

---

## Detailed State Table

| Status | Claim Liveness | Validity | Doctor Diagnosis | Doctor Action | Coordinator Action |
|---|---|---|---|---|---|
| **open** | unclaimed | Valid | No action needed. Issue ready to claim. | — | Offer task in ready queue. |
| **open** | claim-active | **Invalid** | Issue is `open` but has an active claim. Impossible state — claims require issues to transition out of `open`. | Reset claim: emit `claim` op with same issue-id to overwrite, then force status back to `open`. Or: delete stale claim ops if the claiming worker ID is unreachable. | Re-attempt claim on behalf of stale worker, or mark task as ready if original claimer is gone. |
| **open** | claim-expired | **Invalid** | Issue is `open` but has an expired claim. Claim should have been released or task promoted. | Emit a `claim` op with fresh TTL (represents "opened for re-claim"), or delete the expired claim ops and leave status `open`. | Treat as ready task; re-issue to available workers. |
| **claimed** | unclaimed | **Invalid** | Issue status is `claimed` but no `claim` op exists. Materialization error or op corruption. | Backfill: emit a `claim` op with recent timestamp and a placeholder TTL (e.g., 60 min). | Force transition to `open` and re-issue; or manually verify worker and emit missing claim op. |
| **claimed** | claim-active | Valid | Worker holds active claim; TTL still valid. Normal operating state. | — | Task is in-flight. Coordinator monitors heartbeats; if no heartbeat appears within expected interval (e.g., 10 min for typical tasks), escalate to `in-progress` check or issue advisory note. |
| **claimed** | claim-expired | **Invalid** (D2 — Orphaned Claim) | Claim has expired; no heartbeat from claiming worker within TTL. Worker has abandoned the issue (crashed, session lost, or voluntarily unclaimed without transition). | Emit `transition` op: `{"to": "open", "outcome": "Claim expired without worker transition; re-opening"}`. This resets the task to `open` for re-claim. Record a `note` with timestamp and original claimer ID for auditability. | **Re-dispatch protocol:** Run `arm claim ISSUE_ID` to seize the claim (or reset to `open` first if `arm claim` refuses `claimed` status). Then dispatch a fresh worker with updated render-context. Do NOT skip to the next task; re-dispatch is mandatory to unblock downstream blockers. |
| **in-progress** | unclaimed | **Invalid** | Status is `in-progress` but no `claim` op. Materialization or op corruption. | Backfill: emit a `claim` op with recent timestamp. | Assume worker is actively working; pull latest ops and monitor for heartbeat within SLA. If none appears, escalate to `claimed` + `claim-expired` case. |
| **in-progress** | claim-active | Valid | Worker holds active claim; at least one heartbeat exists from the claiming worker within TTL. Task actively in-flight. | — | Coordinator continues to monitor. Expect `transition` op (to `done` or `blocked`) within the task's estimated complexity window. If no transition appears after 2× estimated window, escalate to advisory or `in-progress` + `claim-expired` case. |
| **in-progress** | claim-expired | **Invalid** (D2 — Orphaned Claim + Starvation) | Status is `in-progress` but claim has expired. Worker was actively working but has gone silent; no heartbeat for more than TTL minutes. Worker crash or session loss. | Emit `transition` op: `{"to": "blocked", "outcome": "Claim expired mid-work; worker unreachable. Manual investigation required."}` OR reset to `open` if issue is low-priority and can be re-worked. Record full context (worker ID, last heartbeat timestamp, task scope) in `note` op for incident investigation. | **Critical:** This is the **redispatch starvation** scenario. Coordinator must detect and remediate. Check `git log --all --grep=ISSUE_ID` to see if any implementation commit exists despite expired claim. If commit exists, transition to `done` and advance merge detection. If no commit exists: either transition to `blocked` and investigate (human-supervised recovery), or reset to `open` and re-dispatch. Do NOT leave `in-progress` + `claim-expired` unattended. |
| **done** | unclaimed | **Invalid** | Status is `done` (worker transitioned to done) but no `claim` op. Materialization or op corruption; the `transition` op exists but parent `claim` op is missing. | Backfill: emit a `claim` op dated before the `transition` op, with the same worker ID as the `transition` op. | Proceed to merge detection. Check if code is on main; if yes, auto-promote to `merged`. |
| **done** | claim-active | Valid (but unusual) | Worker transitioned to `done` with claim still nominally active (TTL not yet expired). Normal: task is done, PR is under review, claim still "active" until reviewer closes. | — | Monitor PR status. Once PR merges, CLI auto-detects and promotes to `merged`. If PR is rejected or blocked, Coordinator may re-open with `arm reopen` if more work is needed. |
| **done** | claim-expired | Valid (expected) | Worker transitioned to `done` and claim has since expired. Normal progression: claim TTL was per-task; after transition, claim is no longer relevant. No action needed. | — | Proceed to merge detection. Check `git log main --grep=ISSUE_ID` for proof of merge. If merge detected, auto-promote to `merged`. If no merge detected after configurable threshold (default: 3 days), raise advisory: "Task done 3 days ago but PR not merged; check review status." |
| **merged** | unclaimed | Valid | Status is `merged`; no claim ops. Task is complete and closed. | — | Task is complete. Unblock any `blocked_by` dependent issues. Proceed to next ready tasks. |
| **merged** | claim-active | Valid (unusual but harmless) | Status is `merged`; claim is nominally active but irrelevant (task is terminal). | — | Ignore claim liveness; task is terminal. |
| **merged** | claim-expired | Valid (expected) | Status is `merged`; claim expired long ago. Normal terminal state. | — | Ignore; task is complete. |
| **blocked** | unclaimed | Valid (but incomplete) | Status is `blocked` (worker transitioned to blocked) but no prior `claim` op. Unusual: suggests worker was never claimed or ops are corrupted. | Backfill: emit a `claim` op if the `transition` op has a worker ID. This improves auditability but does not change the recovery action. | Operator or human must resolve the blocker (external dependency, decision required, etc.). Do NOT re-dispatch until blocker is cleared. `arm doctor` should flag `blocked` issues and surface them in a report. |
| **blocked** | claim-active | Valid | Worker transitioned to `blocked` with claim still nominally active. Claim is no longer meaningful (work is blocked), but timing is coherent. | — | Resolve external blocker. Once blocker is cleared, reset to `open` and re-dispatch with `arm reopen`. |
| **blocked** | claim-expired | Valid | Worker transitioned to `blocked` and claim has expired. Normal: blocker remains unresolved; claim is stale. No action needed. | — | Resolve external blocker. Once blocker is cleared, reset to `open` and re-dispatch. |
| **cancelled** | unclaimed | Valid | Status is `cancelled`; no claim ops. Task is closed. | — | Task is cancelled; do not attempt to re-claim. |
| **cancelled** | claim-active | Valid (harmless) | Status is `cancelled`; claim nominally active but irrelevant. | — | Ignore; task is terminal. |
| **cancelled** | claim-expired | Valid (expected) | Status is `cancelled`; claim expired. Normal terminal state. | — | Ignore; task is complete. |

---

## Failure Scenario Recovery Procedures

### Scenario 1: D1 — Branch Divergence

**Symptom:** `arm doctor` emits "Git commits reference issues not in done/merged state."

**Root Cause:** A commit on a feature branch (or main) references an issue ID in the message but that issue's status is `open`, `claimed`, or `in-progress` — not yet promoted to `done`/`merged`.

**Recovery Steps:**

1. Identify the offending commit: `git log --all --grep=ISSUE_ID`
2. Determine the issue status: `arm show ISSUE_ID`
3. **If issue is actually done/merged but not yet detected:** Run `arm merged ISSUE_ID` to manually trigger merge detection, then re-run `arm doctor`.
4. **If issue is in `in-progress`/`claimed`:** Either (a) the commit references the wrong issue, or (b) the worker forgot to transition. Contact the worker or coordinator to resolve:
   - Run `arm transition ISSUE_ID --to done --outcome "..."` to complete the work.
   - Verify the commit message includes the issue ID for proper detection.
5. **If issue is in `open`:** The commit should not reference an issue that was never claimed. Verify commit message; rebase if needed to remove stale issue reference, or create a new issue for the actual work.

**Prevention:**

- Use `prepare-commit-msg` hook to stamp issue ID in commits automatically.
- `arm doctor D1` should trigger before any merge to protected main.
- Coordinator should verify `arm doctor` passes before opening PRs.

---

### Scenario 2: D2 — Orphaned Claim (Task is `claimed`/`in-progress` with Expired TTL)

**Symptom:** `arm doctor` emits "Stale claims — claimed issues with expired TTL."

**Root Cause:** A worker claimed an issue but never emitted a heartbeat or transition within the TTL window. The claim has expired without release, leaving the issue in limbo.

**Root Cause Sub-cases:**

- **Worker crashed or lost session** → Worktree lost, claim never transitioned, TTL expired.
- **Worker unclaimed without transition** → Rare, indicates coordination error (worker should always transition).
- **Worker is stuck** → Working on the task but not emitting heartbeats; TTL will expire.

**Recovery Steps:**

1. **Identify the stale claim:** `arm list --status claimed` or `arm list --status in-progress | grep "TTL expired"`.
2. **Attempt to contact the worker:**
   - Check if the worker is still active: `arm list --worker WORKER_ID`.
   - Review recent `note` ops for the issue to see if the worker left context.
3. **If worker is reachable:**
   - Ask worker to emit a heartbeat: `arm heartbeat ISSUE_ID`.
   - Ask worker to complete or transition the task: `arm transition ISSUE_ID --to done/blocked --outcome "..."`.
4. **If worker is unreachable or non-responsive (TTL already expired):**
   - **Check for partial work:** `git log --all --grep=ISSUE_ID` to see if the worker made an implementation commit.
     - **If commit exists:** Coordinator should transition to `done` and run merge detection: `arm transition ISSUE_ID --to done --outcome "Recovered from stale claim; work detected in history." && arm merged ISSUE_ID`.
     - **If no commit exists:** Reset the task: `arm transition ISSUE_ID --to open --outcome "Stale claim expired; re-opening for re-dispatch."`.
5. **Re-dispatch (if needed):** If task was reset to `open`, re-issue it to the ready queue. A fresh worker will claim and execute.

**Prevention:**

- Workers MUST emit heartbeats at regular intervals (every 5–10 minutes for long-running tasks).
- Coordinator MUST monitor for expired claims: `arm doctor` should surface D2 before tasks become "stuck" (e.g., if TTL expires, emit advisory within 10 min).
- For ephemeral workers (CI agents, session-limited harnesses), use shorter TTLs (e.g., `ttl: 15`) so expiration is detected quickly.

---

### Scenario 3: Redispatch Starvation (Coordinator Skips Re-claim After TTL Expiration)

**Symptom:** Task remains in `in-progress` or `claimed` state for hours/days after TTL expiration. No new worker is dispatched. Issue is silently stuck, blocking downstream work.

**Root Cause:** Coordinator detects task is in progress, but does not notice claim has expired. Coordinator assumes worker is still working and waits. Meanwhile, worker's session was killed, so no heartbeat ever arrives. Task never transitions and is never re-dispatched.

**Example from Dogfood:**

> Coordinator recovered after session loss, found task `ARCHIMP-S16-T3` in `done` state with no implementation commit. The task's worktree no longer existed. Coordinator chose to implement directly instead of re-dispatching a worker. Result: workflow goal (haiku worker dispatch) was bypassed, and the armature-worker skill's pre-flight checks did not run.

**Recovery Steps:**

1. **Detect redispatch starvation:**
   - Coordinator maintains a list of in-flight tasks after dispatch.
   - For each task, check: (a) status is `claimed` or `in-progress`, and (b) claim TTL has expired.
   - If both, the task is starving: `arm list --status claimed --status in-progress | grep "TTL expired"`.
2. **Investigate the issue:**
   - Run `arm show ISSUE_ID --full` to see the claim timestamp and last heartbeat.
   - Check `git log --all --grep=ISSUE_ID` for implementation commits.
   - Check worktree status: `git worktree list` to see if the worker's worktree still exists.
3. **Remediate:**
   - **If code commit exists:** Transition to `done` and run merge detection. Do not re-dispatch; work is recoverable.
   - **If no code commit:** Reset the task with `arm transition ISSUE_ID --to open --outcome "Stale claim expired; work never committed. Re-dispatching."`.
   - **Then:** Re-dispatch a fresh worker with the most recent render-context: `arm render-context ISSUE_ID` and send to a new worker via the armature-worker skill.
4. **Critical:** Do NOT bypass the worker skill just because the task is "small" or the fix "obvious". The armature-worker skill runs essential pre-flight checks (`make check`, `arm validate`, `arm doctor`). Direct coordinator implementation bypasses these safeguards.

**Prevention:**

- Coordinator MUST implement a "heartbeat monitor" loop:
  - After dispatching workers, periodically poll `arm list --status in-progress` for expiring claims.
  - Upon detecting expiration, escalate to re-dispatch (not direct implementation).
- Coordinator skill MUST document the re-dispatch protocol clearly (see armature-coordinator skill, "Worker Recovery" section).
- `arm doctor` should emit urgent errors (not just warnings) for tasks starving in `in-progress` or `claimed` with expired TTL.

---

## Implementation Notes for `arm doctor` and Coordinator

### `arm doctor` Enhancements

**D1 (Branch Divergence)** — Already implemented. No change needed.

**D2 (Stale Claims)** — Already implemented. No change needed. Enhanced with:
- Surface specific issue IDs and staleness duration (e.g., "ISSUE-ID: claim expired 2 hours ago").
- Emit `--strict` error (not just warning) if staleness > 12 hours.

**D3–D7** — Existing checks (orphaned ops, broken refs, cycles, uncited issues). No change needed for this document.

**New: D8 (Redispatch Starvation Monitor)** — Optional, future enhancement:
- Detect tasks in `in-progress`/`claimed` with expired claims that have been starving for >30 minutes.
- Emit urgent error: "Task ISSUE-ID is starving: claim expired without re-dispatch."
- Do NOT merge/commit main until resolved.

### Coordinator Skill Enhancements

**Worker Recovery Protocol** — Add section:

> **Recovery Case: Stale Claim with No Implementation Commit**
> 
> If a task is `in-progress` or `claimed` with an expired claim, check:
> ```bash
> git log --all --oneline --grep=ISSUE_ID | head -5
> ```
> - **If commit exists:** Run `arm transition ISSUE_ID --to done --outcome "..."` and `arm merged ISSUE_ID`.
> - **If no commit exists:** Run `arm transition ISSUE_ID --to open --outcome "Claim expired; re-opening."` and re-dispatch with `arm render-context ISSUE_ID` sent to a fresh worker.
> 
> **DO NOT skip re-dispatch even if the change appears trivial.** The worker skill enforces pre-flight checks that coordination cannot replicate.

---

## Claim Compensation Race Prevention (LNGHZN-S5-T9)

`arm claim`'s worktree provisioning can fail partway through after the `claim`
op has already been appended (e.g. a filesystem error, a branch conflict). On
failure, the command appends a compensating `transition` op ("rollback") and,
for a fresh worktree, force-removes the partially provisioned directory. Two
races around this recovery path were closed here:

**1. Replay-order-independent compensating ops (`claim_token` /
`if_claim_token`).** Ops are append-only and replay is last-write-wins, so if
a second worker's claim legitimately takes over while the first worker's
provisioning is still failing, an unconditional compensating op could land
*after* the second worker's claim in the log and erase it on replay. Every
claim op now carries a unique per-claim nonce, `claim_token` (16 random bytes,
hex-encoded; `ops.Payload.ClaimToken`, materialized as `Issue.ClaimToken`).
Rollback's compensating `transition` op stamps `if_claim_token` with the exact
token of the claim it is compensating for
(`ops.Payload.IfClaimToken`). `materialize.applyTransition` treats a non-empty
`if_claim_token` as a condition, not an instruction: it applies the op only if
the issue's current `claim_token` still matches, `claimed_by` still matches
the op's `WorkerID`, and the issue's status is not terminal (`done`, `merged`,
`cancelled`) — otherwise it is a deterministic no-op. This makes the
compensating op safe regardless of where it lands in the log or what order it
replays relative to a superseding claim: correctness is a property of replay,
not of append-time ordering. `claim_token` is also why this closes the
`ClaimedAt`-uniqueness gap: `ClaimedAt` has only 1-second (epoch) resolution,
so two claims by the same worker on the same issue within the same second were
previously indistinguishable by `ClaimedBy + ClaimedAt` alone; `claim_token`
gives every claim a distinct identity regardless of timing.

Legacy ops with no `claim_token` / `if_claim_token` are unaffected: an absent
`if_claim_token` (empty string, the default) makes a transition apply
unconditionally exactly as before — this is purely additive and backward
compatible.

**2. Per-clone claim lock (destructive-filesystem race).** Between the
ownership recheck and the destructive filesystem action (`git worktree
remove --force`, or moving an adopted worktree back), a second worker could
land a legitimate claim and start using the worktree at the canonical path —
letting the first worker's cleanup discard the second worker's uncommitted
work. This race is inherently same-clone-only (a remote claimant provisions
into an entirely separate filesystem), so `arm claim` now takes an OS-level
advisory lock (`flock`, non-blocking) scoped to the issue, in a lock file
under the main repo's git common dir (`armature-claim-<issue-id>.lock`),
before appending the claim op and holding it through the end of the command
(including rollback and cleanup). A concurrent `arm claim` for the same issue
in the same clone fails fast with a clear error instead of racing. See
`cmd/armature/claim_lock.go`.

---

## References

- **D1 Dogfood Finding:** `2026-06-27T1954Z-5207ee28-coordination-session-recovery-branch-divergence.md`
- **D2 Dogfood Finding:** `2026-06-28T2200Z-claude-workflow-worker-left-task-claimed.md`
- **Redispatch Starvation Dogfood Finding:** `2026-06-27T2030Z-5207ee28-coordination-recovery-skips-worker-dispatch.md`
- **Architecture:** `docs/design/architecture.md` (sections 3, 4, 5, 10)
- **Workflow:** `docs/agents/workflow.md`
- **Coordinator Skill:** `.claude/skills/armature-coordinator`
- **Worker Skill:** `.claude/skills/armature-worker`
