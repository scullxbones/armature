# ADR: Scope-Disjoint Wave Planning Is File-Level Only

## Status

Accepted

## Principles touched

none

## Context

The feature `arm ready --waves` partitions ready-eligible issues into scope-disjoint waves via greedy first-fit, respecting priority-tier boundaries and excluding ancestor/descendant pairs. A wave is a batch of tasks intended for parallel dispatch, and scope disjointness is a useful heuristic for reducing coordination burden — if two tasks touch no overlapping files, they can often proceed in parallel without interfering.

However, dogfooding on 2026-07-06 surfaced a critical limitation: two tasks can touch completely different files while both depending on a shared interface or contract (e.g., a struct field, a public function signature, a protocol definition). If both tasks try to evolve that shared contract differently, they will semantically conflict *even though no files overlap*. This is a **shared-contract drift** problem, and it is invisible to file-level scope partitioning.

A wave manifest computed by `arm ready --waves` is published at query time, but the actual dispatch happens afterward, potentially with intervening issue transitions and claims. The wave's scope-disjointness guarantee is only as good at dispatch time as it was at query time — but the Coordinator already runs overlap-audit and companion-check logic *after* a wave completes, using a recorded manifest of what was actually dispatched. If `--waves` output is relied upon to skip that post-wave audit, semantic conflicts can slip through.

Finally, `arm ready --waves` is a pure query over existing ready-issue data with no new persisted operations. It groups issues identically to `arm ready`, just organized differently — no new op type is introduced.

## Decision

`arm ready --waves` guarantees freedom from *file-level scope conflict only*. It does NOT guarantee freedom from shared-contract drift or other semantic conflicts that arise from shared dependencies between tasks touching different files.

The wave-partitioning output is **advisory pre-dispatch guidance**, not a replacement for existing overlap-audit or companion-check logic the Coordinator already runs after a wave completes. The Coordinator must continue to run its post-wave Parallel Branch Overlap Audit (documented in the armature-coordinator skill) even when using `--waves` output to plan dispatch. These checks catch semantic conflicts that file-level partitioning cannot.

`arm ready --waves` output is computed at query time and not persisted. If issues are claimed or transitioned between the `--waves` query and actual dispatch, the output can diverge from the Coordinator's wave manifest (recorded before dispatch in the Coordinator skill's "Record Wave Manifest" step). The **Coordinator's wave manifest remains the authoritative source of truth** for what was actually dispatched in a wave, not the transient `--waves` query result.

The `--waves` flag is a pure computation over existing ready-issue data (computed identically to `arm ready`, just grouped differently). It introduces **no new persisted op type** and does **not write to the ops log**. It is stateless.

## Consequences

Wave planning remains a useful coordinator tool for reducing human-managed dispatch complexity, but operators must understand its limitations and not treat file-level scope disjointness as a substitute for semantic overlap review. The post-wave audit logic is non-negotiable and stays in the Coordinator's critical path.

Documentation of `arm ready --waves` must prominently call out the scope-disjointness limitation and the requirement to run companion checks post-wave. Training and dogfooding must reinforce that this is a *heuristic aid* to planning, not a *guarantee* that the wave is safe to execute.

The stateless nature of `--waves` means that operators querying the same ready-issue set at different times may see different wave assignments, and that is correct behavior — the computation captures the live state at query time, which is intentional.
