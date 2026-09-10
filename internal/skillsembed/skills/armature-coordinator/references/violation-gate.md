# Violation Gate and Worktree Teardown

`arm merged --issue TASK-ID` `[escape hatch]` reads the task worktree's `armature-hook.log`. Entries:

- `decision:` resolved scope allow/block
- `pass-through:` no binding (warning only; does not block merge)
- `violation:` a write resolved to no binding (enforcement gap)

Manual inspect:

```bash
WT=$(git worktree list --porcelain | awk '/^worktree /{p=$2} /^branch refs\/heads\/task\/TASK-ID$/{print p}')
if [ -n "$WT" ]; then
  GIT_DIR=$(git -C "$WT" rev-parse --git-dir)
  grep "violation:" "$GIT_DIR/armature-hook.log" 2>/dev/null && echo "WARNING: TASK-ID has violations"
fi
```

When `arm merged --issue TASK-ID` `[escape hatch]` runs:

1. Violations and no `--force`: error, worktree kept, status stays `done`. Review, remediate, or override.
2. Violations with `--force`: acknowledged, task `merged`, worktree torn down.
3. Pass-through does not block. A stderr message is enough.

A wave with `violation:` entries must not integrate to main without operator review.

```bash
# [escape hatch]
arm merged --issue TASK-ID --force
```

Use `--force` only after explicit review.

## Worktree cleanup order

Finish `arm review prepare` / `arm review record` for a task before removing its worktree. Prepare reads `<repo>/.git/worktrees/<name>/armature-activity.log`. Record re-reads that path to verify the digest. Teardown first makes activity citations fail as "log missing or unreadable".

```bash
git worktree list
git worktree remove <path> --force
git branch -d <worker-branch>
```
