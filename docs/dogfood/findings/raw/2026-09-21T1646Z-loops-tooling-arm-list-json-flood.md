---
date: 2026-09-21
agent: loops
area: tooling
task: MK reshape planner survey
tags: [arm, list, json, agent-tooling]
---

# arm list default JSON floods agent tool capture

## User Goal

Survey epics/stories before writing a decomposition plan.

## Observed

`arm list` / `arm list --group` produced ~100KB single-line (or few-line) JSON that overflowed the agent shell capture path (tool result written to a huge file, truncated at 100k chars on Read).

## Impact

Broke the survey step; required redirect-to-file + python slicing. Coordination agents will keep hitting this.

## Evidence

Shell tool wrote `agent-tools/*.txt` at ~102KB for a routine `arm list --group`. `arm ready --format json` was a manageable 2.8KB.

## Suggested Follow-Up

Default human format for interactive TTY; document `arm list --type story` / `--parent` filters in planner skill; consider `--field` / summary mode for agent surveys.
