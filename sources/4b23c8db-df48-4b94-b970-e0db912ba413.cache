# Narrow Gaps Addendum

**Date:** 2026-07-07
**Purpose:** Six gaps identified as missing from all three prior planning rounds — `docs/design/top-tier-gap-analysis.md` (GAP), `docs/design/long-horizon-proposals.md` (LH), and `docs/design/the-next-ten.html` (Round Three). This document is the citation source for a new tranche of TOPTIER stories closing them.

**Grounding:** the three prior planning documents, the dogfood findings corpus (`docs/dogfood/findings/`), and the live `arm` CLI surface as of this date. Each gap below states explicitly what existing proposals it borders and why it is distinct from them, following the "adjacency disclosed, never hidden" rule Round Three established.

---

## G1. Real dollar/token spend observability

**Gap:** Context Economics (Next-Ten №03) budgets artifact *byte-weight* — skill size, rendered-context size. `arm stats` (LH F5) measures latency and rework rate. Nothing in any of the three documents tracks actual LLM API spend per story, per worker, or per wave.

**Why it matters:** Once a team runs a real agent fleet against Armature, the number they will ask for first is "what did this story cost in tokens/dollars," not "how many bytes did the coordinator skill weigh." The two are correlated but not the same metric, and neither existing proposal computes the second.

**Incremental changes:**
- **G1.1** Instrument dispatch points (coordinator/worker/reviewer prompts) to record token counts (input/output) as structured fields alongside existing ops, without requiring a new op type — piggyback on the outcome/assessment records already written at `done`/review time.
- **G1.2** Add a `--cost` view to `arm stats` (or a standalone `arm cost`) that aggregates recorded token counts into per-story and per-wave dollar estimates using a configurable per-model rate table.
- **G1.3** Surface a running total in `arm show <story>` so a coordinator can see spend-to-date before deciding whether to keep dispatching waves.

## G2. Backup / disaster recovery for the ops branch itself

**Gap:** Ops compaction (LH C2) and op-schema versioning (GAP T5.2) protect the ops log's *format* and *growth*. Nothing addresses "origin is lost, corrupted, or force-pushed over" for the branch that is the system's single source of truth.

**Why it matters:** Every other proposal — compaction, versioning, recovery state machines (GAP T3) — assumes the ops branch survives. None of them define what happens if it doesn't, and an event-sourced system with no replication story has a single point of total, silent failure.

**Incremental changes:**
- **G2.1** Document the current failure mode explicitly: what happens today if the ops branch is deleted or corrupted (a "what we don't guarantee" doctor note, in the spirit of GAP T4.3's honest-guarantees approach for the harness hook).
- **G2.2** Add a `doctor` check that warns when the ops branch has no verified remote tracking branch, or when local-only ops exist beyond a configurable staleness threshold.
- **G2.3** Document a manual backup recipe (e.g., periodic `git bundle` of the ops branch, or a second remote) as a stopgap ahead of any automated solution.

## G3. Reviewer disagreement / consensus policy

**Gap:** Reviewer self-validation (LH C6) and model-tier dispatch policy (LH C8) both improve the quality of a *single* reviewer's output. Neither, nor the `reviewer-agent-reliability` dogfood theme, addresses what happens when two independently-dispatched reviewers rate the same delivery differently.

**Why it matters:** The dogfood corpus's `effective-patterns` theme documents heavy reliance on a fix→broad-review→implement cascade (a *second* reviewer catching what the first missed) as the validated pattern — but that is sequential escalation, not peer consensus on a disputed rating. As multi-reviewer dispatch becomes more common (parallel haiku reviewers, the tiered dispatch policy in C8), rating disagreement between peers is a case none of the existing proposals resolve.

**Incremental changes:**
- **G3.1** Define what `arm review record` does when two assessments exist for the same delivery fingerprint with different ratings — today behavior is undefined/order-dependent.
- **G3.2** Add an escalation rule: on disagreement, either the higher-severity rating wins by default, or a designated tie-break reviewer (per LH C8's tier policy) is dispatched automatically.
- **G3.3** Record disagreement events distinctly (not just the final rating) so the dogfood findings pipeline can measure how often reviewers actually disagree — currently invisible.

## G4. A supported extensibility seam

**Gap:** The Subtractive Release (Next-Ten №02) is about *removing* unused surface. Nothing in any of the three documents proposes a supported way for an adopter to add a custom issue type, a custom validator, or a custom hook trigger without forking the codebase.

**Why it matters:** Once external adopters exist (the explicit trigger condition for several deferred items like the Second Substrate and multi-repo federation), some will have domain-specific issue taxonomies or validation rules Armature can't anticipate. Today the only extension point is editing Go source.

**Incremental changes:**
- **G4.1** Identify the narrowest seam first: a registry pattern for issue-type validation rules (parallel to the existing `type` enum) that an adopter can extend via config rather than a source change.
- **G4.2** Document explicitly, in the Armature Constitution (Next-Ten №01) once it exists, what is and isn't intended to be extensible — this is itself a constitution-shaped decision, not just an engineering one.
- **G4.3** Defer actual plugin *execution* (arbitrary third-party code running inside the CLI) as a non-goal unless a concrete adopter need materializes — this closely borders the "long-running process" tripwire the constitution proposes, and should be evaluated against it explicitly rather than built first.

## G5. Human-newcomer onboarding diagnostics

**Gap:** The agent-grade error contract (LH C3) is explicitly agent-facing. The Paved Road (Next-Ten №04) and quickstart rewrite (GAP D1.1) define and document the blessed *command* sequence. Nothing targets a human unfamiliar with git worktrees hitting the exact friction the `git-worktree-friction` dogfood theme documents for agents constantly (gopls false positives, checked-out-branch failures, worktree changes leaking into the main worktree).

**Why it matters:** Every fix in `git-worktree-friction` and its incremental changes (documenting the checked-out-branch restriction, investigating `.worktrees/` placement) helps an agent that reads the skill prose. A human onboarding without reading skill internals has no equivalent guided diagnostic today.

**Incremental changes:**
- **G5.1** Add a `doctor --explain` (or a `--human` output mode on existing `doctor`/`validate`) that renders detected worktree/config problems as a guided narrative with a suggested fix command, rather than the terse machine-oriented output built for C3.
- **G5.2** Extend the quickstart (GAP D1.1) with a "if something goes wrong" troubleshooting appendix covering the specific failure classes the git-worktree-friction theme has already catalogued.

## G6. Authorship / copyright clarity for agent-authored commits

**Gap:** The Strategy Memo's (Next-Ten №09) pre-mortem lists model-behavior drift, bus factor, harness API churn, context-price collapse, and category consolidation as risks. None of the three documents addresses the legal-attribution question for commits an autonomous agent authors and a human merges — distinct from the killed "trust & authorship model for multi-writer ops" item, which was about *operational* trust between writers in the ops log, not *legal* copyright/authorship of the resulting code.

**Why it matters:** This is a real and increasingly common blocker for OSS adoption of agent-driven tools, independent of whether the ops-writer trust model (deferred until adversarial writers exist) is ever built.

**Incremental changes:**
- **G6.1** Add a short, explicit statement to CONTRIBUTING.md (GAP D4.1) or a dedicated `AUTHORSHIP.md` on how agent-authored commits are attributed (commit trailer convention, co-author lines) and what license terms apply to agent-generated contributions.
- **G6.2** Note this as a named risk with a tripwire in the Strategy Memo's pre-mortem table (Next-Ten №09) once that memo is written, rather than leaving it undocumented.

---

## Sequencing note

These six gaps are independent of each other and of the existing TOPTIER stories — none blocks or is blocked by S1–S10. G2 (ops-branch backup) and G6 (authorship) are the cheapest to act on immediately (a doctor check and a CONTRIBUTING.md paragraph, respectively). G1 (cost observability) and G3 (reviewer consensus) have the highest leverage once a real multi-worker fleet is running against this repo at volume. G4 (extensibility) and G5 (human onboarding) are lowest urgency until external adopters exist, consistent with the "zero adopters" timing argument the prior three rounds already applied to several deferred items.
