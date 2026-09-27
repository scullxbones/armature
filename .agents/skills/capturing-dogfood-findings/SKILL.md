---
name: Capturing dogfood findings
description: >-
  Use when dogfooding a product, workflow, CLI, agent skill, or integration — to
  capture friction as it happens, log raw findings, or curate recurring themes
  from a findings pile.
---
# Capturing dogfood findings

Side-channel capture: record friction without derailing the current task. File-only — no issues, commands, or implementation work unless asked. The "user" here may be an agent.

## Layout

Default to the current project's local pile:

```text
docs/dogfood/findings/
  raw/
  themes/
```

If the user names another findings root, use that root with equivalent `raw/` and `themes/` directories. Create missing directories as needed. Do not use a global pile by default.

## Capture mode

1. Preserve the current task. Capture, then continue.
2. Choose a stable writer identity: prefer a project-provided one; otherwise use the agent/runtime name (`codex`, `claude`, `agent`, `cursor`, `loops`).
3. Create one raw finding file under `raw/`. Do not modify it after creation except immediate typo or formatting fixes.

Filename:

```text
YYYY-MM-DDTHHMMZ-<writer>-<area>-<short-slug>.md
```

If exact UTC time or writer is unavailable, omit what's unknown:

```text
YYYY-MM-DD-<area>-<short-slug>.md
```

Area vocabulary — use the project's if defined, otherwise:

```
setup · documentation · workflow · integration · automation
validation · tooling · discovery · permissions · performance · other
```

Use `other` with a free-form tag if none fits. Use the raw-finding template below for the body.

## Curate mode

1. Read raw findings without editing them.
2. Group repeated patterns into `themes/<theme>/README.md`.
3. Keep themes as curated briefs with links to supporting raw findings.
4. Do not move raw files into an archive as a curation signal.
5. Do not add status or scoring unless the project already uses it.

Use the theme-readme template below for new theme briefs.

## A useful finding says

- what the agent-user was trying to do
- what happened
- how it changed behavior, confidence, or time spent
- what evidence supports the observation

If capture would derail the task, write a shorter finding with incomplete evidence and continue.

## Template: raw finding

Write new files from this shape:

```markdown
---
date: YYYY-MM-DD
agent: agent
area: other
task:
tags: []
---

# <short finding title>

## User Goal

What the agent-user was trying to accomplish.

## Observed

What happened from the agent-user perspective.

## Impact

How this changed behavior, caused delay, reduced confidence, or exposed product friction.

## Evidence

Commands, files, output snippets, links, or exact prompts.

## Suggested Follow-Up

Optional small follow-up idea. Leave blank if unclear.
```

## Template: theme README

```markdown
# Theme: <name>

## Summary

What pattern keeps showing up?

## Evidence

- `../../raw/<finding-file>.md` - one-line relevance.

## Candidate Follow-Ups

- Follow-up idea.
```
