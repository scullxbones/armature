# Armature Configuration Reference

The `.armature/config.json` file stores Armature's configuration for your repository. This file is created by `arm bootstrap` and controls core behaviors like task TTL, token budgets, and lifecycle hooks.

## Configuration File Location

The `.armature/config.json` file is stored on the `_armature` branch and accessed via the `.armature/` ops worktree at `.armature/config.json`.

## Configuration Fields

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `project_type` | string | auto-detected | Project type, auto-detected from repo markers. Possible values: `go`, `node`, `python`, `rust`, `make`, `unknown`. |
| `default_ttl` | integer | `60` | Default task time-to-live in minutes. Tasks without an explicit `ttl` use this value. After TTL expires, task status is flagged as stale. |
| `token_budget` | integer | `1600` | Token budget for context assembly. `arm render-context` respects this budget (approximately `chars / 4`) when truncating large context layers. |
| `low_stakes_push_threshold` | integer | `5` | Number of ops to accumulate before auto-pushing to the ops branch. Once this threshold is hit, ops are automatically committed and pushed to `_armature`. Lower values = more frequent pushes; higher values = fewer pushes but larger batches. |
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
  "required": boolean      // If true, transition is blocked if hook fails
}
```

### Hook Behavior

- **Execution:** Hooks run sequentially in array order before the transition is materialized.
- **Required hooks:** If a required hook exits with non-zero status, the transition is rejected and the op is not appended.
- **Optional hooks:** If an optional hook fails, a warning is logged but the transition proceeds.
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

The `token_budget` field controls how `arm render-context` truncates large task contexts. When a task's context exceeds the budget:

1. **Fixed layers** (spec, snippets) are never truncated.
2. **Truncatable layers** (blocker outcomes, parent chain, decisions, notes, sibling outcomes) are dropped in priority order.

Example: With `token_budget: 1600` (approximately 6400 characters), if your spec alone is 4000 characters, only essential blocker information will be included.

## Interaction with Stale Detection

Tasks with `ttl` fields use this formula to determine staleness:

```
is_stale = (now - last_status_change_time) > default_ttl * 60 seconds
```

Update `default_ttl` to adjust when tasks are flagged stale. See `arm list` for staleness indicators.

## See Also

- [Getting Started](getting-started.md) — Setup workflow
- [Commands Reference](commands.md) — Full `arm` command documentation
- [Architecture](design/architecture.md) — Implementation details
