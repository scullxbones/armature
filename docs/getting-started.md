# Getting Started with Armature

Armature is a git-native work orchestration system for multi-agent AI coordination. This guide will walk you through installation, setup, and your first task.

## 1. Installation

### Prerequisites
- **Git** (v2.25+)
- **Go** 1.26+ (for `go install` or building from source)

### Released binaries
Download the `arm` archive for your OS/arch from
[GitHub Releases](https://github.com/scullxbones/armature/releases),
verify the checksum, unpack it, and place `arm` on your `PATH` (for example
`~/.local/bin/arm`).

### go install

```bash
GOBIN="${GOBIN:-$HOME/.local/bin}" go install github.com/scullxbones/armature/cmd/armature@latest
mv "$GOBIN/armature" "$GOBIN/arm"
```

`go install` names the binary after the package directory (`armature`); rename
it to `arm`. Pin a release tag instead of `@latest` when you need a fixed
version.

### Build from source
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
Bootstrap creates an orphan `_armature` branch and an ops worktree at
`.armature/`. Coordination state (ops logs, materialized issues, config) lives
at the root of that worktree. Older clones used a dual layout (`.arm/` worktree
containing an inner `.armature/` state dir); bootstrap migrates those to the
single `.armature/` worktree. Bootstrap also deploys bundled skills to
`.claude/skills/` (and sibling harness skill dirs) — there is no separate
skill-install step for the default path.

For detailed configuration options (TTL, token budget, hooks), see [Configuration Reference](configuration.md).

### Initialize Worker

Register the current clone as a worker in Armature's coordination system.

```bash
arm worker-init --check || arm worker-init
```

This command registers a unique worker UUID in your git config. It only needs to run once per clone—subsequent invocations will detect the existing registration and skip initialization.

### Optional: refresh or global skills

To refresh bundled skills in the project after an upgrade, run `arm bootstrap`
again. To install skills globally for Claude Code:

```bash
arm bootstrap --global
```

This deploys the bundled skills (`armature`, `coordinator`, `worker`, `planner`, `auditor`) to `~/.claude/skills/`. For other agent platforms, copy the skill files manually to the appropriate skills directory.

## 3. Register Knowledge Sources

Armature uses source documents (PRDs, Architecture docs) to define work.

```bash
# Add a source document from the local filesystem
arm sources add --url docs/requirements.md --type filesystem

# Sync to cache the content locally
arm sources sync
arm sources verify
```

## 4. Decompose Requirements into Tasks

Use an AI agent to break down your requirements into a Task DAG. The README
quickstart shows a minimal runnable `plan.json`; in production an agent writes
that file from `arm dag context` output.

```bash
# 1. Generate context for the AI agent
SOURCE_UUID=$(arm sources verify | awk '/OK/{print $1; exit}')
arm dag context --sources "$SOURCE_UUID" > context.json

# 2. Provide context.json to your AI agent and ask it to produce plan.json
#    (each issue must cite a source UUID).

# 3. Apply the plan and promote draft nodes
arm dag apply --plan plan.json
arm dag transition --issue DEMO-S1
```

## 5. Dispatch Work

The coordinator loop: find ready tasks, claim each one, render context, dispatch a worker agent, and repeat until the story is done.

### Find Ready Tasks
```bash
arm ready
# DEMO-S1-T1  Write greeting   [ready]
```

### Claim and Dispatch
```bash
arm claim DEMO-S1-T1 --worktree
arm render-context DEMO-S1-T1 --format agent
# Pass the render-context output to your AI agent as its task spec
```

### Complete in the claim worktree
```bash
cd .worktrees/DEMO-S1-T1
arm note DEMO-S1-T1 --msg "Started implementation"
# implement, commit with feat(DEMO-S1-T1): ...
arm transition DEMO-S1-T1 --to done --outcome "Implemented the greeting file for the demo story" --force
```

> The done-branch check reads the primary checkout (often still `main`) even
> when the claim worktree is already on `task/<id>`, so `--force` is required
> for that path. Prefer a concrete outcome string; vague outcomes are refused.

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
