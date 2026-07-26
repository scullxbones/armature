# Finding: unconditionally-enforcing new gate broke ~40 pre-existing tests it wasn't designed to touch

**Writer:** claude
**Area:** workflow

## What I was trying to do
Implement LNGHZN-S4-T2: wire a new delivery gate (clean tree, scope
containment, commit reference) into `arm transition --to done`, per the
story's own grilling-session decision to ship it "enforcing from the moment
it merges" with no staged/warn-only rollout.

## What happened
Once wired in, `go test ./...` broke in ~40 places across `cmd/armature`,
`internal/e2eharness`, and `internal/skilltranscript` — every pre-existing
test that transitions a fixture issue to `done` without a clean tree and a
conventional-commit reference (which is nearly all of them, since these
tests predate the gate and were never written to satisfy it). This included
the repo's own golden-transcript test
(`internal/skilltranscript/golden_test.go`), which demonstrates the
coordinator command sequence for documentation purposes rather than doing
real delivery work in a claimed worktree.

Remediation required adding `--skip-delivery-gate` to every one of those
~40 call sites (plus, for the golden transcript, baking the flag into the
shared `TestRepo.Transition` helper itself, since that transcript feeds
generated skill documentation and workers reading it should not learn from a
demonstration of the ungated path). This is mechanical but high-volume, and
a rushed version of it risks over-broadly disabling the gate rather than
narrowly exempting non-delivery test fixtures.

## How it changed behavior, confidence, or time spent
Turned a two-task "wire a flag into transition" story into a much larger
diff (8+ files touched outside T2's own 2-file declared scope) purely to
keep the test suite green. The holistic review flagged this diff-scope
overrun as worth a second look, and ultimately assessed it as reasonable
given the alternative (a staged/warn-only rollout) was explicitly rejected in
the story's own notes — but the friction is a direct, predictable
consequence of retrofitting an unconditionally-enforcing gate onto a large
pre-existing self-hosted test suite that never had to satisfy it.

## Evidence
- `go test ./cmd/armature/...` immediately after wiring the gate:
  35+ `--- FAIL` lines, e.g. `TestMergedForceOverridesViolations_REQ_HOOKBIND_T4`,
  `TestTransitionCommand`, `TestReopenCommand`, all failing with
  `delivery gate check failed: 1. CleanTree: ... 2. CommitReference: No
  commits found matching conventional-commit format`.
- Remediation commits `901ef4d2` (test fixtures) and the `--skip-delivery-gate`
  auto-injection in `internal/skilltranscript/harness.go` (commit `7baaa69e`).
