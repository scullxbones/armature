# Theme: Log-slot invalid and collision

## Summary

`ARM_LOG_SLOT` now fails loud on invalid values (uppercase `A`/`B`) instead of falling back to the unslotted log, and overlapping same-id appends return `LOG-SLOT-COLLISION` via `TryLock` rather than blocking `Lock`.

## Evidence

- `../../raw/2026-09-27T1213Z-cursor-validation-invalid-arm-log-slot-no-longer-ignored.md` - `ARM_LOG_SLOT=A arm claim` → `LOG-SLOT-INVALID` (`^[a-z0-9_-]{1,64}$`).
- `../../raw/2026-09-27T1214Z-cursor-validation-overlapping-appends-log-slot-collision.md` - two concurrent `arm transition` on one worker id; second `TryLock` returns `LOG-SLOT-COLLISION`.

## Candidate Follow-Ups

- Keep fail-loud for invalid slots; tests use `a`/`b` not `A`/`B`.
- Keep collision (spec §9), not blocking `Lock`.
