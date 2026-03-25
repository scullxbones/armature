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
trls worker-init --check || trls worker-init              # register once per clone (persists in git config)
trls ready                                               # list actionable issues
trls claim --issue ID [--ttl 3600]                       # claim an issue
trls render-context --issue ID [--budget 4000]           # assemble task context
```

`render-context` output is your complete task specification. Read it before starting work.

## Issue Management

```
trls create --title "X" --type task --parent ID          # create sub-issue
trls list [--parent ID] [--type TYPE]                    # list issues with optional filters
trls decompose-apply --plan plan.json                    # bulk load issues
trls validate [--ci]                                     # validate op log consistency
```

Valid types: `task`, `feature`, `bug`, `story`

## Loading a Plan

`decompose-apply` bulk-loads a structured plan JSON into the issue graph, creating all issues as `draft` confidence. Draft issues are hidden from `trls ready` until promoted with `dag-transition`.

### Schema

Use `--example` to see the expected JSON structure:

```
trls decompose-apply --example
```

The plan has a `version`, `title`, and an `issues` array. Each issue must have `id`, `title`, and `type`. Optional fields include `parent`, `priority`, `dod`, `scope`, and `blocked_by`.

### Workflow

1. **Generate plan JSON** — write or generate a plan matching the schema shown by `--example`.
2. **Preview with dry-run** — validate the plan and see what would be created without writing anything:
   ```
   trls decompose-apply --plan plan.json --dry-run
   ```
   Output lists each issue that would be created (`would create: ID (title)`) and a summary count.
3. **Apply the plan** — write the ops:
   ```
   trls decompose-apply --plan plan.json
   ```
   Issues already in the graph are skipped automatically.
4. **Promote from draft** — use `dag-transition` on each root issue to make them visible to workers:
   ```
   trls dag-transition --issue ID
   ```

If you need to undo a plan, use `decompose-revert --plan plan.json`.

## Citation

Every issue must be linked to a source document or have an accepted-risk rationale before the story is done. Cite as you go — don't leave it for a remediation pass.

```
trls sources add --path PATH [--label TEXT]              # register a source document
trls source-link --issue ID --source-id UUID             # link issue to a registered source
trls accept-citation --issue ID --rationale TEXT --ci    # accept risk (no source exists)
```

- `--source-id` must exist in the manifest; `source-link` rejects unknown IDs.
- `--rationale` must be ≥ 3 words; use `--ci` to skip interactive confirmation.
- Use `trls validate` to check coverage: `COVERAGE: N/N cited` with no ERROR lines is the goal.

## Rate Limits

| Operation | Limit |
|---|---|
| Heartbeat | 1 per minute per issue |
| Create | 500 total per repository |
