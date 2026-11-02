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
   finds candidate pattern literals (Go string lits; schema `pattern`; shell
   `grep -E/-P` / `sed` / `[[ =~ ]]` / `re.compile`), compiles them with Go
   `regexp`, and fails any pattern that matches a 40-char lowercase hex OID
   but rejects the 64-char equivalent. POSIX classes / `\w` / `{40,40}` fall
   out of MatchString behavior — no syntax-regex list. `len(x)==40` is kept as
   a separate AST fence (not a regexp). Per-site waivers are file+line+needle
   in `scripts/repoguard/allow.go` (stale when the line moves).

## Consequences

Review decode, adapters git parsers, and harnesshook HEAD detection share one
helper aligned with the schema. Uppercase hex is rejected. Future OID checks
must call `gitoid.Valid` or fail the object-id guard.
