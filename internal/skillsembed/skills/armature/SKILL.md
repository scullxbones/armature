---
name: armature
description: >
  Quick reference for arm command syntax — use when working in an
  armature-managed repo to find actionable work, claim issues, record
  progress, and complete work with transition.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Command Reference

## Setup

```
arm worker-init --check || arm worker-init              # register once per clone
arm install-skills                                      # deploy bundled skills to .claude/skills/
```

## Finding and Starting Work

```
arm ready                                               # list actionable issues
arm claim --issue ID [--ttl 3600]                       # claim an issue
arm render-context --issue ID [--budget 4000]           # assemble task context
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
arm list [--parent ID] [--type TYPE] [--status STATUS]  # list issues
arm show ISSUE-ID [ISSUE-ID ...]                        # show issue details
arm decompose-apply --plan plan.json [--dry-run]        # bulk load issues
arm dag-transition --issue ID                           # promote draft issues
arm validate [--ci]                                     # validate citation coverage
arm doctor [--strict]                                   # repo health check
```

## Citation

```
arm sources add --url PATH --title "TEXT" --type filesystem
arm sources sync && arm sources verify
arm source-link --issue ID --source-id UUID
arm accept-citation --issue ID --rationale TEXT --ci
```

## Scope Management

```
arm scope-rename <old-path> <new-path>
arm scope-delete <path>
```
