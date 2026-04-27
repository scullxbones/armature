# README Positioning Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the README hook line, overview section, and Key Features list with the approved copy from the positioning spec.

**Architecture:** Pure content edit to `README.md`. Three sections change: the hook quote (line 5), the Overview section (lines 9–15), and the Key Features list (lines 17–23). All other sections (Installation, Quickstart, License) are untouched.

**Spec:** `docs/superpowers/specs/2026-04-26-readme-positioning-design.md`

**Tech Stack:** Markdown, git

---

## Chunk 1: README Updates

### Task 1: Update the hook line

**Files:**
- Modify: `README.md:5`

- [ ] **Step 1: Replace the hook quote**

Open `README.md`. Find line 5:

```markdown
> "Context rot is a memory problem. Armature gives your agents memory."
```

Replace with:

```markdown
> "Every agent decision. Every requirement cited. All of it in git."
```

- [ ] **Step 2: Verify the change**

Run:
```bash
grep -n "Every agent decision" README.md
```
Expected: `5:> "Every agent decision. Every requirement cited. All of it in git."`

---

### Task 2: Update the Overview section

**Files:**
- Modify: `README.md:9–15`

- [ ] **Step 1: Replace the Overview content**

Find the entire Overview section body (between `## Overview` and `## Key Features`):

```markdown
Armature is a file-based, git-native work orchestration system that gives AI coding agents persistent memory. It enables humans and AI workers to coordinate on software projects without merge conflicts, external dependencies, or context rot.

AI coding agents today suffer from a fundamental architectural flaw: they forget everything between sessions. When multiple agents work in the same codebase, they step on each other with no coordination primitive to prevent conflicts. Armature solves this by treating context rot as a memory problem, providing deterministic context assembly and append-only event-sourced logs for every decision, claim, and outcome.

All state lives in git. No database, no server, no daemon. A single Go binary (`arm`) and git are the only requirements.
```

Replace with:

```markdown
Armature is a git-native work orchestration system for AI coding agents. It solves two problems that compound as teams scale: agents that lose context between sessions and forget architectural decisions, and AI-generated work with no traceable record connecting decisions back to the requirements that originated them.

Multiple agents working in the same codebase step on each other, duplicate effort, and produce changes no one can audit after the fact. Armature coordinates them through a typed task DAG with append-only event-sourced logs — merge-conflict-free by construction, because each worker writes exclusively to its own log file. Every claim, transition, and outcome is structurally cited back to its source document.

Armature ships skills in the agentskills.io format covering every role in the workflow — planner, coordinator, worker, and auditor — usable by any compatible tool. Your agents participate immediately, no custom prompt engineering required.

All state lives in git. No database, no server, no daemon. A single Go binary (`arm`) and git are the only requirements.
```

- [ ] **Step 2: Verify paragraph count**

Run:
```bash
awk '/^## Overview/,/^## Key Features/' README.md
```
Expected: 4 body paragraphs between the two headings.

---

### Task 3: Update the Key Features section

**Files:**
- Modify: `README.md:17–23`

- [ ] **Step 1: Replace the Key Features list**

Find the existing five bullets under `## Key Features`:

```markdown
- **Zero Infrastructure**: Git-only. No persistent server processes or cloud dependencies.
- **Merge-Conflict-Free**: Uses Mergeable Replicated Data Types (MRDT) to ensure conflict-free coordination by construction.
- **Cross-Platform**: Compatible with all major AI coding agents (Claude Code, Cursor, Windsurf, Gemini CLI, Kiro).
- **Deterministic Context**: Assemblies minimal-token context (650–1,600 tokens) to minimize context rot and API costs.
- **Enterprise Traceability**: Structural source citations with a full audit trail of every decision.
```

Replace with:

```markdown
- **Source Traceability**: Every claim, transition, and agent decision is structurally cited back to the source document that originated the work. Full audit trail, verifiable at sign-off.

- **DAG-Structured Context**: Requirements decompose into a typed dependency graph (epic → story → task). Each agent receives a deterministic context assembly of 650–1,600 tokens using a layered algorithm — core task definition, acceptance criteria, and scope are always preserved; when the token budget is exceeded, lower-priority context (sibling outcomes, prior notes) is dropped first, preserving the highest-signal content.

- **Merge-Conflict-Free by Construction**: Uses Mergeable Replicated Data Types (MRDT) with a single-writer principle — each worker appends only to its own log, no worker ever writes another's. Current state is derived by replay. Merge conflicts on coordination state are architecturally impossible.

- **Zero Infrastructure**: Git-only. No persistent server, no database, no daemon. All coordination state is stored as append-only JSONL event journals — human-readable, not meant to be edited directly, but never locked in a binary format you can't access. A single Go binary (`arm`) and git are the only requirements.

- **Workflow Skills Included**: Ships skills in the agentskills.io format for every workflow role — planner, coordinator, worker, and auditor — usable by any compatible tool.
```

- [ ] **Step 2: Verify bullet order and count**

Run:
```bash
grep -n "^\- \*\*" README.md
```
Expected output (5 bullets in this order):
```
- **Source Traceability**
- **DAG-Structured Context**
- **Merge-Conflict-Free by Construction**
- **Zero Infrastructure**
- **Workflow Skills Included**
```

- [ ] **Step 3: Verify no unintended changes below Key Features**

Run:
```bash
grep -n "Installation\|Prerequisites\|Building from Source\|5-Minute Quickstart\|License" README.md
```
Expected: all five headings present somewhere in the file (line numbers will shift slightly due to the Overview growing by one paragraph — that's expected).

---

### Task 4: Commit

- [ ] **Step 1: Stage and review the diff**

```bash
git diff README.md
```
Confirm only lines 5 and 9–23 changed. Nothing below `## Installation` should be touched.

- [ ] **Step 2: Commit**

```bash
git add README.md docs/superpowers/specs/2026-04-26-readme-positioning-design.md docs/superpowers/plans/2026-04-26-readme-positioning.md
git commit -m "docs: update README hook, overview, and key features with new positioning

Lead with source traceability and auditability as primary differentiators.
Rewrites overview to address the evaluator audience comparing against Beads,
Taskmaster, and similar tools. Adds agentskills.io skills mention. Reorders
key features with Source Traceability first."
```
