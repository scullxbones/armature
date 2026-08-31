# Armature Configuration Reference

The `.armature/config.json` file stores Armature's configuration for your repository. This file is created by `arm bootstrap` and controls core behaviors like task TTL, token budgets, and lifecycle hooks.

## Configuration File Location

The `.armature/config.json` file is stored on the `_armature` branch and accessed via the `.armature/` ops worktree at `.armature/config.json`.

## Configuration Fields

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `project_type` | string | auto-detected | Project type, auto-detected from repo markers. Possible values: `go`, `node`, `python`, `rust`, `make`, `unknown`. |
| `default_ttl` | integer | `60` | Default claim TTL in minutes. `arm claim` uses this when `--ttl` is omitted; an explicit `--ttl` always wins. The chosen value is written onto the claim op and drives claim staleness (see [Interaction with Stale Detection](#interaction-with-stale-detection)). If the field is omitted, the builtin fallback is 60. A present `0` is out of range: `arm doctor` D10 fails. |
| `token_budget` | integer | `1600` | Default token budget for `arm render-context`. Used when `--budget` is omitted; an explicit `--budget` always wins. Truncation approximates 4 characters per token (see [Interaction with Context Assembly](#interaction-with-context-assembly)). If the field is omitted, the builtin fallback is 4000. A present `0` is out of range: `arm doctor` D10 fails. `arm bootstrap` writes 1600. |
| `low_stakes_push_threshold` | integer | `5` | After this many consecutive low-stakes ops (notes, heartbeats, decisions), the pending-push counter resets. The field does not push `_armature` and does not change batch size; only a high-stakes op pushes (committed and a `_armature` push attempted immediately). That class includes `claim`, `transition`, `assign`, `unassign`, `ready` when it claims, and `doctor --fix`; notes, heartbeats, and decisions are not in it. If the field is omitted, the builtin fallback is 5. A present `0` is out of range: `arm doctor` D10 fails. |
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

Hooks run before every `arm transition`. Use them to enforce validation rules or run automated checks.

### Hook Configuration

Each hook in the `hooks` array has this structure:

```json
{
  "name": "string",        // Unique hook identifier
  "command": ["string"]    // argv (e.g., ["sh", "-c", "echo '{\"allowed\":true}'"])
}
```

### Hook Behavior

- **Execution:** Hooks run sequentially in array order on every `arm transition`, before the transition is materialized. They are not filtered by `name` or event type.
- **Input:** JSON on stdin with `issue_id`, `from_status`, `to_status`, and `worker_id`.
- **Output:** JSON on stdout: `{"allowed": true}` or `{"allowed": false, "message": "..."}`. A non-zero exit, invalid JSON, or `allowed: false` rejects the transition and the op is not appended.
- **Working directory:** Hooks inherit the caller's process cwd (typically the code worktree where `arm` was invoked). The runner does not change the command directory to the ops worktree.

### Example: Block transitions unless tests pass

```json
{
  "hooks": [
    {
      "name": "test",
      "command": ["sh", "-c", "make test >/dev/null && echo '{\"allowed\":true}'"]
    }
  ]
}
```

This runs on every `arm transition`, not only `--to done`. A failing `make test` is a non-zero exit and blocks. A passing `make test` still blocks unless stdout is valid JSON with `allowed` true.

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
      "command": ["sh", "-c", "make lint >/dev/null && echo '{\"allowed\":true}'"]
    },
    {
      "name": "test",
      "command": ["sh", "-c", "make test >/dev/null && echo '{\"allowed\":true}'"]
    }
  ]
}
```

## Modifying Configuration

Edit `.armature/config.json` directly with a text editor. Changes take effect immediately on the next `arm` command.

## Interaction with Context Assembly

`arm render-context` takes its token budget from, in order: an explicit `--budget` flag, then `token_budget` in config when that value is greater than zero, then the builtin 4000. `--raw` skips truncation. Command-path fallback for a missing or non-positive value does not make a present `0` valid: `arm doctor` D10 still fails.

When assembled context exceeds `budget * 4` characters, lowest-priority layers are dropped until the bundle fits. At least one layer is always kept (typically the spec). Bootstrap's `token_budget: 1600` is about 6400 characters; if the spec itself is 4000 characters, only a few additional layers will remain.

## Interaction with Stale Detection

`default_ttl` is the default written onto each claim as `claim_ttl` when `arm claim` is run without `--ttl`. Claim staleness uses that recorded TTL:

```
last_activity = max(claimed_at, last_heartbeat, claiming_worker_activity)
if claim_ttl <= 0:
    never stale   # explicit `arm claim --ttl 0`; not the omitted-config fallback
else:
    is_stale = now > last_activity + claim_ttl * 60 seconds
```

An explicit `--ttl` on `arm claim` overrides config for that claim only. `--ttl 0` is accepted and never expires (`IsClaimStale` is false for TTL ≤ 0). That is distinct from omitting `default_ttl` in config, which falls back to 60, and from writing `"default_ttl": 0`, which is D10-invalid. Worker idle/stale classification also uses `default_ttl` when a claim recorded no TTL. See `arm list` for staleness indicators.

## See Also

- [Getting Started](getting-started.md) — Setup workflow
- [Commands Reference](commands.md) — Full `arm` command documentation
- [Architecture](design/architecture.md) — Implementation details
