# Bootstrap a repo

`arm bootstrap` turns a normal git repo into an Armature repo: orphan `_armature` branch, `.armature/` ops worktree, local harness skill deploy (default: `.claude/`), and a worker UUID if none exists. It is the first user command after `git init`. Idempotent: a second run reports already initialized and leaves coordination state in place.

## Sub-features

- Dual-branch layout: code on `main`, ops on `_armature` bound at `.armature/`
- `git -C "$TARGET" config armature.ops-worktree-path` set to the ops worktree
- Local harness artifacts for verified platforms (claude skills + plugin metadata; hook config skipped unless `--with-hooks`)
- Git hooks installed into `$TARGET/.git/hooks/` independently of `--with-hooks`: `pre-commit`, `post-commit`, `post-merge`. `prepare-commit-msg` is obsolete and removed when Armature-managed (`obsoleteHooks` in `cmd/armature/bootstrap.go`). Occupied unmanaged hook paths are skipped (`skipped_hooks`; `installHooks`). `--with-hooks` governs *harness* hook config only, not these.
- Worker identity created only when `armature.worker-id` is missing
- `--platform` restricts deploy; `--global` deploys to `~/.claude/` (do not use in verification)

## How to get to it (user POV)

Start in a **clean** git repo with at least one commit. From the repo root, or with `--repo <path>`:

```bash
arm --repo "$TARGET" --format agent --non-interactive bootstrap
```

Human mode prints `Bootstrap complete.` Agent/json prints the `repo_setup` + `harness_setup` object.

## Driving it with arm-verify.sh

Preconditions: `scripts/arm-verify.sh launch` (and `doctor`) succeeded. Target is an unbootstrapped git repo on `main` with a clean tree. `ARM_LOG_SLOT` unset.

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh drive bootstrap
```

Raw equivalent:

```bash
"$ARM" --repo "$TARGET" --format agent --non-interactive bootstrap
"$ARM" --repo "$TARGET" --format agent --non-interactive bootstrap   # second read: already_initialized
git -C "$TARGET" worktree list
git -C "$TARGET" config --get armature.ops-worktree-path
test -d "$TARGET/.armature/ops"
test -f "$TARGET/.armature/config.json"
test -d "$TARGET/.claude/skills"
test -d "$TARGET/.claude/plugins/armature"
ls "$TARGET/.git/hooks/" | rg -v '\.sample$'   # pre-commit post-commit post-merge; no prepare-commit-msg
```

Proof: first stdout `repo_setup.status` is `initialized` (exit 0); second is `already_initialized`; `git -C "$TARGET" worktree list` shows `$TARGET/.armature` on `_armature`; `git -C "$TARGET" config armature.ops-worktree-path` points at it; ops dir exists; `harness_setup` reports `ok`/`install` for claude `skills` and `plugin_metadata`, and `harness_hook_config` with `status=skipped` and `action=skip` (the drive omits `--with-hooks`), with the matching directories present and non-empty; `.git/hooks/` has the three Armature hooks and no `prepare-commit-msg`. Evidence: `evidence/<run-id>/drive/02-bootstrap/`.

## Gotchas

- Dirty working tree → refused before any mutation.
- Refuses to bootstrap a checkout that is already on `_armature`.
- `--global` writes outside the temp repo; verification must omit it.
- **Every `git commit` in a bootstrapped target repo runs the three installed hooks.** Bootstrap skips occupied unmanaged hook paths. `prepare-commit-msg` is not installed, so it does not rewrite the subject. `post-commit` runs `arm hook run post-commit`, which returns immediately when there is no active claim and does not print `push-ops: push failed`. Put the just-built `bin/` on `PATH` for that commit so the hook invokes this checkout's `arm`. A missing `arm` is `command not found` on stderr while the commit still exits 0.
- `--with-hooks` does **not** gate the git hooks above; it only adds harness hook configuration to the `harness_setup` plan.
- Bootstrap is one of the few commands that works **before** ops-worktree-path exists (`arm doctor` does not).
- Default deploy is local `.claude/`; that directory is inside the temp repo and is deleted with cleanup.
