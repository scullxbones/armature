---
name: armature-worker
description: >
  Use when starting work in an armature-managed repository — picks up ready
  issues, claims them, assembles context, and drives implementation. Enforces
  per-task commits and story-level push/PR strategy.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Worker

A worker receives a pre-claimed task from the Coordinator, implements it, records
progress, and transitions the task to `done`.

## Prerequisites

If `arm` is not found, stop and resolve this before proceeding.

Run `worker-init` once per machine/clone — the worker ID persists in local git config
across sessions:

```
arm worker-init --check || arm worker-init
```

`--check` is a no-op if the ID is already set. Re-running `worker-init` without
`--check` generates a new UUID, which is almost never what you want.

> Workers receive task context from the Coordinator at dispatch time.
> For finding work, claiming issues, dispatching workers, and story-level PR:
> see the **armature-coordinator** skill.

## Step-by-Step

### 1. Initialize
```
arm worker-init --check || arm worker-init
arm doctor
```

Run `arm doctor` to verify repo health (no broken parent refs, no orphaned ops,
no dependency cycles). Fix any errors before starting work.

### 2. Receive Task Context

The Coordinator dispatches you with a pre-claimed issue and the full output of
`arm render-context`. That output is your complete task specification — it
contains the issue description, definition of done, blocker outcomes, parent chain,
decisions, and notes.

**Do not open plan files. Do not read docs/superpowers/plans/. The render-context
output is sufficient.**

The issue is already claimed. Do NOT run `arm claim`. Do NOT run `arm worker-init`
again.

### 3. Record Progress

While implementing, record progress and decisions:

```
arm note ISSUE-ID --msg "..."
arm decision ISSUE-ID --topic X --choice Y --rationale Z
```

**Call `arm heartbeat ISSUE-ID` for any work taking more than a few minutes —
maximum once per minute.** Claims expire after the TTL; without periodic heartbeats
another worker may steal the claim. Issue heartbeat calls at natural checkpoints
(e.g. after each test run, after each file written).

### 4. Cite Every Issue Touched

Before completing work, cite every issue you touched or created:

```
arm source-link --issue ISSUE-ID --source SOURCE-UUID   # if a source doc exists
# or
arm accept-citation --issue ISSUE-ID --ci               # if no source exists
```

Do not leave issues uncited.

### 5. Complete and Commit

```
arm transition ISSUE-ID --to done --outcome "what was accomplished"
git add <each file from the task scope> .armature/
git commit -m "feat(ISSUE-ID): brief description of what was implemented"
```

Stage files **explicitly by name or path** — taken directly from the task's `scope` field.
Do **not** use `git commit -am`: the `-a` flag only auto-stages already-tracked files and
silently skips new files and directories created by the task.

Record a concrete outcome. Commit immediately after the task — small focused commits
are easier to review.

**Always stage `.armature/` alongside code files.** Every `arm` command (note,
decision, heartbeat, transition) writes ops to `.armature/`. If you omit `.armature/`
from the commit, those ops are left behind and will not be delivered with the code.

If using dual-branch mode, see `references/dual-branch.md` before committing.

**Commit message format:** `<type>(<ISSUE-ID>): <description>`
Types: `feat`, `fix`, `refactor`, `test`, `docs`

**Branch discipline:** `arm transition --to done` will fail if you are on the
main or master branch (unless you use `--force`). The `--force` flag should only
be used in exceptional cases (e.g., emergency hotfixes to main).

## Valid Transition Targets

| Target | When |
|---|---|
| `done` | Work complete |
| `blocked` | Cannot proceed, external dependency |
| `cancelled` | Work abandoned |

**Valid status values use hyphens:** `in-progress`, `done`, `cancelled`, `blocked`. Underscores are rejected.

## Setting Your Log Slot

When the Coordinator dispatches you as part of a parallel wave, it will assign you
a log slot. Set it before running any `arm` command:

```
export ARM_LOG_SLOT=<assigned-slot>
```

This ensures your ops go to a slot-specific log file and do not race with other
parallel workers. The Coordinator assigns slots — workers set the slot they are
given but do not assign slots to others.

For tasks spanning 10+ files, see `references/batch-strategy.md`.

## Common Mistakes

| Mistake | Fix |
|---|---|
| `arm: command not found` | Run `make install`, ensure `~/.local/bin` is on PATH |
| Reading plan files for task instructions | Use `render-context` output only |
| Using `in_progress` (underscore) | Use `in-progress` (hyphen) |
| Skipping `worker-init` on a fresh clone | Required once per clone — ops without worker ID will fail |
| Running `worker-init` every session | Generates a new UUID each time, creating phantom workers; use `--check` to verify instead |
| Running `arm claim` when dispatched by Coordinator | The Coordinator pre-claims the issue; do not re-claim |
| Skipping heartbeat on long tasks | Claim expires after TTL; other workers can steal it |
| Skipping commit after task | Small commits make review and revert tractable |
| Using `git commit -am` | `-a` only stages tracked files — new files and directories are silently skipped; always use explicit `git add <scope files>` |
| Omitting `.armature/` from `git add` | Ops left behind, not delivered with code; always include `.armature/` in every commit (see `references/dual-branch.md` for dual-branch mode exception) |
| Leave issues uncited | Run `arm source-link` or `arm accept-citation --ci` before returning |
| Repeating `transition` then `commit` manually | Use a bundled command: `arm transition ID ... && git add . .armature/ && git commit -m ...` |
| Transitioning to done while on main | `arm transition --to done` will fail on main/master branch — use feature branch or `--force` only in emergencies |
| Scope overlap WARNING on `arm validate` | Add `arm link --source ISSUE-A --dep ISSUE-B` so overlapping tasks execute serially, not in parallel |
| MISSING entries in `arm sources verify` | Run `arm sources sync` to fetch and fingerprint; re-run `arm sources verify` until all show OK |
