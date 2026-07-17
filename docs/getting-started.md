# Getting Started with Armature

Armature is a git-native work orchestration system for multi-agent AI coordination. This guide will walk you through installation, setup, and your first task.

## 1. Installation

### Prerequisites
- **Git** (v2.25+)
- **Go** (to build from source)

### Build and Install
Clone the repository and install the `arm` binary:

```bash
git clone https://github.com/scullxbones/armature.git
cd armature
make install
```

The binary will be installed to `~/.local/bin/arm`. Ensure this directory is in your `PATH`.

## 2. Initialize Armature

Run `arm bootstrap` in your project root to set up Armature.

```bash
arm bootstrap
```

### Initialization Details
Armature creates an orphan `_armature` branch for coordination data, storing all state in the `.armature/` worktree. This separation ensures code and coordination state never conflict, enabling reliable multi-agent coordination.

For detailed configuration options (TTL, token budget, hooks), see [Configuration Reference](configuration.md).

### Initialize Worker

Register the current clone as a worker in Armature's coordination system.

```bash
arm worker-init --check || arm worker-init
```

This command registers a unique worker UUID in your git config. It only needs to run once per clone—subsequent invocations will detect the existing registration and skip initialization.

### Deploy Bundled Skills

The `arm bootstrap` command deploys bundled skills to `.claude/skills/` by default. To refresh skills in your project:

```bash
arm bootstrap
```

To make bundled skills available globally to Claude Code agents, use `--global`:

```bash
arm bootstrap --global
```

This deploys the bundled skills (`armature`, `coordinator`, `worker`, `planner`, `auditor`) to `~/.claude/skills/`. For other agent platforms, copy the skill files manually to the appropriate skills directory.

## 3. Register Knowledge Sources

Armature uses source documents (PRDs, Architecture docs) to define work.

```bash
# Add a source document from the local filesystem
arm sources add --url docs/armature-prd.md --type filesystem

# Sync to cache the content locally
arm sources sync
```

## 4. Decompose Requirements into Tasks

Use an AI agent to break down your requirements into a Task DAG.

```bash
# 1. Generate context for the AI agent
arm dag context --sources all > context.json

# 2. Provide context.json to your AI agent (e.g., Claude, Gemini) 
# and ask it to produce a `plan.json`.

# 3. Apply the plan to create the tasks
arm dag apply plan.json
```

## 5. Dispatch Work

The coordinator loop: find ready tasks, claim each one, render context, dispatch a worker agent, and repeat until the story is done.

### Find Ready Tasks
```bash
arm ready
# TASK-001  Write authentication middleware   [ready]
# TASK-002  Add user profile endpoint         [ready]
```

### Claim and Dispatch
```bash
arm claim TASK-001
arm render-context TASK-001 --format agent
# Pass the render-context output to your AI agent as its task spec
```

### Create a Feature Branch and Complete
```bash
git checkout -b feature/TASK-001
arm note TASK-001 --msg "Started implementation"
arm transition TASK-001 --to done --outcome "Implemented auth middleware with JWT support"
```

> `arm transition --to done` rejects transitions on `main`/`master` to enforce
> the PR workflow. Create a feature branch first, or pass `--force` to override
> in rare emergency cases.

### Loop Until Done
```bash
arm ready   # check for the next wave of unblocked tasks
```

## Summary of Commands
| Command | Purpose |
| --- | --- |
| `arm bootstrap` | Initialize Armature and deploy skills in a repo |
| `arm sources add` | Register a source document |
| `arm ready` | List tasks ready for work |
| `arm claim` | Claim a task |
| `arm render-context` | Assemble task context for an agent |
| `arm transition` | Record task completion or status change |
| `arm list --group` | Show project overview grouped by status |
