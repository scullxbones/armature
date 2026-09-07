# Product doctor health check

`arm doctor` is the user-facing D1–D10 health report for a **bootstrapped** repo: git/ops divergence, stale claims, orphaned ops, parent refs, cycles, uncited issues, worker-id mismatches, scope artifacts, unmanaged worktrees, config.json. Exit 0 means no error-severity checks (warnings still print). This is distinct from the verification skill's isolation Doctor.

## Sub-features

- Default report: JSON `{checks:[{check,severity,message,items?,verbose_items?}]}` under `--format agent|json` (`verbose_items` appears only on D3 under `--verbose`)
- `--strict`: warnings become failing
- `--verbose`: extra **D3** context only (adds a `verbose_items` field with `file:line` locations). It does **not** change D6 — D6 already lists uncited issue ids in `items` on every run. The flag's own help text claims otherwise; the output does not.
- `--fix`: append ops to release expired `claimed` → `open` and expire `in-progress` → `blocked`. A third path also releases a claim whose worktree/branch has gone missing (`releaseMissingWorktreeClaim`, `internal/doctor/fix.go:219`), which is not TTL-driven.
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
.agents/skills/verify-armature/scripts/arm-verify.sh drive doctor
```

Raw equivalent:

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive doctor
# second read: same command; checks D1–D10 present
test -f "$TARGET/.armature/config.json"
```

The drive first seeds a deterministic warning by creating an **uncited** issue (`TASK-VERIFY-DOCTOR`), which D6 reports at warning severity. Without it a pristine repo has nothing for `--strict` to promote, so a `--strict` that silently ignored the flag would still exit 0.

Proof: default `doctor` exits 0 and reports `D6` at `warning` severity; the same state under `--strict` exits nonzero (`doctor --strict: N warning(s) promoted to errors`); stdout is a `checks` array containing D1–D10. Evidence: `evidence/<run-id>/drive/00-create-uncited/`, `01-doctor/` and `02-doctor-strict/`.

To prove `--fix --dry-run` skips writes: capture `wc -c` of `.armature/ops/*.log` (or `arm log --json` line count) before and after; sizes must match. Empty plan prints JSON `null`.

To prove the `--verbose` scope, diff the D6 finding with and without the flag — they are byte-identical:

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive doctor           | jq -c '.checks[]|select(.check=="D6")'
"$ARM" --repo "$TARGET" --format agent --non-interactive doctor --verbose | jq -c '.checks[]|select(.check=="D6")'
```

## Gotchas

- Unbootstrapped `--repo` is a Command Failure, not a doctor report. Bootstrap first.
- D6 uncited issues are **warnings** (exit 0) unless `--strict`.
- `--fix` is not read-only. Always try `--fix --dry-run` and observe the ops log before `--fix`.
- Product doctor succeeding does not prove isolation; still require the skill Doctor (`arm-verify.sh doctor`).
- D9 unmanaged `.worktrees/` entries are warnings; `--strict` fails on them. `arm worktree list` reports the same anomaly with exit 0 by design.
