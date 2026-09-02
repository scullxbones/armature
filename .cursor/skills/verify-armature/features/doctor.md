# Product doctor health check

`arm doctor` is the user-facing D1–D10 health report for a **bootstrapped** repo: git/ops divergence, stale claims, orphaned ops, parent refs, cycles, uncited issues, worker-id mismatches, scope artifacts, unmanaged worktrees, config.json. Exit 0 means no error-severity checks (warnings still print). This is distinct from the verification skill's isolation Doctor.

## Sub-features

- Default report: JSON `{checks:[{check,severity,message,items?}]}` under `--format agent|json`
- `--strict`: warnings become failing
- `--verbose`: extra D3/D6 context
- `--fix`: append ops to release expired `claimed` → `open` and expire `in-progress` → `blocked`
- `--dry-run` (only with `--fix`): print the plan, write no ops

## How to get to it (user POV)

After bootstrap:

```bash
arm --repo "$TARGET" --format agent --non-interactive doctor
```

Human prints `✓ D1: ...` lines. Unbootstrapped repos fail with `GENERAL-1` / `armature.ops-worktree-path must be set`.

## Driving it with arm-verify.sh

Preconditions: launch succeeded; target is bootstrapped (helper bootstraps if needed). This drive is the **product** command, not `arm-verify.sh doctor`.

```bash
.cursor/skills/verify-armature/scripts/arm-verify.sh drive doctor
```

Raw equivalent:

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive doctor
# second read: same command; checks D1–D10 present
test -f "$TARGET/.armature/config.json"
```

Proof: exit 0 on a fresh bootstrapped repo; stdout is a `checks` array containing D1–D10. Evidence: `evidence/<run-id>/drive/01-doctor/`.

To prove `--fix --dry-run` skips writes: capture `wc -c` of `.armature/ops/*.log` (or `arm log --json` line count) before and after; sizes must match. Empty plan prints JSON `null`.

## Gotchas

- Unbootstrapped `--repo` is a Command Failure, not a doctor report. Bootstrap first.
- D6 uncited issues are **warnings** (exit 0) unless `--strict`.
- `--fix` is not read-only. Always try `--fix --dry-run` and observe the ops log before `--fix`.
- Product doctor succeeding does not prove isolation; still require the skill Doctor (`arm-verify.sh doctor`).
- D9 unmanaged `.worktrees/` entries are warnings; `--strict` fails on them. `arm worktree list` reports the same anomaly with exit 0 by design.
