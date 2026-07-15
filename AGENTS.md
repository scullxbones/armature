# Armature — Agent Setup

Armature is a git-native work orchestration system: state is append-only ops materialized into task state under `.armature/`, with no external DB or server.

Go project. Build with `make`, not raw `go build`.

## Constitution

**Version:** 1 — ratified by ADR 0009. Amendments require a new ADR; wording-only fixes don't. See `docs/adr/0009-ratify-the-armature-constitution.md` and the full text in `CONSTITUTION.md`.

**Invariants** (we will / we will never):
- **I1 — Git-native.** All state lives in git. No database, no server, no daemon.
- **I2 — Append-only.** History is never rewritten.
- **I3 — Merge-conflict-free by construction.** Each worker writes exclusively to its own log file.
- **I4 — Agents are the primary users.** Defaults, docs, and UX optimize for agent consumption first.
- **I5 — Deterministic gates decide.** LLM judgment is advisory only, never an automated merge decision.
- **I6 — `done` ≠ `merged`.** Self-reported completion and confirmed-on-main are distinct states.
- **I7 — Humans are accountable.** Armature, harnesses, and LLMs are tools; humans own the outcome. Accountability never transfers to the system, however reliably it functions.

**Non-goals:** not a CI system (N1); not an agent framework (N2); not a chat bus (N3); not a merge authority (N4); not a human-first PM suite (N5).

**Tripwires** — dies on contact, cite don't re-argue: needs a long-running process (T1); rewrites history (T2); syncs bidirectionally with a mutable remote (T3).

## First-time setup

```bash
make install                                 # build → ~/.local/bin/arm
arm bootstrap                                # init repo + deploy skills to .claude/skills/, .gemini/skills/, .codex/skills/
arm worker-init --check || arm worker-init   # register worker identity once per clone (don't rerun without --check)
```

## Details

- [Workflow & operating model](docs/agents/workflow.md)
- [Quality gates — TDD, `make check`, coverage/mutation thresholds](docs/agents/quality-gates.md)
- [Dogfood findings capture](docs/agents/dogfood-findings.md)
- [Repo-local skills](docs/agents/skills.md)

## Canonical references

- `docs/conventions.md` — test naming, commit format, branch naming conventions (required reading for workers)
- `docs/commands.md` — CLI surface
- `docs/harness-hook.md` — harness integration
- `docs/design/architecture.md` — architecture and repo model
- `CONTEXT.md` — domain glossary
