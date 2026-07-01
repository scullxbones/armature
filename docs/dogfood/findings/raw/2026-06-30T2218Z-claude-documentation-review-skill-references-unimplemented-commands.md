---
date: 2026-06-30
agent: claude
area: documentation
task: DF-S5
tags: [arm-review, command-reference]
---

# armature-reviewer/coordinator skills document arm review subcommands that don't exist

## User Goal

After recording assessments for DF-S5, tried to list/verify them per the
armature-reviewer skill's "Command Reference" section:
`arm review list --story STORY-99` and `arm review show TASK-42`.

## Observed

```
$ arm review list --story DF-S5
Error: unknown flag: --story
$ arm review --help
Available Commands:
  prepare     Prepare a review bundle for an issue
  record      Record a conformance assessment for an issue
```

Only `prepare` and `record` exist under `arm review`. There is no `list` or `show`
subcommand at all, despite both being documented with worked examples in
`internal/skillsembed/skills/armature-reviewer/SKILL.md` ("Returning Results to the
Coordinator" section: `arm review show TASK-42`, and the Command Reference block).

## Impact

Had no way to programmatically enumerate or display recorded assessments for the
story after `arm review record` succeeded — had to fall back to re-reading the raw
assessment JSON files kept on disk from the review dispatch step instead of a
durable, queryable record.

## Evidence

`arm review --help` output shown above; grep for `review show` /
`review list --story` in `internal/skillsembed/skills/armature-reviewer/SKILL.md`
lines ~314-380.

## Suggested Follow-Up

Either implement `arm review show <issue-id>` and `arm review list --story <id>`
(reading recorded AssessmentAttestation ops), or remove/mark-aspirational the
worked examples referencing them in the skill docs so workers don't attempt them.
