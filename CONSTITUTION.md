# Armature Constitution

**Version:** 1 — ratified by ADR 0009. Amendments require a new ADR; wording-only fixes to an entry's phrasing (not its meaning) don't. See `docs/adr/0009-ratify-the-armature-constitution.md`.

Target length: ~2KB. This is a guideline, not a gate — favor conciseness over grammatical polish when the two trade off.

## Invariants

We will / we will never:

- **I1 — Git-native.** All state lives in git. No database, no server, no daemon.
- **I2 — Append-only.** History is never rewritten.
- **I3 — Merge-conflict-free by construction.** Each worker writes exclusively to its own log file.
- **I4 — Agents are the primary users.** Defaults, documentation, and UX design optimize for agent consumption first.
- **I5 — Deterministic gates decide.** LLM judgment is advisory input only, never an automated merge decision.
- **I6 — `done` ≠ `merged`.** Self-reported completion and confirmed-on-main are distinct states.
- **I7 — Humans are accountable.** Armature, agentic harnesses, and the LLMs they run are tools. Humans own the outcome of the work performed through them. Accountability for merges, releases, and overrides never transfers to the system — no matter how reliably it functions, and no matter how much of the day-to-day workflow agents drive.

## Non-goals

Armature is not:

- **N1** — a CI system.
- **N2** — an agent framework.
- **N3** — a chat bus.
- **N4** — a merge authority.
- **N5** — a human-first PM suite.

## Tripwires

A proposal that does any of the following dies on contact — cite the tripwire, don't re-argue it:

- **T1** — needs a long-running process.
- **T2** — rewrites history.
- **T3** — syncs bidirectionally with a mutable remote.
