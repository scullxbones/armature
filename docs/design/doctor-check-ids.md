# Doctor check IDs

Agent-facing registry of `arm doctor` check IDs. Live IDs come from `doctor.LiveCheckIDs()` (the `Run` path, in order). Open-task reservations come from `doctor.OpenReservations()`. Do not hand-edit the live table: regenerate this file from those functions (the drift test `TestCheckIDsDocMatchesLiveAndReservations_REQ_TOPTIER_S18_T3` fails if they diverge).

This is an allocation ledger, not the product reference for triggers and remediation — see [validation-codes.md](../validation-codes.md) and [commands.md](../commands.md).

## Live checks

Generated from `doctor.LiveCheckIDs()`. `RunChecks` still omits D7 (worker-ID mismatches need the validated ops stream); that is documented, not changed.

<!-- live-check-ids: generated; do not hand-edit -->
| ID |
|----|
| `D1` |
| `D2` |
| `D3` |
| `D4` |
| `D5` |
| `D6` |
| `D7` |
| `D8` |
| `D9` |
| `D10` |

## Open reservations

Git-tracked in `internal/doctor/reservations.go` (`OpenReservations`). A held story may reserve a future ID here; it must not ship a DoD that claims a live ID. S12 remains held — reserve D11 only; do not implement `TOPTIER-S12-T2`.

<!-- open-reservations: generated from OpenReservations() -->
| ID | Issue | Planned |
|----|-------|--------|
| `D11` | `TOPTIER-S12-T2` | Ops-branch backup / disaster-recovery doctor check (narrow-gaps G2.2). S12 remains held; this row reserves D11 only. |

## Allocating a new ID

1. Read `LiveCheckIDs()` and `OpenReservations()` (or this document after a green drift test).
2. Take the next unused `Dn` that appears in neither table.
3. If the check is not yet wired into `Run`, add a reservation row (and put `internal/doctor/doctor.go` in the task scope, or rewrite the DoD as helper-only / not wired — see `internal/taskcontract`).
4. When the check lands in `Run`, remove the reservation. `LiveCheckIDs` stays in lockstep with `Run` via `TestLiveCheckIDsMatchesRun_REQ_TOPTIER_S18_T0`.
