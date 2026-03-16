# Trellis Command Reference

`trls` must be on PATH (`make install` to install).

## During Work

```
trls note --issue ID --msg "..."                          # log progress
trls decision --issue ID --topic "X" --choice "Y" \
              --rationale "Z"                             # record a decision
trls heartbeat --issue ID                                 # keep claim alive (max 1/min)
trls transition --issue ID --to STATUS --outcome "..."   # complete or block work
```

**Valid `--to` values:** `in-progress`, `done`, `cancelled`, `blocked`
(Hyphens required — underscores are rejected.)

## Finding and Starting Work

```
trls worker-init                                          # register this agent (once)
trls ready                                               # list actionable issues
trls claim --issue ID [--ttl 3600]                       # claim an issue
trls render-context --issue ID [--budget 4000]           # assemble task context
```

`render-context` output is your complete task specification. Read it before starting work.

## Issue Management

```
trls create --title "X" --type task --parent ID          # create sub-issue
trls decompose-apply --plan plan.json                    # bulk load issues
trls validate [--ci]                                     # validate op log consistency
```

Valid types: `task`, `feature`, `bug`, `story`

## Rate Limits

| Operation | Limit |
|---|---|
| Heartbeat | 1 per minute per issue |
| Create | 500 total per repository |
