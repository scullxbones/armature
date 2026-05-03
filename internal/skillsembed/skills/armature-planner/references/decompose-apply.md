# Decompose-Apply Workflow

Use this for any work involving more than one or two tasks.

## 1. Inspect the Schema

```bash
arm decompose-apply --example
```

This prints a minimal plan JSON. Use it as a starting template but remember to
add `acceptance` to every task — it is omitted from the example output.

## 2. Write plan.json

Create a file (e.g. `plan.json`) following this structure:

```json
{
  "version": 1,
  "title": "Plan Title",
  "issues": [
    {
      "id": "STORY-T1",
      "title": "First task",
      "type": "task",
      "parent": "STORY-ID",
      "priority": "high",
      "blocked_by": [],
      "dod": "what done looks like — concrete and verifiable",
      "scope": "path/to/file.go, path/to/new_file.go (new)",
      "acceptance": ["TestFoo passes", "make check green"]
    },
    {
      "id": "STORY-T2",
      "title": "Second task",
      "type": "task",
      "parent": "STORY-ID",
      "priority": "normal",
      "blocked_by": ["STORY-T1"],
      "dod": "what done looks like",
      "scope": "path/to/other_file.go",
      "acceptance": ["TestBar passes", "make check green"]
    }
  ]
}
```

- `id` values in `blocked_by` must match `id` values in the plan
- `parent` must be an existing issue ID in the repo
- `type` values: `task`, `feature`, `bug`, `story`

## 3. Dry-Run First

```bash
arm decompose-apply --plan plan.json --dry-run
```

This validates the plan and prints what would be created without writing
anything. Fix any errors before proceeding.

Common dry-run errors:
- Missing required fields (`dod`, `scope`, `acceptance`)
- Unknown parent ID
- Duplicate `id` values in the plan
- Malformed `blocked_by` references

## 4. Apply the Plan

```bash
arm decompose-apply --plan plan.json
```

All issues are created in `draft` state.

## 5. Promote from Draft

```bash
arm dag-transition --issue STORY-ID   # promotes the story and all its tasks
```

Verify promotion:
```bash
arm list --parent STORY-ID   # all tasks should show status: open or in-progress
```
