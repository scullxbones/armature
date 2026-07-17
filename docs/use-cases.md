# Armature Use Cases — Persona Workflow Walkthroughs

This document shows how different roles use Armature in practice. Each section follows a realistic workflow for one of the five Armature personas, using actual `arm` commands. If you are unsure which persona fits you, read the short description at the top of each section.

The coordinator loop is the standard execution path: find ready tasks with `arm ready`, claim and render context for each, dispatch worker agents, integrate outcomes, and repeat until the story is done.

---

## P1: Lone Wolf — Solo Freelance Developer

**Who this is:** A single developer working alone on a personal or freelance project. One AI agent, one repo, no branch protection, no team to coordinate with. You want task tracking that stays out of your way.

### Setup

Initialize Armature in your project repository.

```bash
cd my-project
arm bootstrap
# Creates _armature orphan branch and .arm/ ops worktree
# Initializes coordination data
```

Register your requirements document as a source and generate your task plan.

```bash
arm sources add --url docs/requirements.md --type filesystem
arm sources sync

arm dag context --sources all > context.json
# Feed context.json to your AI agent, get back plan.json
arm dag apply --plan plan.json
```

### Daily Workflow

Each morning, check what is ready to work on.

```bash
arm ready
# TASK-001  Write authentication middleware   [ready]
# TASK-002  Add user profile endpoint         [ready]
```

Claim, dispatch, and complete each task.

```bash
arm claim TASK-001 --worktree ./task-001-work
arm render-context TASK-001 --format agent
# dispatch agent with the render-context output
arm transition TASK-001 --to done --outcome "Implemented auth middleware"
```

Check the project overview at any time.

```bash
arm list --group
```

### Notes for Lone Wolf

- As a solo developer, you can push directly to `main` if your repo permits. Tasks tracked by Armature remain independent of your branching strategy.
- Keep the workflow lightweight: `ready` → `claim` → `render-context` → agent → `transition done` → repeat.
- If you need to pause and come back, `arm list --group` shows exactly where everything stands.

---

## P2: Gatekeeper — Enterprise Solo Developer

**Who this is:** A solo developer at a company where `main` is protected behind a pull request review workflow. You merge via PR, not direct push. You need Armature to respect that — downstream tasks must not unblock until code is actually merged, not just marked done.

### Setup

Initialize Armature to set up the ops-branch architecture.

```bash
cd my-project
arm bootstrap
# Creates orphan branch _armature for coordination data
# Creates worktree at .arm/ for simultaneous access
```

The orphan `_armature` branch stores all `.armature/` data. Your `main` branch stays clean. A secondary worktree at `.arm/` lets you read and write coordination state while your code changes live on a feature branch.

### Two-Phase Completion

A task goes through two completion phases:

1. **done** — orchestration completed successfully and task acceptance gates passed.
2. **merged** — Armature auto-detects that the PR landed on `main`.

Downstream tasks only unblock after `merged`. This prevents agents from starting work that depends on code that has not actually shipped yet.

### Daily Workflow

```bash
arm ready                                      # find unblocked tasks
arm claim TASK-ID --worktree ./task-worktree
arm render-context TASK-ID --format agent       # get task spec for agent
# dispatch agent — agent implements, transitions to done
```

Open and merge PRs using your normal team workflow after each task is done.

Dependent tasks unblock at `merged`, not merely `done`.

### Notes for Gatekeeper

- Never use `--to merged` manually. Armature watches for the merge commit and handles promotion automatically.
- The `_armature` orphan branch and `.arm/` worktree are managed by Armature — do not edit them directly.
- `arm list --group` shows which tasks are `done` (PR open) vs `merged` (code landed).

---

## P3: Conductor — Team Lead / Architect

**Who this is:** An architect or team lead running multiple AI agents on a shared repository. Your job is to decompose requirements, set agents in motion, monitor progress, and intervene when things go sideways.

### Setup

Initialize Armature to set up the ops-branch architecture.

```bash
cd team-project
arm bootstrap
```

Register all relevant source documents — PRD, architecture docs, API specs.

```bash
arm sources add --url docs/prd.md --type filesystem
arm sources add --url docs/architecture.md --type filesystem
arm sources sync
```

### Decomposing Requirements

Generate the context bundle for your AI agent to analyze.

```bash
arm dag context --sources all > context.json
```

Feed `context.json` to your AI agent (Claude, Gemini, etc.) and instruct it to produce a `plan.json` describing the task DAG. Then apply the plan.

```bash
arm dag apply --plan plan.json
```

Review the task graph before any agents start working.

```bash
arm dag summary
# Shows tasks, their dependencies, and current status
```

Inspect the context that agents will receive for a specific task to confirm it is accurate.

```bash
arm render-context TASK-023
```

### Monitoring a Running Team

Once orchestrators are running, watch progress.

```bash
arm list --group
# Shows issues bucketed under === in-progress ===, === done ===, === open ===, etc.
```

Check for tasks that have been claimed but have not had a heartbeat in a while (stale tasks).

```bash
arm sources stale-review
```

Validate that no issues are in impossible states (e.g., in-progress without a worker, blocked with all dependencies resolved).

```bash
arm validate
```

If a task is stuck or escalated, inspect its notes and decisions.

```bash
arm show TASK-031
```

### Notes for Conductor

- Run `arm dag summary` after `dag apply` to verify the dependency graph looks correct before unleashing orchestrators.
- Use `arm validate` regularly — it catches data inconsistencies before they cascade.
- `arm sources stale-review` surfaces tasks where a run may have stalled before completion.

---

## P4: Wrangler — Agent Operator / Platform Engineer

**Who this is:** A platform or DevOps engineer responsible for deploying and maintaining the infrastructure that Conductors and AI Workers run on. You set up Armature installations, tune configuration, and fix broken state.

### Initial Setup

Initialize a fresh Armature installation in a repository.

```bash
cd project-repo
arm bootstrap
```

If Armature state becomes corrupted or inconsistent, run `arm doctor` first to diagnose structural issues before taking any destructive action:

```bash
arm doctor
# Shows diagnostics for D1–D6 checks (config, ops logs, state files, hooks, worktree)
```

The ops logs (source of truth for the issue DAG) live on the `_armature` git branch, not in the local `.arm/.armature/` directory. If the worktree checkout is corrupted, you can safely remove it and re-initialize — the history is preserved on the branch:

```bash
rm -rf .arm/.armature    # ops are on _armature branch
arm bootstrap
```

### Configuring Defaults

Armature reads configuration from `.armature/config.json`. Edit this file to set project-level defaults.

```json
{
  "project_type": "go",
  "default_ttl": 60,
  "token_budget": 1600,
  "low_stakes_push_threshold": 5,
  "hooks": [
    {
      "name": "notify_slack",
      "command": ["scripts/notify-slack.sh"],
      "required": true
    },
    {
      "name": "page_oncall",
      "command": ["scripts/page-on-call.sh"],
      "required": true
    }
  ]
}
```

Key settings:

| Setting | What it controls |
|---|---|
| `project_type` | Project language/framework: `"go"`, `"node"`, `"python"`, `"rust"`, `"make"`, or `"unknown"` |
| `default_ttl` | TTL written to claim ops by some coordinator paths (minutes). Note: `arm claim --ttl` defaults to 60 min independently; this setting does not override that CLI default. |
| `token_budget` | Context token budget used by the harness context path. Note: standalone `arm render-context` uses its own `--budget` flag (default: 4000) and does not read this config value. |
| `low_stakes_push_threshold` | After this many low-stakes ops, the counter resets so the next high-stakes op triggers a push. This is a coalescing hint, not an auto-push trigger. |
| `hooks` | Array of pre-transition hooks: name (label only), command array, and required flag |

### Hook Configuration

Armature can fire hooks on state transitions. Configure them in `.armature/config.json` under the `hooks` key as an array of objects, each with a name, command, and required flag.

```json
{
  "hooks": [
    {
      "name": "notify_slack",
      "command": ["scripts/notify-slack.sh"],
      "required": false
    },
    {
      "name": "page_oncall",
      "command": ["scripts/page-on-call.sh"],
      "required": false
    }
  ]
}
```

Each hook is invoked as `command[0] command[1] ... command[n]` on every `arm transition` call — hooks run unconditionally and are not filtered by name or event type. If any hook exits non-zero, the transition is blocked regardless of the `required` field (the `required` field is reserved for future dispatcher-level filtering and has no effect in the current implementation).

### Routine Operations

Check overall system health.

```bash
arm validate
arm list --group
```

Review stale claims and decide whether to release them.

```bash
arm sources stale-review
```

If an agent crashed and left a task in `in-progress`, you can release the claim so another agent can pick it up.

```bash
arm amend TASK-055 --type task   # reset fields if needed
arm transition --issue TASK-055 --to ready \
  --outcome "Releasing stale claim; agent restart detected."
```

### Notes for Wrangler

- If state becomes corrupted, delete `.arm/.armature/` and run `arm bootstrap` to reinitialize. Use `arm doctor` first to diagnose issues.
- Keep `default_ttl` generous enough that slow tasks do not get falsely flagged as stale.
- Hooks with `required: true` will block operations if they fail; use sparingly for critical integrations.

---

## P5: The Swarm — Agent Fleet

**Who this is:** Operators running multiple AI agents in parallel. A coordinator pre-claims tasks from each wave, dispatches agents concurrently, and integrates results before the next wave.

### Pulling Work

Ask for the list of tasks that are ready to be worked on.

```bash
arm ready
# TASK-042  Implement cache invalidation   [ready]
# TASK-043  Write unit tests for auth       [ready]
# TASK-044  Update API documentation        [ready]
```

Pre-claim and render context for each task in the wave, then dispatch agents concurrently:

```bash
arm claim TASK-042 --worktree ./task-042-work && arm render-context TASK-042 --format agent > ctx-042.json
arm claim TASK-043 --worktree ./task-043-work && arm render-context TASK-043 --format agent > ctx-043.json
# dispatch agents with their context packages; agents transition to done when complete
```

### Notes for Agent Fleet

- Pre-claim all tasks in a wave before dispatching agents — this prevents claim races during parallel execution.
- Assign each parallel agent a log slot: `export ARM_LOG_SLOT=<n>` before any `arm` command.
- If a claim race occurs, the losing agent sees the task as `in-progress` and calls `arm ready` for the next available task.
- After each wave, run `arm merged --issue <id>` for completed tasks to unblock the next wave.

---

## Multi-Agent Conflict Resolution

When multiple agents are dispatched concurrently, two can both see the same task as `ready` and race to claim it. Armature handles this safely without locks.

### How It Happens

1. Agent A runs `arm claim TASK-099 --worktree ./task-099-a`; Agent B runs `arm claim TASK-099 --worktree ./task-099-b` at nearly the same time.
2. Both claim ops are appended to their respective log files.
3. On the next materialization cycle, claim race resolution runs — one claim wins, one loses.
4. Both writes are merge-safe (MRDT guarantee).

### Resolution

On the next pull-and-materialize cycle, Armature merges all log files and applies conflict resolution rules:

- **First claim by timestamp wins.** The orchestrator run whose claim operation has the earlier timestamp retains the claim.
- **Tiebreaker:** If timestamps are identical (rare), the agent with the lexicographically smaller worker ID wins.

The losing agent discovers it no longer holds the claim and calls `arm ready` again for a different task.

```bash
# Agent B discovers it lost the claim
arm ready
# TASK-099 is no longer listed as ready (Agent A holds it)
# TASK-100  Add pagination support   [ready]
arm claim TASK-100 --worktree ./task-100-work && arm render-context TASK-100 --format agent
```

### Observing Conflicts as the Conductor

The Conductor can watch conflict events in real time.

```bash
arm list --group
# Shows current claim holders and issue status across the project
```

If a conflict resolution produced an unexpected outcome (e.g., the wrong agent won), the Conductor can intervene by releasing the claim and re-queuing the task.

```bash
arm transition TASK-099 --to ready \
  --outcome "Releasing claim for manual reassignment."
# the task will appear in the next arm ready output for any available agent
```

### Why This Is Safe

Armature uses a Merge-CRDT (MRDT) approach where each agent appends to its own log file. There are no shared mutable files, so concurrent writes never corrupt state. The materialized view is computed deterministically from all logs on every sync. The same inputs always produce the same output, regardless of the order in which logs are received.

---

## Quick Reference by Persona

| Persona | Key Commands |
|---|---|
| P1 Lone Wolf | `arm bootstrap`, `arm ready`, `arm claim`, `arm render-context`, `arm transition` |
| P2 Gatekeeper | same as P1, plus commit-message scan for `merged` promotion |
| P3 Conductor | `arm sources add/sync`, `arm dag context`, `arm dag apply`, `arm dag summary`, `arm validate`, `arm sources stale-review` |
| P4 Wrangler | `arm bootstrap`, config editing, `arm validate`, `arm sources stale-review` |
| P5 Agent Fleet | `arm ready`, `arm claim`, `arm render-context`, `arm merged`, `arm list --group` |
