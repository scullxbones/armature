# Materialize MK-1..11 reshape — A-primary + B grafts

Status: agreed direction for Comment Sicko deferred MUST KILLs in `internal/materialize` (wave 1). Full Asserted/Derived + state v2 held as follow-on.

## Goal

Encode laws that today live only as long comments in `engine.go` so sermons can die without changing append-only `_armature` JSONL history.

## Shape

- **Base (Arena A):** same apply path; one `Issue.Status` + `RollupStatusBefore`; named helpers (`claimLostRace`, `compensationApplies`, `WorktreeRestore`, `DAGEra`); `MaterializeCold` / `MaterializeIncremental` with retract mandatory on incremental entrypoints.
- **Grafts (Arena B):** `ops.DecodeScope`, DAG Mode / Canonical classify at decode-on-read, typed `Compensation` / worktree tri-state. No log rewrite.
- **Hold:** `Issue.Asserted` + `Derived` overlay, drop `RollupStatusBefore`, `CurrentStateVersion = 2`, consumer cutover.

## Task sequence (verifiable units)

1. `DecodeScope` — legacy `", "` scopes at ops/load boundary
2. `claimLostRace` + claimant-only heartbeat clocks
3. `WorktreeRestore` / typed Compensation + `PlanCompensation` encode
4. Shrink `applyTransition` (IfClaimToken gate, status assert, lease restore)
5. Handler `MissingTarget` meta; delete error-string parse
6. `DAGEra` / Mode at decode
7. `MaterializeCold` / `MaterializeIncremental`; retract inside `ApplyOpsSorted`

## Proof

Existing `internal/materialize` and `internal/claim` tests are the behavior spec. Gate: `go test` on touched packages; `make check` at story integrate. Sermon paragraphs for MK-1..11 leave `engine.go`.

## Non-goals

New op types; JSONL rewrite; unifying `ResolveClaim` with lose-race; fail-loud publish; changing TTL/race winner; Asserted/Derived in this story.

## Arena refs

- A: bc-d5affdb3 (minimal same-path)
- B: bc-2b5d0f28 (derived layer + parse eras)
