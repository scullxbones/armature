# Bootstrap a repo

`arm bootstrap` turns a normal git repo into an Armature repo: orphan `_armature` branch, `.armature/` ops worktree, local harness skill deploy (default: `.claude/`), and a worker UUID if none exists. It is the first user command after `git init`. Idempotent: a second run reports already initialized and leaves coordination state in place.

## Sub-features

- Dual-branch layout: code on `main`, ops on `_armature` bound at `.armature/`
- `git config armature.ops-worktree-path` set to the ops worktree
- Local harness artifacts for verified platforms (claude skills + plugin metadata; hook config skipped unless `--with-hooks`)
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
```

Proof: first stdout `repo_setup.status` is `initialized` (exit 0); second is `already_initialized`; `git worktree list` shows `$TARGET/.armature` on `_armature`; ops dir exists. Evidence: `evidence/<run-id>/drive/02-bootstrap/`.

## Gotchas

- Dirty working tree → refused before any mutation.
- Refuses to bootstrap a checkout that is already on `_armature`.
- `--global` writes outside the temp repo; verification must omit it.
- Bootstrap is one of the few commands that works **before** ops-worktree-path exists (`arm doctor` does not).
- Default deploy is local `.claude/`; that directory is inside the temp repo and is deleted with cleanup.
