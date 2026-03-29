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

The plan has a `version`, `title`, and an `issues` array. Each issue must have `id`, `title`, and `type`. Optional fields include `parent`, `priority`, and `blocked_by`.

**Tasks require these three fields or `trls validate` will ERROR:**
- `dod` — definition of done (non-empty string)
- `scope` — files this task touches (non-empty string; use `(new)` suffix for files to be created)
- `acceptance` — JSON array of strings, e.g. `["TestFoo passes", "go build clean"]`

> Note: `--example` omits `acceptance`; add it manually to every task entry in the plan JSON.

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

5. **Register the source document and link every new issue** — all issues must be cited or `trls validate` will ERROR:
   ```
   trls sources add --url PATH --title "TEXT" --type filesystem
   trls sources sync                           # fetch and fingerprint
   trls sources verify                         # confirm all OK, none MISSING
   trls source-link --issue ID --source-id UUID   # repeat for every new issue
   ```
   If no source doc exists, use `accept-citation` per issue instead:
   ```
   trls accept-citation --issue ID --rationale "TEXT" --ci
   ```

6. **Validate to green** — run `trls validate` and resolve all ERRORs and WARNINGs before the plan load is complete. `INFO: phantom scope` lines for not-yet-created files may be ignored.

   Scope overlap WARNINGs must be resolved by adding a dependency so the two tasks execute serially:
   ```
   trls link --source ISSUE-A --dep ISSUE-B   # A is blocked_by B; eliminates parallel execution
   ```

If you need to undo a plan, use `decompose-revert --plan plan.json`.

## Citation

Every issue must be linked to a source document or have an accepted-risk rationale before the story is done. Cite as you go — don't leave it for a remediation pass.

```
trls sources add --url PATH --title "TEXT" --type filesystem  # register a source document
trls sources sync                                         # fetch and fingerprint all sources
trls sources verify                                       # confirm all sources show OK (not MISSING)
trls source-link --issue ID --source-id UUID             # link issue to a registered source
trls accept-citation --issue ID --rationale TEXT --ci    # accept risk (no source exists)
```

- `--source-id` must exist in the manifest; `source-link` rejects unknown IDs.
- `--rationale` must be ≥ 3 words; use `--ci` to skip interactive confirmation.
- Use `trls validate` to check coverage: `COVERAGE: N/N cited` with no ERROR lines is the goal.

## Repo Health

Run `trls doctor` to check for common repo health issues before pushing or opening a PR.

```
trls doctor           # run all checks; exits non-zero on errors
trls doctor --strict  # promote warnings to errors
trls doctor --format json  # machine-readable output
```

Checks performed:

| Check | Severity | Description |
|---|---|---|
| D1 | warning | Git commits reference issues not in `done`/`merged` state |
| D2 | warning | Claimed issues with expired TTL (stale claims) |
| D3 | error | Op files reference issue IDs not in the graph |
| D4 | error | Issues whose `parent` points to a non-existent ID |
| D5 | error | `blocked_by` chains that form a dependency cycle |
| D6 | warning | Issues without source-link or accept-citation |

`trls doctor` exits zero if there are no errors (warnings are advisory). Use `--strict` to treat warnings as errors.

## Rate Limits

| Operation | Limit |
|---|---|
| Heartbeat | 1 per minute per issue |
| Create | 500 total per repository |
