---
name: armature
description: >
  Quick reference for arm command syntax — use when working in an
  armature-managed repo to find actionable work, claim issues, and manage
  issue state.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Command Reference

## Setup

```
arm bootstrap                                           # initialize repo and deploy bundled skills
arm worker-init --check || arm worker-init              # register once per clone
```

## Finding and Starting Work

```
arm ready                                               # list actionable issues
arm claim ID --worktree --ttl 60                        # claim an issue with worktree
arm render-context ID --budget 4000                      # get task context
```

## During Work

```
arm note --issue ID --msg "..."                         # log progress
arm decision --issue ID --topic "X" --choice "Y" \
              --rationale "Z"                            # record a decision
arm heartbeat --issue ID                                # keep claim alive (max 1/min)
arm transition --issue ID --to STATUS --outcome "..."   # complete or block work
```

Valid `--to` values: `in-progress`, `done`, `cancelled`, `blocked`

## Issue Management

```
arm create --title "X" --type task --parent ID          # create sub-issue
arm list --parent ID --type TYPE --status STATUS        # list issues
arm show ISSUE-ID                                       # show issue details
arm dag apply --plan plan.json --dry-run          # bulk load issues
arm dag transition --issue ID                           # promote draft issues
arm validate --ci                                       # validate citation coverage
arm doctor --strict                                     # repo health check
```

## Citation

```
arm sources add --url PATH --title "TEXT" --type filesystem
arm sources sync && arm sources verify
arm sources link ID --source-id UUID
arm sources accept-citation ID --rationale TEXT --ci
```

## Scope Management

```
arm scope-rename OLD-PATH NEW-PATH
arm scope-delete PATH
```

## Semantic Conformance Review

```
arm review prepare --issue ID --base BASE-SHA --head HEAD-SHA # create review bundle
arm review record --issue ID --assessment assessment.json     # record reviewer output
```

Semantic review validates that delivered work conforms to acceptance criteria and scope. Use the `armature-reviewer` skill with the bundle from `review prepare`; persist results with `review record`.
