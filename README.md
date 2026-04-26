# Armature

**Git-Native Work Orchestration for Multi-Agent AI Coordination**

> "Context rot is a memory problem. Armature gives your agents memory."

---

## Overview

Armature is a file-based, git-native work orchestration system that gives AI coding agents persistent memory. It enables humans and AI workers to coordinate on software projects without merge conflicts, external dependencies, or context rot.

AI coding agents today suffer from a fundamental architectural flaw: they forget everything between sessions. When multiple agents work in the same codebase, they step on each other with no coordination primitive to prevent conflicts. Armature solves this by treating context rot as a memory problem, providing deterministic context assembly and append-only event-sourced logs for every decision, claim, and outcome.

All state lives in git. No database, no server, no daemon. A single Go binary (`arm`) and git are the only requirements.

## Key Features

- **Zero Infrastructure**: Git-only. No persistent server processes or cloud dependencies.
- **Merge-Conflict-Free**: Uses Mergeable Replicated Data Types (MRDT) to ensure conflict-free coordination by construction.
- **Cross-Platform**: Compatible with all major AI coding agents (Claude Code, Cursor, Windsurf, Gemini CLI, Kiro).
- **Deterministic Context**: Assemblies minimal-token context (650–1,600 tokens) to minimize context rot and API costs.
- **Enterprise Traceability**: Structural source citations with a full audit trail of every decision.

## Installation

### Prerequisites

- **Git** (v2.25+ for sparse checkout support)
- **Go** (for building from source)

### Building from Source

```bash
git clone https://github.com/google/trellis.git
cd trellis
make install
```

This will build the `arm` binary and install it to `~/.local/bin/arm`. Ensure `~/.local/bin` is in your `PATH`.

---

## 5-Minute Quickstart

### 1. Initialize a Repository

From your project root, run:

```bash
arm init
```

Armature will detect if your repository has branch protection and set up either a dual-branch (`_armature` orphan branch) or single-branch mode accordingly.

### 2. Add Requirements

Register source documents (PRDs, architecture docs) that define your project's work:

```bash
arm sources add docs/trellis-prd.md
arm sources sync
```

### 3. Decompose into Tasks (via AI)

Generate a decomposition context for your AI agent to break down requirements into a task DAG:

```bash
arm decompose-context --sources src-001 > context.json
# Feed context.json to your AI agent to produce plan.json
arm decompose-apply plan.json
```

### 4. Claim and Execute Work

Find the next ready task, claim it, and start working:

```bash
# See ready tasks
arm ready

# Claim the highest priority task
arm claim <issue-id>

# Get the task context
arm render-context <issue-id>
```

### 5. Complete and Verify

Once you've finished the code changes, transition the task to `done`:

```bash
arm transition <issue-id> done --outcome "Brief summary of work"
```

Armature will automatically detect when your code is merged into the main branch to promote the task to `merged`.

---

## License

Armature is open-source software licensed under the Apache 2.0 License.
