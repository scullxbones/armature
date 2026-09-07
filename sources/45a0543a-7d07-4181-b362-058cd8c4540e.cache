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
arm claim TASK-001 --worktree
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
arm claim TASK-ID --worktree
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
      "command": ["sh", "-c", "scripts/notify-slack.sh >/dev/null && echo '{\"allowed\":true}'"]
    },
    {
      "name": "page_oncall",
      "command": ["sh", "-c", "scripts/page-on-call.sh >/dev/null && echo '{\"allowed\":true}'"]
    }
  ]
}
```

Key settings:

| Setting | What it controls |
|---|---|
| `project_type` | Project language/framework: `"go"`, `"node"`, `"python"`, `"rust"`, `"make"`, or `"unknown"` |
| `default_ttl` | Default claim TTL in minutes. `arm claim` uses this when `--ttl` is omitted (explicit `--ttl` always wins). The value is written onto the claim op and drives staleness until a claim, heartbeat, or claimant-authored transition extends it. Notes, decisions, and other claimant ops do not renew the claim. If the field is omitted, the builtin fallback is 60. A present `0` is out of range: `arm doctor` D10 fails. |
| `token_budget` | Default token budget for `arm render-context`. Used when `--budget` is omitted (explicit `--budget` always wins). Truncation drops lowest-priority context layers until the bundle fits, approximating 4 characters per token. If the field is omitted, the builtin fallback is 4000; `arm bootstrap` writes 1600. A present `0` is out of range: `arm doctor` D10 fails. |
| `low_stakes_push_threshold` | After this many consecutive low-stakes ops (notes, heartbeats, decisions), the pending-push counter resets. The field does not push `_armature` and does not change batch size; only a high-stakes op pushes (committed and a `_armature` push attempted immediately). That class includes `claim`, `transition`, `assign`, `unassign`, `ready` when it claims, and `doctor --fix`; notes, heartbeats, and decisions are not in it. If the field is omitted, the builtin fallback is 5. A present `0` is out of range: `arm doctor` D10 fails. |
| `hooks` | Array of pre-transition hooks: `name` and `command` array |

### Hook Configuration

Armature can fire hooks on state transitions. Configure them in `.armature/config.json` under the `hooks` key as an array of objects, each with a name and command.

```json
{
  "hooks": [
    {
      "name": "notify_slack",
      "command": ["sh", "-c", "scripts/notify-slack.sh >/dev/null && echo '{\"allowed\":true}'"]
    },
    {
      "name": "page_oncall",
      "command": ["sh", "-c", "scripts/page-on-call.sh >/dev/null && echo '{\"allowed\":true}'"]
    }
  ]
}
```

Each hook is invoked as `command[0] command[1] ... command[n]` on every `arm transition` call. JSON input (`issue_id`, `from_status`, `to_status`, `worker_id`) is written to stdin. The command must print JSON on stdout: `{"allowed":true}` or `{"allowed":false,"message":"..."}`. Hooks run in array order and are not filtered by name or event type. A non-zero exit, invalid JSON, or `allowed: false` blocks the transition. Hooks inherit the caller's process cwd (typically the code worktree); the runner does not set the command directory to the ops worktree.

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
- Keep `default_ttl` generous enough that slow tasks do not expire their claims before heartbeats can renew them.
- A failing hook (non-zero exit) blocks the transition; use them sparingly for critical integrations.

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
arm claim TASK-042 --worktree && arm render-context TASK-042 --format agent > ctx-042.json
arm claim TASK-043 --worktree && arm render-context TASK-043 --format agent > ctx-043.json
# dispatch agents with their context packages; agents transition to done when complete
```

### Notes for Agent Fleet

- Pre-claim all tasks in a wave before dispatching agents — this prevents claim races during parallel execution.
- Assign each parallel agent a log slot: `export ARM_LOG_SLOT=<n>` before any `arm` command.
- If a claim race occurs, the losing agent sees the task as `in-progress` and calls `arm ready` for the next available task.
- After each wave, run `arm merged --issue <id>` for completed tasks to unblock the next wave.

---

## The Delivery Gate

When a worker transitions a claimed task, bug, feature, or story to `done`, Armature runs an automated **delivery gate** against that issue's bound worktree. This prevents incomplete, out-of-scope, or untraceable changes from being marked done. Epics have no claimed worktree, and an unclaimed coordinator-level story has no bound worktree, so those transitions are exempt. A story with a recorded claimant but no discoverable claimed worktree fails closed rather than being treated as unclaimed.

### What the Gate Checks

**1. Clean Tree**

The working tree must be clean: the gate runs `git status --porcelain --ignored` and requires empty output. All work must be staged and committed; there can be no outstanding changes. Because `--ignored` is included, gitignored build artifacts (e.g. `bin/arm`, `coverage.out`) count as dirty too and must be removed or cleaned before transitioning to `done` -- a plain `git status --porcelain` (without `--ignored`) showing clean is not sufficient. Note: `.armature/` state (Armature's internal coordination data) is exempted from this check and does not count as outstanding work.

*Remedy:* Stage any uncommitted changes (`git add <files>`) and commit them with a conventional-commit message.

**2. Scope Containment**

The diff between your base commit and `HEAD` must be a subset of the issue's declared scope. The base commit is resolved with a three-tier fallback, most-precise first:

1. **Dynamic parent-branch merge-base** — recomputed fresh on every gate check as `git merge-base` between the task branch and the parent branch it was cut from (recorded as git config at claim time). This is the branch the coordinator's checkout was actually on when the task branch was created — often a story branch containing completed sibling-task commits, not `main`. Recomputing on demand (rather than trusting a value cached once) means it self-corrects if the task branch is later rebased onto an updated parent tip.
2. **Claim-time recorded SHA** — if no parent-branch record exists (e.g. a worktree claimed before this mechanism existed), fall back to the branch-point SHA persisted once at claim time.

If none of the three tiers can determine a base commit, the transition fails closed (use `--skip-delivery-gate` to override). Scope validation uses the internal `claim.IsWithinScope` check.

**Known limitation — worktrees created by hand outside Armature:** tier 1 (dynamic parent-branch merge-base) requires the parent branch name that Armature records when it creates a fresh worktree. For an Armature-driven sub-task created from a story worktree, use `arm claim SUBTASK-ID --worktree /path/to/new-task-worktree --from /path/to/story-worktree`; this preserves the parent branch and tip metadata. The gap applies only to a worktree or branch a human created by hand outside Armature, where Armature cannot derive a trustworthy parent from the checkout alone. In that case it deliberately leaves tier 1 unavailable and uses the claim-time SHA (tier 2) or a default-branch merge-base (tier 3). This is intentional and covered by tests, not an oversight. Re-running that same existing-worktree claim cannot repair the missing parent metadata: the metadata writes are absence-only and no trustworthy parent is available to record. If its true parent later rebases, the recorded/fallback base can become stale and produce a spurious gate failure. Create a new worktree through the normal fresh-claim path from the known parent when that is safe; otherwise use `--skip-delivery-gate` with an outcome that explains the audited override.

This prevents scope creep: you cannot deliver changes that fall outside the issue's boundaries.

*Remedy:* Either narrow the diff to fit the declared scope (revert out-of-scope changes), or broaden the scope in the original issue if the additional changes are justified.

**3. Commit Reference**

At least one commit since the base commit must match the conventional-commit format `<type>(<ISSUE-ID>): ...` per `docs/conventions.md`. This ensures your work is traceable and tied to the issue ID.

*Remedy:* Add at least one properly-formatted commit. For example: `feat(LNGHZN-S4-T3): document the delivery gate in worker skill`.

### On Gate Failure

If any check fails, `arm transition --to done` refuses the transition and prints a per-check remediation message. Fix the issues and retry.

### Skipping the Gate

The `--skip-delivery-gate` flag bypasses all three checks for a transition to `done`; Armature rejects the flag for other target states:

```bash
arm transition TASK-ID --to done --skip-delivery-gate \
  --outcome "Skipped gate: <reason>"
```

Use this **only when the gate assumption does not hold**. Examples:

- A docs-only or demonstration transcript task that does not execute real delivery work.
- A hotfix or emergency patch where normal scope/commit conventions cannot be followed.
- An external constraint (e.g., code generated by a tool, not hand-written) makes gate compliance impossible.

When you skip the gate, the transition op records `Payload.SkippedDeliveryGate` as an audit flag and its `outcome` records the supplied reason for future review.

**Guidance:** Prefer fixing the underlying issue over using the override. Clean trees, scope containment, and proper commit messages are hygiene practices that make the codebase and Armature's state machine trustworthy. Use the override sparingly and document the reason clearly.

---

## Multi-Agent Conflict Resolution

When multiple agents are dispatched concurrently, two can both see the same task as `ready` and race to claim it. Armature handles this safely without locks.

### How It Happens

1. Agent A runs `arm claim TASK-099 --worktree`; Agent B retries `arm claim TASK-099 --worktree` at nearly the same time; claim arbitration and the canonical `.worktrees/TASK-099` path prevent duplicate ownership.
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
arm claim TASK-100 --worktree && arm render-context TASK-100 --format agent
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
