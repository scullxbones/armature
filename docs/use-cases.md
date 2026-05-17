# Armature Use Cases — Persona Workflow Walkthroughs

This document shows how different roles use Armature in practice. Each section follows a realistic workflow for one of the five Armature personas, using actual `arm` commands. If you are unsure which persona fits you, read the short description at the top of each section.

Worker runtime mode is now the default execution path: workers run `arm worker run` to pull, claim, orchestrate, and repeat. `arm orchestrate --issue <id>` remains the single-task manual fallback, and manual `claim`/`render-context`/`transition` flows are retained for edge cases and advanced operator control.

---

## P1: Lone Wolf — Solo Freelance Developer

**Who this is:** A single developer working alone on a personal or freelance project. One AI agent, one repo, no branch protection, no team to coordinate with. You want task tracking that stays out of your way.

### Setup

Initialize Armature. Because `main` is not protected, `arm init` picks up single-branch (solo) mode automatically.

```bash
cd my-project
arm init
# Armature detects: no branch protection → solo mode
# Creates .armature/ on main branch
```

Register your requirements document as a source and generate your task plan.

```bash
arm sources add --url docs/requirements.md --type filesystem
arm sources sync

arm decompose-context --sources all > context.json
# Feed context.json to your AI agent, get back plan.json
arm decompose-apply plan.json
```

### Daily Workflow

Each morning, check what is ready to work on.

```bash
arm ready
# TASK-001  Write authentication middleware   [ready]
# TASK-002  Add user profile endpoint         [ready]
```

Run the default runtime loop.

```bash
arm worker run
```

Check the project overview at any time.

```bash
arm list --group
```

### Notes for Lone Wolf

- No branch protection means there is no PR step. Tasks move directly from `done` to complete.
- Keep the workflow lightweight: `ready` → `orchestrate` → repeat.
- If you need to pause and come back, `arm list --group` shows exactly where everything stands.

---

## P2: Gatekeeper — Enterprise Solo Developer

**Who this is:** A solo developer at a company where `main` is protected behind a pull request review workflow. You merge via PR, not direct push. You need Armature to respect that — downstream tasks must not unblock until code is actually merged, not just marked done.

### Setup

Initialize Armature. Branch protection is detected and dual-branch mode is activated automatically.

```bash
cd my-project
arm init
# Armature detects: main is protected → dual-branch mode
# Creates orphan branch _armature for coordination data
# Creates worktree at .arm/ for simultaneous access
```

The orphan `_armature` branch stores all `.armature/` data. Your `main` branch stays clean. A secondary worktree at `.arm/` lets you read and write coordination state while your code changes live on a feature branch.

### Two-Phase Completion

In dual-branch mode, a task goes through two completion phases:

1. **done** — orchestration completed successfully and task acceptance gates passed.
2. **merged** — Armature auto-detects that the PR landed on `main`.

Downstream tasks only unblock after `merged`. This prevents agents from starting work that depends on code that has not actually shipped yet.

### Daily Workflow

```bash
arm worker run --max-tasks 1
```

The orchestrator executes the task lifecycle for you (claim, context assembly,
harness dispatch, verification, retries, transition).

Open and merge PRs using your normal team workflow after orchestration succeeds.

When orchestration succeeds, the task lifecycle is advanced by the orchestrator. In dual-branch workflows, dependent tasks still unblock at `merged`, not merely `done`.

### Notes for Gatekeeper

- Never use `--to merged` manually. Armature watches for the merge commit and handles promotion automatically.
- The `_armature` orphan branch and `.arm/` worktree are managed by Armature — do not edit them directly.
- `arm list --group` shows which tasks are `done` (PR open) vs `merged` (code landed).

---

## P3: Conductor — Team Lead / Architect

**Who this is:** An architect or team lead running multiple AI agents on a shared repository. Your job is to decompose requirements, set agents in motion, monitor progress, and intervene when things go sideways.

### Setup

Initialize in dual-branch mode (typical for team repos with protected `main`).

```bash
cd team-project
arm init
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
arm decompose-context --sources all > context.json
```

Feed `context.json` to your AI agent (Claude, Gemini, etc.) and instruct it to produce a `plan.json` describing the task DAG. Then apply the plan.

```bash
arm decompose-apply plan.json
```

Review the task graph before any agents start working.

```bash
arm dag-summary
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
arm stale-review
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

- Run `arm dag-summary` after `decompose-apply` to verify the dependency graph looks correct before unleashing orchestrators.
- Use `arm validate` regularly — it catches data inconsistencies before they cascade.
- `arm stale-review` surfaces tasks where a run may have stalled before completion.

---

## P4: Wrangler — Agent Operator / Platform Engineer

**Who this is:** A platform or DevOps engineer responsible for deploying and maintaining the infrastructure that Conductors and AI Workers run on. You set up Armature installations, tune configuration, and fix broken state.

### Initial Setup

Initialize a fresh Armature installation in a repository.

```bash
cd project-repo
arm init
```

For repositories that already have a partial or corrupted Armature state, use `--repair` to reconcile without losing existing issue data.

```bash
arm init --repair
```

### Configuring Defaults

Armature reads configuration from `.armature/config.json`. Edit this file to set project-level defaults.

```json
{
  "ttl_seconds": 1800,
  "stale_threshold_seconds": 900,
  "verification_commands": [
    "make check",
    "go vet ./..."
  ]
}
```

Key settings:

| Setting | What it controls |
|---|---|
| `ttl_seconds` | How long a claim is valid before it is considered stale (default 1800 s) |
| `stale_threshold_seconds` | How long without a heartbeat before `stale-review` flags a task |
| `verification_commands` | Commands agents must run before marking a task done |

### Hook Configuration

Armature can fire hooks on state transitions. Configure them in `.armature/config.json` under the `hooks` key.

```json
{
  "hooks": {
    "on_transition": "scripts/notify-slack.sh",
    "on_stale": "scripts/page-on-call.sh"
  }
}
```

### Routine Operations

Check overall system health.

```bash
arm validate
arm list --group
```

Review stale claims and decide whether to release them.

```bash
arm stale-review
```

If an agent crashed and left a task in `in-progress`, you can release the claim so another agent can pick it up.

```bash
arm amend TASK-055 --type task   # reset fields if needed
arm transition --issue TASK-055 --to ready \
  --outcome "Releasing stale claim; agent restart detected."
```

### Notes for Wrangler

- Run `arm init --repair` rather than deleting `.armature/` — it preserves existing task data while fixing structural problems.
- Keep `ttl_seconds` generous enough that slow tasks do not get falsely flagged as stale.
- The `verification_commands` list is enforced before `arm transition --to done` completes.

---

## P5: The Swarm — Orchestrator Fleet

**Who this is:** Operators running multiple orchestrators in parallel. Harnesses (Claude/Codex/Devin) are invoked by `arm orchestrate`; the orchestrator owns verification, retries, and completion.

### Pulling Work

Ask for the list of tasks that are ready to be worked on.

```bash
arm ready
# TASK-042  Implement cache invalidation   [ready]
# TASK-043  Write unit tests for auth       [ready]
# TASK-044  Update API documentation        [ready]
```

Run orchestrate on a ready task:

```bash
arm orchestrate --issue TASK-042
```

The orchestrator claims, prepares context, dispatches the harness, verifies output, retries if needed, and exits with either success or escalation.

### Notes for Orchestrator Fleet

- Run multiple orchestrators in parallel for higher throughput.
- If an orchestrator loses a claim race, it should call `arm ready` and pick the next task.
- Use `--dry-run` to diagnose state and preflight problems before dispatch.
- Manual worker commands are still available for exception handling, but are not required for routine flow.

---

## Multi-Agent Conflict Resolution

When multiple orchestrators run concurrently, two processes can both see the same task as `ready` and race on the same issue. Armature handles this safely without locks.

### How It Happens

1. Orchestrator A runs `arm ready` and sees TASK-099 as ready.
2. Orchestrator B runs `arm ready` at nearly the same time and also sees TASK-099 as ready.
3. Both run `arm orchestrate --issue TASK-099`.
4. Under the hood, each orchestration run performs claim logic; one claim wins, one loses.
5. Both writes are merge-safe (MRDT guarantee).

### Resolution

On the next pull-and-materialize cycle, Armature merges all log files and applies conflict resolution rules:

- **First claim by timestamp wins.** The orchestrator run whose claim operation has the earlier timestamp retains the claim.
- **Tiebreaker:** If timestamps are identical (rare), the agent with the lexicographically smaller worker ID wins.

The losing orchestrator discovers it no longer holds the claim, exits that run, then polls `arm ready` again for a different task.

```bash
# Orchestrator B discovers it lost the claim
arm ready
# TASK-099 is no longer listed as ready (Agent A holds it)
# TASK-100  Add pagination support   [ready]
arm orchestrate --issue TASK-100
```

### Observing Conflicts as the Conductor

The Conductor can watch conflict events in real time.

```bash
arm list --group
# Shows current claim holders and issue status across the project
```

If a conflict resolution produced an unexpected outcome (e.g., the wrong agent won), the Conductor can intervene by releasing the claim and re-queuing the task.

```bash
arm transition --issue TASK-099 --to ready \
  --outcome "Releasing claim for manual reassignment."
arm orchestrate --issue TASK-099   # or let another orchestrator pick it up naturally
```

### Why This Is Safe

Armature uses a Merge-CRDT (MRDT) approach where each agent appends to its own log file. There are no shared mutable files, so concurrent writes never corrupt state. The materialized view is computed deterministically from all logs on every sync. The same inputs always produce the same output, regardless of the order in which logs are received.

---

## Quick Reference by Persona

| Persona | Key Commands |
|---|---|
| P1 Lone Wolf | `arm init`, `arm ready`, `arm orchestrate` |
| P2 Gatekeeper | same as P1, plus dual-branch PR detection for `merged` promotion |
| P3 Conductor | `arm sources add/sync`, `arm decompose-context`, `arm decompose-apply`, `arm dag-summary`, `arm validate`, `arm stale-review` |
| P4 Wrangler | `arm init`, `arm init --repair`, config editing, `arm validate`, `arm stale-review` |
| P5 Orchestrator Fleet | `arm ready`, `arm orchestrate`, `arm list --group`, `arm validate` |
