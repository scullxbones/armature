# ADR: Ratify the Armature Constitution

## Status

Accepted

## Context

Armature's invariants — git-native, no daemon, append-only history, merge-conflict-free by construction — existed only as "Key Constraints," §1 of `docs/design/architecture.md`, an 1,800+ line document whose stated audience is implementers and whose surrounding detail drifts from the live tree faster than a small, stable charter should. Two prior planning rounds (the gap analysis and the long-horizon proposals) each had to reconstruct the constraint set by hand before they could reason about new work, because no citable boundary document existed independent of the implementer-facing architecture doc.

The project already has a house convention of recording kills "so they aren't re-litigated." A constitution is that convention promoted to standing policy: a small, versioned, citable document that future proposals, kill rounds, and agent sessions can point at instead of re-deriving or re-arguing.

The chief objection raised against this proposal (Round Three, item 01, red team) was that a values document becomes wallpaper — beautiful, unread, uncited. The answer is a concrete consumption mechanism: a required "Principles touched" field on every ADR, referencing invariant/tripwire/non-goal IDs by number, enforced by a lint check. That field rides on an ADR template that does not exist in this repository yet; `docs/design/top-tier-gap-analysis.md`'s D5.2 ("ADR hygiene: renumber/rename the stray ADR; add an ADR template with status field") is the item that creates it. `docs/design/next-work-sequencing.md` was updated in the same planning session to pull GAP·D5 forward from Tier C to Tier S, ahead of this Constitution, specifically so the template exists before the "Principles touched" field needs to attach to it. Until GAP·D5.2 lands, this field and its lint check are disclosed as not-yet-enforceable — adjacency and gaps are disclosed, not hidden, per house convention.

## Decision

`CONSTITUTION.md` is created at the repo root as the sole source of truth for Armature's invariants, non-goals, and tripwires, and its full text is inlined into `AGENTS.md` (not merely linked), so every agent session's standing context includes it without an extra hop.

The document has three parts, each independently and permanently numbered (IDs are never reused, even if an entry is later amended):

- **Invariants (I1–I7):** git-native (I1, collapsing the prior separate "no daemon" language — no database, no server, no daemon is one invariant); append-only (I2); merge-conflict-free by construction (I3); agents are the primary users (I4) — a design-target claim about who defaults and docs optimize for, not an accountability claim; deterministic gates decide, LLM judgment is advisory only and scoped to automated behavior (I5) — it does not constrain human override, which is a permissions question, not a constitution invariant; `done` ≠ `merged` (I6); humans are accountable (I7) — a deliberately sweeping claim that Armature, agentic harnesses, and the LLMs they run are tools, and that accountability for merges, releases, and overrides never transfers to the system regardless of how reliably it functions or how much of the workflow agents drive.
- **Non-goals (N1–N5):** not a CI system, not an agent framework, not a chat bus, not a merge authority, not a human-first PM suite.
- **Tripwires (T1–T3):** needs a long-running process; rewrites history; syncs bidirectionally with a mutable remote. A future proposal that trips one of these dies on contact — citable by number, not re-argued.

`docs/design/architecture.md`'s former "Key Constraints" section (§1) is reduced to implementation-only constraints (single binary, zero external deps) plus a one-line pointer to `CONSTITUTION.md` for invariants. Architecture.md may still elaborate on *how* an invariant is implemented; it never again re-states *what* an invariant is, removing the two-document drift this ADR exists to close.

**Amendment process:** any change to an Invariant's, Non-goal's, or Tripwire's *meaning* requires a new ADR in the existing `docs/adr/` sequence and bumps `CONSTITUTION.md`'s version (a plain incrementing integer, not semver). Wording-only fixes that don't change meaning do not require an ADR. The amending ADR must show the resulting file stays near its ~2KB target; this is a conciseness guideline the amendment's author is expected to honor, not a hard byte gate that blocks ratification.

**Consumption mechanism:** once GAP·D5.2 lands, every ADR gains a required, non-empty "Principles touched" field (invariant/tripwire/non-goal IDs, or the literal value `none`), and a lint check fails CI if the field is empty. This ADR ships without that field populated on itself, since the template doesn't exist yet at ratification time — a disclosed, temporary gap, not a silent one.

## Consequences

`CONSTITUTION.md` and its inlined copy in `AGENTS.md` must be kept in sync by hand until/unless a build step generates one from the other; today's amendment process (one ADR touches both) makes drift a review-time catch, not a structural guarantee. `architecture.md` loses standing as the citable source for invariants — any doc, skill, or ADR currently paraphrasing "git-native" or "merge-conflict-free" style language should, going forward, cite `CONSTITUTION.md` by invariant ID instead. The Constitution is sequenced after GAP·D5 in `docs/design/next-work-sequencing.md`; issue decomposition (`/to-issues` via `/armature-planner`) must preserve GAP·D5 (specifically D5.2) as a prerequisite of this Constitution's "Principles touched" lint work, not an unordered sibling.
