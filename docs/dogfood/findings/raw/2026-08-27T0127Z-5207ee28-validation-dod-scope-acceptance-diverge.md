---
date: 2026-08-27
agent: 5207ee28
writer: 5207ee28
area: validation
task: LNGHZN-S7-T2
story: LNGHZN-S7
tags: [planning, dod, scope, acceptance, doctor-check-id, wrap]
---

# Task DoD promised `arm doctor` D9; scope could not wire Run(); wrap Yellow is first fail

## User Goal

Drain the AOC → S6 → S7 critical path via `aoc-critical-path-2`. For
`LNGHZN-S7-T2` (LH D1: strict config decode + doctor config-health),
implement in declared scope, pass named acceptance tests, record a
full-profile gate, and wrap Green so the workflow can open a stacked PR
without a human resume.

## Observed

Create-time DoD (2026-07-19) copied LH D1 product language: "arm doctor
gains a check (next free D-number) … so the config file can never
silently lie again." Create-time scope was four *new* files
(`internal/config/strict.go`, `strict_test.go`,
`internal/doctor/config_check.go`, `config_check_test.go`). It never
included `internal/doctor/doctor.go` (the only `Run`/`RunChecks` wiring
site) or `internal/config/config.go` (`LoadConfig` / production decode).
Scope was never amended.

Acceptance named unit tests (`TestStrictDecodeRejectsUnknownField_…`,
`TestDoctorConfigCheck_…` later renamed `…D9…`) and `make check green` —
not "`arm doctor` emits the new check."

Grilling (2026-07-21) only replaced "(next free D-number)" with **D9** to
avoid `TOPTIER-S12-T2`'s *planned* D8. It did not add `doctor.go`. It did
not consult live `Run()`. Later `LNGHZN-S5-T8` *did* put `doctor.go` in
scope and consumed **live D9** (unrecognized worktrees). Live D8 is
already scope-violation artifacts from `TOPTIER-S5-T2`.

`arm validate` accepted the plan (`dag transition` to verified). It
checks DoD presence and length, not "DoD verbs ⊆ scope files" and not
"acceptance tests the same surface as DoD." There is no doctor-check ID
registry.

The 2026-08-26 worker stayed in scope, exported `CheckD9ConfigHealth`,
tested it directly, recorded a decision not to rewire `Run()`, and
shipped a citable `arm gate run full` at HEAD. Wrap: acceptance all
satisfied; DoD `partially_satisfied`; rating Yellow;
`remaining_blocking=false`. The workflow paused for HITL because wrap
`passed` is only true for Green.

## Impact

- ~12h of a 256-agent sequential run stopped on a **planning contract
  defect that existed at create**, not on missing gate evidence (that
  class was already fixed).
- Worker behavior was correct (scope containment). Punishing it with
  in-scope remedia cannot make `arm doctor` grow a check.
- `docs/design/next-work-sequencing.md` still called T2 "fully scoped"
  and assumed "T2's doctor output," so coordinators dispatched as if the
  product sentence were implementable.
- Check IDs D8/D9 are now a three-way collision (open DoD vs live
  `Run()`), so even a later wiring task cannot honestly use D9.

## Evidence

- `arm show LNGHZN-S7-T2`: DoD "arm doctor gains check D9"; scope the
  four new files; acceptance the named tests + `make check`; Review
  yellow (3 satisfied, 1 partially_satisfied).
- Worker note: "doctor.go is out of scope so the check is exported and
  tested directly rather than wired into Run."
- Worker decision topic `D9 check-id collision`: keep `Finding.Check=D9`
  on the helper; do not rewire `Run()`.
- Wrap `docs`/`.armature/review/LNGHZN-S7-T2-eecec560-wrap.json`: DoD
  rationale that `Run` never calls `CheckD9ConfigHealth`; live D9 is
  unrecognized worktrees; `LoadConfig` still `json.Unmarshal`.
- Contrast: `TOPTIER-S12-T2` and `LNGHZN-S5-T8` both put `doctor.go` in
  scope when their DoD was "arm doctor gains/enforces."
- `arm validate` E6/E9/W7: presence, 500-char DoD, vague words — not
  DoD∩scope.

## Suggested Follow-Up

- Do not amend T2 DoD down to hide the overclaim; the Yellow wrap is the
  audit trail.
- File a follow-on task whose scope includes `doctor.go` and the
  production load path, with a **live** free check id (not D9).
- Planner/`arm validate` rule: if DoD says `arm doctor` / "gains check
  Dn", `internal/doctor/doctor.go` must be in scope or the DoD must say
  "exported helper, not wired."
- Doctor check-ID registry allocated from live `Run()` plus open-task
  reservations.
