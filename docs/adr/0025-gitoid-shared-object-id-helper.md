# ADR 0025: Shared `gitoid` helper for SHA-1 and SHA-256 object IDs

## Status

Accepted

## Principles touched

I4, I5

## Context

Git repositories may use SHA-1 (40-hex object IDs) or SHA-256 (64-hex). After
OPORDER-LOCATE (#412), adapters already accepted both widths when parsing git
output, but `internal/review/decode.go` and the published review-bundle schema
still required exactly 40 hex digits. `internal/harnesshook` had a third,
length-only check. Duplicating the 40|64 rule would reintroduce SHA-256 holes.

`review` cannot import `adapters` (review-boundary). A pure shared package is
the one place both layers can call. Under ADR 0023, growing a depguard
allow-list is ADR-class.

## Decision

1. **`internal/gitoid`** owns one canonical predicate: `Valid`, which matches
   `PatternLower` exactly — lowercase hex of length 40 or 64. The published
   review-bundle schema `delivery.base_sha` / `head_sha` patterns must equal
   `PatternLower`. Decode and harnesshook both call `Valid`. No other package
   may hard-code a 40-hex-only OID pattern.
2. **Fence.** Add `gitoid-pure` (deny other `internal`). Widen
   `review-boundary` and `harnesshook-boundary` to allow `gitoid` only.
3. **Guard.** `scripts/repoguard -check object-id` (wired into `make lint`)
   fails new 40-hex-only OID regex / schema / script patterns that omit 64-hex,
   including `{40,40}`, `\w{40}`, and `len(x)==40` forms. Scope matches this
   ADR: no other package (and no unchecked script) may hard-code a 40-only
   pattern. Per-site waivers live in `scripts/repoguard/allow.go`.

## Consequences

Review decode, adapters git parsers, and harnesshook HEAD detection share one
helper aligned with the schema. Uppercase hex is rejected. Future OID checks
must call `gitoid.Valid` or fail the object-id guard. A bare `{64}` substring
in the same string does not waive a 40-only hit — only a hex-class `{64}`
quantifier (dual-width) does.
