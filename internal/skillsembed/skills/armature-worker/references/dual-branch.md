# Dual-Branch Mode

## When it applies

If `git config --local armature.mode` returns `dual-branch`, armature automatically
commits ops to a separate `_armature` branch after each command. This means ops do
**not** appear as pending changes in the code worktree.

## What to change in your workflow

**Step 5 — Complete and Commit:** Omit `.armature/` from `git add`:

```bash
arm transition ISSUE-ID --to done --outcome "..."
git add <each file from task scope>
git commit -m "feat(ISSUE-ID): ..."
```

Do **not** run `git add .armature/`. In dual-branch mode, the code branch's
`.armature/` copy is stale — including it stages stale data and will fail the
pre-commit guard.

## Common mistake

| Mistake | Fix |
|---|---|
| Including `.armature/` in `git add` in dual-branch mode | Stages stale data; ops are already on `_armature` branch — omit `.armature/` from code commits |
