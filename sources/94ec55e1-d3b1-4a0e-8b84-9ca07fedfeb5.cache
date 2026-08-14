# Long-Horizon Improvement Proposals

**Date:** 2026-07-07
**Purpose:** Twenty long-term changes that would materially raise Armature's ceiling, complementing (never duplicating) the short-term work in `top-tier-gap-analysis.md`. Five new features, five deprecations/tech-debt removals, and ten open-category proposals selected by rubric and adversarial review.

**Grounding:** CONTEXT.md glossary, docs/design/architecture.md, the dogfood findings corpus (all eight theme READMEs plus raw findings through 2026-07-05), ADRs 0001–0008, the live CLI surface (`arm --help`), config/hook implementation (`internal/config`, `internal/hooks`, use-cases.md §P4), legacy-shim census (`internal/harnesshook/platform_codex.go`, `platform_devin.go`, `internal/ops/opstream.go:72-108`, `internal/validate/validate.go:477`), and the actual `.arm/.armature/ops/` directory contents.

**Exclusions:** Everything already proposed in the gap analysis (T1–T5, D1–D5 there) is out of bounds. Where a proposal is adjacent to a gap-analysis item, the distinction is stated explicitly.

---

## Method

A pool of ~35 candidates was generated from the dogfood themes, the architecture doc's stated constraints, the code census, and the market analysis. Each survived (or died in) three adversarial rounds:

1. **Duplication round** — is this already in the gap analysis, or an existing command? Kills anything the TOPTIER epic will already deliver.
2. **Constraint round** — does it violate Armature's own architecture (git-native, zero infrastructure, no daemon, merge-conflict-free by construction)? Does YAGNI apply at v0.0.1?
3. **Rot round** — will this become maintenance debt faster than it produces value? Who maintains it in year two?

**Notable kills** (recorded so they aren't re-litigated):

- *GitHub Issues bidirectional bridge* — died in rounds 2 and 3: a sync engine against a mutable remote contradicts merge-conflict-free-by-construction, and bidirectional sync is a maintenance tarpit. One-way export may ride along with `arm stats` later.
- *Multi-repo federation* — architecture explicitly defers this (v1 = separate instances); premature at v0.0.1 with zero external adopters.
- *`arm undo` / compensating ops* — `decompose-revert`, `reopen`, and `transition` already cover the real cases; generic inverse-op machinery is speculative.
- *`arm next` (atomic claim+render)* — pure sugar; it does not actually shrink the claim-race window because races are resolved at materialization, not claim time.
- *`arm simulate` scenario sandbox* — collapsed into gap-analysis T2 (the e2e harness is the simulation sandbox); anything more is duplication.

The ten open-category survivors were scored on a six-axis rubric (§Creative below).

---

## Part 1 — New Features (5)

### F1. Managed worktree lifecycle: `arm worktree` integrated with claim

**What:** Armature creates, tracks, and garbage-collects worker worktrees itself. `arm claim <id> --new-worktree` provisions a worktree at a configured root (default `.worktrees/<issue-id>`), checked out detached from the target base to avoid the "branch already checked out" failure, records the worktree path *in the claim op*, and installs project-type mitigations (e.g. `go.work` exclusion for gopls). `arm worktree list|gc` reconciles worktrees against claim state and removes them when the issue reaches `merged`/`cancelled`.

**Why long-term:** An entire dogfood theme (`git-worktree-friction`) plus half of `session-recovery-gaps` is downstream of the fact that `--worktree` is mandatory but worktrees are entirely the agent's problem: lost `/tmp` worktrees after session death, checkout-race contamination, gopls diagnostic bleed, worktree changes leaking into the main worktree. Owning the lifecycle removes the whole class rather than documenting around it.

**Gap-analysis adjacency:** T3.2 reconciles *unbound* worktrees during recovery; this proposal makes worktrees a first-class managed resource so there is far less to reconcile. Recording the path in the claim op is also what makes T3's recovery deterministic.

#### F1.1 — `arm claim --from`: Armature-owned branch creation for sub-task worktrees *(follow-on, added 2026-08-11)*

**What:** `arm claim <id> --worktree <new-path> --from <parent-worktree-path>` cuts the new task branch live from the parent worktree's current branch, persisting the same parent-branch config and base-commit metadata the fresh-worktree claim path already writes. `--from` is accepted only when `<new-path>` does not yet exist; the parent resolves as an existing worktree and its current branch serves both as the ref to branch from and as the recorded parent. No merge-base computation is needed, since the branch is cut from the parent's tip at that moment.

**Why this is a follow-on and not part of F1 proper:** F1 as originally written provisions a worktree *from the target base*, which is the right behavior for a task dispatched off `main`. It does not cover the case where a sub-task is cut from an already-open story worktree — there, the parent's branch has commits the base does not, and provisioning from base silently drops them. Today that case is worked around by pre-creating the branch by hand and having Armature adopt it after the fact, which defeats F1's central claim that worktrees are an Armature-managed resource: the one piece of provenance that matters (what this branch was actually cut from) is exactly the piece Armature did not write. Tier-1 dynamic base-commit resolution therefore cannot answer for sub-tasks.

**Provenance:** this was identified during `LNGHZN-S5`'s PR #89 review (thread `claim.go:666`), not in the original long-horizon round. It is recorded here so `LNGHZN-S9` has a citable source ID in the same shape as every other item, and so F1 is not read as complete while the sub-task path is still unmanaged. Sequenced as item 14a in `docs/design/next-work-sequencing.md`.

### F2. Scope-disjoint wave planning: `arm ready --waves`

**What:** `arm ready` gains a planning mode that partitions the ready queue into dispatch waves whose members have pairwise-disjoint scopes (and no shared `blocked_by` frontier), emitting the wave structure in `--format agent`. A companion check warns at claim time when a to-be-claimed issue's scope intersects an actively claimed issue's scope.

**Why long-term:** The `parallel-coordination-conflicts` theme documents parallel branches silently semantically reverting each other with clean git merges — the worst failure mode a coordination system can have, because git reports success. Armature already holds the only data (declared scopes + claim state) that can prevent it *before dispatch*. Today this logic lives, badly, in coordinator-skill prose; moving scheduling into the engine is the same compiled-and-testable philosophy as gap-analysis T1.3 but is a new capability, not a rewrite of documented pseudocode.

**Gap-analysis adjacency:** none — T1.3 replaces existing skill pseudocode (commit discovery); wave computation exists nowhere today.

### F3. Event stream: `arm events --follow`

**What:** A read-side event API: replaying ops emits a normalized JSONL event stream (`issue.claimed`, `issue.transitioned`, `review.recorded`, `claim.expired`, …) with `--follow` tailing new ops via the same fsnotify machinery the TUI already uses internally (`internal/tui/app/model.go`). No daemon; it is a foreground subscription over files, consistent with the zero-infrastructure constraint.

**Why long-term:** Every consumer today polls (`arm list --group` in a loop) or gets nothing. An event stream gives coordinators a "wave finished" signal, gives CI and chat integrations a subscription point, and — critically — provides the event vocabulary that the hooks redesign (D2 below) filters on. It converts the ops log from a private storage format into a public integration surface.

### F4. Time-travel state: `arm show --as-of` and `arm state-diff`

**What:** Point-in-time materialization: `arm show <id> --as-of 2026-07-05T14:00Z` replays ops up to a timestamp (or op offset) and renders the issue as it then stood; `arm state-diff --from <t1> --to <t2>` summarizes what changed between two instants (claims gained/lost, transitions, links).

**Why long-term:** Event sourcing makes this nearly free — the materializer already replays ops incrementally — and it is exactly the tool every session-recovery incident needed: "what did the world look like when the session died, and what changed since?" Today that forensic work means reading raw JSONL by hand. No issue tracker built on mutable rows can offer this; it is the clearest product expression of the append-only architecture and a genuine differentiator.

### F5. Flow analytics: `arm stats`

**What:** Derived metrics computed from the ops log: claim→done and done→merged latency distributions, rework rate (red/yellow conformance assessments per issue), claim-expiry frequency, wave throughput, per-worker activity. Output in human and `--format json` for dashboards.

**Why long-term:** The ops log is a complete, trustworthy event record and *nothing reads it analytically*. The Conductor persona's monitoring story today is `arm list --group` and squinting. Stats close the improvement loop the project already runs manually through dogfood curation (e.g. "reviewer tier X produces N× more schema-invalid assessments") and become the adoption hook for team leads once external users exist.

---

## Part 2 — Deprecations and Tech-Debt Removals (5)

### D1. Make configuration honest: wire or delete every dead knob

**What:** `.armature/config.json` currently contains fields whose documented behavior is "does not do what you'd expect": `default_ttl` does not govern the `arm claim --ttl` default; `token_budget` is not read by standalone `render-context`; `low_stakes_push_threshold` is "a coalescing hint, not an auto-push trigger" (use-cases.md §P4 documents all three caveats). Decide, per field: wire it as the single source of the default, or delete it. Add strict decode (unknown fields rejected) and a `doctor` check so the config file can never silently lie again.

**Why:** A config file that requires footnotes explaining which settings are inert is worse than no config file — it actively teaches operators wrong mental models, and the Wrangler persona's whole job is tuning these knobs. This debt compounds with every new setting added.

### D2. Redesign transition hooks; deprecate unconditional-fire semantics

**What:** Today every configured hook runs on *every* `arm transition`, unfiltered; the `name` field is a label only; `required` is documented as having "no effect in the current implementation" (use-cases.md §P4). Replace with event-filtered hooks — each entry declares `on:` triggers drawn from the F3 event vocabulary — with `required` actually governing block-vs-warn. Old-format configs get one migration release with a loud deprecation warning, then the old semantics are removed.

**Why:** The current design guarantees that anyone who configures a Slack notification also accidentally installs a transition-blocking gate on every state change. Shipping a hooks subsystem where half the schema is inert is debt at the exact integration boundary adopters touch first.

### D3. Adopt a shim-retirement policy and delete the current legacy carve-outs

**What:** Three back-compat shims already live in a v0.0.1 codebase: recognition/migration of pre-marker codex and devin harness configs (`internal/harnesshook/platform_codex.go`, `platform_devin.go`), acceptance of legacy base worker IDs for slotted ops logs (`internal/ops/opstream.go:72-108`), and parsing of legacy comma-separated scope entries (`internal/validate/validate.go:477`). Write a one-paragraph deprecation policy (shims live N minor versions, migration happens in `bootstrap`/`doctor --fix`, then deletion), apply it retroactively: migrate on next bootstrap, delete the shims at the version after.

**Why:** Pre-1.0 with zero external adopters is the cheapest deletion window this project will ever have. Without a stated policy, every shim is immortal — and the harness-hook file-recognition shims in particular are subtle string-matching code in the security-relevant path.

**Gap-analysis adjacency:** T5.2 defines *forward* compatibility (versioned op records, replay fixtures). This is the reverse edge: a rule for when *backward* carve-outs die. The two together form the complete compatibility contract.

### D4. One merged-promotion path; deprecate the rest

**What:** `done → merged` promotion currently has three overlapping mechanisms: manual `arm merged --issue`, `arm sync` auto-detection, and passive detection during `arm list` "via commit-message scan" (concepts.md §7) — and the docs contradict each other about which to use (use-cases P2: "Never use `--to merged` manually"; use-cases P5: "run `arm merged --issue <id>`"). Consolidate: detection by git ancestry/branch containment (robust to squash merges and reworded titles, which break commit-message grep), exposed through exactly one verb (`arm sync`); `arm merged` becomes a thin alias for one release, then is removed; `arm list` stops mutating state as a side effect.

**Why:** Merged-promotion is the load-bearing gate of the two-phase completion model — the thing that keeps agents from building on unshipped code. Three code paths with different detection heuristics means the most important invariant in the system has the least predictable trigger. A list command that silently writes transition ops is also a correctness landmine for read-only tooling.

### D5. Collapse the `.arm/.armature/` double-dotdir before v0.1.0

**What:** Coordination state today lives at `.arm/.armature/ops/…` — an ops *worktree* named `.arm/` containing a state *directory* named `.armature/`, two near-identical names with different meanings. Pick one visible name (worktree at `.armature/`, state at its root; or keep `.arm/` and drop the inner rename), migrate via `bootstrap`, and delete the dual-name handling.

**Why:** This is residue of the Trellis→Armature rename, and it is the confusion engine behind the README drift the gap analysis documents (its D1 fixes the *description*; this fixes the *thing described*). Every skill, doc, and error message currently has two chances to name the wrong path. It is a breaking layout change, which is precisely why it must happen before the first real release rather than after — this proposal has an expiry date.

---

## Part 3 — Open Category: Rubric-Selected Proposals (10)

### Rubric

Each candidate scored 1–5 on six axes; survivors needed ≥24/30 *and* to survive the three adversarial rounds:

| Axis | Question |
|---|---|
| **Leverage** | How many workflows/personas improve at once? |
| **Evidence** | Is it grounded in observed dogfood failures or architectural fact, not speculation? |
| **Differentiation** | Does it exploit the event-sourced, git-native design in a way competitors can't cheaply copy? |
| **Agent-first** | Does it reduce agent error rates or context burn specifically? |
| **Durability** | Does the value grow (or at least hold) as the project scales? |
| **Cost inverse** | Low implementation + maintenance burden scores high. |

| # | Proposal | Lev | Evi | Dif | Agt | Dur | Cost | Σ |
|---|---|---|---|---|---|---|---|---|
| C1 | Autonomic heartbeats via harness hook | 5 | 5 | 4 | 5 | 4 | 5 | 28 |
| C2 | Ops compaction & snapshot checkpoints | 4 | 4 | 5 | 3 | 5 | 3 | 24 |
| C3 | Agent-grade error contract | 5 | 5 | 3 | 5 | 5 | 4 | 27 |
| C4 | Transition-time delivery gate | 4 | 5 | 4 | 5 | 4 | 4 | 26 |
| C5 | Redaction firewall for durable ops | 3 | 4 | 4 | 3 | 5 | 4 | 23* |
| C6 | Reviewer self-validation (`arm review validate`) | 4 | 5 | 3 | 5 | 4 | 5 | 26 |
| C7 | Scope & context suggestion from co-change mining | 4 | 4 | 4 | 4 | 4 | 3 | 23* |
| C8 | Model-tier dispatch policy | 3 | 5 | 3 | 5 | 4 | 4 | 24 |
| C9 | Findings as a product loop (`arm finding`) | 3 | 4 | 5 | 3 | 5 | 4 | 24 |
| C10 | Session handoff bundle (`arm handoff`) | 4 | 5 | 4 | 4 | 4 | 3 | 24 |

\* C5 and C7 score below threshold on the sum but survived on an override argued in the red-team round (noted inline); the items they displaced (simulation sandbox, `arm next`) died outright in earlier rounds.

### C1. Autonomic heartbeats via the harness hook

The harness hook already intercepts every tool call a bound worker makes (ADR 0007 issue binding). Piggyback a heartbeat on that interception (rate-limited, e.g. at most once per few minutes): a claim can then never go stale *while the agent is actively working*, and a stale claim becomes a reliable signal that the worker is actually dead. This structurally eliminates the "worker left task claimed despite reporting success"/false-stale class from `session-recovery-gaps` instead of tuning TTLs around it, and it makes gap-analysis T3's recovery rules crisper because staleness finally means something. Cheapest correctness win per line of code in this document.

### C2. Ops compaction and snapshot checkpoints

The ops store is append-only forever, and the live repo already shows the growth pattern: per-worker-per-slot log files, many holding one or two ops (`.arm/.armature/ops/` today contains a dozen such fragments). Design a compaction op now: a signed snapshot record that folds a closed range of ops into a checkpoint, hash-chained to the compacted range so auditability survives, with old ranges archived rather than rewritten (append-only is preserved; nothing is ever edited). Materialization cost and clone size then stop growing without bound. **Red-team note:** must be co-designed with T5.2's versioning fixtures — a snapshot format is itself a versioned op. Doing this *late* means migrating a giant live corpus; doing it now is a design exercise.

### C3. Agent-grade error contract

Every `arm` error gets a stable code, a one-line cause, and a machine-actionable remediation (in `--format agent`, a structured `next_actions` field naming the exact command to run). The dogfood corpus is full of the alternative: `--bundle` given JSON content fails with "a cryptic file-not-found error"; missing `--worktree` fails every wave dispatch until the agent reads `--help`. Agents recover from errors exactly as well as the error text allows — this makes the CLI self-healing for its primary user class and pays off on every future command automatically. (Distinct from gap-analysis D2, which fixes the *docs* around envelopes; this fixes the runtime conversation.)

### C4. Transition-time delivery gate

`arm transition --to done` currently accepts the worker's word. Make it verify, in the bound worktree, before appending the op: working tree clean, no untracked build artifacts, delivery diff ⊆ declared scope, and at least one commit referencing the issue ID. Failures return C3-style remediations. Evidence: out-of-scope Makefile edit, stray compiled binary in repo root, worktree changes leaking into main — all currently caught (if at all) by a human or a later audit. **Red-team note (T4.2 adjacency):** T4.2 proposes *post-hoc* doctor checks and violation logging; this is a *blocking gate at the state machine*, a different enforcement point with different guarantees — it prevents the bad `done` op from ever entering permanent history rather than flagging it afterward. Ship T4.2's checks first, then promote them into this gate.

### C5. Redaction firewall for durable ops *(threshold override)*

Outcomes, notes, decisions, and review assessments are free text written to a branch that is pushed, shared, and — by architectural commitment — never rewritten. Nothing today scans that text for credentials or internal hostnames before it becomes permanent. Add pattern-based redaction (gitleaks-class rules) at op-append time plus a `doctor` audit for the existing corpus. **Override rationale:** the sum is below threshold because leverage is low *until the day it isn't* — and append-only storage means a leaked secret cannot be scrubbed later without breaking the hash-chained history that C2 and T5.2 formalize. This is the one proposal whose cost of deferral is a ratchet; `docs/sensitive-environments.md` already acknowledges the deployment context.

### C6. Reviewer self-validation: `arm review validate`

Expose the exact validation `arm review record` performs (schema, criterion-ID format, citation line-bounds against diff hunks) as a standalone read-only command the *reviewer agent* runs before returning its assessment, with auto-fix suggestions (e.g. "line 104 exceeds hunk; downgrade citation to path-level"). The `reviewer-agent-reliability` theme documents the current shape: every out-of-bounds citation and every haiku schema deviation becomes coordinator post-processing, "eroding the value of delegating review." Self-service validation moves the retry loop inside the reviewer where it costs nothing.

### C7. Scope and context suggestion from co-change mining *(threshold override)*

`arm context suggest <issue>` mines git history for files that co-change with the issue's declared scope and proposes scope additions and `context_files`. Evidence: "DAG scope gaps left files uncovered," "phantom scope for file created by earlier task," and the broad-scope warnings planners rationalize away. **Override rationale:** scored down on cost (heuristics need tuning) but it is the only proposal addressing planning quality — the stage where every downstream scope-enforcement mechanism (T4, F2, C4) inherits its ground truth. Strictly advisory output keeps the risk contained: a bad suggestion is ignorable; a bad declared scope corrupts everything built on it.

### C8. Model-tier dispatch policy

Encode the empirically-validated dispatch rules (`effective-patterns`, `reviewer-agent-reliability`: haiku cannot reliably emit schema-valid assessments from prose; sonnet-with-verbatim-template can; opus for semantic expansion) as machine-readable config: per role, a minimum model tier and a required prompt-template reference, surfaced in `render-context`/`review prepare` output so coordinators enforce it mechanically instead of re-learning it per session. **Red-team note:** this flirts with skills territory, but the gap analysis only fixes skill *prose*; a policy the engine emits with each dispatch survives skill drift and session amnesia — which is the point.

### C9. Findings as a product loop: `arm finding`

The dogfood findings pipeline — capture raw friction, curate into themes, promote to work — is this project's actual QA engine, and it currently runs on hand-named markdown files and discipline. Productize the capture step: `arm finding add` stamps ID/timestamp/worker, links the currently bound issue and recent command history, and `arm finding promote` creates a linked Bug in the DAG with the finding as its source citation. Every adopter operating agent fleets needs exactly this friction-capture loop; no competing tracker treats operational learning as a first-class object. This also converts an Armature-repo convention into a product capability, which is the strongest kind of dogfooding.

### C10. Session handoff bundle: `arm handoff`

Serialize the coordinator's session-spanning state — active wave membership, claim→worktree map (from F1), pending review dispatches, last integrated commit per story — into a durable handoff record; `arm handoff --resume` renders it as re-entry context for a fresh session. Evidence: the entire `session-recovery-gaps` theme, whose worst finding is a coordinator abandoning the worker protocol entirely because reconstructing state was too expensive. **Red-team note (T3 adjacency):** T3 reconciles *Armature's own* state after a crash; handoff preserves the *coordinator's operational context* that Armature state does not capture (wave intent, dispatch history). T3 makes recovery correct; this makes it cheap.

---

## Sequencing note

Three dependency chains matter; otherwise items are independent:

1. **F3 (events) → D2 (hooks redesign)** — the hook `on:` vocabulary is the event vocabulary.
2. **F1 (managed worktrees) → C10 (handoff)** — the claim-recorded worktree path is half the handoff bundle; both feed gap-analysis T3. F1.1 (`--from`) extends the same claim-recorded metadata to sub-task branches; it does not gate C10, since a handoff bundle built from F1's data alone is already useful, but sub-task entries in that bundle will carry incomplete provenance until F1.1 lands.
3. **T5.2 (versioned ops) → C2 (compaction) → C5 (redaction)** — compaction and redaction both touch the permanence guarantees and must respect the versioning contract.

D5 (dotdir collapse) and D3 (shim deletion) have expiry dates: they are only cheap before v0.1.0 ships to real adopters. Everything in Part 3 scored ≥24/30 on the rubric or carries an explicit override rationale; the full kill list from adversarial review is recorded in §Method.
