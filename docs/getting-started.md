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

Run `arm init` in your project root to set up Armature.

```bash
arm init
```

### Solo vs Dual-Branch Modes
- **Solo Mode (Single-Branch):** If your repository doesn't have branch protection on `main`, Armature stores all data in a `.armature/` folder on your `main` branch.
- **Dual-Branch Mode:** If `main` is protected (e.g., GitHub/GitLab PR workflow), Armature creates an orphan `_armature` branch for coordination data. It also creates a secondary worktree at `.arm/` so you can work on code and coordination state simultaneously without conflicts.

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
arm decompose-context --sources all > context.json

# 2. Provide context.json to your AI agent (e.g., Claude, Gemini) 
# and ask it to produce a `plan.json`.

# 3. Apply the plan to create the tasks
arm decompose-apply plan.json
```

## 5. Your First Agent Task

Now your AI agent can pick up work.

### Find Ready Tasks
```bash
arm ready
```

### Claim a Task
```bash
arm claim <issue-id>
```

### Get Task Context
The `render-context` command provides the agent with exactly what it needs to know, minimizing token usage.
```bash
arm render-context <issue-id>
```

### Complete the Task
Once the code changes are made and verified:
```bash
arm transition --issue <issue-id> --to done --outcome "Implemented the feature X in Y."
```

In dual-branch mode, Armature will automatically detect when your PR is merged to promote the task to `merged`.

## Summary of Commands
| Command | Purpose |
| --- | --- |
| `arm init` | Initialize Armature in a repo |
| `arm sources add` | Register a source document |
| `arm ready` | List tasks ready for work |
| `arm claim` | Start working on a task |
| `arm render-context` | Get task-specific context |
| `arm transition` | Move task to a new status |
| `arm list --group` | Show project overview grouped by status |
