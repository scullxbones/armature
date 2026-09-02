# Armature verification feature map

Agent index for driving the `arm` CLI. User-visible paths only. Operating procedure lives in `../SKILL.md`.

## Baseline preconditions

- Source checkout of armature; GNU make + Go available.
- `make build` has produced `<source>/bin/arm`.
- `--repo` is an isolated temp git repo created by `scripts/arm-verify.sh launch` (`/tmp/arm-verify-target.*`), never the source checkout.
- `ARM_LOG_SLOT` unset unless the case is parallel writers on one clone.
- Commands use `--format agent --non-interactive` except `arm show` (use `--format json`) and `arm worker-init` (human `Worker ID:` line).
- Do not pass `bootstrap --global`. Do not start `arm tui` / `arm dag summary` unless the case is the TUI itself.

## Driving conventions

1. Prefer `make test-e2eharness` in the **source** tree for the composed lifecycle (claim race, coordinator recovery, happy-path merge). It already uses temp origins/clones.
2. Prefer `scripts/arm-verify.sh drive <feature>` for a single mapped path against the isolated repo from `launch`.
3. Stable handles: issue ids you pass (`TASK-VERIFY-CREATE`), worker UUID from `git config armature.worker-id`, ops file `.armature/ops/<worker-uuid>.log`, worktree `.worktrees/<issue-id>`.
4. Every drive: capture the mutating command, then a second read (`list` / `show --format json` / `log --json` / git worktree / `.armature` files).

## Proof / skip reporting

When you drive a feature, record under `../evidence/<run-id>/`:

- **proof**: command, exit 0, stdout shape, second-read state, side-effect files
- **skip**: feature name, reason (missing preconditions you could not seed, or TUI-only), what you did instead

Do not report "passed" from helper unit tests or by reading Go source without running `arm`.

## Features

| Feature | File | User path |
| --- | --- | --- |
| Bootstrap a repo | [bootstrap.md](bootstrap.md) | `arm bootstrap` |
| Register worker identity | [worker-init.md](worker-init.md) | `arm worker-init --check` |
| Create and list an issue | [create-list.md](create-list.md) | `arm create` then `arm list` / `arm show` |
| Product doctor health check | [doctor.md](doctor.md) | `arm doctor` |
| Ready queue and claim | [ready-claim.md](ready-claim.md) | `arm ready` then `arm claim --worktree` |
