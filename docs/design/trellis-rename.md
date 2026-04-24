# Trellis Rename — Decision Archive
 
**Date:** April 19, 2026
**Status:** Leading candidate identified (Armature); final availability checks pending
**Context:** The existing project name "Trellis" collides with [mindfold-ai/Trellis](https://github.com/mindfold-ai/Trellis) (4.3k stars, agent harness, uses `.trellis/` directory, similar positioning language). Rename required before OSS launch.
 
---
 
## Product Framing for Naming Purposes
 
The product is three things sharing one thread:
 
1. **A substrate for agent memory** — externalizes episodic, semantic, and procedural memory into git so nothing rots between sessions.
2. **A coordination layer** — multiple workers (human and AI) push independently, never collide, converge via replay (MRDT model).
3. **A decomposition engine** — source docs become a typed DAG where each task renders as ~1,600 tokens of precisely-assembled working memory.
The unifying metaphor: **structure that holds things in place while growth happens**. This is why "Trellis" worked originally. The distinctive technical asset is the DAG itself — parent/child/blocker/sibling adjacency is what enables precision context assembly.
 
## Naming Criteria
 
**Must-haves:**
- Concrete, physical object rather than abstract jargon
- Two syllables ideal, clean short CLI possible
- Structural-support or precision-selection metaphor
- No existing footprint in AI coding agent / memory / orchestration space
**Anti-criteria:**
- No `agent-` or `ai-` prefixes
- No names already established in dev tooling (beads, loom, fabric, forge, helm, flux, codex)
- No cutesy neologisms without meaning
---
 
## Candidates Evaluated and Rejected
 
### Round 1: Structural / Memory / Weaving Metaphors
 
| Name | Status | Reason |
|---|---|---|
| **Cairn** | REJECTED | Saturated in agent-memory space: dead `cairn-dev/cairn`, `@principled/cairn` (decision traceability), `@akubly/cairn` (agent memory framework), `cairn-work` (direct conceptual overlap — markdown-based agent work tracking). Multiple independent arrivals at the same metaphor means no distinctiveness. |
| **Arbor** | REJECTED | `penso/arbor` is an active agentic coding workflows project. |
| **Stele** | REJECTED | `sincover/stele` markets itself as "persistent memory substrate for AI agents" — near-identical positioning. `IronAdamant/stele-context` is "persistent memory for AI coding agents" via MCP. Two direct in-thesis collisions. |
| **Strata** | REJECTED | `noopz/strata` is a "structural editing plugin for Claude Code... to reduce context consumption." `Strata by klavis.ai` is a launched commercial product for agent context reduction. Two independent takes on the same thesis. |
| **Scion** | REJECTED | `GoogleCloudPlatform/scion` is an AI agent orchestration project in Go using `.scion/` directory and git worktrees. Same space, same language, Google backing. Fatal. |
 
### Round 2: Neologisms
 
| Name | Status | Reason |
|---|---|---|
| **Mnemora** | REJECTED | `mnemora.dev` is live and sits directly on the same SEO space. |
| **Ligatum** | NOT PURSUED | Medical/surgical association thin but present; user deprioritized. |
| **Keron** | NOT PURSUED | Pure neologism with zero semantic handle; too much brand work required without funding backstop. |
| **Scriv** | REJECTED | `nedbat/scriv` (active Python changelog CLI with `scriv` command) and `mooship/scriv` (active Rust CLI note manager storing local NDJSON). Two CLI-level collisions. |
 
---
 
## Final Two: Armature vs Vinculum
 
Both cleared the AI/agent-space filter. Neither was unambiguous elsewhere.
 
### Armature
 
**Metaphor:** The rigid internal frame sculptors build around — invisible structural skeleton that gives shape to fundamentally pliable work. Precise fit for the DAG's role beneath agent work.
 
**Availability findings:**
- GitHub: user handle `Armature` claimed but inactive; org namespace needs direct check. Existing repos are `danielparks/armature` (abandoned Puppet tool) and `carls3d/ArmatureTools` (Blender add-on) — neither in AI/dev-tools space.
- npm: unscoped `armature` claimed by `@lpghatguy` (abandoned 2016 TypeScript component model). `@softheon/armature` is active healthcare Angular library. Neither blocks the Go binary distribution but scoped name is preferable if needed.
- crates.io: **`armature-framework`** by Joseph R. Quinn is an active Rust HTTP framework (Angular/NestJS-inspired). Low traction (35–124 downloads across companion crates) but present in developer tooling.
- Domains: `armature.dev`, `armature.io`, `armature.tools` need direct registrar check.
- Wikipedia: generic disambiguation (sculpture, animation, electrical, Armature Studio game dev — dissolved Jan 2026).
**Verdict:** YELLOW. Ships. Will share search results with a Rust framework and dissolved game studio for the first year. High instant comprehension.
 
### Vinculum
 
**Metaphor:** Latin for "bond/tie"; also the math notation bar over grouped terms. *That which binds.* Precise fit for the DAG's edges — the typed relationships between work items.
 
**Availability findings:**
- GitHub: Clean in AI/agent space. Existing repos (`hawcode/vinculum` Java CMS dep; `jmelberg/vinculum` iOS Keychain 2018, inactive; `jordanfine/vinculum` visual novel game) are all unrelated. **`vinculum-official`** org active with Linux distro.
- npm / crates.io: no conflicts in AI/agent space.
- Domains: **`vinculum.ai` is registered and parked** by Vinculum Technology (ESA Space Solutions affiliate).
- Trademark: **Vinculum Group** is an established Indian omnichannel-retail SaaS company — real trademark, not in AI/dev tools but in B2B software.
**Verdict:** YELLOW. Slightly cleaner in AI-adjacent search than Armature. Low instant comprehension; requires explanation.
 
### Head-to-Head Comparison
 
| Dimension | Armature | Vinculum |
|---|---|---|
| Collisions in AI/agent space | None | None |
| Main competing entity | `armature-framework` (low-traction Rust HTTP framework) | Vinculum Group (established SaaS, B2B retail) |
| `.ai` domain | Unknown; needs check | Parked, owned |
| Instant comprehension | High | Low |
| Pronounceability first-hearing | Unambiguous | Slight friction (VINK-yoo-lum) |
| Memorability after one mention | High | Moderate |
| SEO ownership ceiling | Medium | High |
| Metaphor precision | Excellent (structure + load-bearing) | Excellent (edges + binding) |
| CLI | `arm` (collides with ARM) or `armature` | `vinc` (clean) |
 
---
 
## Decision: Armature (Pending Final Availability Check)
 
**Rationale:**
 
1. **The Rust framework collision is small and in an adjacent-not-competing audience.** Rust web-framework developers are not the primary audience for AI coding agent infrastructure. Even if `armature-framework` grows, the two projects will not compete for the same mindshare.
2. **Adoption velocity outweighs SEO purity at OSS launch stage.** Distribution is via word-of-mouth, HN, Discord, Twitter, and conference mentions — not search ads. A name that survives one hearing compounds; a name that requires explanation compounds less reliably.
3. **Metaphor precision is slightly better for the product's actual function.** "Invisible structural frame that gives shape to pliable work" captures both the DAG and its role for agents. "That which binds" captures edges but not load-bearing function.
**Vinculum remains the fallback** if Armature fails the final availability pass.
 
---
 
## Related Decisions Made Along the Way
 
### HNSW / Vector Retrieval — REJECTED
 
Considered using `coder/hnsw` for semantic retrieval over DAG nodes. Rejected because:
 
- Breaks zero-dependency constraint (requires embedding model — external API or local model both problematic).
- Breaks determinism guarantee (embeddings drift between model versions, creating a second source of truth that can disagree with the op log).
- Blurs value prop against RAG-style competitors (CASS, Mem0, OpenViking) whose differentiator is "structure beats vibes."
- Loosely-related prior work is a documented hallucination source; adding it to `render-context` could *raise* the anti-metric being tracked.
- Not in a gap users have reported; decomposition-time explicit linking (Conductor sign-off) already covers the need.
Acceptable niche: `trls-search` as a separate binary for Conductor-facing, human-evaluated decomposition-time lookups. Not in the core `trls` binary.
 
### Retrieval Algorithms Worth Considering
 
Zero-dep, deterministic, non-embedding alternatives surfaced:
 
- **Graph-walk retrieval (Personalized PageRank / Random Walk with Restart)** — uses structure already present; deterministic; fills the cousin-node gap HNSW was tempting for. Recommended first if any retrieval enhancement is built.
- **BM25 over text fields** — classic lexical retrieval; deterministic; ~300–500 LOC hand-roll; supports `trls search` for Conductor persona.
- **MinHash for near-duplicate detection** — targeted at import and decomposition-time duplicate flagging. Narrow but high-value at its job. Defer until adoption surfaces the need.
Rejected: LSH (needs vectors), AST-based code search (breaks zero-dep), symbol extraction (needs language server), Levenshtein (not the problem).
 
---
 
## Next Steps
 
1. Run direct registrar check on `armature.dev`, `armature.io`, `armature.tools`.
2. Verify GitHub org handle `armature` claimability.
3. If org handle is taken, evaluate `armature-dev` / `armature-cli` or pivot to Vinculum.
4. Trademark search in software categories (USPTO, EUIPO) for Armature in Class 9 / 42.
5. On commit: update repo name, CLI name (`trls` → `arm` or `armature`), documentation, landing page, `.issues/` → `.armature/` directory convention.
6. Defer branding work (logo, visual identity) until name is locked.
## Branding Direction (Reference Only)
 
If Armature is selected, aesthetic direction to explore later:
 
- **Visual:** wireframe cube with one vertex highlighted, or a clean line drawing of a sculptor's armature rod. Industrial/technical register.
- **Palette:** graphite, off-white, single saturated signal color (electric blue or safety orange).
- **Wordmark:** technical monospace.
- **Taglines to test:** *"The structure beneath the work."* / *"Rigid enough for agents. Invisible enough for humans."*