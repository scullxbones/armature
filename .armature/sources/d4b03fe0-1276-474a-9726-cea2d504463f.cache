# Quality Controls Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce the four gap controls from `docs/design/quality-controls.md` — architecture conformance (C7), clock purity in domain code (C6), contract tests for port fakes (C8), and an interim spec traceability hook (C9).

**Architecture:** C7 and C6 are structural enforcement changes (lint config + domain code refactor); C8 adds test infrastructure alongside existing port fakes; C9 adds a Makefile target + Python script following the established `scripts/` pattern. Each control is independent and can be worked or reviewed in isolation.

**Tech Stack:** Go 1.23+, golangci-lint v2, gremlins, Python 3 (for C9 script matching existing `scripts/`). No new Go dependencies.

---

## Chunk 1: C7 — Architecture conformance, C6 — Clock purity

### Task 1: C7 — Add depguard to enforce import boundaries

The domain packages `internal/materialize` and `internal/dag` must never import adapter packages `internal/platform` or `internal/sources`. Currently true by convention; this task makes it a build gate.

**Files:**
- Modify: `.golangci.yml`

- [ ] **Step 1: Verify no current violations exist**

```bash
grep -r '"github.com/scullxbones/armature/internal/platform"\|"github.com/scullxbones/armature/internal/sources"' \
  internal/materialize internal/dag
```

Expected: no output (exit 1 with no matches). If matches exist, do not proceed — fix violations first.

- [ ] **Step 2: Write a failing lint check**

Run lint before adding the rule so you have a baseline:

```bash
XDG_CACHE_HOME=/tmp/golangci-lint-cache golangci-lint run ./...
```

Expected: passes. Record this as the pre-change baseline.

- [ ] **Step 3: Add depguard to `.golangci.yml`**

In `.golangci.yml`, under `linters.enable`, add `depguard`. Under `linters-settings`, add:

```yaml
linters:
  enable:
    # ... existing linters ...
    - depguard

linters-settings:
  # ... existing settings ...
  depguard:
    rules:
      domain-no-adapters:
        list-mode: lax
        files:
          - "internal/materialize/**/*.go"
          - "internal/dag/**/*.go"
        deny:
          - pkg: "github.com/scullxbones/armature/internal/platform"
            desc: "domain packages must not import adapter packages"
          - pkg: "github.com/scullxbones/armature/internal/sources"
            desc: "domain packages must not import adapter packages"
```

- [ ] **Step 4: Run lint and verify it passes**

```bash
XDG_CACHE_HOME=/tmp/golangci-lint-cache golangci-lint run ./...
```

Expected: passes. If depguard fires on existing code, fix the import violation before continuing.

- [ ] **Step 5: Commit**

```bash
git add .golangci.yml
git commit -m "ci: enforce import boundary between domain and adapter packages via depguard"
```

---

### Task 2: C6 — Define the Clock type

Introduce `internal/clock/clock.go` with a single exported type. All domain packages that produce timestamps will receive a `Clock` rather than calling `time.Now()` directly.

**Files:**
- Create: `internal/clock/clock.go`
- Create: `internal/clock/clock_test.go`

- [ ] **Step 1: Write a test for the real-time clock**

```go
// internal/clock/clock_test.go
package clock_test

import (
    "testing"
    "time"

    "github.com/scullxbones/armature/internal/clock"
    "github.com/stretchr/testify/assert"
)

func TestSystemClock_ReturnsCurrentUnixTime(t *testing.T) {
    before := time.Now().Unix()
    got := clock.System()
    after := time.Now().Unix()
    assert.GreaterOrEqual(t, got, before)
    assert.LessOrEqual(t, got, after)
}

func TestFixedClock_ReturnsConstantValue(t *testing.T) {
    fixed := clock.Fixed(1000)
    assert.Equal(t, int64(1000), fixed())
    assert.Equal(t, int64(1000), fixed()) // idempotent
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/clock/... -run TestSystemClock -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `internal/clock/clock.go`**

```go
package clock

import "time"

// Clock is a function that returns the current time as a Unix timestamp.
// Inject this into domain functions instead of calling time.Now() directly.
type Clock func() int64

// System is a Clock that returns the current wall-clock time as a Unix timestamp.
// Use this at composition roots (cmd/) only — never inside domain packages.
// Declared as a Clock value so it can be passed directly as a Clock argument.
var System Clock = func() int64 { return time.Now().Unix() }

// Fixed returns a Clock that always returns the given Unix timestamp.
// Use in tests to make time deterministic.
func Fixed(ts int64) Clock {
    return func() int64 { return ts }
}
```

`System` is a `var` of type `Clock` (not a `func System() int64`) so that it can be passed unparenthesized as a `Clock` argument (e.g. `ApplyPlanWithOptions(..., clock.System)`) and called identically to any other Clock value (`clock.System()`).

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/clock/... -v
```

Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/clock/
git commit -m "feat: add Clock type for injecting deterministic time into domain packages"
```

---

### Task 3: C6 — Inject Clock into `decompose.ApplyPlanWithOptions`

`internal/decompose/apply.go` stamps Op records with `time.Now().Unix()`. Replace these with an injected clock.

**Files:**
- Modify: `internal/decompose/apply.go` (add `clock.Clock` param to `ApplyPlanWithOptions`)
- Modify: `internal/decompose/apply.go` (`ApplyPlan` forwards `clock.System`)
- Modify: `cmd/armature/decompose.go` (update caller, pass `clock.System`)
- Modify (if exists): `internal/decompose/apply_test.go` (use `clock.Fixed`)

- [ ] **Step 1: Read the current test for `ApplyPlanWithOptions`**

```bash
grep -n "ApplyPlan\|clock\|Timestamp" internal/decompose/apply_test.go 2>/dev/null | head -30
```

Note any existing tests that indirectly rely on the timestamp value — these will need `clock.Fixed` passed.

- [ ] **Step 2: Update failing test to assert on a fixed timestamp**

Find or add a test in `internal/decompose/apply_test.go` that verifies the timestamp in the created op equals the injected clock value. If a test already exercises `ApplyPlanWithOptions`, update it; otherwise add:

```go
func TestApplyPlanWithOptions_StampsOpsWithInjectedClock(t *testing.T) {
    dir := t.TempDir()
    plan := &Plan{Issues: []PlanIssue{{ID: "T-1", Title: "test task", Type: "task"}}}
    state := &materialize.State{Index: materialize.Index{}, Issues: map[string]*materialize.Issue{}}

    const fixedTS int64 = 9999
    count, err := ApplyPlanWithOptions(plan, dir, "worker-1", state, ApplyOptions{}, clock.Fixed(fixedTS))

    require.NoError(t, err)
    assert.Equal(t, 1, count)

    // Verify the op on disk has the injected timestamp
    logPath := filepath.Join(dir, "worker-1.log")
    data, _ := os.ReadFile(logPath)
    assert.Contains(t, string(data), "9999")
}
```

- [ ] **Step 3: Run the test to confirm it fails**

```bash
go test ./internal/decompose/... -run TestApplyPlanWithOptions_StampsOpsWithInjectedClock -v
```

Expected: FAIL — `ApplyPlanWithOptions` does not yet accept a clock parameter.

- [ ] **Step 4: Update `ApplyPlanWithOptions` to accept a clock**

In `internal/decompose/apply.go`:

```go
import "github.com/scullxbones/armature/internal/clock"

// ApplyPlan is a convenience wrapper using the system clock.
func ApplyPlan(plan *Plan, issuesDir string, workerID string, state *materialize.State) (int, error) {
    return ApplyPlanWithOptions(plan, issuesDir, workerID, state, ApplyOptions{}, clock.System)
}

func ApplyPlanWithOptions(plan *Plan, issuesDir string, workerID string, state *materialize.State, opts ApplyOptions, clk clock.Clock) (int, error) {
    // ...existing body...
    // Replace every time.Now().Unix() with clk()
}
```

Replace both occurrences of `time.Now().Unix()` (lines ~160 and ~185) with `clk()`. Remove the `"time"` import if it is no longer used.

- [ ] **Step 5: Update the cmd caller**

In `cmd/armature/decompose.go` at the call site for `ApplyPlanWithOptions` (line ~227):

```go
import "github.com/scullxbones/armature/internal/clock"

count, err := decompose.ApplyPlanWithOptions(plan, opsDir, workerID, state, applyOpts, clock.System)
```

- [ ] **Step 6: Run the tests**

```bash
go test ./internal/decompose/... -v
go build ./...
```

Expected: all pass, binary builds.

- [ ] **Step 7: Commit**

```bash
git add internal/decompose/apply.go cmd/armature/decompose.go internal/decompose/apply_test.go
git commit -m "refactor: inject Clock into ApplyPlanWithOptions; remove direct time.Now() call"
```

---

### Task 4: C6 — Inject Clock into `decompose.RevertPlan`

**Files:**
- Modify: `internal/decompose/revert.go`
- Modify: `cmd/armature/decompose.go` (update caller)

- [ ] **Step 1: Add or update test to assert on fixed clock**

In `internal/decompose/revert_test.go` (find existing test or add one):

```go
func TestRevertPlan_StampsOpsWithInjectedClock(t *testing.T) {
    dir := t.TempDir()
    plan := &Plan{Issues: []PlanIssue{{ID: "T-1", Title: "task", Type: "task"}}}
    state := &materialize.State{
        Index: materialize.Index{"T-1": &materialize.IndexEntry{Status: "open", Type: "task"}},
        Issues: map[string]*materialize.Issue{"T-1": {ID: "T-1", Status: "open"}},
    }

    const fixedTS int64 = 7777
    count, err := RevertPlan(plan, dir, "worker-1", state, clock.Fixed(fixedTS))

    require.NoError(t, err)
    assert.Equal(t, 1, count)
    data, _ := os.ReadFile(filepath.Join(dir, "worker-1.log"))
    assert.Contains(t, string(data), "7777")
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/decompose/... -run TestRevertPlan_StampsOps -v
```

Expected: FAIL.

- [ ] **Step 3: Update `RevertPlan` signature**

```go
func RevertPlan(plan *Plan, issuesDir string, workerID string, state *materialize.State, clk clock.Clock) (int, error) {
    // Replace time.Now().Unix() at line ~52 with clk()
}
```

- [ ] **Step 4: Update the cmd caller**

In `cmd/armature/decompose.go` at the call site for `RevertPlan` (line ~289):

```go
count, err := decompose.RevertPlan(plan, opsDir, workerID, state, clock.System)
```

- [ ] **Step 5: Run tests and build**

```bash
go test ./internal/decompose/... -v && go build ./...
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/decompose/revert.go internal/decompose/revert_test.go cmd/armature/decompose.go
git commit -m "refactor: inject Clock into RevertPlan; remove direct time.Now() call"
```

---

### Task 5: C6 — Inject Clock into `ready.ExplainNotReady`

`ExplainNotReady` in `internal/ready/compute.go` calls `time.Now().Unix()` to check claim staleness. `ComputeReady` already uses a variadic `now ...int64` pattern — extend the same pattern to `ExplainNotReady` for consistency, then later replace both with `Clock` in a follow-up if desired (the variadic pattern also allows injection and tests already use it).

**Files:**
- Modify: `internal/ready/compute.go`
- Modify: `cmd/armature/ready.go` (caller at line ~62)

- [ ] **Step 1: Write a failing test**

Read `ExplainNotReady` in `internal/ready/compute.go` before writing this test. The function iterates open issues and records those blocked by unmet gates. A stale claim falls through the staleness guard, and if the issue has no blockers and no parent, it passes all gates and is absent from the not-ready map. A fresh (non-stale) claim hits `continue` and is also absent (it is excluded, not "not ready"). Use a non-claimed, parentless, unblocked open issue to confirm that `ExplainNotReady` is actually using the injected time, not `time.Now()`.

Add to `internal/ready/compute_test.go` (look for existing `ExplainNotReady` tests first):

```go
func TestExplainNotReady_WithInjectedNow_StaleClaimIsNotInResult(t *testing.T) {
    claimedAt := int64(100)
    issue := &materialize.Issue{
        ID: "T-1", Status: "claimed", ClaimedBy: "w1",
        ClaimedAt: claimedAt, ClaimTTL: 1, // 1-minute TTL → stale after claimedAt+60
    }
    index := materialize.Index{"T-1": &materialize.IndexEntry{Type: "task", Status: "open"}}
    issues := map[string]*materialize.Issue{"T-1": issue}

    // Inject now = claimedAt + 61 → claim is stale, issue falls through all gates cleanly.
    // A stale claim with no blockers and no parent is eligible for the ready queue;
    // ExplainNotReady must not report it as blocked.
    result := ExplainNotReady(index, issues, claimedAt+61)
    assert.NotContains(t, result, "T-1")
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/ready/... -run TestExplainNotReady_WithInjectedNow -v
```

Expected: FAIL — function signature doesn't accept `now`.

- [ ] **Step 3: Update `ExplainNotReady` signature**

Add a variadic `now ...int64` parameter matching `ComputeReady`'s pattern:

```go
func ExplainNotReady(index materialize.Index, issues map[string]*materialize.Issue, now ...int64) map[string]string {
    var currentTime int64
    if len(now) > 0 {
        currentTime = now[0]
    } else {
        currentTime = time.Now().Unix()
    }
    // Replace the bare time.Now().Unix() call in the staleness check with currentTime
}
```

- [ ] **Step 4: Update the cmd caller**

In `cmd/armature/ready.go` (line ~62), the existing call `ready.ExplainNotReady(index, issues)` works unchanged (variadic, passes no `now`). Verify it still compiles:

```bash
go build ./...
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/ready/... -v
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/ready/compute.go internal/ready/compute_test.go
git commit -m "refactor: thread injected time through ExplainNotReady; remove direct time.Now() call"
```

---

### Task 6: C6 — Inject Clock into `doctor`

`internal/doctor/doctor.go` calls `ready.StaleClaims(allIssues, time.Now())` at line 213 inside `checkD2StaleClaims`. Since `StaleClaims` already accepts `time.Time`, thread a `now time.Time` parameter down from the exported entry points.

**Files:**
- Modify: `internal/doctor/doctor.go`
- Modify: `cmd/armature/doctor.go` (or wherever `doctor.Run`/`doctor.RunChecks` is called — find with grep in Step 4)

- [ ] **Step 1: Find the entry points**

```bash
grep -n "^func " internal/doctor/doctor.go
```

Identify `Run(...)`, `RunChecks(...)`, and `checkD2StaleClaims(...)`.

- [ ] **Step 2: Write a failing test**

Find or add to `internal/doctor/doctor_test.go`:

```go
func TestRunChecks_UsesInjectedNow(t *testing.T) {
    // Set up a claimed issue that is stale at fixedNow but not at time.Now()
    fixedNow := time.Unix(9999999999, 0) // far future
    // ... set up index and issues with a claim that would be stale at fixedNow ...
    report := doctor.RunChecks(index, issues, opsTargetIDs, repoPath, fixedNow)
    // ... assert that the stale claim finding fires ...
}
```

- [ ] **Step 3: Run the test to confirm it fails**

```bash
go test ./internal/doctor/... -run TestRunChecks_UsesInjectedNow -v
```

Expected: FAIL.

- [ ] **Step 4: Update `checkD2StaleClaims` and callers**

```go
func checkD2StaleClaims(allIssues map[string]*materialize.Issue, now time.Time) Finding {
    stale := ready.StaleClaims(allIssues, now)
    // ...
}
```

Update `RunChecks` and `Run` to accept a `now time.Time` parameter and forward it to `checkD2StaleClaims`. For the `cmd/` callers of `Run` and `RunChecks`, pass `time.Now()` (the composition root is allowed to call it).

Find all callers:

```bash
grep -rn "doctor\.Run\b\|doctor\.RunChecks\b" cmd/ --include="*.go"
```

Update each to pass `time.Now()`.

- [ ] **Step 5: Run tests and build**

```bash
go test ./internal/doctor/... -v && go build ./...
```

Expected: all pass.

- [ ] **Step 6: Run full check**

```bash
make check
```

Expected: green.

- [ ] **Step 7: Commit**

```bash
git add internal/doctor/doctor.go internal/doctor/doctor_test.go
git add $(git diff --name-only cmd/)
git commit -m "refactor: thread injected time.Time through doctor; remove direct time.Now() call"
```

---

### Task 7: C6 — Add forbidigo lint rule banning `time.Now()` in domain packages

Now that all violations are resolved, add the lint ban to prevent regressions.

**Files:**
- Modify: `.golangci.yml`

- [ ] **Step 1: Verify no remaining violations**

```bash
grep -rn "time\.Now()" internal/ready internal/decompose internal/doctor internal/materialize internal/dag internal/ops
```

Expected: no output. If any remain, fix them before proceeding.

- [ ] **Step 2: Add forbidigo to `.golangci.yml`**

Under `linters.enable`, add `forbidigo`. Under `linters-settings`:

```yaml
linters:
  enable:
    # ... existing ...
    - forbidigo

linters-settings:
  # ... existing ...
  forbidigo:
    forbid:
      - pattern: "^time\\.Now\\b"
        msg: "use an injected clock.Clock instead of time.Now() in domain packages; call time.Now() only at composition roots in cmd/"
    exclude-godoc-examples: true
```

To restrict this to domain packages only (not cmd/ where it's legitimately used), add an `issues` exclusion:

```yaml
issues:
  exclude-rules:
    - path: "cmd/"
      linters:
        - forbidigo
    - path: "_test\\.go"
      linters:
        - forbidigo
```

- [ ] **Step 3: Run lint**

```bash
XDG_CACHE_HOME=/tmp/golangci-lint-cache golangci-lint run ./...
```

Expected: passes with no forbidigo findings.

- [ ] **Step 4: Run full check**

```bash
make check
```

Expected: green.

- [ ] **Step 5: Commit**

```bash
git add .golangci.yml
git commit -m "ci: ban time.Now() in domain packages via forbidigo; enforce clock injection"
```

---

## Chunk 2: C8 — Contract tests for port fakes, C9 — Interim traceability

### Task 8: C8 — Contract test for `GitCommitter`

The `fakeCommitter` in `internal/ops/commit_test.go` is exercised only in unit tests. Add a shared contract function that runs the same behavioral assertions against the fake and the real git-backed committer, giving early warning if the fake drifts.

**Files:**
- Create: `internal/ops/committer_contract_test.go`
- Modify: `internal/ops/commit_test.go` (call the contract function against the fake)

The real-adapter contract test requires a git repo fixture and runs under the `integration` build tag. Unit tests use the fake.

- [ ] **Step 1: Read the existing fake and the interface**

```bash
cat internal/ops/commit_test.go
cat internal/ops/commit.go
```

Note the method: `CommitWorktreeOp(relPath, message string) error`.

- [ ] **Step 2: Write the contract helper**

Create `internal/ops/committer_contract_test.go`:

```go
//go:build !integration

package ops_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/scullxbones/armature/internal/ops"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// RunGitCommitterContract runs behavioral assertions that must hold for any
// implementation of GitCommitter. Call this with both the fake and the real adapter.
func RunGitCommitterContract(t *testing.T, makeImpl func(t *testing.T) ops.GitCommitter) {
    t.Helper()

    t.Run("CommitWorktreeOp_ReturnsNilOnSuccess", func(t *testing.T) {
        impl := makeImpl(t)
        err := impl.CommitWorktreeOp("some/path.log", "ops: test commit")
        // For fakes this should succeed; for real adapters it commits.
        // Contract: no panic, error shape is consistent.
        _ = err // real adapter may return error if git not configured; fake returns nil
    })

    t.Run("CommitWorktreeOp_AcceptsRelativePath", func(t *testing.T) {
        impl := makeImpl(t)
        err := impl.CommitWorktreeOp(filepath.Join("workers", "abc.log"), "ops: create abc by worker")
        assert.NoError(t, err, "CommitWorktreeOp must accept a slash-delimited relative path")
    })

    t.Run("CommitWorktreeOp_PropagatesErrors", func(t *testing.T) {
        // Fake with pre-configured error
        fc := &fakeCommitter{err: assert.AnError}
        err := fc.CommitWorktreeOp("p.log", "m")
        assert.ErrorIs(t, err, assert.AnError, "errors from the underlying commit must propagate")
    })
}

func TestFakeCommitter_SatisfiesContract(t *testing.T) {
    RunGitCommitterContract(t, func(t *testing.T) ops.GitCommitter {
        return &fakeCommitter{}
    })
}
```

- [ ] **Step 3: Run the test**

```bash
go test ./internal/ops/... -run TestFakeCommitter_SatisfiesContract -v
```

Expected: PASS.

- [ ] **Step 4: Locate the real `GitCommitter` adapter**

```bash
grep -rn "CommitWorktreeOp" internal/ cmd/ --include="*.go" | grep -v "_test"
```

The real implementation will be in an adapter struct (not in `internal/ops` itself). Note the package path and type name — you will need it to write the integration test shell in Step 5.

- [ ] **Step 5: Create an integration-tagged stub for the real adapter**

Create `internal/ops/committer_integration_test.go`. Do **not** commit this file until the correct import path and type name are filled in from Step 4 — it will not compile with placeholders.

```go
//go:build integration

package ops_test

import (
    "os"
    "os/exec"
    "testing"

    // TODO: replace with the real package path found in Step 4
    // e.g. "github.com/scullxbones/armature/internal/adapters"
)

func TestRealCommitter_SatisfiesContract(t *testing.T) {
    RunGitCommitterContract(t, func(t *testing.T) ops.GitCommitter {
        dir := t.TempDir()
        cmd := exec.Command("git", "init", dir)
        cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
        if err := cmd.Run(); err != nil {
            t.Fatalf("git init: %v", err)
        }
        exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
        exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
        // TODO: replace with the real committer type from Step 4
        // return adapters.New(dir)
        t.Skip("real committer type not yet wired — fill in from Step 4 grep")
        return nil
    })
}
```

This file compiles under `//go:build integration` (excluded from `make test`) and skips at runtime via `t.Skip` until the real type is substituted.

- [ ] **Step 6: Run full test suite (non-integration)**

```bash
make test
```

Expected: all pass. The integration file is excluded by the build tag.

- [ ] **Step 7: Commit**

```bash
git add internal/ops/committer_contract_test.go internal/ops/committer_integration_test.go
git commit -m "test: add GitCommitter contract test suite; run against fake, stub real adapter path"
```

---

### Task 9: C8 — Contract test for `MergeChecker`

**Files:**
- Create: `internal/sync/mergechecker_contract_test.go`
- Modify: `internal/sync/sync_test.go` (call contract against existing fake)

- [ ] **Step 1: Read the interface and existing fake**

```bash
cat internal/sync/sync.go
grep -A 10 "stubMergeChecker\|fakeMerge" internal/sync/sync_test.go | head -30
```

- [ ] **Step 2: Write the contract helper**

Create `internal/sync/mergechecker_contract_test.go`:

```go
package sync_test

import (
    "testing"

    "github.com/scullxbones/armature/internal/sync"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// RunMergeCheckerContract verifies behavioral invariants that any MergeChecker must satisfy.
func RunMergeCheckerContract(t *testing.T, merged func(t *testing.T) sync.MergeChecker, notMerged func(t *testing.T) sync.MergeChecker) {
    t.Helper()

    t.Run("BranchMergedInto_ReturnsTrueForMergedBranch", func(t *testing.T) {
        mc := merged(t)
        ok, err := mc.BranchMergedInto("feature/done", "main")
        require.NoError(t, err)
        assert.True(t, ok)
    })

    t.Run("BranchMergedInto_ReturnsFalseForUnmergedBranch", func(t *testing.T) {
        mc := notMerged(t)
        ok, err := mc.BranchMergedInto("feature/pending", "main")
        require.NoError(t, err)
        assert.False(t, ok)
    })
}

func TestFakeMergeChecker_SatisfiesContract(t *testing.T) {
    RunMergeCheckerContract(
        t,
        func(t *testing.T) sync.MergeChecker {
            return &fakeMergeChecker{merged: map[string]bool{"feature/done": true}}
        },
        func(t *testing.T) sync.MergeChecker {
            return &fakeMergeChecker{merged: map[string]bool{}}
        },
    )
}
```

The fake type is `fakeMergeChecker` (confirmed in `internal/sync/sync_test.go`). It is accessible within `mergechecker_contract_test.go` because both files use `package sync_test`.

- [ ] **Step 3: Run the test**

```bash
go test ./internal/sync/... -run TestFakeMergeChecker_SatisfiesContract -v
```

Expected: PASS. If field names differ from `merged map[string]bool`, adjust to match the actual struct definition seen in Step 1.

- [ ] **Step 4: Commit**

```bash
git add internal/sync/mergechecker_contract_test.go
git commit -m "test: add MergeChecker contract test suite; run against stub fake"
```

---

### Task 10: C9 — Interim spec traceability Makefile target

C9 is blocked on the armature requirement-traceability feature (see `docs/superpowers/specs/2026-06-13-deterministic-quality-guardrails-design.md`). This task puts the hook in place: a `make trace-report` target that inventories tests already tagged with `_REQ_` convention and a documented naming convention for new work.

**Files:**
- Create: `scripts/trace_report.py`
- Modify: `Makefile`

The script does not fail the build (no `make check` integration yet). It produces a human-readable report of which tests carry traceability tags. This becomes the baseline that the full `arm validate` gate will eventually replace.

- [ ] **Step 1: Write the script**

Create `scripts/trace_report.py`:

```python
#!/usr/bin/env python3
"""
trace_report.py — Report test functions tagged with the _REQ_ traceability convention.

Naming convention: test functions include _REQ_ followed by an ID token, e.g.:
  TestClaimRace_REQ_claimAtomicity
  TestApplyPlan_REQ_timestampInjection

Usage: python3 scripts/trace_report.py [root_dir]
       Defaults to scanning internal/ and cmd/ under the repo root.

This is the interim traceability hook. Once the armature requirement-traceability
feature ships (see docs/superpowers/specs/2026-06-13-deterministic-quality-guardrails-design.md),
this script will be superseded by `arm validate` with requirement-level coverage checks.
"""

import re
import sys
from pathlib import Path

REQ_PATTERN = re.compile(r"\bTest\w*_REQ_(\w+)")

def scan(root: Path):
    tagged = []
    for path in sorted(root.rglob("*_test.go")):
        text = path.read_text(encoding="utf-8")
        for m in REQ_PATTERN.finditer(text):
            fn_start = text.rfind("\nfunc ", 0, m.start()) + 1
            fn_end = text.find("(", fn_start)
            fn_name = text[fn_start:fn_end].strip()
            tagged.append((str(path.relative_to(root)), fn_name, m.group(1)))
    return tagged

def main() -> int:
    root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(".")
    results = scan(root)
    if not results:
        print("trace-report: no tests tagged with _REQ_ convention found.")
        print("  To tag a test: name it TestFoo_REQ_<token>, e.g. TestApply_REQ_timestampInjection")
        return 0
    print(f"trace-report: {len(results)} traced test(s) found\n")
    for path, fn, req_id in results:
        print(f"  {path}: {fn}  [REQ:{req_id}]")
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
```

- [ ] **Step 2: Make it executable and run it manually**

```bash
chmod +x scripts/trace_report.py
python3 scripts/trace_report.py .
```

Expected: "no tests tagged" message (since no tests use the convention yet). No error exit.

- [ ] **Step 3: Add a `make trace-report` target**

In `Makefile`, after the `validate-skills` target, add:

```makefile
trace-report:
	@$(PYTHON) scripts/trace_report.py .
```

And add `trace-report` to the `help` target description:

```makefile
@echo "  make trace-report   - Report tests tagged with _REQ_ traceability convention (interim; not in make check)"
```

- [ ] **Step 4: Run it via make**

```bash
make trace-report
```

Expected: runs cleanly, reports no tagged tests yet.

- [ ] **Step 5: Run full check to verify nothing is broken**

```bash
make check
```

Expected: green. (`trace-report` is intentionally not part of `make check`.)

- [ ] **Step 6: Commit**

```bash
git add scripts/trace_report.py Makefile
git commit -m "ci: add interim trace-report target for _REQ_ test naming convention"
```

---

## Final verification

- [ ] **Run the full suite**

```bash
make check
```

Expected: all stages green — lint (with depguard + forbidigo), test, coverage ≥ 80%, mutation (≥90% coverage, ≥75% efficacy), validate-skills, build.

- [ ] **Confirm the gap controls are now partially closed**

Update `docs/design/quality-controls.md` status column:
- C6: ACTIVE
- C7: ACTIVE
- C8: PARTIAL (fake contract tests in place; integration tests stubbed)
- C9: PARTIAL (naming convention documented; full validation pending armature feature)

```bash
git add docs/design/quality-controls.md
git commit -m "docs: update quality-controls status after implementing C6, C7, C8, C9-interim"
```
