# Armature

**Git-Native Work Orchestration for Multi-Agent AI Coordination**

> "Every agent decision. Every requirement cited. All of it in git."

---

## Overview

Armature is a git-native work orchestration system for AI coding agents. It solves two problems that compound as teams scale: agents that lose context between sessions and forget architectural decisions, and AI-generated work with no traceable record connecting decisions back to the requirements that originated them.

Multiple agents working in the same codebase step on each other, duplicate effort, and produce changes no one can audit after the fact. Armature coordinates them through a typed task DAG with append-only event-sourced logs — merge-conflict-free by construction, because each worker writes exclusively to its own log file. Every claim, transition, and outcome is structurally cited back to its source document.

Armature ships skills in the agentskills.io format covering every role in the workflow — planner, coordinator, worker, and auditor — usable by any compatible tool. Your agents participate immediately, no custom prompt engineering required.

All state lives in git. No database, no server, no daemon. A single Go binary (`arm`) and git are the only requirements.

## Key Features

- **Source Traceability**: Structural citations link every claim, transition, and agent decision to its originating source document. The result is a full, inspectable audit trail — useful for sign-off review, compliance, and understanding why any given decision was made.

- **DAG-Structured Context**: Requirements decompose into a typed dependency graph (epic → story → task). Each agent receives a deterministic context assembly of 650–1,600 tokens using a layered algorithm — core task definition, acceptance criteria, and scope are always preserved; when the token budget is exceeded, lower-priority context (sibling outcomes, prior notes) is dropped first, preserving the highest-signal content.

- **Merge-Conflict-Free by Construction**: Uses Mergeable Replicated Data Types (MRDT, a variant of CRDTs) with a single-writer principle — each worker appends only to its own log, no worker ever writes another's. Current state is derived by replay. Merge conflicts on coordination state are architecturally impossible.

- **Zero Infrastructure**: Git-only. No persistent server, no database, no daemon. All coordination state is stored as append-only JSONL event journals — plain text and always inspectable, not meant to be edited directly. A single Go binary (`arm`) and git are the only requirements.

- **Workflow Skills Included**: Ships skills in the agentskills.io format for every workflow role — planner, coordinator, worker, and auditor — usable by any compatible tool. No custom prompt engineering required to wire your agents in.

## Runtime Architecture

```mermaid
flowchart LR
    U[Coordinator] --> R[arm ready]
    R --> CL[arm claim]
    CL --> CTX[arm render-context]
    CTX --> W[Worker agent]
    W --> OP[(append-only ops log)]
    OP --> M[materialize state]
    M --> V[ready/list/show/validate views]
    V --> U
```

## Coordinator Flow

```mermaid
flowchart TD
    S([survey story DAG]) --> R[arm ready]
    R -->|none ready| V[arm validate]
    R -->|ready tasks| C[arm claim + render-context]
    C --> D[dispatch worker agents]
    D --> I[wait + integrate]
    I --> R
    V --> T[arm transition story done]
    T --> PR[push + open PR]
```

## Installation

### Prerequisites

- **Git** (v2.25+ for sparse checkout support)
- **Go** (for building from source)

### Building from Source

```bash
git clone https://github.com/scullxbones/armature.git
cd armature
make install
```

This will build the `arm` binary and install it to `~/.local/bin/arm`. Ensure `~/.local/bin` is in your `PATH`.

---

## 5-Minute Quickstart

### 1. Initialize a Repository

From your project root, run:

```bash
arm bootstrap
```

Armature will create an orphan `_armature` branch for coordination data and an ops worktree at `.armature/`, enabling safe separation of code and coordination state.

### 2. Register Worker (Once Per Clone)

Initialize the worker coordination system. Run this once per clone before decomposing tasks:

```bash
arm worker-init
```

This registers your worker identity and sets up log coordination.

### 3. Install Skills

Deploy workflow skills for all agent roles:

```bash
arm bootstrap
```

(This step is already included in step 1 if you ran `arm bootstrap` — the bootstrap command both initializes the repository and deploys skills.)

### 4. Add Requirements

Register source documents (PRDs, architecture docs) that define your project's work:

```bash
arm sources add --url docs/armature-prd.md --type filesystem
arm sources sync
arm sources verify   # note the UUID shown — you'll need it in the next step
```

### 5. Decompose into Tasks (via AI)

Generate a decomposition context for your AI agent to break down requirements into a task DAG:

```bash
arm dag context --sources SOURCE-UUID > context.json
# Feed context.json to your AI agent to produce plan.json
arm dag apply --plan plan.json
```

### 6. Dispatch Work

Find ready tasks and dispatch a worker agent for each one:

```bash
arm ready                                      # list unblocked tasks
arm claim ISSUE-ID --worktree ./issue-worktree  # claim a task
arm render-context ISSUE-ID --format agent      # get task context for the agent
# dispatch agent with render-context output
arm transition ISSUE-ID --to done --outcome "what was done"
```

### 7. Complete and Verify

Once you've finished the code changes, transition the task to `done`:

```bash
arm transition ISSUE-ID --to done --outcome "Brief summary of work"
```

Armature will automatically detect when your code is merged into the main branch to promote the task to `merged`.

## Documentation

- **[Core Concepts](docs/concepts.md)** — Agent reference for the 8 operational concepts: ops log & materialization, worker identity, claim lifecycle, DAG hierarchy, citations, confidence levels, branch modes, and decomposition
- **[Getting Started](docs/getting-started.md)** — Setup workflow and first task
- **[Commands Reference](docs/commands.md)** — Complete command documentation
- **[Configuration Reference](docs/configuration.md)** — TTL, token budgets, hooks, and mode settings
- **[Use Cases](docs/use-cases.md)** — Persona-based workflow walkthroughs (lone wolf, gatekeeper, team coordinator, etc.)
- **[Validation Codes](docs/validation-codes.md)** — Error and warning reference (E1–E12, W1–W11)
- **[Provider Smoke Tests](docs/provider-smoke-tests.md)** — Testing source document providers

---

## License

Armature is open-source software licensed under the Apache 2.0 License.
