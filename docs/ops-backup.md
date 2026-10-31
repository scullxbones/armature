# Ops-branch backup (manual stopgap)

**Status:** Operator documentation for today's failure mode (G2.1) and a manual
backup recipe (G2.3). No automated backup. No doctor code in this document.

**Related:** Narrow-gaps addendum G2; doctor check **D11** is reserved for
`TOPTIER-S12-T2` (warn when ops lacks verified remote tracking or local-only
ops go stale). Until D11 lands, treat this page as the only durable statement of
the disaster-recovery gap. See also
[`docs/design/ops-branch-shared-sync-policy.md`](design/ops-branch-shared-sync-policy.md)
(lag / publish policy; not disaster recovery) and doctor **D12** (ops lag behind
`origin/_armature`).

---

## What Armature does not guarantee today

Coordination truth lives on the `_armature` orphan branch (see
[`docs/concepts.md`](concepts.md) §7). Clones see each other's claims, notes,
and transitions only after that branch is published. Armature does **not**:

- Replicate `_armature` to a second remote by itself
- Take periodic snapshots or git bundles of the ops branch
- Detect or recover from a deleted, corrupted, or force-pushed ops tip
- Promise that local `.arm/` / ops worktree history alone is a complete backup

Doctor **D12** warns about lag behind a healthy `origin/_armature`. It does not
cover missing upstream, corruption, or rewritten history. That is the D11
reservation. Until D11 ships, nothing in `arm doctor` names this class of loss.

---

## Failure mode (G2.1)

If the published ops branch is **deleted**, **corrupted**, or **force-pushed**
over on the forge:

1. **New clones** bootstrap against a missing or rewritten tip. Materialize /
   ready / show replay whatever remains. Issues, claims, and notes that lived
   only on the lost tip are gone from the shared graph.
2. **Existing clones** may still hold older commits in their local `_armature`
   / `.arm/` worktree. Those copies are accidental and uneven. They are not a
   coordinated restore story. Two clones can disagree about which tip is true.
3. **Code on `main` is unaffected.** Feature branches and PRs survive. Only
   orchestration state (the DAG, claims, decisions, citations) is at risk.
4. **There is no product recovery command.** Operators recover with ordinary
   git: restore a tip from a clone that still has it, from a bundle, or from a
   second remote. Armature will not invent a tip or merge divergent orphan
   histories for you.

This is a single point of total, silent failure for event-sourced coordination.
Compaction and schema versioning (other backlog items) protect format and
growth; they assume the branch still exists.

---

## Manual stopgap recipe (G2.3)

Use one or both. Neither is automated by Armature.

### A. Periodic git bundle of `_armature`

From a clone that can see the current published tip:

```bash
# Refresh the remote tip with an explicit destination refspec so
# refs/remotes/origin/_armature updates even when remote.origin.fetch
# excludes _armature (same pattern as internal/adapters/git.go for
# single-branch clones).
git fetch origin +refs/heads/_armature:refs/remotes/origin/_armature

# Do NOT update-ref the live refs/heads/_armature (that rewinds unpublished
# ops commits in an Armature checkout). Point a temporary ref at the tip,
# then advertise refs/heads/_armature via a throwaway bare so list-heads
# matches the restore fetch below (bundle create takes rev-list args, not
# push refspecs — a remotes/ tracking name alone will not restore).
git update-ref refs/bundle-backup/_armature refs/remotes/origin/_armature
_tmp=$(mktemp -d /tmp/armature-ops-bundle-XXXXXX)
git init --bare "$_tmp"
git push "$_tmp" refs/bundle-backup/_armature:refs/heads/_armature
git --git-dir="$_tmp" bundle create \
  "/path/to/backups/armature-ops-$(date -u +%Y%m%dT%H%MZ).bundle" \
  refs/heads/_armature
rm -rf "$_tmp"
git update-ref -d refs/bundle-backup/_armature

# Optional integrity check
git bundle verify "/path/to/backups/armature-ops-….bundle"
```

Restore into a clone (or a throwaway remote) when the forge tip is gone or wrong:

```bash
git fetch /path/to/backups/armature-ops-….bundle refs/heads/_armature:refs/remotes/backup/_armature
# Inspect the recovered tip, then publish it under the rules in "After restore"
# (I2: never overwrite a live shared origin/_armature tip).
```

Schedule the bundle however you already schedule backups (cron, CI artifact,
human checklist). Armature does not schedule it.

### B. Second remote

Keep a forge or bare repo that only mirrors `_armature`:

```bash
git remote add ops-backup git@example.com:org/armature-ops-mirror.git
git push ops-backup refs/remotes/origin/_armature:refs/heads/_armature
```

After high-stakes publishes (or on a timer), push again. On disaster, fetch from
`ops-backup` and restore under the same publish rules as a bundle restore.

Protect `origin/_armature` against force-push on the primary forge when the host
allows branch rules separate from PR requirements. Local worktrees may still
hold history for recovery, but that is not a substitute for an explicit second
copy.

---

## After restore

Recovery must stay inside I2's append-only invariant: do **not** overwrite a
live shared `origin/_armature` tip. Publish the recovered tip only as follows:

1. Confirm what origin currently has:
   `git ls-remote origin refs/heads/_armature`.
2. Load the recovered tip (from the bundle or second remote) into a local ref
   you can inspect, e.g. `refs/remotes/backup/_armature`.
3. **Publish path (missing tip or fast-forward only):**
   - If `origin/_armature` is **missing**, push the recovered tip as a fresh
     branch: `git push origin refs/remotes/backup/_armature:refs/heads/_armature`.
   - If `origin/_armature` still exists and the recovered tip is a **fast-forward**
     of that tip, push that fast-forward the same way.
4. **Diverged tip (not a fast-forward):** do **not** push to
   `refs/heads/_armature`. Push the recovered history aside and escalate:
   `git push origin refs/remotes/backup/_armature:refs/heads/_armature-recovered`.
   Stop. Operators decide how to reconcile; Armature will not invent a merge
   across divergent orphan histories for you. There is no automated backup and
   no product command that overwrites the shared tip.
5. In each active clone: `git -C .arm fetch origin _armature` (or your ops
   worktree path) and fast-forward local ops tracking to the published tip once
   origin is healthy again.
6. Run `arm validate --ci` and `arm doctor`. Expect D12-style lag warnings until
   every clone catches up; do not treat green D12 as proof the tip was never
   damaged.

---

## What comes next

- **D11** (`TOPTIER-S12-T2`): doctor warning for missing or stale ops remote
  tracking. That check should link here once live.
- Automated backup, compaction checkpoints, and multi-remote federation remain
  out of scope for this page. Do not read this recipe as a product promise.
