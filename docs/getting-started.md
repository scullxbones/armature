# Getting Started with Trellis

Trellis is a git-native work orchestration system for multi-agent AI coordination. This guide will walk you through installation, setup, and your first task.

## 1. Installation

### Prerequisites
- **Git** (v2.25+)
- **Go** (to build from source)

### Build and Install
Clone the repository and install the `trls` binary:

```bash
git clone https://github.com/google/trellis.git
cd trellis
make install
```

The binary will be installed to `~/.local/bin/trls`. Ensure this directory is in your `PATH`.

## 2. Initialize Trellis

Run `trls init` in your project root to set up Trellis.

```bash
trls init
```

### Solo vs Dual-Branch Modes
- **Solo Mode (Single-Branch):** If your repository doesn't have branch protection on `main`, Trellis stores all data in a `.issues/` folder on your `main` branch.
- **Dual-Branch Mode:** If `main` is protected (e.g., GitHub/GitLab PR workflow), Trellis creates an orphan `_trellis` branch for coordination data. It also creates a secondary worktree at `.trellis/` so you can work on code and coordination state simultaneously without conflicts.

## 3. Register Knowledge Sources

Trellis uses source documents (PRDs, Architecture docs) to define work.

```bash
# Add a source document from the local filesystem
trls sources add --url docs/trellis-prd.md --type filesystem

# Sync to cache the content locally
trls sources sync
```

## 4. Decompose Requirements into Tasks

Use an AI agent to break down your requirements into a Task DAG.

```bash
# 1. Generate context for the AI agent
trls decompose-context --sources all > context.json

# 2. Provide context.json to your AI agent (e.g., Claude, Gemini) 
# and ask it to produce a `plan.json`.

# 3. Apply the plan to create the tasks
trls decompose-apply plan.json
```

## 5. Your First Agent Task

Now your AI agent can pick up work.

### Find Ready Tasks
```bash
trls ready
```

### Claim a Task
```bash
trls claim <issue-id>
```

### Get Task Context
The `render-context` command provides the agent with exactly what it needs to know, minimizing token usage.
```bash
trls render-context <issue-id>
```

### Complete the Task
Once the code changes are made and verified:
```bash
trls transition --issue <issue-id> --to done --outcome "Implemented the feature X in Y."
```

In dual-branch mode, Trellis will automatically detect when your PR is merged to promote the task to `merged`.

## Summary of Commands
| Command | Purpose |
| --- | --- |
| `trls init` | Initialize Trellis in a repo |
| `trls sources add` | Register a source document |
| `trls ready` | List tasks ready for work |
| `trls claim` | Start working on a task |
| `trls render-context` | Get task-specific context |
| `trls transition` | Move task to a new status |
| `trls list --group` | Show project overview grouped by status |
