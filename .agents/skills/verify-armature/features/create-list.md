# Create and list an issue

`arm create` appends a `create` op for a new work item (default type `task`). `arm list` and `arm show` are the user-visible inventory: after create, the issue is `open` with `confidence=draft`. This is the smallest durable write of work, before ready/claim.

## Sub-features

- `--id` for a stable handle; otherwise `{type}-{epoch}`
- `--type` epic|story|feature|task|bug (default task)
- Task E6 fields required at write time: `--scope`, `--acceptance` (JSON array), `--dod`
- Optional `--source <uuid-or-url>` to source-link in the same write
- `arm list` filters: `--status`, `--parent`, `--type`; `--group` is human-only
- `arm show <id>` summary; `--field a,b` prints those fields; structured body needs `--format json`

## How to get to it (user POV)

Repo is bootstrapped and has a worker id. Then:

```bash
arm --repo "$TARGET" --format agent --non-interactive create \
  --id TASK-VERIFY-CREATE --title "Verification create+list" --type task \
  --scope "verify-create.txt" --dod "Issue is listed and showable" \
  --acceptance '[{"type":"test_passes"}]'
arm --repo "$TARGET" --format agent --non-interactive list
arm --repo "$TARGET" --format json --non-interactive show TASK-VERIFY-CREATE
```

Create stdout: `{"id":"TASK-VERIFY-CREATE","status":"created"}`. List stdout: JSON array including that id with `status: open`.

## Driving it with arm-verify.sh

Preconditions: launch + doctor passed. Helper will bootstrap and `worker-init --check` if needed. Target working tree may be empty of issues. Pick a unique `--id` and a scope that does not overlap another open task.

```bash
.agents/skills/verify-armature/scripts/arm-verify.sh drive create-list
```

Proof (helper asserts all of these):

1. create exit 0 and stdout `{"id":"TASK-VERIFY-CREATE","status":"created"}`
2. `arm list` contains `TASK-VERIFY-CREATE`
3. `arm show --format json` has `status=open` and the given title
4. `.armature/ops/<worker-uuid>.log` contains a `create` op for that id (`arm log --json` is the CLI second read)

Evidence: `evidence/<run-id>/drive/01-create/` through `06-ops/`.

## Gotchas

- Missing E6 fields → `cannot introduce Graph Finding ... missing required field` (`GENERAL-1`); nothing is written.
- Two open tasks with the same scope glob → scope-overlap Graph Finding; the second create is refused.
- Uncited create still lists; `arm validate` then errors `uncited node`; product doctor D6 warns; you cannot `dag transition` to verified until `--source` / `sources link`.
- `arm show --format agent` stays human. Use `--format json` for the object.
- `arm list` is a JSON array, not `{count, issues, help[]}`.
- Birth confidence is always `draft`; the issue will **not** appear in `arm ready` until promoted (see ready-claim).
