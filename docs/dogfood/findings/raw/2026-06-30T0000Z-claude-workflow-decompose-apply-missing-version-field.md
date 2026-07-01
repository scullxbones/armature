---
writer: claude
area: workflow
date: 2026-06-30T00:00Z
---

# armature-planner skill's plan JSON example omits the required `version` field

## What the agent was trying to do

Using the `armature-planner` skill to load a 10-task remediation plan
(`docs/superpowers/plans/2026-06-30-dogfood-findings-remediation.md`) into
armature as story `DF-S5` via `arm decompose-apply`.

## What happened

Wrote a plan JSON following the "Complete Well-Formed Task Example" in
`armature-planner/SKILL.md` — which shows only an `"issues": [...]` array, no
top-level `version` or `title` field. Running `arm decompose-apply --plan
plan.json --dry-run` failed immediately:

```
Error: unsupported plan version: 0
```

The skill's own example doesn't mention `version` at all — it starts directly
with `{"id": "STORY-T1", ...}` as if issues were the top-level structure. Had
to run `arm decompose-apply --example` to discover the actual required
top-level shape is `{"version": 1, "title": "...", "issues": [...]}`.

## Why it matters

The skill already tells planners to always run `--example` to inspect the
schema (it's in the Quick Reference), so this was a one-command recovery, not
a dead end. But the "Writing Good Plan JSON" section's "Complete Well-Formed
Task Example" presents itself as the thing to copy-paste, and it silently
omits the wrapper fields that make the file valid at all. A planner who trusts
that example literally (skips `--example` because the docs already show a
"complete" example) hits an opaque `unsupported plan version: 0` error with no
guidance in the skill about what `version` should be or that it's required.

## Evidence

- `arm decompose-apply --plan plan.json --dry-run` → `Error: unsupported plan version: 0`
- `arm decompose-apply --example` output starts with `{"version": 1, "title": "Example Decomposition Plan", "issues": [...]}`
- `.claude/skills/armature-planner/SKILL.md` "Complete Well-Formed Task Example" section shows only a single issue object, not wrapped in `{"version": 1, "title": ..., "issues": [...]}`

## Potential mitigations

- Wrap the "Complete Well-Formed Task Example" in the skill in the actual
  top-level plan shape (`{"version": 1, "title": "...", "issues": [<the task
  object>]}`) so copy-paste produces a valid file.
- Or, if leaving it unwrapped is intentional (to keep the example focused on
  task fields), add one sentence noting the file's top-level shape is
  `{"version": 1, "title", "issues": [...]}` per `arm decompose-apply --example`.
