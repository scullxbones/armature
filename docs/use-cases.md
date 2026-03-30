# Trellis Use Cases — Persona Workflow Walkthroughs

This document shows how different roles use Trellis in practice. Each section follows a realistic workflow for one of the five Trellis personas, using actual `trls` commands. If you are unsure which persona fits you, read the short description at the top of each section.

---

## P1: Lone Wolf — Solo Freelance Developer

**Who this is:** A single developer working alone on a personal or freelance project. One AI agent, one repo, no branch protection, no team to coordinate with. You want task tracking that stays out of your way.

### Setup

Initialize Trellis. Because `main` is not protected, `trls init` picks up single-branch (solo) mode automatically.

```bash
cd my-project
trls init
# Trellis detects: no branch protection → solo mode
# Creates .issues/ on main branch
```

Register your requirements document as a source and generate your task plan.

```bash
trls sources add --url docs/requirements.md --type filesystem
trls sources sync

trls decompose-context --sources all > context.json
# Feed context.json to your AI agent, get back plan.json
trls decompose-apply plan.json
```

### Daily Workflow

Each morning, check what is ready to work on.

```bash
trls ready
# TASK-001  Write authentication middleware   [ready]
# TASK-002  Add user profile endpoint         [ready]
```

Pick a task and claim it so the system knows you are working on it.

```bash
trls claim TASK-001
```

Fetch the task-specific context to feed your AI agent. This gives the agent exactly what it needs without flooding it with irrelevant information.

```bash
trls render-context TASK-001
```

Do the work. When done, mark the task complete with a short outcome summary.

```bash
trls transition --issue TASK-001 --to done \
  --outcome "Authentication middleware implemented with JWT support."
```

Check the project overview at any time.

```bash
trls status
```

### Notes for Lone Wolf

- No branch protection means there is no PR step. Tasks move directly from `done` to complete.
- Keep the workflow lightweight: `ready` → `claim` → work → `transition`.
- If you need to pause and come back, `trls status` shows exactly where everything stands.

---

## P2: Gatekeeper — Enterprise Solo Developer

**Who this is:** A solo developer at a company where `main` is protected behind a pull request review workflow. You merge via PR, not direct push. You need Trellis to respect that — downstream tasks must not unblock until code is actually merged, not just marked done.

### Setup

Initialize Trellis. Branch protection is detected and dual-branch mode is activated automatically.

```bash
cd my-project
trls init
# Trellis detects: main is protected → dual-branch mode
# Creates orphan branch _trellis for coordination data
# Creates worktree at .trellis/ for simultaneous access
```

The orphan `_trellis` branch stores all `.issues/` data. Your `main` branch stays clean. A secondary worktree at `.trellis/` lets you read and write coordination state while your code changes live on a feature branch.

### Two-Phase Completion

In dual-branch mode, a task goes through two completion phases:

1. **done** — you self-report that your work is finished and the PR is open.
2. **merged** — Trellis auto-detects that the PR landed on `main`.

Downstream tasks only unblock after `merged`. This prevents agents from starting work that depends on code that has not actually shipped yet.

### Daily Workflow

```bash
trls ready
# TASK-010  Implement rate limiter   [ready]
```

Claim the task and get context.

```bash
trls claim TASK-010
trls render-context TASK-010
```

Write the code on a feature branch and open a PR.

```bash
git checkout -b feat/rate-limiter
# ... implement the feature ...
git push origin feat/rate-limiter
gh pr create --title "Add rate limiter" --body "Implements TASK-010"
```

Once the PR is open and the code is ready for review, mark the task done.

```bash
trls transition --issue TASK-010 --to done \
  --outcome "Rate limiter implemented; PR #47 open for review."
```

When the PR is merged to `main`, Trellis detects the merge automatically and promotes the task to `merged`. Dependent tasks unblock at that point, not before.

### Notes for Gatekeeper

- Never use `--to merged` manually. Trellis watches for the merge commit and handles promotion automatically.
- The `_trellis` orphan branch and `.trellis/` worktree are managed by Trellis — do not edit them directly.
- `trls status` shows which tasks are `done` (PR open) vs `merged` (code landed).

---

## P3: Conductor — Team Lead / Architect

**Who this is:** An architect or team lead running multiple AI agents on a shared repository. Your job is to decompose requirements, set agents in motion, monitor progress, and intervene when things go sideways.

### Setup

Initialize in dual-branch mode (typical for team repos with protected `main`).

```bash
cd team-project
trls init
```

Register all relevant source documents — PRD, architecture docs, API specs.

```bash
trls sources add --url docs/prd.md --type filesystem
trls sources add --url docs/architecture.md --type filesystem
trls sources sync
```

### Decomposing Requirements

Generate the context bundle for your AI agent to analyze.

```bash
trls decompose-context --sources all > context.json
```

Feed `context.json` to your AI agent (Claude, Gemini, etc.) and instruct it to produce a `plan.json` describing the task DAG. Then apply the plan.

```bash
trls decompose-apply plan.json
```

Review the task graph before any agents start working.

```bash
trls dag-summary
# Shows tasks, their dependencies, and current status
```

Inspect the context that agents will receive for a specific task to confirm it is accurate.

```bash
trls render-context TASK-023
```

### Monitoring a Running Team

Once agents are running, watch progress.

```bash
trls status
# Shows: ready, in-progress, done, merged, blocked counts
```

Check for tasks that have been claimed but have not had a heartbeat in a while (stale tasks).

```bash
trls stale-review
```

Validate that no issues are in impossible states (e.g., in-progress without a worker, blocked with all dependencies resolved).

```bash
trls validate
```

If a task is stuck, you can inspect its notes and decisions.

```bash
trls show TASK-031
```

### Notes for Conductor

- Run `trls dag-summary` after `decompose-apply` to verify the dependency graph looks correct before unleashing agents.
- Use `trls validate` regularly — it catches data inconsistencies before they cascade.
- `trls stale-review` surfaces tasks where an agent may have died or disconnected without releasing the claim.

---

## P4: Wrangler — Agent Operator / Platform Engineer

**Who this is:** A platform or DevOps engineer responsible for deploying and maintaining the infrastructure that Conductors and AI Workers run on. You set up Trellis installations, tune configuration, and fix broken state.

### Initial Setup

Initialize a fresh Trellis installation in a repository.

```bash
cd project-repo
trls init
```

For repositories that already have a partial or corrupted Trellis state, use `--repair` to reconcile without losing existing issue data.

```bash
trls init --repair
```

### Configuring Defaults

Trellis reads configuration from `.issues/config.json`. Edit this file to set project-level defaults.

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

Trellis can fire hooks on state transitions. Configure them in `.issues/config.json` under the `hooks` key.

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
trls validate
trls status
```

Review stale claims and decide whether to release them.

```bash
trls stale-review
```

If an agent crashed and left a task in `in-progress`, you can release the claim so another agent can pick it up.

```bash
trls amend TASK-055 --type task   # reset fields if needed
trls transition --issue TASK-055 --to ready \
  --outcome "Releasing stale claim; agent restart detected."
```

### Notes for Wrangler

- Run `trls init --repair` rather than deleting `.issues/` — it preserves existing task data while fixing structural problems.
- Keep `ttl_seconds` generous enough that slow tasks do not get falsely flagged as stale.
- The `verification_commands` list is enforced before `trls transition --to done` completes.

---

## P5: The Swarm — AI Workers

**Who this is:** The AI agents themselves — Claude Code, Gemini, or any other agent runtime executing tasks. This section shows the full agent lifecycle from startup through task completion.

### Worker Initialization

Before picking up any work, an agent registers itself with Trellis.

```bash
trls worker-init --worker-id agent-7f3a
# Registers agent-7f3a; records capabilities and start time
```

### Finding and Claiming Work

Ask for the list of tasks that are ready to be worked on.

```bash
trls ready
# TASK-042  Implement cache invalidation   [ready]
# TASK-043  Write unit tests for auth       [ready]
# TASK-044  Update API documentation        [ready]
```

Claim a task. This writes a claim operation to the agent's log file. Other agents can still attempt to claim the same task — the conflict is resolved by the MRDT merge rules (see the conflict section below).

```bash
trls claim TASK-042
```

### Getting Context and Starting Work

Retrieve the task-specific context. This includes the issue description, acceptance criteria, scope globs, and relevant excerpts from source documents.

```bash
trls render-context TASK-042
```

Use this context as input to the agent's reasoning loop. Begin implementation.

### Keeping the Claim Alive

For long-running tasks, send heartbeats regularly to prevent the claim from being marked stale.

```bash
trls heartbeat TASK-042
```

Record notable observations or intermediate findings.

```bash
trls note TASK-042 --msg "Cache key format needs to match the pattern in auth.go:L142"
```

When a meaningful architectural or implementation decision is made, record it.

```bash
trls decision TASK-042 \
  --topic "cache-eviction-strategy" \
  --choice "LRU with 512-entry cap" \
  --rationale "Fits within memory budget; hot entries stay resident across requests"
```

### Completing the Task

Once implementation is done and verification commands pass, transition the task.

```bash
trls transition --issue TASK-042 --to done \
  --outcome "Cache invalidation implemented with LRU strategy; all tests pass."
```

In dual-branch mode, the task waits for PR merge before fully unblocking dependents. The agent does not need to do anything for that — Trellis detects the merge automatically.

### Notes for AI Workers

- Always call `trls render-context` before starting work. Do not rely on the issue title alone.
- Send `trls heartbeat` at least once every 15 minutes for long tasks.
- Use `trls note` liberally. Notes are visible to the Conductor and help humans understand what the agent did.
- If `trls claim` succeeds but a subsequent sync shows the claim was lost, call `trls ready` again and claim a different task.

---

## Multi-Agent Conflict Resolution

When multiple AI agents run concurrently, two agents can both see the same task as `ready` and attempt to claim it at the same time. Trellis handles this safely without locks.

### How It Happens

1. Agent A runs `trls ready` and sees TASK-099 as ready.
2. Agent B runs `trls ready` at nearly the same time and also sees TASK-099 as ready.
3. Agent A runs `trls claim TASK-099` — this writes a claim operation to Agent A's log file and pushes it.
4. Agent B runs `trls claim TASK-099` — this writes a claim operation to Agent B's log file and pushes it.
5. Both pushes succeed. Each agent writes to its own log file, so there is no write conflict (MRDT guarantee).

### Resolution

On the next pull-and-materialize cycle, Trellis merges all log files and applies conflict resolution rules:

- **First claim by timestamp wins.** The agent whose claim operation has the earlier timestamp retains the claim.
- **Tiebreaker:** If timestamps are identical (rare), the agent with the lexicographically smaller worker ID wins.

The losing agent discovers it no longer holds the claim when it next syncs. At that point it calls `trls ready` again and picks a different task from the ready queue.

```bash
# Agent B discovers it lost the claim
trls ready
# TASK-099 is no longer listed as ready (Agent A holds it)
# TASK-100  Add pagination support   [ready]
trls claim TASK-100
```

### Observing Conflicts as the Conductor

The Conductor can watch conflict events in real time.

```bash
trls status
# Shows current claim holders, timestamps, and any recent conflict resolutions
```

If a conflict resolution produced an unexpected outcome (e.g., the wrong agent won), the Conductor can intervene by releasing the claim and re-queuing the task.

```bash
trls transition --issue TASK-099 --to ready \
  --outcome "Releasing claim for manual reassignment."
trls claim TASK-099   # or let an agent pick it up naturally
```

### Why This Is Safe

Trellis uses a Merge-CRDT (MRDT) approach where each agent appends to its own log file. There are no shared mutable files, so concurrent writes never corrupt state. The materialized view is computed deterministically from all logs on every sync. The same inputs always produce the same output, regardless of the order in which logs are received.

---

## Quick Reference by Persona

| Persona | Key Commands |
|---|---|
| P1 Lone Wolf | `trls init`, `trls ready`, `trls claim`, `trls transition` |
| P2 Gatekeeper | same as P1, plus dual-branch PR detection for `merged` promotion |
| P3 Conductor | `trls sources add/sync`, `trls decompose-context`, `trls decompose-apply`, `trls dag-summary`, `trls validate`, `trls stale-review` |
| P4 Wrangler | `trls init`, `trls init --repair`, config editing, `trls validate`, `trls stale-review` |
| P5 AI Worker | `trls worker-init`, `trls ready`, `trls claim`, `trls render-context`, `trls heartbeat`, `trls note`, `trls decision`, `trls transition` |
