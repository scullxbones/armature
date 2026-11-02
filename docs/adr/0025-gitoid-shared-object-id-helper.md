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
   bans hex-length knowledge outside `internal/gitoid`. Patterns that quantify
   a gitoid width (`{40}` / `{64}`, including BRE `\{n\}`) are compiled with
   Go `regexp` (compile error = hit) and probed with whole-input
   `FindString`: accepting a 40-hex OID while `FindString(64hex) != 64hex` is
   a hit. Separate AST fences catch 40-hex BasicLits, `len` compared to 40,
   and `const HexLen = 40` near hex/OID names. Text walk covers every quoted /
   regex-looking token on `.sh`/`.bash`/`.py`/`.yml`/`.yaml` (including
   `.github/workflows`) with no command-name prefilter. Per-site waivers are
   file+line+needle in `scripts/repoguard/allow.go` (stale when the line moves).

## Consequences

Review decode, adapters git parsers, and harnesshook HEAD detection share one
helper aligned with the schema. Uppercase hex is rejected. Future OID checks
must call `gitoid.Valid` or fail the object-id guard.
