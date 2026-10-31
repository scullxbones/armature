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

Doctor **D12** is an error when the ops worktree lags behind a healthy
`origin/_armature` (plain `arm doctor` exits 1; see `docs/commands.md` § D12).
It does not cover missing upstream, corruption, or rewritten history. That is
the D11 reservation. Until D11 ships, nothing in `arm doctor` names this class
of loss.

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

Use one or both. Neither is automated by Armature. Every push, mirror, or
bundle of the published tip is preceded by
`git fetch origin +refs/heads/_armature:refs/remotes/origin/_armature` and
reads that tracking ref — never a short-form fetch / bare `FETCH_HEAD`.

Every bash block below is **fail-closed** for cron/CI: it starts with
`set -euo pipefail`, prints an explicit message on stderr when a refresh or
publish step fails in a way that would otherwise leave a stale tip looking
successful, and exits non-zero so the scheduler notices. Do not strip the
`set -euo pipefail` line when copy-pasting into a job.

### A. Periodic git bundle of `_armature`

From a clone that can see the current published tip:

```bash
set -euo pipefail
# Refresh the remote tip with an explicit destination refspec so
# refs/remotes/origin/_armature updates even when remote.origin.fetch
# excludes _armature (same pattern as internal/adapters/git.go for
# single-branch clones). A failed refresh must abort: an older tracking
# tip would otherwise be bundled and look valid under git bundle verify.
if ! git fetch origin +refs/heads/_armature:refs/remotes/origin/_armature; then
  echo "ops-backup A: refresh of refs/remotes/origin/_armature failed; not bundling (tip may be stale)" >&2
  exit 1
fi
if ! git show-ref --verify --quiet refs/remotes/origin/_armature; then
  echo "ops-backup A: refs/remotes/origin/_armature missing after refresh; not bundling" >&2
  exit 1
fi

# Do NOT update-ref the live refs/heads/_armature (that rewinds unpublished
# ops commits in an Armature checkout). Point a temporary ref at the tip,
# then advertise refs/heads/_armature via a throwaway bare so list-heads
# matches the restore fetch below (bundle create takes rev-list args, not
# push refspecs — a remotes/ tracking name alone will not restore).
git update-ref refs/bundle-backup/_armature refs/remotes/origin/_armature
_tmp=$(mktemp -d /tmp/armature-ops-bundle-XXXXXX)
trap 'rm -rf "$_tmp"; git update-ref -d refs/bundle-backup/_armature 2>/dev/null || true' EXIT
git init --bare "$_tmp"
git push "$_tmp" refs/bundle-backup/_armature:refs/heads/_armature
_bundle="/path/to/backups/armature-ops-$(date -u +%Y%m%dT%H%MZ).bundle"
git --git-dir="$_tmp" bundle create "$_bundle" refs/heads/_armature
# Integrity check is required in the fail-closed path (not optional): a
# zero-byte or truncated file must not look like a successful backup.
git bundle verify "$_bundle"
trap - EXIT
rm -rf "$_tmp"
git update-ref -d refs/bundle-backup/_armature
```

Restore into a clone (or a throwaway remote) when the forge tip is gone or wrong:

```bash
set -euo pipefail
_bundle="/path/to/backups/armature-ops-….bundle"
if ! git fetch "$_bundle" refs/heads/_armature:refs/remotes/backup/_armature; then
  echo "ops-backup restore: failed to load $_bundle into refs/remotes/backup/_armature" >&2
  exit 1
fi
if ! git show-ref --verify --quiet refs/remotes/backup/_armature; then
  echo "ops-backup restore: refs/remotes/backup/_armature missing after fetch; not publishing" >&2
  exit 1
fi
# Inspect the recovered tip, then publish it under the rules in "After restore"
# (I2: never overwrite a live shared origin/_armature tip).
```

Schedule the bundle however you already schedule backups (cron, CI artifact,
human checklist). Armature does not schedule it.

### B. Second remote

Keep a forge or bare repo that only mirrors `_armature`. Every mirror push
refreshes `refs/remotes/origin/_armature` first (same catch-up refspec as recipe
A); otherwise a stale tracking tip is what gets mirrored. A failed refresh must
abort before `git push` — the same stale-tip hazard as recipe A.

One-time setup (add the remote, then refresh + mirror):

```bash
set -euo pipefail
git remote add ops-backup git@example.com:org/armature-ops-mirror.git
if ! git fetch origin +refs/heads/_armature:refs/remotes/origin/_armature; then
  echo "ops-backup B: refresh of refs/remotes/origin/_armature failed; not mirroring (tip may be stale)" >&2
  exit 1
fi
if ! git show-ref --verify --quiet refs/remotes/origin/_armature; then
  echo "ops-backup B: refs/remotes/origin/_armature missing after refresh; not mirroring" >&2
  exit 1
fi
git push ops-backup refs/remotes/origin/_armature:refs/heads/_armature
```

After high-stakes publishes (or on a timer), refresh then push again:

```bash
set -euo pipefail
if ! git fetch origin +refs/heads/_armature:refs/remotes/origin/_armature; then
  echo "ops-backup B: refresh of refs/remotes/origin/_armature failed; not pushing (tip may be stale)" >&2
  exit 1
fi
if ! git show-ref --verify --quiet refs/remotes/origin/_armature; then
  echo "ops-backup B: refs/remotes/origin/_armature missing after refresh; not pushing" >&2
  exit 1
fi
git push ops-backup refs/remotes/origin/_armature:refs/heads/_armature
```

On disaster, fetch from `ops-backup` and restore under the same publish rules as
a bundle restore.

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
   you can inspect, e.g. `refs/remotes/backup/_armature` (use the restore block
   above).
3. **Publish path (missing tip or fast-forward only):**

```bash
set -euo pipefail
# Missing tip: publish recovered history as a fresh shared branch.
if [ -z "$(git ls-remote origin refs/heads/_armature)" ]; then
  git push origin refs/remotes/backup/_armature:refs/heads/_armature
  exit 0
fi
# Live tip present: refresh tracking, then push only when backup is a
# fast-forward of origin/_armature. A failed refresh must not compare or
# push against a stale tracking ref.
if ! git fetch origin +refs/heads/_armature:refs/remotes/origin/_armature; then
  echo "ops-backup publish: refresh of refs/remotes/origin/_armature failed; not publishing" >&2
  exit 1
fi
if git merge-base --is-ancestor \
    refs/remotes/origin/_armature refs/remotes/backup/_armature; then
  git push origin refs/remotes/backup/_armature:refs/heads/_armature
else
  echo "ops-backup publish: recovered tip is not a fast-forward of origin/_armature; use the diverge block (do not overwrite)" >&2
  exit 1
fi
```

4. **Diverged tip (not a fast-forward of `refs/remotes/origin/_armature`):** do
   **not** push to `refs/heads/_armature`. Push the recovered history aside and
   escalate. Exit non-zero after the aside push so cron/CI cannot treat
   "shared tip restored" as success:

```bash
set -euo pipefail
if ! git fetch origin +refs/heads/_armature:refs/remotes/origin/_armature; then
  echo "ops-backup diverge: refresh of refs/remotes/origin/_armature failed; not pushing recovered aside" >&2
  exit 1
fi
if git merge-base --is-ancestor \
    refs/remotes/origin/_armature refs/remotes/backup/_armature; then
  echo "ops-backup diverge: backup is a fast-forward of origin/_armature; use the publish block instead" >&2
  exit 1
fi
git push origin refs/remotes/backup/_armature:refs/heads/_armature-recovered
echo "ops-backup diverge: pushed refs/heads/_armature-recovered; shared origin/_armature untouched — escalate" >&2
exit 1
```

   Stop. Operators decide how to reconcile; Armature will not invent a merge
   across divergent orphan histories for you. There is no automated backup and
   no product command that overwrites the shared tip.
5. In each active clone (including `--single-branch` checkouts whose
   `remote.origin.fetch` excludes `_armature`), catch up:

```bash
set -euo pipefail
# OPS_GIT: ops worktree path (Armature default .arm). Must have _armature
# checked out before the fast-forward below.
OPS_GIT="${OPS_GIT:-.arm}"
# Same pattern as FetchTrackingRefWithoutMovingHEAD in internal/adapters/git.go.
# Short-form `fetch origin _armature` / bare FETCH_HEAD is forbidden: under
# --single-branch it does not refresh refs/remotes/origin/_armature.
if ! git -C "$OPS_GIT" fetch origin +refs/heads/_armature:refs/remotes/origin/_armature; then
  echo "ops-backup catch-up: refresh of refs/remotes/origin/_armature failed; not fast-forwarding local ops" >&2
  exit 1
fi
if ! git -C "$OPS_GIT" show-ref --verify --quiet refs/remotes/origin/_armature; then
  echo "ops-backup catch-up: refs/remotes/origin/_armature missing after refresh; not fast-forwarding" >&2
  exit 1
fi
# Fast-forward local ops against the refreshed tracking ref only.
git -C "$OPS_GIT" merge --ff-only refs/remotes/origin/_armature
```

6. Run `arm validate --ci` and `arm doctor`. Expect D12 lag errors (plain
   `arm doctor` exits 1) until every clone catches up; do not treat green D12 as
   proof the tip was never damaged.

---

## What comes next

- **D11** (`TOPTIER-S12-T2`): doctor warning for missing or stale ops remote
  tracking. That check should link here once live.
- Automated backup, compaction checkpoints, and multi-remote federation remain
  out of scope for this page. Do not read this recipe as a product promise.
