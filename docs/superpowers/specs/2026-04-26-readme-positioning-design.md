---
title: README Positioning — Hook, Overview, and Key Features
date: 2026-04-26
status: approved
scope: README.md hook line, overview section, key features list
---

## Context

The current README hook ("Context rot is a memory problem. Armature gives your agents memory.") is weak for developers evaluating Armature against Beads, Taskmaster, Gastown, and similar tools. Every serious competitor makes the same memory claim. The overview buries the strongest differentiators (traceability, MRDT, zero infrastructure) and omits the DAG context model and skills system entirely.

## Target Audience

Developers who are actively evaluating Armature against other agentic task management tools (Beads, Taskmaster, BMAD, Gastown, etc.) — not developers who need to be convinced the problem exists.

## Design Decisions

### Positioning Strategy

Lead with traceability and auditability — the one ground no competitor is standing on. Source citations connecting every agent decision back to originating requirements is architecturally unique. Zero infrastructure and MRDT are strong supporting claims; traceability is the anchor.

Avoid the word "governance" (overloaded in this space). Use: traceability, auditability, audit trail, source citation.

### Hook Line

**Approved:**
> "Every agent decision. Every requirement cited. All of it in git."

Three claims in parallel rhythm: (1) every agent decision is tracked, (2) every requirement is cited — together these deliver the traceability story, (3) "all of it in git" lands the zero-infrastructure punchline. A developer who has bounced off Beads/Dolt feels that last clause immediately.

### Overview (4 paragraphs)

1. **Problem statement** — two compounding problems: context loss between sessions, and AI-generated work with no traceable record back to requirements. Framed as "problems that compound as teams scale" rather than a competitor claim.

2. **Coordination story** — multi-agent conflicts, MRDT, single-writer principle, structural source citations. Paragraph is intentionally narrative; feature bullets add scannable precision. Some overlap with MRDT and Source Traceability bullets is expected in README structure.

3. **Skills system** — ships skills in the agentskills.io format covering every workflow role. No custom prompt engineering required.

4. **Infrastructure punchline** — git-only, JSONL event journals (human-readable, not binary), single Go binary.

### Key Features (5 bullets, ordered by differentiation strength)

1. **Source Traceability** — structural citations, full audit trail, verifiable at sign-off.
2. **DAG-Structured Context** — typed dependency graph, 7-layer deterministic assembly (architecture doc §6), token-budget-driven truncation drops lowest-priority layers first. Token range 650–1,600 is the advisory target from architecture doc §6 ("Token Budget"). Fixed layers (core spec, definition of done, scope, acceptance, context snippets) are never dropped; truncatable layers drop in priority order: sibling outcomes first, then prior notes, open decisions, parent chain, blocker outcomes last. The Approved Copy uses "core task definition, acceptance criteria, and scope" as a reader-friendly paraphrase of these fixed layers — internal architecture terms (definition of done, context snippets) are intentionally omitted from user-facing README text.
3. **Merge-Conflict-Free by Construction** — MRDT + single-writer principle, state derived by replay. Claim scoped to merge conflicts on coordination state (not all coordination problems).
4. **Zero Infrastructure** — git-only, append-only JSONL event journals (human-readable), single Go binary.
5. **Workflow Skills Included** — ships skills in agentskills.io format; usable by any compatible tool.

### agentskills.io Compatibility

Armature ships its workflow skills (planner, coordinator, worker, auditor) authored in the agentskills.io format. Any tool that consumes agentskills.io-compatible skills can use them directly.

## Approved Copy

### Hook

```
"Every agent decision. Every requirement cited. All of it in git."
```

### Overview

Armature is a git-native work orchestration system for AI coding agents. It solves two problems that compound as teams scale: agents that lose context between sessions and forget architectural decisions, and AI-generated work with no traceable record connecting decisions back to the requirements that originated them.

Multiple agents working in the same codebase step on each other, duplicate effort, and produce changes no one can audit after the fact. Armature coordinates them through a typed task DAG with append-only event-sourced logs — merge-conflict-free by construction, because each worker writes exclusively to its own log file. Every claim, transition, and outcome is structurally cited back to its source document.

Armature ships skills in the agentskills.io format covering every role in the workflow — planner, coordinator, worker, and auditor — usable by any compatible tool. Your agents participate immediately, no custom prompt engineering required.

All state lives in git. No database, no server, no daemon. A single Go binary (`arm`) and git are the only requirements.

### Key Features

- **Source Traceability**: Every claim, transition, and agent decision is structurally cited back to the source document that originated the work. Full audit trail, verifiable at sign-off.

- **DAG-Structured Context**: Requirements decompose into a typed dependency graph (epic → story → task). Each agent receives a deterministic context assembly of 650–1,600 tokens using a layered algorithm — core task definition, acceptance criteria, and scope are always preserved; when the token budget is exceeded, lower-priority context (sibling outcomes, prior notes) is dropped first, preserving the highest-signal content.

- **Merge-Conflict-Free by Construction**: Uses Mergeable Replicated Data Types (MRDT) with a single-writer principle — each worker appends only to its own log, no worker ever writes another's. Current state is derived by replay. Merge conflicts on coordination state are architecturally impossible.

- **Zero Infrastructure**: Git-only. No persistent server, no database, no daemon. All coordination state is stored as append-only JSONL event journals — human-readable, not meant to be edited directly, but never locked in a binary format you can't access. A single Go binary (`arm`) and git are the only requirements.

- **Workflow Skills Included**: Ships skills in the agentskills.io format for every workflow role — planner, coordinator, worker, and auditor — usable by any compatible tool.

## Out of Scope

- Full landing page / marketing copy (future work, builds on this)
- Installation section, quickstart, or lower README sections
- Tagline / social copy
