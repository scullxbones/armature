# TRELLIS — Product Requirements Document

**Git-Native Work Orchestration for Multi-Agent AI Coordination**

Version 1.0 | March 2026 | DRAFT

> *"Context rot is a memory problem. Armature gives your agents memory."*

---

## 1. Executive Summary

Armature is a file-based, git-native work orchestration system that gives AI coding agents persistent memory. It enables humans and AI workers to coordinate on software projects without merge conflicts, external dependencies, or context rot.

AI coding agents today suffer from a fundamental architectural flaw: they forget everything between sessions. Every new conversation starts from scratch. When multiple agents work in the same codebase, they step on each other with no coordination primitive to prevent conflicts. The existing tools designed to manage software work (Jira, Linear, GitHub Issues) were built for humans, require network access, and consume thousands of tokens when presented to AI agents.

Armature solves this by treating context rot as a memory problem. Using the CoALA (Cognitive Architectures for Language Agents) taxonomy, Armature provides working memory (deterministic context assembly at ~650–1,600 tokens per task), episodic memory (append-only event-sourced logs capturing every decision, claim, and outcome), semantic memory (source document caching with traceability), and procedural memory (the SKILL.md work loop contract). All state lives in git. No database, no server, no daemon. A single Go binary (`arm`) and git are the only requirements.

The system uses Mergeable Replicated Data Types (MRDT) to ensure that multiple workers—human and AI—can coordinate without merge conflicts by construction. Each worker writes exclusively to its own log file. Current state is derived by replaying all logs in timestamp order. This design is compatible with every major AI coding agent (Claude Code, Cursor, Windsurf, Gemini CLI, Kiro) and requires no platform-specific integration.

---

## 2. Problem Statement

### 2.1 Context Rot

Every frontier LLM degrades as context grows. Research from Chroma (July 2025) found that Claude Sonnet 4 drops from 99% to 50% accuracy on basic tasks as input length increases. The "lost in the middle" effect—strong attention at the beginning and end of context, weak in the middle—persists across all architectures. For coding agents, this manifests as forgetting architectural decisions made minutes ago, suggesting approaches that were explicitly rejected, or declaring completion on a subset of phases while losing track of the overall plan.

The industry term for this is context rot: the progressive degradation of an AI agent's understanding as its context window fills with stale, redundant, or irrelevant information. Context rot is the root cause behind the most common failure modes of AI-assisted development—partial completions, contradictory implementations, and phantom progress where agents report success without delivering it.

### 2.2 No Coordination Primitive

When teams run multiple AI agents in the same codebase, there is no native mechanism for those agents to avoid duplicating effort, claiming the same work, or producing semantically conflicting changes. The merge math is punishing: with 5 parallel agents, the potential conflict surface is N(N−1)/2 branch pairs, and each merge changes the base for remaining branches. Practitioners report that parallel agent workflows degrade superlinearly—far worse than Amdahl's Law predicts—because of the semantic coordination overhead.

Token duplication rates across multi-agent systems quantify this waste: 72% for MetaGPT, 86% for CAMEL, 53% for AgentVerse. Without a coordination layer, agents independently re-derive the same context, produce overlapping implementations, and create conflicting assumptions that surface only at merge time.

### 2.3 Existing Tools Don't Fit

Project management tools like Jira and Linear were designed for human workflows. They require network access, consume 12,000–21,000+ tokens when serialized for AI consumption, and offer no merge-conflict-free coordination mechanism. Markdown-based TODO files (the current workaround) provide no structured context assembly, no claim coordination, and no traceability. Custom agent orchestrators like CrewAI and AutoGen require Python environments, cloud services, and are optimized for business process automation—not codebase-aware software development.

---

## 3. Market Context & Competitive Positioning

### 3.1 Market Size and Trajectory

The AI coding tools market reached $4.9 billion in 2024 and is projected to reach $30.1 billion by 2032. Three companies—GitHub Copilot, Cursor, and Claude Code—have each crossed $1 billion in ARR. 41% of all code is now AI-assisted, and 59% of developers run three or more AI coding tools in parallel. However, the METR randomized controlled trial found that experienced developers with AI tools were 19% slower despite believing they were 24% faster, suggesting the productivity gains remain largely unrealized due to coordination overhead and context management failures.

### 3.2 The Vibe Coding Backlash

The industry is undergoing a correction from undisciplined AI-assisted development. 89% of CTOs surveyed have experienced production disasters from AI-generated code. GitClear's analysis of 211 million lines shows code refactoring dropped from 25% to under 10% while duplication increased 4x. Andrej Karpathy declared "vibe coding" passé in February 2026, proposing "agentic engineering" as the replacement—emphasizing AI agent orchestration with human oversight. This narrative shift from "AI writes code" to "AI agents need coordination infrastructure" creates the precise market opening Armature addresses.

### 3.3 Competitive Landscape

#### 3.3.1 Beads (Steve Yegge)

The category leader by traction with 18,100 GitHub stars. Beads is a distributed, graph-based issue tracker where agents claim tasks, track dependencies, and persist state across sessions. Its architecture recently migrated from SQLite/JSONL to Dolt (a version-controlled SQL database) as its sole backend. Strengths: massive community momentum, rich task-graph semantics, companion orchestrator (Gas Town) handling ~160 concurrent agents. Weaknesses: requires installing and running Dolt (a persistent server process), 100% vibe-coded codebase with high churn, agents must be explicitly instructed to use it via configuration directives.

#### 3.3.2 CASS Memory System (Jeffrey Emanuel)

A cross-agent knowledge unification system with a three-layer cognitive architecture: episodic memory (raw session logs from 11+ agent formats), working memory (structured diary entries), and procedural memory (confidence-tracked rules with 90-day half-life decay). CASS solves learning persistence—agents share accumulated knowledge—but is a knowledge layer, not a coordination layer. Full value requires multiple tools from Emanuel's 14-tool ecosystem. 275 GitHub stars on the memory system, 550 on the companion search engine.

#### 3.3.3 OpenViking (ByteDance/Volcano Engine)

An open-source context database that replaces flat vector storage with a virtual filesystem accessible via `viking://` URIs. Its three-tier progressive loading (L0 summaries at ~100 tokens, L1 overviews at ~2,000 tokens, L2 full content on demand) achieves 83% reduction in input token cost. However, OpenViking is alpha-stage infrastructure serving primarily the Chinese AI ecosystem through OpenClaw integration, not a user-facing coordination tool for software development.

#### 3.3.4 Multi-Agent Orchestration Frameworks

CrewAI (40K+ stars, $18M funding), LangGraph, and AutoGen are optimized for business workflow automation. They fail for software development because codebases represent shared mutable state with semantic dependencies between changes—a coordination surface fundamentally different from document processing pipelines or customer service routing.

#### 3.3.5 Emerging Git-Native Tools

Letta Context Repositories store agent context as git-versioned files. Git-Context-Controller achieved 48% resolution on SWE-Bench-Lite by treating agent context as a versioned filesystem. Clash detects merge conflicts between worktrees before they happen. Each solves a fragment. None delivers a complete coordination system.

### 3.4 Armature Differentiation

Armature occupies a unique position in this landscape by combining five properties no competitor offers together:

| Property | Armature | Beads | CASS | OpenViking | CrewAI |
|---|---|---|---|---|---|
| Zero infrastructure | Yes (git only) | No (Dolt server) | No (Rust + Bun + LLM) | No (embeddings + DB) | No (Python + cloud) |
| Merge-conflict-free | By construction (MRDT) | Hash IDs (Dolt merges) | N/A (not coordination) | N/A | N/A |
| Cross-platform agents | All CLI agents | Requires config directives | 11+ session formats | OpenClaw focus | Python only |
| Task coordination | Full (claims, deps, DAG) | Full (graph-based) | None (knowledge only) | None (context DB) | Role-based |
| Context assembly | Deterministic, 650–1,600 tok | Agent queries tasks | Rule retrieval | Progressive loading | None |
| Enterprise traceability | Structural (source citations with audit trail; citation content verified by reviewer at sign-off gate) | Task history | Session logs | None | None |
| Offline capable | Yes (git queues locally) | Partial (Dolt local) | No (needs LLM) | No | No |

---

## 4. Target Personas

Armature serves five distinct personas. Each persona maps to a deployment topology and feature surface. The persona names are used throughout this document and in the feature matrix (Section 7) to trace requirements to their owners.

### 4.1 P1: Lone Wolf — Solo Freelance Developer

**Profile:** Freelance or indie developer working solo on personal projects or client engagements. Runs one AI coding agent (typically Claude Code or Cursor) alongside their own manual work. Single repository, no branch protection, no team coordination requirements.

**Core need:** Structured task tracking and persistent context for their AI agent without ceremony. They want their agent to remember what it did yesterday, pick up where it left off, and not re-derive context from scratch on every session.

**Pain today:** Manually writing TODO.md files and CLAUDE.md instructions. Losing agent context between sessions. Repeating the same architectural context in every prompt. Agent forgets decisions made in prior sessions and suggests approaches already rejected.

**Deployment:** Single-branch mode. All `.armature/` data on main alongside code. No dual-branch complexity. Hooks optional. One worker.

### 4.2 P2: Gatekeeper — Enterprise Solo Developer

**Profile:** Developer at a company with protected main branch and mandatory PR reviews. Works solo with AI agents but operates within enterprise git workflow constraints. Needs an audit trail for compliance but does not face multi-agent claim races.

**Core need:** All the benefits of Lone Wolf, plus two-phase completion that ensures code is reviewed before downstream work begins. Needs the dual-branch architecture because their main branch is protected—they cannot push ops directly to main.

**Pain today:** Same as Lone Wolf, plus: merge detection is manual (they don't know when their PR was merged and downstream tasks can start), and there is no audit trail connecting agent decisions to source requirements.

**Deployment:** Dual-branch mode (`_armature` orphan branch for ops, feature branches to protected main via PR). Hooks recommended. One worker.

### 4.3 P3: Conductor — Team Lead / Architect

**Profile:** Tech lead or software architect coordinating multiple human developers and AI agents on a shared codebase. Responsible for decomposing requirements into actionable work, reviewing DAG structure, monitoring agent progress, and ensuring source traceability.

**Core need:** Orchestrate a fleet of workers without micromanaging. Decompose source documents (PRDs, architecture docs) into a typed DAG of tasks that agents can claim and execute autonomously. Review exactly what each agent will see (context spot-checking). Govern DAG structure through interactive sign-off.

**Pain today:** Manually writing task breakdowns in markdown. No visibility into what agents are actually working on. Agents produce conflicting implementations because they don't know about each other's scope. No traceability from shipped code back to source requirements. Context review requires reading agent conversation logs.

**Deployment:** Dual-branch mode. Full MRDT coordination. DAG governance via interactive sign-off. Source document management with Confluence/SharePoint integration. Multiple workers.

### 4.4 P4: Wrangler — Agent Operator / Platform Engineer

**Profile:** DevOps engineer or platform engineer responsible for provisioning AI agent environments, monitoring agent health, tuning TTLs, configuring verification hooks, and recovering from agent failures. May manage agents across multiple repositories.

**Core need:** Worker provisioning (`arm init`, `worker-init`), heartbeat monitoring to detect stuck agents, TTL tuning for different agent profiles (ephemeral CI agents vs. long-running supervised agents), hook configuration for project-specific test/lint commands, and worktree repair when things go wrong.

**Pain today:** No standard way to provision agent identities. No heartbeat protocol to detect stuck agents. No configurable verification gates. Agent failures are discovered only when someone notices bad output. Worktree management is manual and error-prone.

**Deployment:** Manages the infrastructure for Conductor and AI Worker deployments. Primary user of `arm init`, `arm init --repair`, hook configuration, and TTL tuning.

### 4.5 P5: The Swarm — AI Workers

**Profile:** The AI coding agents themselves (Claude Code, Gemini CLI, Cursor, Windsurf, Kiro). Ephemeral by nature—they forget everything between sessions. They interact with Armature exclusively through the SKILL.md contract and the `arm` CLI, consuming JSON output and emitting structured commands.

**Core need:** Deterministic, minimal-token context for each task. A clear work loop: find ready task, get context, claim, execute, verify, complete. Machine-parseable output for all commands. Structured error messages with actionable hints. Never need to understand dual branches, worktrees, or materialization—the CLI abstracts everything.

**Pain today:** The "50 First Dates" problem: every session starts from zero. Agents re-derive context, re-plan completed work, and declare premature victory. No structured acceptance criteria to verify against. No mechanism to discover what other agents have done or are doing. Context windows fill with stale information, causing context rot.

**Deployment:** Consumes SKILL.md. Executes work loop. Interacts only via `arm` CLI commands with JSON output. Platform-agnostic by design.

---

## 5. Use Cases

### 5.1 UC-1: Greenfield Decomposition

**Personas:** Conductor, The Swarm

**Trigger:** A new project or feature needs to be broken down from source documents (PRD, architecture doc) into executable work.

The Conductor registers source documents via `arm sources add` and runs `arm sources sync` to cache them locally. They then run `arm decompose-context` to generate a structured context package that an AI agent reads to produce a decomposition plan (`plan.json`). The plan undergoes structural validation via `arm decompose-apply --dry-run`. Once clean, `arm decompose-apply` atomically creates all nodes in the DAG. The Conductor reviews the DAG via the `arm dag-summary` interactive TUI, performing per-item sign-off that promotes draft nodes to verified status. Agents can then begin claiming and executing tasks.

**Success criteria:** Source documents are transformed into a typed DAG (epic → story → task) with source citations on every node, machine-checkable acceptance criteria on every task, and scope isolation preventing file-level conflicts between sibling tasks. Time from source registration to first agent task claim is under 30 minutes for a typical PRD.

### 5.2 UC-2: Agent Work Loop

**Personas:** The Swarm, Wrangler

**Trigger:** An AI agent starts a work session and needs to find, claim, execute, and complete a task.

The agent runs `arm ready --format=json` to get the prioritized list of available tasks (the ready queue filters for: status open, all blockers merged, parent in-progress, no active claim). It selects the highest-priority task and runs `arm render-context` to receive a deterministic context slice containing the task spec, scope, acceptance criteria, blocker outcomes, parent chain, open decisions, and advisory token budget. The agent claims the task via `arm claim` (which handles post-claim verification internally), executes the work within the defined scope, emits heartbeats during long operations, verifies acceptance criteria, and transitions to done with a one-line outcome summary. Code is committed to the feature branch and pushed for PR review. The agent returns to the top of the loop.

**Success criteria:** Each task context is 650–1,600 tokens (vs. 12,000–21,000+ for markdown approaches). The agent never needs to understand dual branches, worktrees, or materialization. Claim races are resolved deterministically with zero wasted work beyond the race window.

### 5.3 UC-3: Multi-Agent Claim Coordination

**Personas:** The Swarm, Conductor

**Trigger:** Multiple AI agents attempt to claim the same task between sync cycles.

Two agents both see a task as ready and emit claim ops to their respective log files. Both pushes succeed because they write to different files (the MRDT guarantee). On the next pull-and-materialize cycle, timestamp-based resolution determines the winner: first claim by timestamp wins, with lexicographic worker ID as the deterministic tiebreaker for identical timestamps. The losing agent discovers it lost on its next sync and returns to the ready queue to find another task. The winning agent proceeds with execution. The Conductor can observe this coordination in real-time via `arm status`.

**Success criteria:** No merge conflicts. No locking. No central coordinator. Deterministic winner selection. Losing agent wastes at most one sync cycle of effort.

### 5.4 UC-4: Solo Developer Workflow

**Personas:** Lone Wolf

**Trigger:** A solo developer wants structured task tracking for their AI agent without team coordination overhead.

The developer runs `arm init`, which detects no branch protection and uses single-branch mode. All `.armature/` data lives on main alongside code. They create tasks manually (`arm create`) or via decomposition. Their single AI agent executes the work loop: find ready task, get context, claim, execute, complete. There are no claim races (one worker), two-phase completion is optional (direct push to main), and hooks are optional. The developer benefits from persistent context across agent sessions, structured acceptance criteria, and a ready-task queue that automatically tracks dependencies and completion.

**Success criteria:** `arm init` to first task execution in under 5 minutes. Zero configuration beyond init. Agent context persists across sessions without manual intervention.

### 5.5 UC-5: Enterprise Onboarding

**Personas:** Gatekeeper, Wrangler

**Trigger:** A developer at an enterprise with protected main branch sets up Armature for the first time.

`arm init` detects branch protection and automatically creates the `_armature` orphan branch, sets up the ops worktree with sparse checkout, proposes verification hooks based on detected project type (`package.json` → `npm test`, `go.mod` → `go test ./...`), and installs git hooks for automatic heartbeat, commit-message stamping, and merge promotion. The two-phase completion model ensures downstream tasks only begin after code passes PR review (status must be `merged`, not just `done`). The Wrangler configures TTL defaults, verification commands, and optional staleness thresholds in the shared `.armature/config.json`.

**Success criteria:** Branch mode auto-detection is correct. Hooks install without conflicting with existing hook managers. Verification hooks match the project's actual toolchain.

### 5.6 UC-6: Brownfield Import

**Personas:** Conductor

**Trigger:** A team with existing work items in Jira, Linear, or CSV wants to adopt Armature without starting over.

The Conductor runs `arm import` with the existing task file. Imported nodes are created with `provenance.confidence` set to `inferred`, meaning they appear in the ready queue with a `requires_confirmation` flag and cannot be claimed until a human confirms each one via `arm confirm`. This rate-limiting is intentional: it forces human review of imported work items rather than allowing bulk rubber-stamping. After confirmation, a second decomposition pass can extend the DAG with new tasks that reference or depend on the imported ones.

**Success criteria:** CSV and JSON import work out of the box. Imported nodes are clearly distinguished from decomposed nodes. Bulk confirmation is not possible. The DAG remains structurally valid after import.

### 5.7 UC-7: Source Staleness Review

**Personas:** Conductor

**Trigger:** Source documents (e.g., PRD in Confluence) are updated mid-sprint.

`arm sources sync` detects that a source document's fingerprint has changed. The Conductor runs the `arm stale-review` interactive TUI, which walks through each affected node showing the source change summary, the node's citation, and the full rendered context. Per node, the Conductor can confirm the citation is still valid, flag the node for re-decomposition, or skip. Each decision is recorded as a log op with worker attribution.

**Success criteria:** Source changes are detected within one sync cycle. Every affected node is surfaced. The review is per-node (not bulk). Audit trail records who reviewed what.

### 5.8 UC-8: Context Spot-Check

**Personas:** Conductor

**Trigger:** The Conductor wants to verify what an agent will see before it starts working.

The Conductor runs `arm render-context` on any task to see exactly the same context slice the agent receives: the task spec, scope restrictions, acceptance criteria, blocker outcomes, open decisions, and the advisory token budget with truncation status. This is the primary tool for catching bad decompositions before agents waste cycles. It takes under 30 seconds per node.

**Success criteria:** `render-context` output is deterministic (same state → same output). Token budget and truncation status are visible. All seven context layers are present and correctly prioritized.

### 5.9 UC-9: Post-Incident Forensics

**Personas:** Conductor, Wrangler

**Trigger:** A bug is traced to an AI-generated change and the team needs to understand why the agent made a specific decision.

The team uses `arm render-context --at <sha>` to reconstruct the exact context the agent received at the time of the decision. The transition op records the branch name and PR number, bridging ops history (on `_armature`) with code history (on `main`). `arm context-history` shows how the task's context changed over time: which ops modified it, when, and by which worker. If necessary, `arm materialize --exclude-worker` removes a specific worker's ops to diagnose whether they introduced corruption.

**Success criteria:** Any historical context state is reconstructable from the append-only log. Code-ops correlation is bidirectional (ops commit → code PR and code commit → ops task).

### 5.10 UC-10: Verification Gate

**Personas:** The Swarm, Wrangler

**Trigger:** An agent attempts to transition a task to done.

Pre-transition hooks configured in `.armature/config.json` run automatically against the task's scoped files. Required hooks (e.g., tests, typecheck) must pass before the transition is recorded. Optional hooks (e.g., lint) report results but do not block. Exit code semantics distinguish actionable failures (code 1: fix your code) from environment errors (other codes: report the issue and move on). Hooks run in the code worktree; the transition op is recorded in the ops worktree. These are strictly two-phase: verify in code, then record in ops.

**Success criteria:** No task can reach done status without passing required verification hooks. AI agents can distinguish test failures from environment errors. Hook configuration is shared across all workers via the ops branch.

---

## 6. Feature Requirements

Features are prioritized into four tiers. P0 features are required for initial release. P1 features should ship at launch if feasible. P2 features are valuable but not blocking. Deferred features are explicitly out of scope for v1 and designed-for in the data model.

### 6.1 P0 — Must Ship

| Feature | Description | Primary Personas |
|---|---|---|
| `arm init` | Auto-detect branch mode, set up ops worktree, propose verification hooks, install git hooks, run worker-init | Lone Wolf, Gatekeeper, Wrangler |
| Worker identity | UUID generation, repo-local git config, one-worktree-per-worker enforcement, uniqueness validation on first push | All |
| Op log engine | Append-only JSONL with positional arrays, per-worker log files, MRDT merge-conflict-free guarantee, filename-worker-ID validation during materialization, CLI-side rate limiting (heartbeats 1/min/issue, creates 500/commit) | All |
| Materialization engine | Incremental processing from local-only checkpoint (state files never committed), merged-status auto-detection, bottom-up rollup, ready-queue recomputation, cold-start full replay on fresh clone | All |
| Ready task computation | 4-rule gate (open + blockers merged + parent in-progress + claim available), priority sort (explicit > depth > unblock count > age) | The Swarm, Conductor |
| Claim system | Timestamp-based race resolution, heartbeat protocol, configurable TTL, post-claim verification, deterministic tiebreaker | The Swarm |
| Context assembly | 7-layer algorithm with fixed and truncatable sections, advisory token budget (chars/4), deterministic output | The Swarm, Conductor |
| Status transitions | open → claimed → in-progress → done → merged, plus blocked, cancelled, and reverse transitions (done→open via `arm reopen`, blocked→open, claimed→open) | All |
| Decomposition workflow | `decompose-context`, `decompose-apply` (with dry-run), `decompose-revert` (double-entry cancellation) | Conductor, The Swarm |
| SKILL.md contract | Complete AI worker interface: work loop, setup, error recovery, rules and constraints | The Swarm |
| Dual-branch architecture | `_armature` orphan branch for ops, ops worktree with sparse checkout, single-branch fallback | Gatekeeper, Conductor, Wrangler |
| Two-phase completion | done (self-reported) then merged (auto-detected), 4-layer merge detection fallback, downstream unblocking requires merged | Gatekeeper, Conductor |
| Pre-transition hooks | Configurable verification commands, required vs optional gates, exit code semantics, scope interpolation | The Swarm, Wrangler |
| Structured error diagnostics | Every error includes state, hint, and `--debug` flag for full internal state dump | All |

### 6.2 P1 — Should Ship

| Feature | Description | Primary Personas |
|---|---|---|
| Git hook automation | post-commit heartbeat, prepare-commit-msg stamping, post-merge materialization. Convenience only, system correct without them. | Gatekeeper, Conductor |
| DAG governance | `dag-summary` interactive TUI with per-item sign-off, attribution logging, coverage metrics | Conductor |
| Source document management | `sources add/sync/verify`, Confluence and SharePoint providers, aggressive caching on ops branch, fingerprinting | Conductor |
| Brownfield import | Import from CSV/JSON with inferred confidence, per-node confirmation via `arm confirm` | Conductor |
| TUI (Charm ecosystem) | Bubble Tea + Lip Gloss + Glamour for human-interactive commands, semantic color palette | Conductor, Wrangler |
| `arm validate` | Structural integrity checks (cycles, orphans, missing fields, scope overlap), CI-friendly output | Conductor, Wrangler |
| Traceability | Source citations per node, `traceability.json` materialization, coverage metrics in `dag-summary` | Conductor |
| `arm sync` | Explicit ops sync with change summary, `--check` for status without pulling, `--code` for code branch | All |

### 6.3 P2 — Nice to Have (v1)

| Feature | Description | Primary Personas |
|---|---|---|
| Time travel / forensics | `render-context --at <sha>`, `context-history`, selective replay (`--exclude-worker`) | Conductor, Wrangler |
| Sprint bookmarks | Git tags on ops branch marking sprint boundaries for state reconstruction | Conductor |
| `arm metrics` | DAG completion statistics, velocity tracking, context budget utilization | Conductor |
| `arm status` | Dashboard view of all workers, active claims, staleness warnings | Conductor, Wrangler |
| Stale review TUI | Interactive per-node review of source document changes with confirm/flag/skip flow | Conductor |
| Cross-document consistency | Warning when source doc timestamps diverge beyond configurable threshold | Conductor |

### 6.4 Deferred (Designed-For)

| Feature | Rationale for Deferral | Design-For Signal in v1 |
|---|---|---|
| Log compaction | Optimize when materialization performance is measured to be a problem | Compaction algorithm designed, `arm compact` command specified |
| Multi-repo hub topology | Implement when a paying customer or significant user cohort requests cross-repo coordination | Task scope includes optional `repo` field, ops worktree path configurable, task IDs globally unique (UUIDs) |
| Adversarial decomposition | Human gate serves same purpose at lower cost for v1 | Plan file format supports multiple decomposition passes |
| Section-level fingerprinting | File-level is sufficient; section parser complexity not justified | Fingerprint model is extensible to section granularity |
| Formal JSON Schema for plan files | Documented by example; add when a second consumer needs it | Plan format is stable and versioned |

---

## 7. Persona-Driven Feature Matrix

This matrix maps deployment topology and feature availability to each persona's primary deployment mode. It is the authoritative reference for which features apply in which context.

| Feature | Lone Wolf | Gatekeeper | Conductor (Monorepo) | Conductor (Multi-Repo) |
|---|---|---|---|---|
| Branch mode | Single-branch | Dual-branch | Dual-branch | Dual-branch |
| Ops location | `.armature/` on main | `_armature` branch | `_armature` branch | Hub repo (future) |
| Claim races | N/A (one worker) | N/A (one worker) | Full MRDT | Full MRDT |
| Two-phase completion | Optional (no PR gate) | Yes | Yes | Yes |
| Merge detection | Immediate (direct push) | Commit-message scan | Commit-message scan | Cross-repo scan (future) |
| Cross-repo deps | N/A | N/A | N/A (monorepo) | Manual (v1), Hub (future) |
| Git hooks | Optional | Recommended | Recommended | Required (hub config) |
| Workers per repo | 1 | 1 | Many | Many across repos |
| DAG governance | Optional | Optional | Required | Required |
| Source management | Optional | Optional | Recommended | Recommended |
| Verification hooks | Optional | Recommended | Recommended | Required |

---

## 8. Non-Functional Requirements

### 8.1 Performance

The `arm` binary must start in under 5ms. Incremental materialization must process new ops in O(new ops), not O(all ops). Context assembly (`render-context`) must complete in under 100ms for typical DAGs (up to 500 nodes). The ready-task queue must be precomputed during materialization, not computed on demand.

### 8.2 Distribution

Single static Go binary compiled with `CGO_ENABLED=0`. Zero runtime dependencies beyond git (v2.25+ for sparse checkout support, released January 2020). Platform matrix: linux/macOS/Windows across amd64/arm64. Binary size target: 8–12MB uncompressed (4–6MB compressed). Distribution via GitHub releases with per-platform binaries, and as an Agent Skills-compliant skill folder with the binary at `scripts/arm`.

### 8.3 Context Efficiency

Target range: 650–1,600 tokens per task context, measured by the `chars/4` proxy. This represents a 10–20x reduction compared to markdown-based approaches (12,000–21,000+ tokens). The advisory budget is configurable per-repository in `.armature/config.json`. Truncation follows a strict priority order: fixed sections (core spec, snippets) are never removed; truncatable sections are removed lowest-priority-first.

### 8.4 Offline Capability

All core operations must work offline. Workers can append ops to their local log, and ops will propagate when network is restored. Stale claims, heartbeats, and transitions are handled correctly by the existing timestamp-based resolution mechanics. The only operations requiring network are: git push/pull (inherent), `sources add`, `sources sync`, and `sources verify` (provider communication).

### 8.5 Compatibility

Armature must be compatible with every AI coding agent that can invoke a subprocess and parse JSON. The compatibility surface is the CLI contract (commands, arguments, exit codes, JSON output). The initial target list includes Claude Code, Gemini CLI, Cursor, Windsurf, and Kiro. No platform-specific integration is required; the SKILL.md skill file is the universal interface.

### 8.6 Auditability

All mutations to coordination state are recorded as append-only ops in JSONL logs stored in git. Every op includes a timestamp and worker ID. The complete history is preserved in git (even after compaction, history remains in git log). The `dag-summary` sign-off records per-item attribution. The transition op records branch and PR metadata for code-ops correlation.

---

## 9. Scope Boundaries

The following concerns are explicitly out of scope for Armature. These boundaries are intentional design decisions that trade completeness for adoption simplicity and focus.

| Concern | Decision | Rationale |
|---|---|---|
| Malicious actors / tampering | Delegated to git host ACL + protected branches + PR/MR approval | Adding a custom security layer would duplicate what every git host already provides, while adding complexity that deters adoption |
| Semantic correctness of decomposition | Developer is accountable at the `dag-summary` sign-off gate | Automating semantic review requires deep domain understanding that is outside the tool's scope; the human gate is both cheaper and more reliable |
| Context budget enforcement | Advisory only. Hard gates rejected. | Agents that exceed context limits will observe failures empirically; hard gates would require bundling a tokenizer binary and add false precision to a proxy metric |
| Compaction | Deferred until materialization performance is measured | Premature optimization; incremental materialization keeps per-invocation cost at O(new ops) |
| Adversarial decomposition | Deferred. Human gate serves same purpose at lower cost. | Two-agent adversarial review adds implementation complexity with marginal benefit over a skilled human reviewer |
| Multi-repo cross-dependencies | Separate instances per repo (v1). Manual cross-repo tracking. | Covers monorepo and loose-coupling scenarios. Hub-repo topology designed-for but not implemented until demanded. |

---

## 10. Success Metrics

Armature's primary mission is to eliminate context rot without reducing task fidelity. Success is measured across four dimensions: context efficiency, adoption, operational velocity, and output quality. Increased hallucinations or decreased task completion rates are explicit failure signals.

### 10.1 Primary KPIs

| KPI | Target | Measurement Method | Why It Matters |
|---|---|---|---|
| Context tokens per task | 650–1,600 tokens (10–20x reduction vs. markdown) | `arm render-context --format=agent` output, measured via chars/4 proxy across representative DAGs | Context minimization is job #1. Lower token count means less context rot, more headroom for agent reasoning, and lower API cost. |
| Agent platform adoption | Compatible with 5+ major agent platforms within 6 months of launch | Track confirmed compatibility (Claude Code, Gemini CLI, Cursor, Windsurf, Kiro) via integration tests and community reports | Market adoption is the ultimate validation. Armature must work everywhere developers already use AI agents. |
| Sources-to-DAG creation time | Under 15 minutes for a typical PRD (10–20 pages, 50–100 tasks) | Wall-clock time from `arm sources add` to `arm decompose-apply` completion (excludes human review) | Measures whether Armature accelerates converting requirements into structured work. Separating creation from review prevents the per-item sign-off gate from inflating this metric. |
| DAG sign-off time | Under 30 minutes for 50–100 tasks | Wall-clock time for `arm dag-summary` interactive sign-off completion | Measures human review overhead. Story/epic-level batch review (with task-level spot-checks) keeps this feasible for large DAGs. |
| Task completion fidelity | 95%+ of tasks completed by agents meet all acceptance criteria on first attempt | Acceptance criteria pass rate tracked via pre-transition hook results across deployments | Context reduction must not come at the cost of task quality. If agents fail more often with Armature, context assembly is too aggressive. |

### 10.2 Secondary KPIs

| KPI | Target | Why It Matters |
|---|---|---|
| Claim race waste rate | Under 5% of claims result in wasted work | Validates that the MRDT coordination model works in practice with real agent timing |
| Hallucination rate on scoped tasks | Measurably lower than unscoped agent sessions (controlled comparison) | Directly validates the anti-context-rot thesis. If scoped agents hallucinate as much as unscoped ones, the architecture is not working. |
| Context drift per session | Zero drift within a single agent session (deterministic assembly) | Ensures `render-context` produces identical output for identical state, preventing the progressive degradation that defines context rot |
| Time to first task claim (new user) | Under 5 minutes from `arm init` for Lone Wolf persona | Measures adoption friction. The single biggest risk is complexity deterring the first use. |
| Merged detection accuracy | 95%+ of done-to-merged promotions are automatic (no manual `arm merged` needed) | Validates the 4-layer merge detection algorithm. Manual fallback should be rare. |
| DAG coverage ratio | 90%+ of nodes have at least one source citation after decomposition | Measures traceability quality. Uncited nodes represent untraceable requirements. |

### 10.3 Anti-Metrics (Failure Signals)

The following metrics, if they worsen after Armature adoption, indicate a fundamental problem with the product thesis:

**Increased hallucination rate:** If agents hallucinate more with Armature-assembled context than with full-context approaches, the truncation algorithm is removing critical information.

**Lower task completion rate:** If agents complete fewer tasks per session with Armature than without, the overhead of the work loop (claim, heartbeat, verify, transition) exceeds the benefit of structured context.

**Increased merge conflicts:** If teams using Armature experience more merge conflicts than teams without, the scope isolation at decomposition time is insufficient.

---

## 11. Risks & Mitigations

### 11.1 Product Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Adoption friction: dual-branch model confuses solo developers | Medium | High | Auto-detection at init time. Single-branch fallback for unprotected repos. The CLI abstracts all worktree management—developers never interact with `_armature` directly. |
| Governance gate rubber-stamping | High | High | Interactive checklist with per-item acknowledgment, mechanically impossible to bulk-approve. Named attribution in audit log. Cannot be fully closed technically—reviewer incentives are outside tool control. |
| Coverage gap: implicit requirements never tagged | Medium | High | Citation required per node. `dag-summary` names uncited nodes by ID. Reviewer signs off acknowledging gaps. Residual: implicit "everyone knows" requirements remain invisible. |
| Context budget overrun causes silent truncation of critical info | Medium | Medium | Truncation follows strict priority: core spec and snippets are never removed. Decisions outrank notes and siblings. `--raw` flag bypasses truncation. Advisory warning in output. |
| Decomposition prompt template quality is load-bearing | Medium | Medium | Template ships with tested defaults and is overridable per-repo. Bad templates produce bad DAGs, but `dag-summary` catches structural issues and the reviewer catches semantic issues. |
| Agent tools evolve faster than Armature adapts | Medium | Medium | CLI contract (commands + JSON output) is the compatibility surface, not platform-specific integrations. Any tool that can invoke a subprocess and parse JSON works. AGENTS.md convergence reduces fragmentation. |

### 11.2 Technical Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Multi-agent semantic conflict at execution time | Medium | Medium | Scope-overlap detection at decomposition (W1) and at claim time (advisory warning + automatic note ops on overlapping tasks). `decision` log op for design choices with last-write-wins conflict resolution (W8). Detection only—parallel agents can complete conflicting work before conflict surfaces. |
| Squash-merge breaks merge detection | Medium | Low | Commit-message scan (not ancestry) is primary method. `prepare-commit-msg` hook stamps issue IDs. Branch-name and explicit command fallbacks. |
| Token proxy chars/4 inaccurate for code or non-English | Low | Low | Advisory only. Agents that hit real limits observe failure empirically. No tokenizer binary required. |
| Clock skew between workers causes wrong claim winner | Low | Medium | NTP keeps skew under 1s. Millisecond timestamps make races negligible. Lexicographic tiebreaker for identical timestamps. |
| Ops branch force-push or deletion | Low | High | Configure force-push protection separately from PR requirements. Local worktrees retain full history for recovery. |

### 11.3 Residual Risks (Accepted)

After adversarial review, these low-severity findings remain accepted:

| Finding | Severity | Rationale for Acceptance |
|---|---|---|
| Token proxy chars/4 inaccurate for code-heavy or non-English content | Low | Advisory only; real tokens not available without a tokenizer binary |
| Glob-based scope overlap detection is approximate | Low | Go's `filepath.Match` handles standard globs; exotic patterns not supported |
| Plan file schema not formally specified as JSON Schema in v1 | Low | Documented by example; formal schema added when a second consumer needs it |
| `decompose-revert` cancellation ops add noise to the log | Low | Preferable to `git reset --hard` which loses audit trail; noise is bounded |
| Advisory budget truncation order may remove a critical decision | Low | Decisions are priority 3 (above notes and siblings); core spec never truncated; `--raw` bypasses |
| Brownfield import requires per-provider parsers (Jira, Linear) | Low | CSV/JSON are baseline; provider-specific parsers added on demand |
| Cold-start materialization on fresh clone requires full log replay | Low | Under 1 second for 10,000 ops at Go speeds; compaction (deferred) addresses long-term growth |
| Worker impersonation via spoofed log filename | Low | Requires repo push access (git host ACL is primary control); optional worker-registry.json cross-references git committer identity |
| Merge detection coverage degrades without `prepare-commit-msg` hook | Low | W9 validation warning monitors coverage; hook is strongly recommended in dual-branch mode; fallback detection methods remain available |
| Source citation content not machine-verified against source text | Low | Structural validation (E8) confirms source_id exists; citation content verification is reviewer responsibility at dag-summary gate |

---

## 12. Open Questions for Product Review

The following items require product-level decisions that are outside the scope of the architecture document. They should be resolved before or during implementation.

**Pricing and licensing model:** Armature is open source. Is there a commercial layer (hosted hub-repo, enterprise support, SaaS dashboard)? If so, what features gate the commercial tier versus the open-source core?

**Community governance:** Contribution model, release cadence, RFC process for breaking changes to the SKILL.md contract or plan file format.

**Agent platform integration priority:** Which agent tools get first-party SKILL.md testing and documentation? Claude Code and Cursor are the market leaders, but Gemini CLI and Kiro are growing.

**Telemetry and feedback:** Should the CLI include opt-in anonymous telemetry to measure adoption KPIs? If so, what is collected and how is consent managed?

**AGENTS.md convergence:** The Linux Foundation-backed AGENTS.md standard is emerging as the cross-tool configuration format. Should Armature generate an AGENTS.md file as part of `arm init`, or wait for the standard to stabilize?

**Agent Skills distribution standard:** The agentskills.io open standard defines skill folder structure. Confirm the final packaging of `arm` as `scripts/arm` with SKILL.md frontmatter and `references/` subdirectory.

**Compaction trigger threshold:** When compaction is eventually implemented, what heuristic triggers it? After N ops? After N days? By which worker? This is deferred but the decision framework should be outlined.

---

*End of Document*
