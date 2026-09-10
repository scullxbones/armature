---
name: armature
description: >
  Quick reference for arm command syntax — use when working in an
  armature-managed repo to find actionable work, claim issues, and manage
  issue state.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Command Reference

Paved-road commands only. Pipeline: bootstrap, plan, dispatch, work, review, sync.
See `docs/paved-road.md`. Off-road verbs are marked `[escape hatch]` there, not here.

## 1. Bootstrap

```
arm bootstrap
arm worker-init --check || arm worker-init
arm version
```

## 2. Plan / decompose

```
arm sources add --url PATH --title "TEXT" --type filesystem
arm sources sync
arm dag context
arm dag apply --plan plan.json --dry-run
arm dag apply --plan plan.json
arm dag transition --issue ID
arm link --source A --dep B
```

Cite with the plan `source` field at apply time.

## 3. Wave dispatch

```
arm list
arm doctor
arm ready
arm claim ID --worktree --ttl 60
arm render-context ID --budget 4000
arm worktree list
arm show ISSUE-ID
```

## 4. Work

```
arm note --issue ID --msg "..."
arm decision --issue ID --topic "X" --choice "Y" --rationale "Z"
arm validate --ci
arm gate run full
arm transition --issue ID --to STATUS --outcome "..."
```

Valid `--to` values: `in-progress`, `done`, `cancelled`, `blocked`

## 5. Review

```
arm review prepare --issue ID --base BASE-SHA --head HEAD-SHA
arm review record --issue ID --assessment assessment.json
arm review commits ID
```

Use `armature-reviewer` with the prepare bundle; persist with `review record`.

## 6. Sync

```
arm push-ops
arm sync
```
