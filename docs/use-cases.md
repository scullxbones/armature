# Armature Use Cases — Persona Workflow Walkthroughs

This document shows how different roles use Armature in practice. Each section follows a realistic workflow for one of the five Armature personas, using actual `arm` commands. If you are unsure which persona fits you, read the short description at the top of each section.

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

Pick a task and claim it so the system knows you are working on it.

```bash
arm claim TASK-001
```

Fetch the task-specific context to feed your AI agent. This gives the agent exactly what it needs without flooding it with irrelevant information.

```bash
arm render-context TASK-001
```

Do the work. When done, mark the task complete with a short outcome summary.

```bash
arm transition --issue TASK-001 --to done \
  --outcome "Authentication middleware implemented with JWT support."
```

Check the project overview at any time.

```bash
arm list --group
```

### Notes for Lone Wolf

- No branch protection means there is no PR step. Tasks move directly from `done` to complete.
- Keep the workflow lightweight: `ready` → `claim` → work → `transition`.
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

1. **done** — you self-report that your work is finished and the PR is open.
2. **merged** — Armature auto-detects that the PR landed on `main`.

Downstream tasks only unblock after `merged`. This prevents agents from starting work that depends on code that has not actually shipped yet.

### Daily Workflow

```bash
arm ready
# TASK-010  Implement rate limiter   [ready]
```

Claim the task and get context.

```bash
arm claim TASK-010
arm render-context TASK-010
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
arm transition --issue TASK-010 --to done \
  --outcome "Rate limiter implemented; PR #47 open for review."
```

When the PR is merged to `main`, Armature detects the merge automatically and promotes the task to `merged`. Dependent tasks unblock at that point, not before.

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

Once agents are running, watch progress.

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

If a task is stuck, you can inspect its notes and decisions.

```bash
arm show TASK-031
```

### Notes for Conductor

- Run `arm dag-summary` after `decompose-apply` to verify the dependency graph looks correct before unleashing agents.
- Use `arm validate` regularly — it catches data inconsistencies before they cascade.
- `arm stale-review` surfaces tasks where an agent may have died or disconnected without releasing the claim.

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

## P5: The Swarm — AI Workers

**Who this is:** The AI agents themselves — Claude Code, Gemini, or any other agent runtime executing tasks. This section shows the full agent lifecycle from startup through task completion.

### Worker Initialization

Before picking up any work, an agent registers itself with Armature.

```bash
arm worker-init --worker-id agent-7f3a
# Registers agent-7f3a; records capabilities and start time
```

### Finding and Claiming Work

Ask for the list of tasks that are ready to be worked on.

```bash
arm ready
# TASK-042  Implement cache invalidation   [ready]
# TASK-043  Write unit tests for auth       [ready]
# TASK-044  Update API documentation        [ready]
```

Claim a task. This writes a claim operation to the agent's log file. Other agents can still attempt to claim the same task — the conflict is resolved by the MRDT merge rules (see the conflict section below).

```bash
arm claim TASK-042
```

### Getting Context and Starting Work

Retrieve the task-specific context. This includes the issue description, acceptance criteria, scope globs, and relevant excerpts from source documents.

```bash
arm render-context TASK-042
```

Use this context as input to the agent's reasoning loop. Begin implementation.

### Keeping the Claim Alive

For long-running tasks, send heartbeats regularly to prevent the claim from being marked stale.

```bash
arm heartbeat TASK-042
```

Record notable observations or intermediate findings.

```bash
arm note TASK-042 --msg "Cache key format needs to match the pattern in auth.go:L142"
```

When a meaningful architectural or implementation decision is made, record it.

```bash
arm decision TASK-042 \
  --topic "cache-eviction-strategy" \
  --choice "LRU with 512-entry cap" \
  --rationale "Fits within memory budget; hot entries stay resident across requests"
```

### Completing the Task

Once implementation is done and verification commands pass, transition the task.

```bash
arm transition --issue TASK-042 --to done \
  --outcome "Cache invalidation implemented with LRU strategy; all tests pass."
```

In dual-branch mode, the task waits for PR merge before fully unblocking dependents. The agent does not need to do anything for that — Armature detects the merge automatically.

### Notes for AI Workers

- Always call `arm render-context` before starting work. Do not rely on the issue title alone.
- Send `arm heartbeat` at least once every 15 minutes for long tasks.
- Use `arm note` liberally. Notes are visible to the Conductor and help humans understand what the agent did.
- If `arm claim` succeeds but a subsequent sync shows the claim was lost, call `arm ready` again and claim a different task.

---

## Multi-Agent Conflict Resolution

When multiple AI agents run concurrently, two agents can both see the same task as `ready` and attempt to claim it at the same time. Armature handles this safely without locks.

### How It Happens

1. Agent A runs `arm ready` and sees TASK-099 as ready.
2. Agent B runs `arm ready` at nearly the same time and also sees TASK-099 as ready.
3. Agent A runs `arm claim TASK-099` — this writes a claim operation to Agent A's log file and pushes it.
4. Agent B runs `arm claim TASK-099` — this writes a claim operation to Agent B's log file and pushes it.
5. Both pushes succeed. Each agent writes to its own log file, so there is no write conflict (MRDT guarantee).

### Resolution

On the next pull-and-materialize cycle, Armature merges all log files and applies conflict resolution rules:

- **First claim by timestamp wins.** The agent whose claim operation has the earlier timestamp retains the claim.
- **Tiebreaker:** If timestamps are identical (rare), the agent with the lexicographically smaller worker ID wins.

The losing agent discovers it no longer holds the claim when it next syncs. At that point it calls `arm ready` again and picks a different task from the ready queue.

```bash
# Agent B discovers it lost the claim
arm ready
# TASK-099 is no longer listed as ready (Agent A holds it)
# TASK-100  Add pagination support   [ready]
arm claim TASK-100
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
arm claim TASK-099   # or let an agent pick it up naturally
```

### Why This Is Safe

Armature uses a Merge-CRDT (MRDT) approach where each agent appends to its own log file. There are no shared mutable files, so concurrent writes never corrupt state. The materialized view is computed deterministically from all logs on every sync. The same inputs always produce the same output, regardless of the order in which logs are received.

---

## Quick Reference by Persona

| Persona | Key Commands |
|---|---|
| P1 Lone Wolf | `arm init`, `arm ready`, `arm claim`, `arm transition` |
| P2 Gatekeeper | same as P1, plus dual-branch PR detection for `merged` promotion |
| P3 Conductor | `arm sources add/sync`, `arm decompose-context`, `arm decompose-apply`, `arm dag-summary`, `arm validate`, `arm stale-review` |
| P4 Wrangler | `arm init`, `arm init --repair`, config editing, `arm validate`, `arm stale-review` |
| P5 AI Worker | `arm worker-init`, `arm ready`, `arm claim`, `arm render-context`, `arm heartbeat`, `arm note`, `arm decision`, `arm transition` |
