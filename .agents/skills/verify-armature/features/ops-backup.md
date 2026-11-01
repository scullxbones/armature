# Ops-branch backup

`arm ops-backup` is the user-facing copy of the published `_armature` tip: `snapshot` writes a git bundle, `verify` checks that bundle is restorable, and `restore-plan` prints an I2-safe disposition without publishing. It is a disaster-recovery command. It is not doctor D11 (that id is reserved and not live) and it does not start a daemon.

## Sub-features

- `snapshot --output <bundle>` (required): fetch `origin` `refs/heads/_armature` into a per-run ref `refs/remotes/origin/_armature-run/<nonce>` (never `refs/remotes/origin/_armature`), fail closed with `OPS-BACKUP-1` and no bundle if refresh fails, never rewind local `refs/heads/_armature`, write a bundle that advertises `refs/heads/_armature`. The per-run ref is removed before the command returns.
- `verify <bundle>`: confirm the advertisement and that the pack fetches into a throwaway bare. No Armature layout required. Success envelope payload key is `verifications`; `advertises` is `refs/heads/_armature`.
- `restore-plan --from <bundle>` (required): re-verify, load the bundle into `refs/remotes/backup/_armature-run/<nonce>`, print `missing`, `fast-forward`, or `diverge→_armature-recovered`. It does not apply a publish (`--apply` is not a flag). `diverge→_armature-recovered` exits 1 with the plan envelope only. Other failures are `OPS-BACKUP-1`.
- Structured stdout for all three is one envelope `{count, <payload>, help}`. Human mode is one or two status lines.

## How to get to it (user POV)

A git repo with `origin` whose `refs/heads/_armature` exists. Bootstrap is optional; the command resolves `--repo` even when `armature.ops-worktree-path` is unset. From a bootstrapped verify target, add a local bare origin and push `_armature` first (no network):

```bash
git init -q --bare "$TARGET/origin.git"
git -C "$TARGET" remote add origin "$TARGET/origin.git"
git -C "$TARGET" push origin _armature
git -C "$TARGET" update-ref -d refs/remotes/origin/_armature   # push installed this; snapshot must not
arm --repo "$TARGET" --format agent --non-interactive ops-backup snapshot --output "$TARGET/ops-tip.bundle"
arm --repo "$TARGET" --format agent --non-interactive ops-backup verify "$TARGET/ops-tip.bundle"
arm --repo "$TARGET" --format agent --non-interactive ops-backup restore-plan --from "$TARGET/ops-tip.bundle"
```

When the bundle tip equals `origin`'s `_armature`, `plans[0].disposition` is `fast-forward` and the command exits 0.

## Driving it with arm-verify.sh

Preconditions: `launch` succeeded. The drive bootstraps if `.armature/` is missing (that creates local `_armature`), then creates `origin.git` inside the temp repo, pushes `_armature`, and runs the three verbs. The bare origin is inside the temp repo so cleanup removes it.

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh drive ops-backup
```

Proof:

1. `snapshot` exit 0, `snapshots[0].status` is `ok`, `tip` is the origin `_armature` SHA, `bundle` is the `--output` path, and the file exists
2. local `refs/heads/_armature` is the same SHA after snapshot as before
3. `verify` exit 0, `verifications[0].advertises` is `refs/heads/_armature`
4. `restore-plan` exit 0, `plans[0].disposition` is `fast-forward`, `recovered` is that same SHA
5. `git -C "$TARGET" for-each-ref` shows no leftover `refs/remotes/origin/_armature-run` or `refs/remotes/backup/_armature-run`

Evidence: `evidence/<run-id>/drive/04-snapshot/` through `07-leftover-refs/`.

## Gotchas

- No `origin`, or `origin` without `refs/heads/_armature`, is `OPS-BACKUP-1`. Do not point this drive at a repo that cannot fetch.
- Snapshot must not create `refs/remotes/origin/_armature`. Comparing against that tracking ref is the wrong second read; read the bare origin's `refs/heads/_armature`.
- `restore-plan` on a recovered tip that is not a fast-forward of current `origin/_armature` (including a strict ancestor) is `diverge→_armature-recovered`, exit 1, plan envelope on stdout, no `{"error":...}` object.
- `missing` is only when fetch reports the remote branch absent. An unreachable origin stays `OPS-BACKUP-1`.
- Cron may call `snapshot`. Armature does not schedule it.
