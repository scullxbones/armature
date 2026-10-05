# ADR 0023: Widening a depguard fence is an ADR-class change

The live `depguard` rules in `.golangci.yml` are the modularity map. Shrinking
an allow-list is welcome; growing one or removing a deny requires a new ADR.

## Status

Accepted — amends ADR-0004 with a ratchet policy. Fence *contents* stay in
`.golangci.yml`; this ADR only classifies allow-list growth and deny removal.

## Principles touched

I4, I5

## Context

ADR 0004 designated deep-module and purity fences and encoded them as
`depguard` rules. Those rules are already green on main and run in `make lint`.
`docs/design/quality-controls.md` C7 still described the control as a GAP
("add depguard"), so agents treated the map as missing rather than as a hold.
Nothing in ADR 0004 said whether a feature could widen an allow-list in the
same PR that needed a new import.

## Decision

1. **Map.** The `depguard` `rules:` block in `.golangci.yml` is the modularity
   map. Do not add surface budgets, canaries, or a second import graph.
2. **Ratchet.** Prefer shrinking allow-lists. Growing an allow-list or removing
   a deny is an ADR-class change (same bar as lowering a coverage or mutation
   threshold). Overnight subtractive passes may shrink fences; feature work
   must not widen without that ADR.
3. **New fences.** Adding a deny or a new strict allow-list is allowed when
   current imports already comply (zero false positives). That is a map
   extension, still recorded against ADR 0004's boundary model, not a silent
   YAML drive-by.

## Consequences

Lint stays the only enforcement. Reviewers reject allow-list growth that
lands without an ADR. C7 in `docs/design/quality-controls.md` is ACTIVE.
