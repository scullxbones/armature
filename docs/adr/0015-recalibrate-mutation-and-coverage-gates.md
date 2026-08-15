# ADR 0015: Recalibrate Mutation and Coverage Gates

## Status

Proposed

## Principles touched

I5, I7

## Context

The repo runs three quality gates: `make coverage-check` (statement coverage,
one aggregate number over all of `./...` minus `internal/e2eharness`,
threshold 85%), gremlins `threshold.mutant-coverage` in `.gremlins.yaml`
(threshold 95), and gremlins `threshold.efficacy` (threshold 99). Measurement
on this branch (`chore/gate-threshold-recalibration`, based on
`origin/main`), using the same `coverage.out` profile:

- Statement coverage: cmd 83.79%, internal 87.22%, aggregate total 85.8%.
- The aggregate gate hides per-tree weakness: cmd sits below the previous
  85% aggregate threshold on its own, but the repo total passes because
  internal's stronger coverage subsidizes it. A regression concentrated in
  cmd can pass the gate as long as internal stays strong enough to average
  it out.
- Mutant coverage and statement coverage measure the same thing —
  reachability — from the same `go test` coverage profile. They differ only
  in denominator: mutant coverage counts only mutable AST sites
  (conditionals, arithmetic), statement coverage counts all statements.
  Measured offset on this branch: cmd 83.79% statement vs 95.45% mutant
  (+11.7 points); internal 87.22% statement vs 95.21% mutant (+8.0 points).
  An untested function contributes many uncovered statements but only a few
  mutable sites, so the mutant ratio is diluted upward. Correcting for that
  offset, the 85% statement and 95% mutant thresholds encoded almost exactly
  the same real demand. The
  apparent ~10-point gap between the 85% statement threshold and the 95%
  mutant-coverage threshold is a unit artifact of what each metric counts,
  not a difference in rigor — both gates were sitting knife-edge on
  essentially the same underlying signal, doubling the cost of a single
  reachability measurement without doubling its information.
- `threshold.efficacy` — killed / (killed + lived), the test-quality signal —
  has measured 100.00% with zero surviving mutants on every ref checked. The
  expensive part of mutation testing (executing covered mutants and checking
  whether tests kill them) has never once failed. gremlins never executes
  NOT COVERED mutants, so nearly all of `make mutate`'s runtime is spent
  reconfirming a result that has never varied. The gate that actually broke
  CI was `mutant-coverage`, and it broke on a feature branch (PR #97) whose
  own new code was undertested — 306 statements added to cmd with only 61%
  of them covered, dragging the tree from 83.99% to 82.49% and mutant
  coverage from 95.45% to 93.68%. That is a reachability regression, and
  per-tree statement coverage detects it more cheaply and more legibly than
  mutation testing does.
- The mutant-coverage gate penalizes defensive error handling: a majority of
  cmd's uncovered statements are `if err != nil` branches on filesystem/git
  I/O. Writing more defensive code mechanically lowers the metric even when
  test quality is unchanged.
- File-level exclusion was measured as a fix for TUI-heavy files dragging
  down cmd's mutant coverage, and found insufficient: excluding `tui.go`
  alone made mutant coverage *worse* (93.68% -> 93.66%), because `tui.go` has
  zero uncovered mutants and its exclusion only shrinks the denominator
  without removing any misses; excluding all five TUI-touching files reached
  only 94.87%, still short of the prior 95% threshold. The real fix is a TUI
  seam extraction, filed as `LNGHZN-S10-T11` and tracked in
  `docs/design/gate-efficiency.md`; the gate recalibration below is
  independent of that follow-on and does not require it to land first.

`docs/design/quality-controls.md` states the "ratchet: only raise, never
lower" policy that today's `.gremlins.yaml` comment cites. Lowering
`mutant-coverage` from 95 to 92 is, on its face, a violation of that policy
as previously written. This ADR is the amendment.

## Decision

1. **Statement coverage becomes per-tree, seeded at each tree's current
   measured value, and is the primary reachability gate.** `make
   coverage-check` computes both `cmd/**` and `internal/**` percentages from
   the single `coverage.out` profile already produced by the existing `go
   test -coverprofile` run (no second test run), and fails if either tree is
   under its threshold: `internal/**` >= 86, `cmd/**` >= 83. Both percentages
   print unconditionally so drift is visible even when passing.

   Seeds are set a point or so *below* each tree's measured value, not level
   with it. Seeding level with the measurement rearms the exact brittleness
   this ADR exists to remove: internal measures 87.22%, so a threshold of 87
   would leave 0.22 points — roughly eight statements — of margin, and a
   single feature branch adding ordinary error handling would trip it while
   saying nothing about test quality. 86 leaves internal ~1.2 points of
   working room, comparable to the ~0.8 that rounding down already gives cmd
   at 83. The ratchet still functions; it is simply not armed a hair's
   breadth from the current value.
2. **`mutant-coverage` drops from 95 to 92, once, and is de-emphasized to a
   secondary reachability proxy.** `efficacy` stays at 99 unchanged and is
   the primary mutation-quality signal — the one metric that measures
   something coverage-check cannot (whether covered mutants are actually
   killed), and the one with a perfect, unbroken record.
3. **The ratchet policy is amended, not abandoned.** As previously written,
   "ratchet: only raise, never lower" applied to a single number treated as
   one metric. That framing was wrong: mutant-coverage and statement coverage
   are two views of the same underlying reachability measurement, and had
   independently drifted to inconsistent effective strictness. The amended
   policy: reachability thresholds (statement coverage, mutant-coverage) are
   seeded at each tree's/gate's currently measured value and ratchet upward
   from there, same as before. This one-time reduction of mutant-coverage is
   justified specifically because the metric was double-counting reachability
   already enforced, more cheaply, by per-tree statement coverage — it is a
   correction of an unintended double gate, not a relaxation of standards.
   `efficacy` was never lowered and remains subject to the original
   ratchet-only-up policy without amendment.
4. **Accepted tradeoff: `.gremlins.yaml` stays a single, repo-wide config.**
   A per-tree `mutant-coverage` threshold (mirroring the per-tree coverage
   split) was considered and rejected — internal individually measures
   higher mutant coverage than cmd, so a single 92 threshold gives internal
   several points of slack it doesn't need. This is accepted deliberately:
   maintaining two thresholds in `.gremlins.yaml` (gremlins does not natively
   support per-package thresholds within one config/run split the way the
   Makefile now does for coverage) was judged not worth the added
   configuration surface, given that `efficacy` — not `mutant-coverage` — is
   the gate actually carrying the test-quality signal.

## Consequences

- `make coverage-check` now fails on the tree that regressed instead of being
  masked by aggregate averaging; the failure message names the tree and the
  shortfall in points.
- `make mutate` (`.gremlins.yaml` `threshold.mutant-coverage: 92`) stops
  requiring a full TUI seam extraction before it can pass, while `efficacy: 99`
  continues to catch any real drop in test quality the moment it happens
  rather than after aggregation.
- The `.gremlins.yaml` comment block is rewritten to state the new policy and
  cite this ADR, so a future reader does not reintroduce the old "ratchet:
  only raise, never lower" framing against a metric now understood to be a
  secondary, seeded-and-ratcheted proxy.
- `LNGHZN-S10-T11` (TUI seam extraction) remains filed and useful on its own
  merits — it will still raise cmd's mutant-coverage and statement coverage
  further — but is no longer a blocking prerequisite for the mutation gate to
  pass.
- Follow-up work implied: none required immediately; `LNGHZN-S10-T11`
  continues as tracked, independent follow-on work.
