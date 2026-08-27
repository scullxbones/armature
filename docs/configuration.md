# Armature Configuration Reference

The `.armature/config.json` file stores Armature's configuration for your repository. This file is created by `arm bootstrap` and controls core behaviors like task TTL, token budgets, and lifecycle hooks.

## Configuration File Location

The `.armature/config.json` file is stored on the `_armature` branch and accessed via the `.armature/` ops worktree at `.armature/config.json`.

## Configuration Fields

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `project_type` | string | auto-detected | Project type, auto-detected from repo markers. Possible values: `go`, `node`, `python`, `rust`, `make`, `unknown`. |
| `default_ttl` | integer | `60` | Default claim TTL in minutes. `arm claim` uses this when `--ttl` is omitted; an explicit `--ttl` always wins. The chosen value is written onto the claim op and drives claim staleness (see [Interaction with Stale Detection](#interaction-with-stale-detection)). If unset or 0, the builtin fallback is 60. |
| `token_budget` | integer | `1600` | Default token budget for `arm render-context`. Used when `--budget` is omitted; an explicit `--budget` always wins. Truncation approximates 4 characters per token (see [Interaction with Context Assembly](#interaction-with-context-assembly)). If unset or 0, the builtin fallback is 4000. `arm bootstrap` writes 1600. |
| `low_stakes_push_threshold` | integer | `5` | After this many consecutive low-stakes ops (notes, heartbeats, decisions), the pending-push counter resets so the next high-stakes op (claim, transition, assign) pushes the accumulated batch to `_armature`. Lower values reset sooner (smaller batches); higher values coalesce more writes into each push. If unset or 0, the builtin fallback is 5. |
| `hooks` | array | `[]` | Array of pre-transition hook configurations (see [Hooks](#hooks) below). |

### Project Type Detection

`arm bootstrap` auto-detects `project_type` by looking for marker files in order:

1. `go.mod` → `go`
2. `package.json` → `node`
3. `pyproject.toml` → `python`
4. `Cargo.toml` → `rust`
5. `Makefile` → `make`
6. (none found) → `unknown`

If the detected type is incorrect, manually edit `project_type` in `.armature/config.json`.

## Hooks

Hooks run before task transitions (e.g., when marking a task as `done`). Use them to enforce validation rules or run automated checks.

### Hook Configuration

Each hook in the `hooks` array has this structure:

```json
{
  "name": "string",        // Unique hook identifier
  "command": ["string"],   // Shell command as array (e.g., ["bash", "-c", "make test"])
  "required": boolean
}
```

### Hook Behavior

- **Execution:** Hooks run sequentially in array order on every `arm transition`, before the transition is materialized. They are not filtered by `name` or event type.
- **Failure:** If any hook exits with non-zero status, the transition is rejected and the op is not appended.
- **Environment:** Hooks run in the context of the ops worktree (`.armature/`).

### Example: Require Tests Before Done

```json
{
  "hooks": [
    {
      "name": "test",
      "command": ["bash", "-c", "make test"],
      "required": true
    }
  ]
}
```

This rejects any `transition --to done` unless `make test` passes.

## Complete Example Configuration

```json
{
  "project_type": "go",
  "default_ttl": 60,
  "token_budget": 1600,
  "low_stakes_push_threshold": 5,
  "hooks": [
    {
      "name": "lint",
      "command": ["bash", "-c", "make lint"],
      "required": false
    },
    {
      "name": "test",
      "command": ["bash", "-c", "make test"],
      "required": true
    }
  ]
}
```

## Modifying Configuration

Edit `.armature/config.json` directly with a text editor. Changes take effect immediately on the next `arm` command.

## Interaction with Context Assembly

`arm render-context` takes its token budget from, in order: an explicit `--budget` flag, then `token_budget` in config when that value is greater than zero, then the builtin 4000. `--raw` skips truncation.

When assembled context exceeds `budget * 4` characters, lowest-priority layers are dropped until the bundle fits. At least one layer is always kept (typically the spec). Bootstrap's `token_budget: 1600` is about 6400 characters; if the spec itself is 4000 characters, only a few additional layers will remain.

## Interaction with Stale Detection

`default_ttl` is the default written onto each claim as `claim_ttl` when `arm claim` is run without `--ttl`. Claim staleness uses that recorded TTL:

```
last_activity = max(claimed_at, last_heartbeat, claiming_worker_activity)
is_stale      = now > last_activity + claim_ttl * 60 seconds
```

An explicit `--ttl` on `arm claim` overrides config for that claim only. Worker idle/stale classification also uses `default_ttl` when a claim recorded no TTL. See `arm list` for staleness indicators.

## See Also

- [Getting Started](getting-started.md) — Setup workflow
- [Commands Reference](commands.md) — Full `arm` command documentation
- [Architecture](design/architecture.md) — Implementation details
