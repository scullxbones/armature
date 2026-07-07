---
date: 2026-07-07
agent: claude
area: setup
task: TOPTIER planning (commit and push ops branch)
tags: [rename, ops-branch, upstream, git]
---

# Ops branch upstream still tracks origin/_trellis after Trellis→Armature rename

## User Goal

Commit and push the TOPTIER planning ops from the ops worktree (`.arm/`, branch `_armature`) to origin.

## Observed

`git status -sb` in the ops worktree reported `## _armature...origin/_trellis [ahead 4257]`. The local `_armature` branch's upstream was never repointed during the Trellis→Armature rename, so it still tracks the stale `origin/_trellis` ref. A bare `git push` would have targeted the wrong remote branch; the push had to be spelled explicitly as `git push origin _armature:_armature`. Meanwhile arm's own auto-push hook pushed `_armature` correctly, so the CLI and raw git disagree about where the branch goes.

## Impact

- Misleading "ahead 4257" divergence count reduces confidence that local ops state is in sync — it looks like a massive unpushed backlog when the branch is actually current.
- Any human or agent running plain `git push`/`git pull` in the ops worktree interacts with the dead `_trellis` ref, risking divergence between `origin/_trellis` and `origin/_armature`.
- Neither `arm doctor` (all D1–D7 green) nor `arm bootstrap` detected or repaired the stale upstream.

## Evidence

```
$ git -C .arm status -sb
## _armature...origin/_trellis [ahead 4257]
$ git -C .arm push origin _armature:_armature
   8bd57530..0dc26477  _armature -> _armature
```

Manual fix (not yet applied): `git -C .arm branch --set-upstream-to=origin/_armature`. The stale `origin/_trellis` ref presumably also still exists on the remote and should be deleted after verifying nothing tracks it.

## Suggested Follow-Up

Add an upstream-tracking check to `arm doctor` (ops branch upstream must match the configured ops branch name), and have `arm bootstrap`/rename tooling repoint the upstream and offer to prune the old remote ref.
