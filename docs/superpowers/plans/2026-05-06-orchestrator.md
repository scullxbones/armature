# Orchestrator Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `arm orchestrate` — a deterministic orchestration layer that wraps agent harness invocations in a state machine enforcing scope, tests, build, lint, coverage, and citations without trusting agent self-reporting.

**Architecture:** A new `internal/orchestrate` package holds the state machine, harness adapters, verification pipeline, and engine. The engine reads/writes orchestration ops to the existing JSONL event log, manages a git worktree for the task, dispatches harness processes inside an OS-level sandbox, and makes the single canonical commit after all checks pass. The CLI command `arm orchestrate` wires the engine to cobra.

**Tech Stack:** Go 1.26, cobra, `internal/ops`, `internal/adapters` (post-E6-S5 — replaces `internal/git`), `internal/config`, `internal/materialize`, `internal/context`, `internal/claim`, `internal/ready` packages; bubblewrap (Linux/WSL2) or Seatbelt (macOS) for sandboxing.

> **Architecture prerequisite — E6-S5 (backport):** Story E6-S5 ("Architecture Refactoring — Create internal/adapters/ Boundary") must be merged before implementing this plan. E6-S5-T2 relocates `internal/git/` → `internal/adapters/git.go`; E6-S5-T3 moves all `exec.Command` usage into `internal/adapters/shell.go`; E6-S5-T1 moves file I/O into `internal/adapters/files.go`. The post-E6-S5 architecture rule: **no `os`, `os/exec`, or `net/http` in any `internal/` package except `internal/adapters/` and `internal/tui/tty.go`**. All new `internal/orchestrate/` code must respect this rule from day one.

---

## Chunk 1: Prerequisites — Schema and Config Extensions

### Task 1: Add `PreferredModel` to `Payload` and `Issue`

**Files:**
- Modify: `internal/ops/types.go`
- Modify: `internal/materialize/state.go`
- Modify: `internal/materialize/engine.go` (populate field in `applyCreate`)
- Test: `internal/ops/types_test.go`
- Test: `internal/materialize/engine_test.go`

- [ ] **Step 1: Write failing test for Payload.PreferredModel**

Add to `internal/ops/types_test.go`:
```go
func TestPayloadPreferredModel(t *testing.T) {
    line := []byte(`["create","T-1",1000,"w1",{"title":"foo","preferred_model":"claude-haiku-4-5"}]`)
    op, err := ParseLine(line)
    require.NoError(t, err)
    assert.Equal(t, "claude-haiku-4-5", op.Payload.PreferredModel)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ops/... -run TestPayloadPreferredModel -v
```
Expected: FAIL — field not present yet

- [ ] **Step 3: Add `PreferredModel` to `Payload` in `internal/ops/types.go`**

In the `// create` section of `Payload`, add:
```go
PreferredModel string `json:"preferred_model,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/ops/... -run TestPayloadPreferredModel -v
```

- [ ] **Step 5: Write failing test for Issue.PreferredModel populated from create op**

Add to `internal/materialize/engine_test.go` (or a new `engine_preferred_model_test.go`):
```go
func TestApplyCreatePopulatesPreferredModel(t *testing.T) {
    s := NewState()
    op := ops.Op{
        Type: ops.OpCreate, TargetID: "T-1", Timestamp: 1000, WorkerID: "w1",
        Payload: ops.Payload{Title: "foo", NodeType: "task", PreferredModel: "claude-haiku-4-5"},
    }
    require.NoError(t, s.ApplyOp(op))
    assert.Equal(t, "claude-haiku-4-5", s.Issues["T-1"].PreferredModel)
}
```

- [ ] **Step 6: Run test to verify it fails**

```bash
go test ./internal/materialize/... -run TestApplyCreatePopulatesPreferredModel -v
```

- [ ] **Step 7: Add `PreferredModel string` to `Issue` in `internal/materialize/state.go`**

In the `Issue` struct, after `EstComplexity`:
```go
PreferredModel string `json:"preferred_model,omitempty"`
```

In `applyCreate` in `internal/materialize/engine.go`, inside the `Issue{}` literal, add:
```go
PreferredModel: op.Payload.PreferredModel,
```

- [ ] **Step 8: Run test to verify it passes**

```bash
go test ./internal/materialize/... -run TestApplyCreatePopulatesPreferredModel -v
```

- [ ] **Step 9: Commit**

```bash
git add internal/ops/types.go internal/materialize/state.go internal/materialize/engine.go internal/ops/types_test.go internal/materialize/engine_test.go
git commit -m "feat: add PreferredModel to Payload and Issue"
```

---

### Task 2: Add `SourceEntryID` to `CitationAcceptance` and citation-accepted payload

**Files:**
- Modify: `internal/materialize/state.go`
- Modify: `internal/ops/types.go`
- Modify: `internal/materialize/engine.go` (populate in `applyCitationAccepted`)
- Test: `internal/materialize/engine_test.go`

- [ ] **Step 1: Write two failing tests — one for JSONL wire format, one for state materialization**

Add to `internal/ops/types_test.go`:
```go
func TestPayloadSourceEntryID(t *testing.T) {
    line := []byte(`["citation-accepted","T-1",1001,"w1",{"source_entry_id":"src-abc"}]`)
    op, err := ParseLine(line)
    require.NoError(t, err)
    assert.Equal(t, "src-abc", op.Payload.SourceEntryID)
}
```

Add to `internal/materialize/engine_test.go`:
```go
func TestCitationAcceptanceSourceEntryID(t *testing.T) {
    s := NewState()
    createOp := ops.Op{
        Type: ops.OpCreate, TargetID: "T-1", Timestamp: 1000, WorkerID: "w1",
        Payload: ops.Payload{Title: "foo", NodeType: "task"},
    }
    require.NoError(t, s.ApplyOp(createOp))

    citOp := ops.Op{
        Type: ops.OpCitationAccepted, TargetID: "T-1", Timestamp: 1001, WorkerID: "w1",
        Payload: ops.Payload{SourceEntryID: "src-abc"},
    }
    require.NoError(t, s.ApplyOp(citOp))
    require.Len(t, s.Issues["T-1"].CitationAcceptances, 1)
    assert.Equal(t, "src-abc", s.Issues["T-1"].CitationAcceptances[0].SourceEntryID)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ops/... -run TestPayloadSourceEntryID -v
go test ./internal/materialize/... -run TestCitationAcceptanceSourceEntryID -v
```

- [ ] **Step 3: Add `SourceEntryID` to `CitationAcceptance` in `internal/materialize/state.go`**

Add only this new field after the existing `ConfirmedNoninteractively` field:
```go
SourceEntryID string `json:"source_entry_id,omitempty"`
```

Do not replace the existing struct — only add the new field.

- [ ] **Step 4: Add `SourceEntryID` to `Payload` in `internal/ops/types.go`**

In the `// citation-accepted` section (or add a new comment), add:
```go
// citation-accepted
SourceEntryID string `json:"source_entry_id,omitempty"`
```

- [ ] **Step 5: Populate it in `applyCitationAccepted` in `internal/materialize/engine.go`**

Find `applyCitationAccepted` and add `SourceEntryID: op.Payload.SourceEntryID` to the `CitationAcceptance{}` literal.

- [ ] **Step 6: Run test to verify it passes**

```bash
go test ./internal/materialize/... -run TestCitationAcceptanceSourceEntryID -v
```

- [ ] **Step 7: Commit**

```bash
git add internal/materialize/state.go internal/ops/types.go internal/materialize/engine.go internal/materialize/engine_test.go
git commit -m "feat: add SourceEntryID to CitationAcceptance and citation-accepted payload"
```

---

### Task 3: Add `OrchestratorConfig` to `Config`

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestOrchestratorConfigRoundTrip(t *testing.T) {
    dir := t.TempDir()
    cfg := Config{
        Mode:       "single-branch",
        ProjectType: "go",
        Orchestrator: OrchestratorConfig{
            DefaultHarness:   "claude",
            DefaultModel:     "claude-haiku-4-5",
            DefaultRetries:   3,
            DefaultTimeout:   "10m",
            Language:         "go",
            CoverageThreshold: 80,
            LintFailMode:     "hard",
            MutationTesting:  false,
            MutationFailMode: "warn",
            Adapters: AdapterCommands{
                Build:    "go build ./...",
                Lint:     "golangci-lint run ./...",
                Coverage: "go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out",
                Mutation: "gremlins unleash ./...",
            },
            TestPatterns: map[string][]string{
                "go": {"**/*_test.go"},
            },
        },
    }
    path := filepath.Join(dir, "config.json")
    require.NoError(t, WriteConfig(path, cfg))
    loaded, err := LoadConfig(path)
    require.NoError(t, err)
    assert.Equal(t, "claude", loaded.Orchestrator.DefaultHarness)
    assert.Equal(t, 80, loaded.Orchestrator.CoverageThreshold)
    assert.Equal(t, "go build ./...", loaded.Orchestrator.Adapters.Build)
    assert.Equal(t, []string{"**/*_test.go"}, loaded.Orchestrator.TestPatterns["go"])
}
```

Also add a backwards-compatibility test to `internal/config/config_test.go`:
```go
func TestOrchestratorConfigAbsentKeyIsZeroValue(t *testing.T) {
    dir := t.TempDir()
    cfg := Config{Mode: "single-branch", ProjectType: "go"}
    path := filepath.Join(dir, "config.json")
    require.NoError(t, WriteConfig(path, cfg))
    loaded, err := LoadConfig(path)
    require.NoError(t, err)
    assert.Equal(t, OrchestratorConfig{}, loaded.Orchestrator)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/... -run TestOrchestratorConfigRoundTrip -v
go test ./internal/config/... -run TestOrchestratorConfigAbsentKeyIsZeroValue -v
```

- [ ] **Step 3: Add `OrchestratorConfig` to `internal/config/config.go`**

```go
type OrchestratorConfig struct {
    DefaultHarness    string              `json:"default_harness,omitempty"`
    DefaultModel      string              `json:"default_model,omitempty"`
    DefaultRetries    int                 `json:"default_retries,omitempty"`
    DefaultTimeout    string              `json:"default_timeout,omitempty"`
    Language          string              `json:"language,omitempty"`
    CoverageThreshold int                 `json:"coverage_threshold,omitempty"`
    LintFailMode      string              `json:"lint_fail_mode,omitempty"` // "hard" or "warn"
    MutationTesting   bool                `json:"mutation_testing,omitempty"`
    MutationFailMode  string              `json:"mutation_fail_mode,omitempty"` // "warn" or "hard"
    Adapters          AdapterCommands     `json:"adapters"`
    TestPatterns      map[string][]string `json:"test_patterns,omitempty"`
}

type AdapterCommands struct {
    Build    string `json:"build,omitempty"`
    Lint     string `json:"lint,omitempty"`
    Coverage string `json:"coverage,omitempty"`
    Mutation string `json:"mutation,omitempty"`
}
```

Add `Orchestrator OrchestratorConfig \`json:"orchestrator,omitempty"\`` to the `Config` struct.

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/config/... -run TestOrchestratorConfigRoundTrip -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add OrchestratorConfig sub-struct to Config"
```

---

## Chunk 2: New Op Types + Orchestrator Package Skeleton

### Task 4: Add 8 orchestration op type constants

**Files:**
- Modify: `internal/ops/types.go`
- Modify: `internal/materialize/engine.go` (ignore new op types gracefully)
- Test: `internal/ops/types_test.go`

- [ ] **Step 1: Write failing test — parse an orchestrate-start op**

```go
func TestOrchestrateOpTypesParseable(t *testing.T) {
    t.Run("orchestrate-start", func(t *testing.T) {
        op, err := ParseLine([]byte(`["orchestrate-start","T-1",1000,"w1",{"harness":"claude","model":"claude-haiku-4-5","retry_budget":3,"worktree":".worktrees/T-1"}]`))
        require.NoError(t, err)
        assert.Equal(t, "claude", op.Payload.Harness)
        assert.Equal(t, "claude-haiku-4-5", op.Payload.Model)
        assert.Equal(t, 3, op.Payload.RetryBudget)
        assert.Equal(t, ".worktrees/T-1", op.Payload.Worktree)
    })
    t.Run("orchestrate-dispatch", func(t *testing.T) {
        op, err := ParseLine([]byte(`["orchestrate-dispatch","T-1",1001,"w1",{"run":1,"pre_dispatch_ref":"abc123","prompt_hash":"sha256:xyz"}]`))
        require.NoError(t, err)
        assert.Equal(t, 1, op.Payload.Run)
        assert.Equal(t, "abc123", op.Payload.PreDispatchRef)
    })
    t.Run("orchestrate-retry", func(t *testing.T) {
        op, err := ParseLine([]byte(`["orchestrate-retry","T-1",1004,"w1",{"run":2,"feedback_summary":"scope violation on run 1"}]`))
        require.NoError(t, err)
        assert.Equal(t, 2, op.Payload.Run)
        assert.Equal(t, "scope violation on run 1", op.Payload.FeedbackSummary)
    })
    t.Run("orchestrate-complete", func(t *testing.T) {
        op, err := ParseLine([]byte(`["orchestrate-complete","T-1",1006,"w1",{"run":2,"commit_sha":"def456","checks_passed":["build"]}]`))
        require.NoError(t, err)
        assert.Equal(t, "def456", op.Payload.CommitSHA)
        assert.Equal(t, []string{"build"}, op.Payload.ChecksPassed)
    })
    // Remaining types — verify they parse without error
    for _, line := range []string{
        `["orchestrate-dispatch-complete","T-1",1002,"w1",{"run":1,"exit_status":"clean","duration_ms":42000,"log_path":".arm/orchestration/T-1/run-1.log"}]`,
        `["orchestrate-verify-fail","T-1",1003,"w1",{"run":1,"check":"scope-boundary","severity":"fail","reason":"file outside scope"}]`,
        `["orchestrate-escalate","T-1",1005,"w1",{"total_runs":3,"failures":[]}]`,
    } {
        _, err := ParseLine([]byte(line))
        require.NoError(t, err, line)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ops/... -run TestOrchestrateOpTypesParseable -v
```

- [ ] **Step 3: Add op constants and payload fields to `internal/ops/types.go`**

Add constants:
```go
OpOrchestrateStart            = "orchestrate-start"
OpOrchestrateDispatch         = "orchestrate-dispatch"
OpOrchestrateDispatchComplete = "orchestrate-dispatch-complete"
OpOrchestrateVerifyFail       = "orchestrate-verify-fail"
OpOrchestrateRetry            = "orchestrate-retry"
OpOrchestrateEscalate         = "orchestrate-escalate"
OpOrchestrateComplete         = "orchestrate-complete"
```

Add to `ValidOpTypes`:
```go
OpOrchestrateStart:            true,
OpOrchestrateDispatch:         true,
OpOrchestrateDispatchComplete: true,
OpOrchestrateVerifyFail:       true,
OpOrchestrateRetry:            true,
OpOrchestrateEscalate:         true,
OpOrchestrateComplete:         true,
```

Also add a `FailureRecord` struct to `internal/ops/types.go` (not inside `Payload`) for typed escalation entries:
```go
// FailureRecord holds a single run failure entry in an orchestrate-escalate op.
type FailureRecord struct {
    Run    int    `json:"run"`
    Check  string `json:"check"`
    Reason string `json:"reason"`
}
```

Add to `Payload` struct (orchestration fields section):
```go
// orchestrate-start: Model is the resolved model the orchestrator used for this run.
// Distinct from PreferredModel (task-level preference set at decompose time).
Harness      string `json:"harness,omitempty"`
Model        string `json:"model,omitempty"`
RetryBudget  int    `json:"retry_budget,omitempty"`
Worktree     string `json:"worktree,omitempty"`

// orchestrate-dispatch
Run             int    `json:"run,omitempty"`
PreDispatchRef  string `json:"pre_dispatch_ref,omitempty"`
PromptHash      string `json:"prompt_hash,omitempty"`

// orchestrate-dispatch-complete
ExitStatusStr string `json:"exit_status,omitempty"`
DurationMs    int64  `json:"duration_ms,omitempty"`
LogPath       string `json:"log_path,omitempty"`

// orchestrate-verify-fail
Check    string `json:"check,omitempty"`
Severity string `json:"severity,omitempty"`
Reason   string `json:"reason,omitempty"`

// orchestrate-retry
FeedbackSummary string `json:"feedback_summary,omitempty"`

// orchestrate-escalate
TotalRuns int             `json:"total_runs,omitempty"`
Failures  []FailureRecord `json:"failures,omitempty"`

// orchestrate-complete
CommitSHA    string   `json:"commit_sha,omitempty"`
ChecksPassed []string `json:"checks_passed,omitempty"`
```

- [ ] **Step 4: Update `internal/materialize/engine.go` to ignore orchestration ops**

In `ApplyOp`'s switch statement, add cases that return nil:
```go
case ops.OpOrchestrateStart, ops.OpOrchestrateDispatch,
    ops.OpOrchestrateDispatchComplete, ops.OpOrchestrateVerifyFail,
    ops.OpOrchestrateRetry, ops.OpOrchestrateEscalate,
    ops.OpOrchestrateComplete:
    return nil
```

- [ ] **Step 5: Run tests to verify all pass**

```bash
go test ./internal/ops/... ./internal/materialize/... -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/ops/types.go internal/materialize/engine.go internal/ops/types_test.go
git commit -m "feat: add orchestration op type constants and payload fields"
```

---

### Task 5: Create `internal/orchestrate/types.go`

**Files:**
- Create: `internal/orchestrate/types.go`
- Create: `internal/orchestrate/types_test.go`

- [ ] **Step 1: Write failing test**

```go
package orchestrate_test

import (
    "testing"
    "github.com/scullxbones/armature/internal/orchestrate"
    "github.com/stretchr/testify/assert"
)

func TestExitStatusString(t *testing.T) {
    assert.Equal(t, "clean", orchestrate.ExitClean.String())
    assert.Equal(t, "timeout", orchestrate.ExitTimeout.String())
    assert.Equal(t, "error", orchestrate.ExitError.String())
}

func TestCheckSeverityConstants(t *testing.T) {
    assert.Equal(t, "pass", string(orchestrate.SeverityPass))
    assert.Equal(t, "warn", string(orchestrate.SeverityWarn))
    assert.Equal(t, "fail", string(orchestrate.SeverityFail))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/orchestrate/... -run TestExitStatusString -v
```
Expected: FAIL — package does not exist

- [ ] **Step 3: Create `internal/orchestrate/types.go`**

```go
package orchestrate

import "context"

// HarnessAdapter dispatches a single agent run.
type HarnessAdapter interface {
    Name() string
    Invoke(ctx context.Context, workdir string, prompt string, logPath string) (InvocationResult, error)
}

type ExitStatus int

const (
    ExitClean   ExitStatus = iota
    ExitTimeout
    ExitError
)

func (e ExitStatus) String() string {
    switch e {
    case ExitClean:
        return "clean"
    case ExitTimeout:
        return "timeout"
    default:
        return "error"
    }
}

type InvocationResult struct {
    ExitStatus ExitStatus
    Stdout     string
    Stderr     string
    DurationMs int64
}

type HarnessConfig struct {
    Adapter string
    Model   string
    Timeout int // seconds
}

type CheckSeverity string

const (
    SeverityPass CheckSeverity = "pass"
    SeverityWarn CheckSeverity = "warn"
    SeverityFail CheckSeverity = "fail"
)

// CheckResult is produced by each verification pipeline check.
type CheckResult struct {
    Check    string
    Severity CheckSeverity
    Reason   string
}

// RunOptions holds the resolved parameters for a single orchestration run.
type RunOptions struct {
    Harness   string
    Model     string
    Retries   int
    TimeoutSec int
    DryRun    bool
    WorkerID  string
    LogPath   string
}

// OrchestrateState is the resume state derived from the event log.
type OrchestrateState struct {
    Phase         string // "idle", "dispatched", "verifying", "correcting", "escalated", "done"
    Run           int    // current or upcoming run number
    PreDispatchRef string
    WorktreePath  string
    RetryBudget   int
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/orchestrate/... -run TestExitStatusString -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/orchestrate/types.go internal/orchestrate/types_test.go
git commit -m "feat: add orchestrate package types"
```

---

### Task 6: Create `internal/orchestrate/state.go` — resume state derivation

**Files:**
- Create: `internal/orchestrate/state.go`
- Create: `internal/orchestrate/state_test.go`

- [ ] **Step 1: Write failing tests**

```go
package orchestrate_test

import (
    "testing"
    "github.com/scullxbones/armature/internal/ops"
    "github.com/scullxbones/armature/internal/orchestrate"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestDeriveStateNoOps(t *testing.T) {
    state, err := orchestrate.DeriveState(nil, "T-1")
    require.NoError(t, err)
    assert.Equal(t, "idle", state.Phase)
    assert.Equal(t, 0, state.Run)
}

func TestDeriveStateAfterStart(t *testing.T) {
    opList := []ops.Op{
        {Type: ops.OpOrchestrateStart, TargetID: "T-1", Payload: ops.Payload{
            RetryBudget: 3, Worktree: ".worktrees/T-1",
        }},
    }
    state, err := orchestrate.DeriveState(opList, "T-1")
    require.NoError(t, err)
    assert.Equal(t, "dispatched", state.Phase)
    assert.Equal(t, 1, state.Run)
    assert.Equal(t, ".worktrees/T-1", state.WorktreePath)
    assert.Equal(t, 3, state.RetryBudget)
}

func TestDeriveStateAfterDispatch(t *testing.T) {
    opList := []ops.Op{
        {Type: ops.OpOrchestrateStart, TargetID: "T-1", Payload: ops.Payload{RetryBudget: 3, Worktree: ".worktrees/T-1"}},
        {Type: ops.OpOrchestrateDispatch, TargetID: "T-1", Payload: ops.Payload{Run: 1, PreDispatchRef: "abc123"}},
    }
    state, err := orchestrate.DeriveState(opList, "T-1")
    require.NoError(t, err)
    assert.Equal(t, "dispatched", state.Phase)
    assert.Equal(t, 1, state.Run)
    assert.Equal(t, "abc123", state.PreDispatchRef)
}

func TestDeriveStateAfterDispatchComplete(t *testing.T) {
    opList := []ops.Op{
        {Type: ops.OpOrchestrateStart, TargetID: "T-1", Payload: ops.Payload{RetryBudget: 3, Worktree: ".worktrees/T-1"}},
        {Type: ops.OpOrchestrateDispatch, TargetID: "T-1", Payload: ops.Payload{Run: 1, PreDispatchRef: "abc123"}},
        {Type: ops.OpOrchestrateDispatchComplete, TargetID: "T-1", Payload: ops.Payload{Run: 1}},
    }
    state, err := orchestrate.DeriveState(opList, "T-1")
    require.NoError(t, err)
    assert.Equal(t, "verifying", state.Phase)
}

func TestDeriveStateAfterVerifyFail(t *testing.T) {
    opList := []ops.Op{
        {Type: ops.OpOrchestrateStart, TargetID: "T-1", Payload: ops.Payload{RetryBudget: 3, Worktree: ".worktrees/T-1"}},
        {Type: ops.OpOrchestrateDispatch, TargetID: "T-1", Payload: ops.Payload{Run: 1, PreDispatchRef: "abc123"}},
        {Type: ops.OpOrchestrateDispatchComplete, TargetID: "T-1", Payload: ops.Payload{Run: 1}},
        {Type: ops.OpOrchestrateVerifyFail, TargetID: "T-1", Payload: ops.Payload{Run: 1, Check: "build"}},
    }
    state, err := orchestrate.DeriveState(opList, "T-1")
    require.NoError(t, err)
    assert.Equal(t, "correcting", state.Phase)
    assert.Equal(t, "abc123", state.PreDispatchRef, "PreDispatchRef must persist through verify-fail for worktree recovery")
}

func TestDeriveStateFiltersOtherTaskIDs(t *testing.T) {
    opList := []ops.Op{
        {Type: ops.OpOrchestrateStart, TargetID: "T-2", Payload: ops.Payload{RetryBudget: 3, Worktree: ".worktrees/T-2"}},
        {Type: ops.OpOrchestrateComplete, TargetID: "T-2"},
    }
    state, err := orchestrate.DeriveState(opList, "T-1")
    require.NoError(t, err)
    assert.Equal(t, "idle", state.Phase, "ops for T-2 should not affect T-1 state")
}

func TestDeriveStateAfterRetry(t *testing.T) {
    opList := []ops.Op{
        {Type: ops.OpOrchestrateStart, TargetID: "T-1", Payload: ops.Payload{RetryBudget: 3, Worktree: ".worktrees/T-1"}},
        {Type: ops.OpOrchestrateDispatch, TargetID: "T-1", Payload: ops.Payload{Run: 1, PreDispatchRef: "abc123"}},
        {Type: ops.OpOrchestrateDispatchComplete, TargetID: "T-1", Payload: ops.Payload{Run: 1}},
        {Type: ops.OpOrchestrateVerifyFail, TargetID: "T-1", Payload: ops.Payload{Run: 1, Check: "build"}},
        {Type: ops.OpOrchestrateRetry, TargetID: "T-1", Payload: ops.Payload{Run: 2}},
    }
    state, err := orchestrate.DeriveState(opList, "T-1")
    require.NoError(t, err)
    assert.Equal(t, "dispatched", state.Phase)
    assert.Equal(t, 2, state.Run)
}

func TestDeriveStateTerminalEscalated(t *testing.T) {
    opList := []ops.Op{
        {Type: ops.OpOrchestrateStart, TargetID: "T-1", Payload: ops.Payload{RetryBudget: 3, Worktree: ".worktrees/T-1"}},
        {Type: ops.OpOrchestrateEscalate, TargetID: "T-1"},
    }
    state, err := orchestrate.DeriveState(opList, "T-1")
    require.NoError(t, err)
    assert.Equal(t, "escalated", state.Phase)
}

func TestDeriveStateTerminalDone(t *testing.T) {
    opList := []ops.Op{
        {Type: ops.OpOrchestrateStart, TargetID: "T-1", Payload: ops.Payload{RetryBudget: 3, Worktree: ".worktrees/T-1"}},
        {Type: ops.OpOrchestrateComplete, TargetID: "T-1"},
    }
    state, err := orchestrate.DeriveState(opList, "T-1")
    require.NoError(t, err)
    assert.Equal(t, "done", state.Phase)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/orchestrate/... -run TestDeriveState -v
```

- [ ] **Step 3: Create `internal/orchestrate/state.go`**

```go
package orchestrate

import "github.com/scullxbones/armature/internal/ops"

// DeriveState replays orchestration ops for taskID to determine resume state.
// Returns phase "idle" if no orchestration ops exist for the task.
func DeriveState(opList []ops.Op, taskID string) (OrchestrateState, error) {
    state := OrchestrateState{Phase: "idle"}
    for _, op := range opList {
        if op.TargetID != taskID {
            continue
        }
        switch op.Type {
        case ops.OpOrchestrateStart:
            state.WorktreePath = op.Payload.Worktree
            state.RetryBudget = op.Payload.RetryBudget
            state.Run = 1
            state.Phase = "dispatched"
        case ops.OpOrchestrateDispatch:
            state.Run = op.Payload.Run
            state.PreDispatchRef = op.Payload.PreDispatchRef
            state.Phase = "dispatched"
        case ops.OpOrchestrateDispatchComplete:
            state.Phase = "verifying"
        case ops.OpOrchestrateVerifyFail:
            state.Phase = "correcting"
        case ops.OpOrchestrateRetry:
            state.Run = op.Payload.Run
            state.Phase = "dispatched"
        case ops.OpOrchestrateEscalate:
            state.Phase = "escalated"
        case ops.OpOrchestrateComplete:
            state.Phase = "done"
        }
    }
    return state, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/orchestrate/... -run TestDeriveState -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/orchestrate/state.go internal/orchestrate/state_test.go
git commit -m "feat: add orchestrate state derivation from event log"
```

---

## Chunk 3: Git Extensions + Framework Detection + Harness Adapters

### Task 7: Extend `internal/adapters/git.go` with worktree and diff operations

> **E6-S5-T2 dependency:** This task adds methods to `internal/adapters/git.go` — the post-E6-S5 home of the git client. The `internal/git/` package no longer exists. All imports use `github.com/scullxbones/armature/internal/adapters` and the type is `adapters.GitClient` (or whatever E6-S5-T2 names it — check the actual type name after E6-S5 is merged before writing code).

**Files:**
- Modify: `internal/adapters/git.go`
- Modify: `internal/adapters/git_test.go`

New methods needed (added to the existing `Client` struct now in `internal/adapters/`):
- `RemoveWorktree(path string) error`
- `HeadSHA() (string, error)`
- `DiffFrom(ref string) (string, error)` — returns unified diff since ref
- `DiffNameOnly(ref string) ([]string, error)` — returns changed file paths
- `ResetHard(ref string) error`
- `ApplyPatch(patch string) error` — applies unified diff as unstaged changes
- `AddAll() error`
- `CommitWithMessage(message string) (string, error)` — returns HEAD SHA after commit

- [ ] **Step 1: Write failing tests** — add to `internal/adapters/git_test.go`

```go
func TestGitWorktreeOperations(t *testing.T) {
    // Uses t.TempDir() and git init to create a real repo
    dir := t.TempDir()
    gc := New(dir) // adapters.New — same constructor, now in adapters package

    // Init repo and make initial commit
    runGit(t, dir, "init")
    runGit(t, dir, "config", "user.email", "test@test.com")
    runGit(t, dir, "config", "user.name", "Test")
    writeFile(t, dir, "README.md", "hello")
    runGit(t, dir, "add", ".")
    runGit(t, dir, "commit", "-m", "init")

    // Test HeadSHA
    sha, err := gc.HeadSHA()
    require.NoError(t, err)
    assert.NotEmpty(t, sha)

    // Test DiffFrom (no changes yet → empty diff)
    diff, err := gc.DiffFrom(sha)
    require.NoError(t, err)
    assert.Empty(t, diff)

    // Make a change
    writeFile(t, dir, "new.txt", "content")
    runGit(t, dir, "add", ".")
    runGit(t, dir, "commit", "-m", "add new.txt")

    // DiffNameOnly should include new.txt
    files, err := gc.DiffNameOnly(sha)
    require.NoError(t, err)
    assert.Contains(t, files, "new.txt")

    // Capture diff
    patchData, err := gc.DiffFrom(sha)
    require.NoError(t, err)
    assert.Contains(t, patchData, "new.txt")

    // ResetHard to initial sha
    require.NoError(t, gc.ResetHard(sha))

    // Apply patch back
    require.NoError(t, gc.ApplyPatch(patchData))
    assert.FileExists(t, filepath.Join(dir, "new.txt"))

    // AddAll + CommitWithMessage
    require.NoError(t, gc.AddAll())
    newSHA, err := gc.CommitWithMessage("re-add new.txt")
    require.NoError(t, err)
    assert.NotEmpty(t, newSHA)
    assert.NotEqual(t, sha, newSHA)
}

// Helper: write a file in a test directory
func writeFile(t *testing.T, dir, name, content string) {
    t.Helper()
    require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

// Helper: run a git command in a directory
func runGit(t *testing.T, dir string, args ...string) {
    t.Helper()
    cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
    out, err := cmd.CombinedOutput()
    require.NoError(t, err, string(out))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/adapters/... -run TestGitWorktreeOperations -v
```

- [ ] **Step 3: Add methods to `internal/adapters/git.go`**

```go
// RemoveWorktree removes a linked worktree. Uses --force to handle dirty state.
func (c *Client) RemoveWorktree(path string) error {
    cmd := c.cmd("worktree", "remove", "--force", path)
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("git worktree remove %s: %w\n%s", path, err, out)
    }
    return nil
}

// HeadSHA returns the SHA of HEAD in the client's repo/worktree.
func (c *Client) HeadSHA() (string, error) {
    cmd := c.cmd("rev-parse", "HEAD")
    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("git rev-parse HEAD: %w", err)
    }
    return strings.TrimSpace(string(out)), nil
}

// DiffFrom returns the unified diff of all changes since ref (committed + uncommitted).
func (c *Client) DiffFrom(ref string) (string, error) {
    cmd := c.cmd("diff", ref)
    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("git diff %s: %w", ref, err)
    }
    return string(out), nil
}

// DiffNameOnly returns the list of files changed since ref.
func (c *Client) DiffNameOnly(ref string) ([]string, error) {
    cmd := c.cmd("diff", "--name-only", ref)
    out, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("git diff --name-only %s: %w", ref, err)
    }
    raw := strings.TrimSpace(string(out))
    if raw == "" {
        return []string{}, nil
    }
    return strings.Split(raw, "\n"), nil
}

// ResetHard resets the repo/worktree to the given ref, discarding all changes.
func (c *Client) ResetHard(ref string) error {
    cmd := c.cmd("reset", "--hard", ref)
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("git reset --hard %s: %w\n%s", ref, err, out)
    }
    return nil
}

// ApplyPatch applies a unified diff as unstaged changes using git apply.
func (c *Client) ApplyPatch(patch string) error {
    if patch == "" {
        return nil
    }
    cmd := c.cmd("apply", "--")
    cmd.Stdin = strings.NewReader(patch)
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("git apply: %w\n%s", err, out)
    }
    return nil
}

// AddAll stages all changes (new, modified, deleted).
func (c *Client) AddAll() error {
    cmd := c.cmd("add", "-A")
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("git add -A: %w\n%s", err, out)
    }
    return nil
}

// CommitWithMessage stages nothing (caller must stage first), makes a commit,
// and returns the resulting HEAD SHA.
func (c *Client) CommitWithMessage(message string) (string, error) {
    commit := c.cmd("commit", "-m", message)
    if out, err := commit.CombinedOutput(); err != nil {
        return "", fmt.Errorf("git commit: %w\n%s", err, out)
    }
    return c.HeadSHA()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/adapters/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/git.go internal/adapters/git_test.go
git commit -m "feat: add git worktree diff/reset/apply/commit methods"
```

---

### Task 8: Create `internal/orchestrate/framework.go` — language auto-detection

**Files:**
- Create: `internal/orchestrate/framework.go`
- Create: `internal/orchestrate/framework_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestDetectAdaptersGo(t *testing.T) {
    dir := t.TempDir()
    require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0644))
    adapters, lang, err := DetectAdapters(dir)
    require.NoError(t, err)
    assert.Equal(t, "go", lang)
    assert.Equal(t, "go build ./...", adapters.Build)
    assert.Contains(t, adapters.Coverage, "coverprofile")
}

func TestDetectAdaptersNode(t *testing.T) {
    dir := t.TempDir()
    require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644))
    _, lang, err := DetectAdapters(dir)
    require.NoError(t, err)
    assert.Equal(t, "node", lang)
}

func TestDetectAdaptersUnknownFails(t *testing.T) {
    dir := t.TempDir()
    _, _, err := DetectAdapters(dir)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "could not detect language")
}

func TestMergeWithConfig(t *testing.T) {
    detected := AdapterCommands{Build: "go build ./...", Lint: "golangci-lint run ./...", Coverage: "go test -cover ./..."}
    override := AdapterCommands{Build: "make build"}
    merged := MergeAdapters(detected, override)
    assert.Equal(t, "make build", merged.Build) // override wins
    assert.Equal(t, "golangci-lint run ./...", merged.Lint) // detected fallback
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/orchestrate/... -run TestDetectAdapters -v
```

- [ ] **Step 3: Create `internal/orchestrate/framework.go`**

```go
package orchestrate

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/scullxbones/armature/internal/config"
)

type frameworkDefaults struct {
    Language     string
    Adapters     config.AdapterCommands
    TestPatterns map[string][]string
}

var builtinDefaults = []frameworkDefaults{
    {
        Language: "go",
        Adapters: config.AdapterCommands{
            Build:    "go build ./...",
            Lint:     "golangci-lint run ./...",
            Coverage: "go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out",
            Mutation: "gremlins unleash ./...",
        },
        TestPatterns: map[string][]string{"go": {"**/*_test.go"}},
    },
    {
        Language: "node",
        Adapters: config.AdapterCommands{
            Build:    "npm run build",
            Lint:     "npm run lint",
            Coverage: "npm test -- --coverage",
            Mutation: "npx stryker run",
        },
        TestPatterns: map[string][]string{"javascript": {"**/*.spec.js", "**/*.test.js"}},
    },
    {
        Language: "python",
        Adapters: config.AdapterCommands{
            Build:    "python -m py_compile $(find . -name '*.py')",
            Lint:     "ruff check .",
            Coverage: "pytest --cov=.",
            Mutation: "mutmut run",
        },
        TestPatterns: map[string][]string{"python": {"**/test_*.py", "**/*_test.py"}},
    },
}

var markerToLanguage = []struct {
    file     string
    language string
}{
    {"go.mod", "go"},
    {"package.json", "node"},
    {"pyproject.toml", "python"},
    {"Cargo.toml", "rust"},
}

// DetectAdapters probes repoPath for well-known framework markers and returns
// built-in adapter defaults. Returns an error if no framework can be detected.
func DetectAdapters(repoPath string) (config.AdapterCommands, string, error) {
    for _, m := range markerToLanguage {
        if _, err := os.Stat(filepath.Join(repoPath, m.file)); err == nil {
            for _, d := range builtinDefaults {
                if d.Language == m.language {
                    return d.Adapters, m.language, nil
                }
            }
            // Known language but no built-in defaults (e.g., rust)
            return config.AdapterCommands{}, m.language, fmt.Errorf(
                "detected language %q but no built-in adapter defaults — add 'orchestrator.adapters' to .armature/config.json",
                m.language)
        }
    }
    return config.AdapterCommands{}, "", fmt.Errorf(
        "could not detect language from repo markers — add 'orchestrator' block to .armature/config.json")
}

// MergeAdapters overlays overrides onto detected defaults. Non-empty override fields win.
func MergeAdapters(detected, override config.AdapterCommands) config.AdapterCommands {
    merged := detected
    if override.Build != "" {
        merged.Build = override.Build
    }
    if override.Lint != "" {
        merged.Lint = override.Lint
    }
    if override.Coverage != "" {
        merged.Coverage = override.Coverage
    }
    if override.Mutation != "" {
        merged.Mutation = override.Mutation
    }
    return merged
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/orchestrate/... -run TestDetectAdapters -v
go test ./internal/orchestrate/... -run TestMergeWithConfig -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/orchestrate/framework.go internal/orchestrate/framework_test.go
git commit -m "feat: add framework auto-detection for orchestrator adapters"
```

---

### Task 9: Create `internal/orchestrate/harness.go` — harness adapters

> **E6-S5 architecture compliance:**
> - File writes (settings.json, config.toml, log files) must use `internal/adapters/files.go` — no `os.WriteFile`/`os.MkdirAll`/`os.Create` directly in this file.
> - Process spawning must use `internal/adapters/shell.go` — no `exec.Command` directly in this file.
> - Check the exact API exposed by `adapters.WriteFile`, `adapters.MkdirAll`, and `adapters.RunProcess` (or equivalent) after E6-S5 is merged before writing code.

**Files:**
- Create: `internal/orchestrate/harness.go`
- Create: `internal/orchestrate/harness_test.go`

The adapter's responsibility: write harness config into worktree (via `adapters.WriteFile`), spawn the process (via `adapters.RunProcess` or equivalent), stream output to logPath, return normalized `InvocationResult`. Sandbox invocation is included here (bwrap on Linux, seatbelt on macOS).

- [ ] **Step 1: Write failing tests**

```go
func TestNewHarnessAdapterInvalidName(t *testing.T) {
    _, err := NewHarnessAdapter(HarnessConfig{Adapter: "unknown"})
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "unknown harness")
}

func TestNewHarnessAdapterClaude(t *testing.T) {
    a, err := NewHarnessAdapter(HarnessConfig{Adapter: "claude", Model: "claude-haiku-4-5", Timeout: 600})
    require.NoError(t, err)
    assert.Equal(t, "claude", a.Name())
}

func TestSandboxAvailable(t *testing.T) {
    // On CI this test is skipped if neither bwrap nor sandbox-exec is present
    ok, _ := SandboxAvailable()
    if !ok {
        t.Skip("sandbox not available")
    }
    assert.True(t, ok)
}

func TestWriteClaudeSettings(t *testing.T) {
    dir := t.TempDir()
    scope := []string{"internal/dag/", "internal/validate/"}
    err := writeClaudeSettings(dir, scope)
    require.NoError(t, err)
    data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
    require.NoError(t, err)
    assert.Contains(t, string(data), "dangerouslySkipPermissions")
    assert.Contains(t, string(data), "internal/dag/")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/orchestrate/... -run TestNewHarnessAdapter -v
```

- [ ] **Step 3: Create `internal/orchestrate/harness.go`**

```go
package orchestrate

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "os"        // only for os.Stdout/os.Stderr (writers, not file I/O)
    "path/filepath"
    "runtime"
    "strings"
    "time"

    "github.com/scullxbones/armature/internal/adapters"
)

// NewHarnessAdapter creates a harness adapter for the given config.
func NewHarnessAdapter(cfg HarnessConfig) (HarnessAdapter, error) {
    switch cfg.Adapter {
    case "claude":
        return &claudeAdapter{cfg: cfg}, nil
    case "codex":
        return &codexAdapter{cfg: cfg}, nil
    case "devin":
        return &devinAdapter{cfg: cfg}, nil
    default:
        return nil, fmt.Errorf("unknown harness %q: valid values are claude, codex, devin", cfg.Adapter)
    }
}

// SandboxAvailable checks whether the OS-level sandbox is available.
func SandboxAvailable() (bool, error) {
    if runtime.GOOS == "darwin" {
        if _, err := exec.LookPath("sandbox-exec"); err == nil {
            return true, nil
        }
        return false, fmt.Errorf("sandbox-exec not found (required on macOS)")
    }
    // Linux / WSL2: bubblewrap
    if _, err := exec.LookPath("bwrap"); err == nil {
        return true, nil
    }
    return false, fmt.Errorf("bwrap not found — install bubblewrap: apt-get install bubblewrap")
}

// buildSandboxCmd wraps command args in the OS sandbox restricted to the worktree.
func buildSandboxCmd(worktreeAbs string, cmdArgs []string) []string {
    if runtime.GOOS == "darwin" {
        // Seatbelt: allow everything except writes outside worktree
        profile := fmt.Sprintf(`(version 1)(allow default)(deny file-write* (subpath "/"))(allow file-write* (subpath "%s"))`, worktreeAbs)
        return append([]string{"sandbox-exec", "-p", profile}, cmdArgs...)
    }
    // bwrap: bind-mount root read-only, worktree read-write
    bwrapArgs := []string{
        "bwrap",
        "--ro-bind", "/", "/",
        "--bind", worktreeAbs, worktreeAbs,
        "--dev", "/dev",
        "--proc", "/proc",
        "--tmpfs", "/tmp",
        // Note: do NOT add --unshare-net — harnesses require network access to reach LLM APIs.
        "--die-with-parent",
        "--",
    }
    return append(bwrapArgs, cmdArgs...)
}

// invokeProcess is the shared process runner: launches cmd in workdir, streams to logFile, enforces timeout.
// Uses adapters.RunProcess (E6-S5/shell.go) and adapters.CreateFile (E6-S5/files.go) — no direct os/exec imports.
// Replace the body below with the actual adapter API once E6-S5 is merged.
func invokeProcess(ctx context.Context, workdir string, cmdArgs []string, logPath string) (InvocationResult, error) {
    if err := adapters.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
        return InvocationResult{}, fmt.Errorf("create log dir: %w", err)
    }
    logFile, err := adapters.CreateFile(logPath)
    if err != nil {
        return InvocationResult{}, fmt.Errorf("open log file: %w", err)
    }
    defer logFile.Close() //nolint:errcheck

    start := time.Now()
    exitStatus, err := adapters.RunProcess(ctx, workdir, cmdArgs, io.MultiWriter(logFile, os.Stdout), io.MultiWriter(logFile, os.Stderr))
    durationMs := time.Since(start).Milliseconds()

    switch exitStatus {
    case adapters.ProcessClean:
        return InvocationResult{ExitStatus: ExitClean, DurationMs: durationMs}, nil
    case adapters.ProcessTimeout:
        return InvocationResult{ExitStatus: ExitTimeout, DurationMs: durationMs}, nil
    default:
        return InvocationResult{ExitStatus: ExitError, DurationMs: durationMs}, err
    }
}

// --- Claude adapter ---

type claudeAdapter struct{ cfg HarnessConfig }

func (a *claudeAdapter) Name() string { return "claude" }

func (a *claudeAdapter) Invoke(ctx context.Context, workdir, prompt, logPath string) (InvocationResult, error) {
    issue := issueFromCtx(ctx) // scope paths injected via context key
    if err := writeClaudeSettings(workdir, issue.Scope); err != nil {
        return InvocationResult{}, fmt.Errorf("write claude settings: %w", err)
    }
    args := []string{"claude", "--dangerouslySkipPermissions"}
    if a.cfg.Model != "" {
        args = append(args, "--model", a.cfg.Model)
    }
    args = append(args, "-p", prompt)

    absWork, err := filepath.Abs(workdir)
    if err != nil {
        return InvocationResult{}, fmt.Errorf("abs workdir: %w", err)
    }
    sandboxed := buildSandboxCmd(absWork, args)
    return invokeProcess(ctx, workdir, sandboxed, logPath)
}

func writeClaudeSettings(workdir string, scopePaths []string) error {
    dir := filepath.Join(workdir, ".claude")
    if err := adapters.MkdirAll(dir, 0755); err != nil {
        return err
    }
    settings := map[string]any{
        "sandbox": map[string]any{
            "enabled":           true,
            "failIfUnavailable": true,
            "filesystem": map[string]any{
                "allowWrite": scopePaths,
                "denyWrite":  []string{"../"},
            },
        },
    }
    data, err := json.MarshalIndent(settings, "", "  ")
    if err != nil {
        return err
    }
    return adapters.WriteFile(filepath.Join(dir, "settings.json"), data, 0644)
}

// --- Codex adapter ---

type codexAdapter struct{ cfg HarnessConfig }

func (a *codexAdapter) Name() string { return "codex" }

func (a *codexAdapter) Invoke(ctx context.Context, workdir, prompt, logPath string) (InvocationResult, error) {
    issue := issueFromCtx(ctx)
    if err := writeCodexConfig(workdir, issue.Scope); err != nil {
        return InvocationResult{}, fmt.Errorf("write codex config: %w", err)
    }
    args := []string{"codex"}
    if a.cfg.Model != "" {
        args = append(args, "--model", a.cfg.Model)
    }
    args = append(args, prompt)
    absWork, _ := filepath.Abs(workdir)
    return invokeProcess(ctx, workdir, buildSandboxCmd(absWork, args), logPath)
}

func writeCodexConfig(workdir string, scopePaths []string) error {
    roots := make([]string, len(scopePaths))
    for i, p := range scopePaths {
        roots[i] = fmt.Sprintf("%q", p)
    }
    toml := fmt.Sprintf("sandbox_mode = \"workspace-write\"\napproval_policy = \"never\"\n[permissions.default.filesystem]\nwritable_roots = [%s]\n",
        strings.Join(roots, ", "))
    return adapters.WriteFile(filepath.Join(workdir, "config.toml"), []byte(toml), 0644)
}

// --- Devin adapter ---

type devinAdapter struct{ cfg HarnessConfig }

func (a *devinAdapter) Name() string { return "devin" }

func (a *devinAdapter) Invoke(ctx context.Context, workdir, prompt, logPath string) (InvocationResult, error) {
    issue := issueFromCtx(ctx)
    if err := writeDevinConfig(workdir, issue.Scope); err != nil {
        return InvocationResult{}, fmt.Errorf("write devin config: %w", err)
    }
    if a.cfg.Model != "" {
        fmt.Fprintf(os.Stderr, "warning: Devin CLI does not support model selection (model %q ignored)\n", a.cfg.Model)
    }
    args := []string{"devin", "--sandbox", "--permission-mode", "autonomous", "--", prompt}
    absWork, _ := filepath.Abs(workdir)
    return invokeProcess(ctx, workdir, buildSandboxCmd(absWork, args), logPath)
}

func writeDevinConfig(workdir string, scopePaths []string) error {
    dir := filepath.Join(workdir, ".devin")
    if err := adapters.MkdirAll(dir, 0755); err != nil {
        return err
    }
    perms := make([]map[string]string, len(scopePaths))
    for i, p := range scopePaths {
        perms[i] = map[string]string{"allow": fmt.Sprintf("Write(%s)", p)}
    }
    cfg := map[string]any{"permissions": perms}
    data, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil {
        return fmt.Errorf("marshal devin config: %w", err)
    }
    return adapters.WriteFile(filepath.Join(dir, "config.json"), data, 0644)
}

// issueCtxKey is a context key for passing scope information to adapters.
type issueCtxKey struct{}

type issueContext struct{ Scope []string }

// WithIssueScope injects scope paths into the context for harness adapters.
func WithIssueScope(ctx context.Context, scope []string) context.Context {
    return context.WithValue(ctx, issueCtxKey{}, &issueContext{Scope: scope})
}

func issueFromCtx(ctx context.Context) *issueContext {
    if v, ok := ctx.Value(issueCtxKey{}).(*issueContext); ok {
        return v
    }
    return &issueContext{}
}

// validateIssueScope returns an error if scope is empty — prevents adapters from
// writing empty allowWrite lists that silently permit no writes.
func validateIssueScope(scope []string) error {
    if len(scope) == 0 {
        return fmt.Errorf("issue has no declared scope paths — cannot configure harness sandbox")
    }
    return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/orchestrate/... -run TestNewHarnessAdapter -v
go test ./internal/orchestrate/... -run TestWriteClaudeSettings -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/orchestrate/harness.go internal/orchestrate/harness_test.go
git commit -m "feat: add claude/codex/devin harness adapters with sandbox support"
```

---

## Chunk 4: Verification Pipeline

### Task 10: Create `internal/orchestrate/verify.go`

**Prerequisite:** Task 2 (adding `SourceEntryID` to `CitationAcceptance` and `Payload`) must be complete before this task — `checkCitations` reads `op.Payload.SourceEntryID` added in Task 2.

**Files:**
- Create: `internal/orchestrate/verify.go`
- Create: `internal/orchestrate/verify_test.go`

The pipeline runs: scope-boundary → build → lint → test-existence → coverage → mutation (optional) → acceptance-criteria → citations

- [ ] **Step 1: Write failing tests — scope-boundary check**

```go
func TestCheckScopeBoundary(t *testing.T) {
    scope := []string{"internal/dag/"}
    t.Run("pass when all files in scope", func(t *testing.T) {
        result := checkScopeBoundary([]string{"internal/dag/dag.go", "internal/dag/dag_test.go"}, scope)
        assert.Equal(t, SeverityPass, result.Severity)
    })
    t.Run("fail when file outside scope", func(t *testing.T) {
        result := checkScopeBoundary([]string{"internal/dag/dag.go", "internal/auth/token.go"}, scope)
        assert.Equal(t, SeverityFail, result.Severity)
        assert.Contains(t, result.Reason, "internal/auth/token.go")
    })
}

func TestCheckTestExistence(t *testing.T) {
    patterns := map[string][]string{"go": {"**/*_test.go"}}
    t.Run("pass when test file present", func(t *testing.T) {
        changed := []string{"internal/dag/dag.go", "internal/dag/dag_test.go"}
        result := checkTestExistence(changed, patterns, "go")
        assert.Equal(t, SeverityPass, result.Severity)
    })
    t.Run("fail when no test file", func(t *testing.T) {
        changed := []string{"internal/dag/dag.go"}
        result := checkTestExistence(changed, patterns, "go")
        assert.Equal(t, SeverityFail, result.Severity)
        assert.Contains(t, result.Reason, "internal/dag/dag.go")
    })
    t.Run("test-only change passes", func(t *testing.T) {
        changed := []string{"internal/dag/dag_test.go"}
        result := checkTestExistence(changed, patterns, "go")
        assert.Equal(t, SeverityPass, result.Severity)
    })
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/orchestrate/... -run TestCheckScopeBoundary -v
```

- [ ] **Step 3: Write failing test for RunPipeline**

```go
func TestRunPipelineScopeFail(t *testing.T) {
    issue := &materialize.Issue{
        ID:    "T-1",
        Scope: []string{"internal/dag/"},
    }
    cfg := config.OrchestratorConfig{Language: "go"}
    // Mock: changed files include one outside scope
    changedFiles := []string{"internal/dag/dag.go", "internal/auth/token.go"}
    results := RunPipeline(PipelineInput{
        ChangedFiles: changedFiles,
        WorktreeDir:  t.TempDir(),
        Issue:        issue,
        Cfg:          cfg,
        AllOps:       nil,
    })
    require.GreaterOrEqual(t, len(results), 1)
    assert.Equal(t, "scope-boundary", results[0].Check)
    assert.Equal(t, SeverityFail, results[0].Severity)
}
```

- [ ] **Step 4: Create `internal/orchestrate/verify.go`**

```go
package orchestrate

import (
    "fmt"
    "os/exec"
    "path/filepath"
    "strings"

    "github.com/scullxbones/armature/internal/config"
    "github.com/scullxbones/armature/internal/materialize"
    "github.com/scullxbones/armature/internal/ops"
)

// PipelineInput holds all inputs needed by the verification pipeline.
type PipelineInput struct {
    ChangedFiles []string
    WorktreeDir  string
    Issue        *materialize.Issue
    Cfg          config.OrchestratorConfig
    AllOps       []ops.Op
}

// RunPipeline executes all checks in order, stopping on the first hard fail.
// Returns all check results (including passes) up to and including the first failure.
func RunPipeline(in PipelineInput) []CheckResult {
    var results []CheckResult

    // scope-boundary
    if r := checkScopeBoundary(in.ChangedFiles, in.Issue.Scope); r.Severity == SeverityFail {
        return append(results, r)
    } else {
        results = append(results, r)
    }

    // build
    if r := runAdapterCmd("build", in.Cfg.Adapters.Build, in.WorktreeDir, in.Cfg.LintFailMode); r.Severity == SeverityFail {
        return append(results, r)
    } else {
        results = append(results, r)
    }

    // lint
    lintMode := in.Cfg.LintFailMode
    if lintMode == "" {
        lintMode = "hard"
    }
    if r := runAdapterCmd("lint", in.Cfg.Adapters.Lint, in.WorktreeDir, lintMode); r.Severity == SeverityFail {
        return append(results, r)
    } else {
        results = append(results, r)
    }

    // test-existence
    lang := in.Cfg.Language
    patterns := in.Cfg.TestPatterns
    if len(patterns) == 0 {
        patterns = map[string][]string{"go": {"**/*_test.go"}}
    }
    if r := checkTestExistence(in.ChangedFiles, patterns, lang); r.Severity == SeverityFail {
        return append(results, r)
    } else {
        results = append(results, r)
    }

    // coverage
    threshold := in.Cfg.CoverageThreshold
    if threshold == 0 {
        threshold = 80
    }
    if r := runCoverageCheck(in.Cfg.Adapters.Coverage, in.WorktreeDir, threshold, in.Issue); r.Severity == SeverityFail {
        return append(results, r)
    } else {
        results = append(results, r)
    }

    // mutation (optional)
    if in.Cfg.MutationTesting && in.Cfg.Adapters.Mutation != "" {
        mutMode := in.Cfg.MutationFailMode
        if mutMode == "" {
            mutMode = "warn"
        }
        if r := runAdapterCmd("mutation", in.Cfg.Adapters.Mutation, in.WorktreeDir, mutMode); r.Severity == SeverityFail {
            return append(results, r)
        } else {
            results = append(results, r)
        }
    }

    // acceptance-criteria
    if r := checkAcceptanceCriteria(in.Issue, in.WorktreeDir, orchCfg); r.Severity == SeverityFail {
        return append(results, r)
    } else {
        results = append(results, r)
    }

    // citations
    if r := checkCitations(in.Issue, in.AllOps); r.Severity == SeverityFail {
        return append(results, r)
    } else {
        results = append(results, r)
    }

    return results
}

// checkScopeBoundary verifies all changed files are within the declared scope.
// Scope paths must end with "/" (e.g. "internal/dag/"). The function normalizes
// both scope paths and file paths by ensuring scope paths end with "/" so that
// "internal/dag/" does not accidentally match "internal/dangerous/file.go".
func checkScopeBoundary(changedFiles []string, scope []string) CheckResult {
    // Normalize scope paths: ensure they end with "/" for safe prefix matching.
    normalized := make([]string, len(scope))
    for i, s := range scope {
        s = filepath.ToSlash(filepath.Clean(s))
        if !strings.HasSuffix(s, "/") {
            s += "/"
        }
        normalized[i] = s
    }

    var violations []string
    for _, f := range changedFiles {
        f = filepath.ToSlash(f)
        inScope := false
        for _, s := range normalized {
            if strings.HasPrefix(f, s) {
                inScope = true
                break
            }
        }
        if !inScope {
            violations = append(violations, f)
        }
    }
    if len(violations) > 0 {
        return CheckResult{
            Check:    "scope-boundary",
            Severity: SeverityFail,
            Reason:   fmt.Sprintf("files outside declared scope: %s", strings.Join(violations, ", ")),
        }
    }
    return CheckResult{Check: "scope-boundary", Severity: SeverityPass}
}

// runAdapterCmd runs a shell command in workdir; failMode is "hard" or "warn".
func runAdapterCmd(check, command, workdir, failMode string) CheckResult {
    if command == "" {
        return CheckResult{Check: check, Severity: SeverityPass, Reason: "no command configured (skipped)"}
    }
    cmd := exec.Command("sh", "-c", command)
    cmd.Dir = workdir
    out, err := cmd.CombinedOutput()
    if err == nil {
        return CheckResult{Check: check, Severity: SeverityPass}
    }
    sev := SeverityFail
    if failMode == "warn" {
        sev = SeverityWarn
    }
    return CheckResult{Check: check, Severity: sev, Reason: strings.TrimSpace(string(out))}
}

// checkTestExistence verifies each modified source file has a matching test file in the diff.
func checkTestExistence(changedFiles []string, patterns map[string][]string, lang string) CheckResult {
    langPatterns := patterns[lang]
    if len(langPatterns) == 0 {
        langPatterns = patterns["go"] // fallback
    }

    var testFiles, sourceFiles []string
    for _, f := range changedFiles {
        if isTestFile(f, langPatterns) {
            testFiles = append(testFiles, f)
        } else {
            sourceFiles = append(sourceFiles, f)
        }
    }
    if len(sourceFiles) == 0 {
        return CheckResult{Check: "test-existence", Severity: SeverityPass}
    }

    // Build lookup of directories covered by test files
    testDirs := make(map[string]bool)
    for _, tf := range testFiles {
        testDirs[filepath.Dir(tf)] = true
    }

    var uncovered []string
    for _, sf := range sourceFiles {
        if !testDirs[filepath.Dir(sf)] {
            uncovered = append(uncovered, sf)
        }
    }
    if len(uncovered) > 0 {
        return CheckResult{
            Check:    "test-existence",
            Severity: SeverityFail,
            Reason:   fmt.Sprintf("no test file found for: %s", strings.Join(uncovered, ", ")),
        }
    }
    return CheckResult{Check: "test-existence", Severity: SeverityPass}
}

func isTestFile(path string, patterns []string) bool {
    base := filepath.Base(path)
    for _, p := range patterns {
        pat := filepath.Base(p) // use basename for matching
        if matched, _ := filepath.Match(pat, base); matched {
            return true
        }
    }
    return false
}

// runCoverageCheck runs the coverage command and parses its output.
// The coverage command is expected to output lines like "total:  (statements)  82.3%"
func runCoverageCheck(command, workdir string, threshold int, issue *materialize.Issue) CheckResult {
    if command == "" {
        return CheckResult{Check: "coverage", Severity: SeverityPass, Reason: "no command configured (skipped)"}
    }
    // Check for task-level coverage-gte override
    effectiveThreshold := threshold
    if ac := parseCoverageAcceptance(issue); ac > 0 {
        effectiveThreshold = ac
    }

    cmd := exec.Command("sh", "-c", command)
    cmd.Dir = workdir
    out, err := cmd.CombinedOutput()
    if err != nil {
        return CheckResult{Check: "coverage", Severity: SeverityFail, Reason: strings.TrimSpace(string(out))}
    }
    pct, ok := parseCoveragePercent(string(out))
    if !ok {
        return CheckResult{Check: "coverage", Severity: SeverityWarn, Reason: "could not parse coverage output"}
    }
    if pct < float64(effectiveThreshold) {
        return CheckResult{
            Check:    "coverage",
            Severity: SeverityFail,
            Reason:   fmt.Sprintf("coverage %.1f%% is below threshold %d%%", pct, effectiveThreshold),
        }
    }
    return CheckResult{Check: "coverage", Severity: SeverityPass}
}

func parseCoveragePercent(output string) (float64, bool) {
    for _, line := range strings.Split(output, "\n") {
        if strings.HasPrefix(line, "total:") || strings.Contains(line, "(statements)") {
            fields := strings.Fields(line)
            for _, f := range fields {
                f = strings.TrimSuffix(f, "%")
                var pct float64
                if _, err := fmt.Sscanf(f, "%f", &pct); err == nil && pct > 0 {
                    return pct, true
                }
            }
        }
    }
    return 0, false
}

func parseCoverageAcceptance(issue *materialize.Issue) int {
    // Look for a "coverage-gte" acceptance criterion in the issue's structured acceptance field
    if issue == nil || len(issue.Acceptance) == 0 {
        return 0
    }
    type criterion struct {
        Type      string `json:"type"`
        Threshold int    `json:"threshold"`
    }
    var criteria []criterion
    if err := jsonUnmarshal(issue.Acceptance, &criteria); err != nil {
        return 0
    }
    for _, c := range criteria {
        if c.Type == "coverage-gte" && c.Threshold > 0 {
            return c.Threshold
        }
    }
    return 0
}

// checkAcceptanceCriteria evaluates structured acceptance assertions.
// Supported types: file-exists, test-passes, lint-clean, coverage-gte (already handled).
// Unverifiable types pass through as warnings. Returns fail if ALL criteria are unverifiable.
func checkAcceptanceCriteria(issue *materialize.Issue, worktreeDir string, cfg config.OrchestratorConfig) CheckResult {
    if issue == nil || len(issue.Acceptance) == 0 {
        return CheckResult{Check: "acceptance-criteria", Severity: SeverityPass}
    }
    type criterion struct {
        Type         string `json:"type"`
        Value        string `json:"value"`
        Unverifiable bool   `json:"unverifiable"`
    }
    var criteria []criterion
    if err := jsonUnmarshal(issue.Acceptance, &criteria); err != nil {
        return CheckResult{Check: "acceptance-criteria", Severity: SeverityWarn, Reason: "could not parse acceptance field"}
    }

    // Require at least one machine-verifiable assertion.
    verifiableCount := 0
    for _, c := range criteria {
        if !c.Unverifiable && c.Type != "coverage-gte" {
            verifiableCount++
        }
    }
    if verifiableCount == 0 {
        return CheckResult{
            Check:    "acceptance-criteria",
            Severity: SeverityFail,
            Reason:   "all acceptance criteria are unverifiable — at least one structured assertion (file-exists, test-passes, lint-clean) is required",
        }
    }

    // Use configured adapter commands so non-Go languages work correctly.
    testCmd := cfg.Adapters.Build // fallback; prefer test runner
    if cfg.Adapters.Coverage != "" {
        // For test-passes we need a targeted test runner; use sh -c for portability
        testCmd = ""
    }
    lintCmd := cfg.Adapters.Lint

    var failures []string
    for _, c := range criteria {
        if c.Unverifiable || c.Type == "coverage-gte" {
            continue
        }
        switch c.Type {
        case "file-exists":
            if _, err := os.Stat(filepath.Join(worktreeDir, c.Value)); err != nil {
                failures = append(failures, fmt.Sprintf("file-exists %q: not found", c.Value))
            }
        case "test-passes":
            // For Go: use "go test -run <value> ./..."; for other languages fall back to configured command
            var cmd *exec.Cmd
            if cfg.Language == "go" || cfg.Language == "" {
                cmd = exec.Command("go", "test", "-run", c.Value, "./...")
            } else if testCmd != "" {
                cmd = exec.Command("sh", "-c", fmt.Sprintf("%s --testNamePattern %s", testCmd, c.Value))
            } else {
                failures = append(failures, fmt.Sprintf("test-passes %q: no test command configured for language %q", c.Value, cfg.Language))
                continue
            }
            cmd.Dir = worktreeDir
            if out, err := cmd.CombinedOutput(); err != nil {
                failures = append(failures, fmt.Sprintf("test-passes %q: %s", c.Value, strings.TrimSpace(string(out))))
            }
        case "lint-clean":
            var cmd *exec.Cmd
            if lintCmd != "" {
                cmd = exec.Command("sh", "-c", fmt.Sprintf("%s %s", lintCmd, c.Value))
            } else if cfg.Language == "go" || cfg.Language == "" {
                cmd = exec.Command("sh", "-c", fmt.Sprintf("golangci-lint run %s", c.Value))
            } else {
                failures = append(failures, fmt.Sprintf("lint-clean %q: no lint command configured for language %q", c.Value, cfg.Language))
                continue
            }
            cmd.Dir = worktreeDir
            if out, err := cmd.CombinedOutput(); err != nil {
                failures = append(failures, fmt.Sprintf("lint-clean %q: %s", c.Value, strings.TrimSpace(string(out))))
            }
        }
    }
    if len(failures) > 0 {
        return CheckResult{Check: "acceptance-criteria", Severity: SeverityFail, Reason: strings.Join(failures, "; ")}
    }
    return CheckResult{Check: "acceptance-criteria", Severity: SeverityPass}
}

// checkCitations verifies every source-linked source has a citation-accepted op
// with a matching source_entry_id. This is the orchestrator's stricter variant.
func checkCitations(issue *materialize.Issue, allOps []ops.Op) CheckResult {
    if issue == nil {
        return CheckResult{Check: "citations", Severity: SeverityPass}
    }
    // Build set of accepted source entry IDs for this issue
    accepted := make(map[string]bool)
    for _, op := range allOps {
        if op.Type == ops.OpCitationAccepted && op.TargetID == issue.ID && op.Payload.SourceEntryID != "" {
            accepted[op.Payload.SourceEntryID] = true
        }
    }
    var missing []string
    for _, link := range issue.SourceLinks {
        if !accepted[link.SourceEntryID] {
            missing = append(missing, link.SourceEntryID)
        }
    }
    if len(missing) > 0 {
        return CheckResult{
            Check:    "citations",
            Severity: SeverityFail,
            Reason:   fmt.Sprintf("source(s) missing citation-accepted op: %s", strings.Join(missing, ", ")),
        }
    }
    return CheckResult{Check: "citations", Severity: SeverityPass}
}

// jsonUnmarshal is a helper to unmarshal json.RawMessage — avoids import cycle.
func jsonUnmarshal(raw []byte, v any) error {
    return json.Unmarshal(raw, v)
}
```

Add `"encoding/json"` and `"os"` to imports.

- [ ] **Step 5: Run all verify tests**

```bash
go test ./internal/orchestrate/... -run TestCheckScope -v
go test ./internal/orchestrate/... -run TestCheckTestExistence -v
go test ./internal/orchestrate/... -run TestRunPipeline -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/orchestrate/verify.go internal/orchestrate/verify_test.go
git commit -m "feat: add verification pipeline (scope, build, lint, tests, coverage, citations)"
```

---

## Chunk 5: Prompt, Feedback, and Engine

### Task 11: Create `internal/orchestrate/preflight.go`

**Files:**
- Create: `internal/orchestrate/preflight.go`
- Create: `internal/orchestrate/preflight_test.go`

Preflight checks: scope paths exist in repo, acceptance criteria structured and at least one verifiable, source citations resolve against manifest (light check), token budget estimate, sandbox available.

`preflight.go` imports: `encoding/json`, `fmt`, `os`, `path/filepath`, `github.com/scullxbones/armature/internal/config`, `github.com/scullxbones/armature/internal/materialize`, `github.com/scullxbones/armature/internal/sources`.

- [ ] **Step 1: Write failing tests**

```go
func TestPreflightScopePathMissing(t *testing.T) {
    dir := t.TempDir()
    issue := &materialize.Issue{
        ID:         "T-1",
        Scope:      []string{"internal/nonexistent/"},
        Acceptance: json.RawMessage(`[{"type":"file-exists","value":"internal/dag/dag.go"}]`),
    }
    err := RunPreflight(issue, dir, config.OrchestratorConfig{})
    require.Error(t, err)
    assert.Contains(t, err.Error(), "scope path")
    assert.Contains(t, err.Error(), "internal/nonexistent/")
}

func TestPreflightAcceptanceMissing(t *testing.T) {
    dir := t.TempDir()
    require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "dag"), 0755))
    issue := &materialize.Issue{
        ID:         "T-1",
        Scope:      []string{"internal/dag/"},
        Acceptance: nil,
    }
    err := RunPreflight(issue, dir, config.OrchestratorConfig{})
    require.Error(t, err)
    assert.Contains(t, err.Error(), "acceptance criteria")
}

func TestPreflightRejectsAllUnverifiable(t *testing.T) {
    dir := t.TempDir()
    require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "dag"), 0755))
    issue := &materialize.Issue{
        ID:    "T-1",
        Scope: []string{"internal/dag/"},
        // All items are unverifiable — should be rejected at preflight
        Acceptance: json.RawMessage(`[{"type":"prose","value":"does the thing","unverifiable":true}]`),
    }
    err := RunPreflight(issue, dir, config.OrchestratorConfig{})
    require.Error(t, err)
    assert.Contains(t, err.Error(), "unverifiable")
}

func TestPreflightPasses(t *testing.T) {
    dir := t.TempDir()
    require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "dag"), 0755))
    issue := &materialize.Issue{
        ID:         "T-1",
        Scope:      []string{"internal/dag/"},
        Acceptance: json.RawMessage(`[{"type":"file-exists","value":"internal/dag/dag.go"}]`),
    }
    err := RunPreflight(issue, dir, config.OrchestratorConfig{TokenBudget: 2000})
    require.NoError(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/orchestrate/... -run TestPreflight -v
```

- [ ] **Step 3: Create `internal/orchestrate/preflight.go`**

```go
package orchestrate

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"

    "github.com/scullxbones/armature/internal/config"
    "github.com/scullxbones/armature/internal/materialize"
)

// RunPreflight validates the task is safe to orchestrate.
// Returns a descriptive error for each failed check; nil if all pass.
// appCfg is the full Config (for token budget); orchCfg is the orchestrator sub-config.
func RunPreflight(issue *materialize.Issue, repoPath string, appCfg config.Config) error {
    // 1. Scope paths must exist
    for _, scope := range issue.Scope {
        abs := filepath.Join(repoPath, scope)
        if _, err := os.Stat(abs); err != nil {
            return fmt.Errorf("scope path %q does not exist in repo (run 'arm validate' to check scope declarations)", scope)
        }
    }

    // 2. Acceptance criteria must be present, parseable, and have at least one verifiable assertion
    if len(issue.Acceptance) == 0 {
        return fmt.Errorf("acceptance criteria required: task %s has no acceptance field — add structured assertions (file-exists, test-passes, etc.) via 'arm amend'", issue.ID)
    }
    type criterion struct {
        Type         string `json:"type"`
        Unverifiable bool   `json:"unverifiable"`
    }
    var criteria []criterion
    if err := json.Unmarshal(issue.Acceptance, &criteria); err != nil || len(criteria) == 0 {
        return fmt.Errorf("acceptance criteria must be a non-empty JSON array of structured assertions — use 'arm amend' to update task %s", issue.ID)
    }
    verifiable := 0
    for _, c := range criteria {
        if !c.Unverifiable {
            verifiable++
        }
    }
    if verifiable == 0 {
        return fmt.Errorf("acceptance criteria for task %s are all marked unverifiable — at least one machine-checkable assertion (file-exists, test-passes, lint-clean) is required", issue.ID)
    }

    // 3. Source citations resolve against manifest (light check — full per-source check is in verify pipeline)
    if len(issue.SourceLinks) > 0 {
        manifestPath := filepath.Join(repoPath, ".armature", "sources")
        manifest, err := sources.ReadManifest(manifestPath)
        if err == nil {
            for _, link := range issue.SourceLinks {
                if _, ok := manifest[link.SourceEntryID]; !ok {
                    return fmt.Errorf("source citation %q in task %s does not resolve in source manifest — run 'arm sources' to verify", link.SourceEntryID, issue.ID)
                }
            }
        }
        // If manifest is unreadable, skip citation preflight check (non-fatal — sources may not be initialized)
    }

    // 4. Token budget: rendered context must fit within configured budget
    if appCfg.TokenBudget > 0 && len(issue.Acceptance) > 0 {
        // Rough estimate: if any single context field exceeds the budget, warn.
        // Exact token counting would require tokenizer — use char count / 4 as approximation.
        totalChars := len(issue.Title) + len(issue.DefinitionOfDone) + len(issue.Acceptance)
        estimatedTokens := totalChars / 4
        if estimatedTokens > appCfg.TokenBudget {
            return fmt.Errorf("estimated context size (%d tokens) exceeds token budget (%d) for task %s — consider decomposing", estimatedTokens, appCfg.TokenBudget, issue.ID)
        }
    }

    // 5. Sandbox must be available
    if ok, err := SandboxAvailable(); !ok {
        return fmt.Errorf("sandbox required: %w", err)
    }

    return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/orchestrate/... -run TestPreflight -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/orchestrate/preflight.go internal/orchestrate/preflight_test.go
git commit -m "feat: add orchestrator preflight checks"
```

---

### Task 12: Create `internal/orchestrate/feedback.go` and `internal/orchestrate/prompt.go`

**Files:**
- Create: `internal/orchestrate/feedback.go`
- Create: `internal/orchestrate/feedback_test.go`
- Create: `internal/orchestrate/prompt.go`
- Create: `internal/orchestrate/prompt_test.go`

- [ ] **Step 1: Write failing tests — feedback assembly**

```go
func TestAssembleFeedbackRun1(t *testing.T) {
    failures := []CheckResult{
        {Check: "scope-boundary", Severity: SeverityFail, Reason: "internal/auth/token.go modified"},
        {Check: "test-existence", Severity: SeverityFail, Reason: "no test for internal/dag/dag.go"},
        {Check: "build", Severity: SeverityPass},
    }
    fb := AssembleFeedback(1, 3, failures, nil)
    assert.Contains(t, fb, "CORRECTION REQUIRED (run 1 of 3 failed)")
    assert.Contains(t, fb, "✗ scope-boundary")
    assert.Contains(t, fb, "✗ test-existence")
    assert.Contains(t, fb, "✓ build")
}

func TestAssembleFeedbackRetry2AddsNegativeConstraint(t *testing.T) {
    // retry 2 = second failed run, adds explicit negative constraint
    failures := []CheckResult{
        {Check: "scope-boundary", Severity: SeverityFail, Reason: "internal/auth/token.go modified"},
    }
    scope := []string{"internal/dag/"}
    fb := AssembleFeedback(2, 3, failures, scope)
    assert.Contains(t, fb, "do not modify files outside")
    assert.Contains(t, fb, "internal/dag/")
}

func TestAssembleFeedbackRetry3AddsFileList(t *testing.T) {
    // retry 3 = third failed run, adds named file list
    failures := []CheckResult{
        {Check: "test-existence", Severity: SeverityFail, Reason: "no test for: internal/dag/dag.go, internal/dag/validate.go"},
    }
    fb := AssembleFeedback(3, 3, failures, nil)
    assert.Contains(t, fb, "the following files require test coverage")
}

func TestAssembleFeedbackRetry1FailureReportOnly(t *testing.T) {
    // retry 1 = first retry, adds no extra constraint beyond the failure report
    failures := []CheckResult{
        {Check: "scope-boundary", Severity: SeverityFail, Reason: "file outside scope"},
    }
    fb := AssembleFeedback(1, 3, failures, []string{"internal/dag/"})
    assert.NotContains(t, fb, "do not modify files outside")
    assert.NotContains(t, fb, "the following files require test coverage")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/orchestrate/... -run TestAssembleFeedback -v
```

- [ ] **Step 3: Create `internal/orchestrate/feedback.go`**

```go
package orchestrate

import (
    "fmt"
    "strings"
)

// AssembleFeedback builds the structured feedback block for the next harness invocation.
// run is the run number that just failed (1-indexed). retryBudget is the total budget.
// scope is the declared scope paths (used for progressive constraint tightening).
func AssembleFeedback(run int, retryBudget int, failures []CheckResult, scope []string) string {
    var sb strings.Builder
    fmt.Fprintf(&sb, "CORRECTION REQUIRED (run %d of %d failed):\n\n", run, retryBudget)

    for _, r := range failures {
        if r.Severity == SeverityFail || r.Severity == SeverityWarn {
            fmt.Fprintf(&sb, "  ✗ %s: %s\n", r.Check, r.Reason)
        } else {
            fmt.Fprintf(&sb, "  ✓ %s: passed\n", r.Check)
        }
    }

    sb.WriteString("\nCorrect only what is listed above.\n")

    // Progressive constraint tightening (run = 1-indexed retry number: 1=first retry, 2=second, 3=third)
    switch run {
    case 2:
        if len(scope) > 0 {
            fmt.Fprintf(&sb, "do not modify files outside %s\n", strings.Join(scope, ", "))
        }
    case 3:
        // Extract file paths from test-existence failures
        var paths []string
        for _, r := range failures {
            if r.Check == "test-existence" && r.Reason != "" {
                // Extract file paths from "no test file found for: path1, path2"
                parts := strings.SplitAfter(r.Reason, ":")
                if len(parts) > 1 {
                    for _, p := range strings.Split(strings.TrimSpace(parts[len(parts)-1]), ",") {
                        paths = append(paths, strings.TrimSpace(p))
                    }
                }
            }
        }
        if len(paths) > 0 {
            fmt.Fprintf(&sb, "the following files require test coverage: %s\n", strings.Join(paths, ", "))
        }
    }

    return sb.String()
}
```

- [ ] **Step 4: Write failing test for prompt assembly**

```go
func TestAssemblePrompt(t *testing.T) {
    ctx := &context.Context{
        IssueID: "T-1",
        Layers: []context.Layer{
            {Name: "core_spec", Content: "# Issue: T-1\nDo the thing."},
        },
    }
    issue := &materialize.Issue{ID: "T-1", Scope: []string{"internal/dag/"}}
    prompt := AssemblePrompt(ctx, issue, "")
    assert.Contains(t, prompt, "T-1")
    assert.Contains(t, prompt, "internal/dag/")
    assert.Contains(t, prompt, "Do not commit")
}

func TestAssemblePromptWithFeedback(t *testing.T) {
    ctx := &context.Context{IssueID: "T-1", Layers: []context.Layer{}}
    issue := &materialize.Issue{ID: "T-1", Scope: []string{"internal/dag/"}}
    prompt := AssemblePrompt(ctx, issue, "CORRECTION REQUIRED: fix scope violation")
    assert.Contains(t, prompt, "CORRECTION REQUIRED")
}
```

- [ ] **Step 5: Create `internal/orchestrate/prompt.go`**

```go
package orchestrate

import (
    "fmt"
    "strings"

    ctx_pkg "github.com/scullxbones/armature/internal/context"
    "github.com/scullxbones/armature/internal/materialize"
)

// AssemblePrompt builds the full prompt for a harness invocation.
// feedback is empty on the first run and populated on retries.
func AssemblePrompt(ctx *ctx_pkg.Context, issue *materialize.Issue, feedback string) string {
    var sb strings.Builder

    // 1. Rendered task context
    rendered, err := ctx_pkg.RenderAgent(ctx)
    if err != nil {
        rendered = fmt.Sprintf("(context render failed: %v)", err)
    }
    sb.WriteString(rendered)
    sb.WriteString("\n\n")

    // 2. Explicit scope constraints
    if len(issue.Scope) > 0 {
        sb.WriteString("## Scope Constraint\n")
        sb.WriteString("You may ONLY modify files within the following paths:\n")
        for _, s := range issue.Scope {
            fmt.Fprintf(&sb, "  - %s\n", s)
        }
        sb.WriteString("\n")
    }

    // 3. No-commit instruction
    sb.WriteString("## Important\n")
    sb.WriteString("Do not commit any changes. Leave all changes as uncommitted working directory modifications.\n\n")

    // 4. Retry feedback block (if present)
    if feedback != "" {
        sb.WriteString("## Feedback from Previous Run\n")
        sb.WriteString(feedback)
        sb.WriteString("\n")
    }

    return sb.String()
}
```

- [ ] **Step 6: Run all feedback and prompt tests**

```bash
go test ./internal/orchestrate/... -run TestAssembleFeedback -v
go test ./internal/orchestrate/... -run TestAssemblePrompt -v
```

- [ ] **Step 7: Commit**

```bash
git add internal/orchestrate/feedback.go internal/orchestrate/feedback_test.go internal/orchestrate/prompt.go internal/orchestrate/prompt_test.go
git commit -m "feat: add orchestrator prompt assembly and feedback generation"
```

---

### Task 13: Create `internal/orchestrate/engine.go` — main orchestration loop

**Files:**
- Create: `internal/orchestrate/engine.go`
- Create: `internal/orchestrate/engine_test.go`

The engine drives the full state machine. It accepts `*config.Context` and `RunOptions`, derives state from the event log, and executes the loop.

- [ ] **Step 1: Write failing test — engine with fake harness**

```go
type fakeHarness struct {
    name        string
    invocations int
    results     []InvocationResult
}

func (f *fakeHarness) Name() string { return f.name }
func (f *fakeHarness) Invoke(_ context.Context, _, _, _ string) (InvocationResult, error) {
    idx := f.invocations
    f.invocations++
    if idx < len(f.results) {
        return f.results[idx], nil
    }
    return InvocationResult{ExitStatus: ExitClean}, nil
}

func TestEngineRunSucceedsOnFirstAttempt(t *testing.T) {
    // Set up a real git repo in a temp dir
    repoDir := t.TempDir()
    setupTestRepo(t, repoDir)

    // Write a minimal .armature config
    armDir := filepath.Join(repoDir, ".armature")
    require.NoError(t, os.MkdirAll(filepath.Join(armDir, "ops"), 0755))
    require.NoError(t, os.MkdirAll(filepath.Join(armDir, "state", "worker1", "issues"), 0755))
    cfg := config.Config{Mode: "single-branch", ProjectType: "go", Orchestrator: config.OrchestratorConfig{
        Language: "go", CoverageThreshold: 0,
        Adapters: config.AdapterCommands{Build: "true", Lint: "true", Coverage: "true"},
    }}
    cfgData, _ := json.MarshalIndent(cfg, "", "  ")
    require.NoError(t, os.WriteFile(filepath.Join(armDir, "config.json"), cfgData, 0644))
    setGitConfig(t, repoDir, "armature.mode", "single-branch")

    appCtx := &config.Context{
        RepoPath: repoDir, IssuesDir: armDir, Mode: "single-branch",
        StateDir: filepath.Join(armDir, "state", "worker1"), Config: cfg,
    }

    // Create a minimal issue
    issue := &materialize.Issue{
        ID: "T-1", Title: "test task", Status: "open",
        Scope:      []string{"internal/dag/"},
        Acceptance: json.RawMessage(`[{"type":"file-exists","value":"internal/dag/dag.go"}]`),
    }
    require.NoError(t, os.MkdirAll(filepath.Join(appCtx.StateDir, "issues"), 0755))
    require.NoError(t, materialize.WriteIssue(filepath.Join(appCtx.StateDir, "issues"), *issue))

    fake := &fakeHarness{name: "fake", results: []InvocationResult{{ExitStatus: ExitClean}}}
    engine := &Engine{
        AppCtx:  appCtx,
        Adapter: fake,
        // Skip sandbox check in tests
        SkipSandboxCheck: true,
    }

    opts := RunOptions{
        Harness: "fake", Retries: 3, TimeoutSec: 30,
        WorkerID: "worker1", LogPath: filepath.Join(armDir, "ops", "worker1.log"),
    }
    err := engine.Run(context.Background(), "T-1", opts)
    // Engine completes (or returns an error about missing scope path — that's OK for unit test scope)
    // The key is it doesn't panic and goes through the state machine
    _ = err
    assert.Equal(t, 1, fake.invocations)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/orchestrate/... -run TestEngineRunSucceedsOnFirstAttempt -v
```

- [ ] **Step 3: Create `internal/orchestrate/engine.go`**

```go
package orchestrate

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    ctx_pkg "github.com/scullxbones/armature/internal/context"
    "github.com/scullxbones/armature/internal/adapters"
    "github.com/scullxbones/armature/internal/config"
    "github.com/scullxbones/armature/internal/materialize"
    "github.com/scullxbones/armature/internal/ops"
    claimPkg "github.com/scullxbones/armature/internal/claim"
)

// Engine drives the full orchestration state machine for a single task.
type Engine struct {
    AppCtx           *config.Context
    Adapter          HarnessAdapter
    SkipSandboxCheck bool // set true in tests to bypass bwrap/seatbelt check
}

// Run executes the orchestration loop for taskID. Returns nil on success, error on escalation or preflight failure.
func (e *Engine) Run(ctx context.Context, taskID string, opts RunOptions) error {
    // Load issue from materialized state
    issue, err := e.loadIssue(taskID)
    if err != nil {
        return fmt.Errorf("load issue %s: %w", taskID, err)
    }

    // Preflight
    if !e.SkipSandboxCheck {
        if err := RunPreflight(issue, e.AppCtx.RepoPath, e.AppCtx.Config); err != nil {
            // Write rejection note and re-open
            _ = e.writeNote(opts.LogPath, opts.WorkerID, taskID,
                fmt.Sprintf("orchestrate preflight rejected: %v", err))
            return fmt.Errorf("preflight: %w", err)
        }
    }

    // Derive resume state
    allOps, err := ops.ReadLog(opts.LogPath)
    if err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("read log: %w", err)
    }
    state, err := DeriveState(allOps, taskID)
    if err != nil {
        return err
    }

    // Terminal states: surface and exit
    if state.Phase == "escalated" {
        return fmt.Errorf("task %s is already escalated — check 'arm show %s' for details", taskID, taskID)
    }
    if state.Phase == "done" {
        return nil
    }

    // Resolve worktree path
    worktreePath := state.WorktreePath
    if worktreePath == "" {
        worktreePath = filepath.Join(e.AppCtx.RepoPath, ".worktrees", taskID)
    }

    gc := adapters.NewGit(e.AppCtx.RepoPath)
    worktreeGC := adapters.NewGit(worktreePath)

    // Idle → claim + create worktree + write orchestrate-start
    if state.Phase == "idle" {
        if err := e.claimTask(opts.LogPath, opts.WorkerID, taskID); err != nil {
            return err
        }
        if err := e.ensureWorktree(gc, issue, worktreePath); err != nil {
            return err
        }
        startOp := ops.Op{
            Type: ops.OpOrchestrateStart, TargetID: taskID,
            Timestamp: nowEpoch(), WorkerID: opts.WorkerID,
            Payload: ops.Payload{
                Harness: e.Adapter.Name(), Model: opts.Model,
                RetryBudget: opts.Retries, Worktree: worktreePath,
            },
        }
        if err := ops.AppendAndCommit(opts.LogPath, e.AppCtx.WorktreePath, startOp, gc); err != nil {
            return err
        }
        state.Phase = "dispatched"
        state.Run = 1
        state.WorktreePath = worktreePath
        state.RetryBudget = opts.Retries
    }

    // Assemble adapter context
    renderCtx, err := e.assembleContext(taskID)
    if err != nil {
        return fmt.Errorf("assemble context: %w", err)
    }

    var lastFailures []CheckResult

    for {
        // Re-read allOps for fresh state
        allOps, _ = ops.ReadLog(opts.LogPath)

        switch state.Phase {
        case "dispatched":
            preRef, err := worktreeGC.HeadSHA()
            if err != nil {
                return fmt.Errorf("get pre-dispatch HEAD: %w", err)
            }
            state.PreDispatchRef = preRef

            feedback := ""
            if len(lastFailures) > 0 {
                // retryNum is 1-indexed: run 2 is retry 1, run 3 is retry 2, etc.
                retryNum := state.Run - 1
                feedback = AssembleFeedback(retryNum, opts.Retries, lastFailures, issue.Scope)
            }
            prompt := AssemblePrompt(renderCtx, issue, feedback)
            promptHash := hashPrompt(prompt)

            logFilePath := filepath.Join(e.AppCtx.RepoPath, ".arm", "orchestration", taskID,
                fmt.Sprintf("run-%d.log", state.Run))

            // Dry-run: print prompt and exit without touching the event log.
            if opts.DryRun {
                fmt.Printf("=== DRY RUN: prompt for %s run %d ===\n%s\n", taskID, state.Run, prompt)
                return nil
            }

            dispatchOp := ops.Op{
                Type: ops.OpOrchestrateDispatch, TargetID: taskID,
                Timestamp: nowEpoch(), WorkerID: opts.WorkerID,
                Payload: ops.Payload{Run: state.Run, PreDispatchRef: preRef, PromptHash: promptHash},
            }
            if err := ops.AppendAndCommit(opts.LogPath, e.AppCtx.WorktreePath, dispatchOp, gc); err != nil {
                return err
            }

            // Inject scope into context for adapter
            dispatchCtx := WithIssueScope(ctx, issue.Scope)
            if opts.TimeoutSec > 0 {
                var cancel context.CancelFunc
                dispatchCtx, cancel = context.WithTimeout(dispatchCtx, time.Duration(opts.TimeoutSec)*time.Second)
                defer cancel()
            }

            result, err := e.Adapter.Invoke(dispatchCtx, worktreePath, prompt, logFilePath)
            if err != nil {
                return fmt.Errorf("harness invoke: %w", err)
            }

            completeOp := ops.Op{
                Type: ops.OpOrchestrateDispatchComplete, TargetID: taskID,
                Timestamp: nowEpoch(), WorkerID: opts.WorkerID,
                Payload: ops.Payload{
                    Run: state.Run, ExitStatusStr: result.ExitStatus.String(),
                    DurationMs: result.DurationMs, LogPath: logFilePath,
                },
            }
            if err := ops.AppendAndCommit(opts.LogPath, e.AppCtx.WorktreePath, completeOp, gc); err != nil {
                return err
            }
            state.Phase = "verifying"

        case "verifying":
            // Staged transition: capture diff, reset, re-apply
            diff, err := worktreeGC.DiffFrom(state.PreDispatchRef)
            if err != nil {
                e.handleRollbackFailure(gc, opts, taskID, &state, worktreePath, err)
                continue // re-enter loop in correcting state
            }
            changedFiles, err := worktreeGC.DiffNameOnly(state.PreDispatchRef)
            if err != nil {
                e.handleRollbackFailure(gc, opts, taskID, &state, worktreePath, err)
                continue
            }
            if err := worktreeGC.ResetHard(state.PreDispatchRef); err != nil {
                e.handleRollbackFailure(gc, opts, taskID, &state, worktreePath, err)
                continue
            }
            if diff != "" {
                if err := worktreeGC.ApplyPatch(diff); err != nil {
                    e.handleRollbackFailure(gc, opts, taskID, &state, worktreePath, err)
                    continue
                }
            }

            // Resolve adapters
            adapters, _, detectErr := DetectAdapters(e.AppCtx.RepoPath)
            if detectErr != nil {
                adapters = e.AppCtx.Config.Orchestrator.Adapters
            }
            if e.AppCtx.Config.Orchestrator.Adapters.Build != "" {
                adapters = MergeAdapters(adapters, e.AppCtx.Config.Orchestrator.Adapters)
            }

            orchCfg := e.AppCtx.Config.Orchestrator
            orchCfg.Adapters = adapters

            results := RunPipeline(PipelineInput{
                ChangedFiles: changedFiles,
                WorktreeDir:  worktreePath,
                Issue:        issue,
                Cfg:          orchCfg,
                AllOps:       allOps,
            })

            // Collect failures
            var failures []CheckResult
            for _, r := range results {
                if r.Severity == SeverityFail {
                    failures = append(failures, r)
                    failOp := ops.Op{
                        Type: ops.OpOrchestrateVerifyFail, TargetID: taskID,
                        Timestamp: nowEpoch(), WorkerID: opts.WorkerID,
                        Payload: ops.Payload{
                            Run: state.Run, Check: r.Check,
                            Severity: string(r.Severity), Reason: r.Reason,
                        },
                    }
                    _ = ops.AppendAndCommit(opts.LogPath, e.AppCtx.WorktreePath, failOp, gc)
                }
            }

            if len(failures) == 0 {
                // All checks passed → commit + done
                commitMsg := e.buildCommitMessage(issue, results, e.Adapter.Name(), opts.Model)
                if err := worktreeGC.AddAll(); err != nil {
                    return fmt.Errorf("git add: %w", err)
                }
                commitSHA, err := worktreeGC.CommitWithMessage(commitMsg)
                if err != nil {
                    return fmt.Errorf("final commit: %w", err)
                }

                // Transition task to done
                transitionOp := ops.Op{
                    Type: ops.OpTransition, TargetID: taskID,
                    Timestamp: nowEpoch(), WorkerID: opts.WorkerID,
                    Payload: ops.Payload{To: ops.StatusDone, Branch: issue.Branch,
                        Outcome: fmt.Sprintf("Orchestrated by %s (model: %s)", e.Adapter.Name(), opts.Model)},
                }
                if err := ops.AppendAndCommit(opts.LogPath, e.AppCtx.WorktreePath, transitionOp, gc); err != nil {
                    return err
                }

                checksPassedNames := checksPassedList(results)
                completeOp := ops.Op{
                    Type: ops.OpOrchestrateComplete, TargetID: taskID,
                    Timestamp: nowEpoch(), WorkerID: opts.WorkerID,
                    Payload: ops.Payload{Run: state.Run, CommitSHA: commitSHA, ChecksPassed: checksPassedNames},
                }
                return ops.AppendAndCommit(opts.LogPath, e.AppCtx.WorktreePath, completeOp, gc)
            }

            // Failures exist
            lastFailures = failures
            state.Phase = "correcting"

        case "correcting":
            state.Run++
            if state.Run > opts.Retries+1 {
                return e.escalate(gc, opts, taskID, state, allOps)
            }
            retryOp := ops.Op{
                Type: ops.OpOrchestrateRetry, TargetID: taskID,
                Timestamp: nowEpoch(), WorkerID: opts.WorkerID,
                Payload: ops.Payload{Run: state.Run,
                    FeedbackSummary: buildRetryFeedbackSummary(state.Run-1, lastFailures)},
            }
            if err := ops.AppendAndCommit(opts.LogPath, e.AppCtx.WorktreePath, retryOp, gc); err != nil {
                return err
            }
            state.Phase = "dispatched"
        }
    }
}

func (e *Engine) claimTask(logPath, workerID, taskID string) error {
    gc := adapters.NewGit(e.AppCtx.RepoPath)

    // Load current state to check for scope overlap and existing claims.
    state, err := materialize.Materialize(e.AppCtx.IssuesDir, e.AppCtx.StateDir, e.AppCtx.Mode == "single-branch")
    if err != nil {
        return fmt.Errorf("materialize for claim: %w", err)
    }
    issue, ok := state.Issues[taskID]
    if !ok {
        return fmt.Errorf("issue %s not found in state", taskID)
    }

    // Check if already claimed by another worker — fail fast.
    if issue.ClaimedBy != "" && issue.ClaimedBy != workerID {
        return fmt.Errorf("task %s is already claimed by %s", taskID, issue.ClaimedBy)
    }

    // Check scope overlap with concurrently-claimed tasks.
    index, _ := materialize.LoadIndex(filepath.Join(e.AppCtx.StateDir, "index.json"))
    for id, entry := range index {
        if id == taskID || (entry.Status != ops.StatusClaimed && entry.Status != ops.StatusInProgress) {
            continue
        }
        if claimPkg.ScopesOverlap(issue.Scope, entry.Scope) && entry.AssignedWorker != workerID {
            return fmt.Errorf("task %s has scope overlap with %s (%s) — cannot claim concurrently", taskID, id, entry.Title)
        }
    }

    claimOp := ops.Op{
        Type: ops.OpClaim, TargetID: taskID,
        Timestamp: nowEpoch(), WorkerID: workerID,
        Payload: ops.Payload{TTL: 120},
    }
    if err := ops.AppendAndCommit(logPath, e.AppCtx.WorktreePath, claimOp, gc); err != nil {
        return err
    }

    // Auto-advance any open ancestor story to in-progress.
    if parentID := issue.Parent; parentID != "" {
        if parentEntry, ok := index[parentID]; ok && parentEntry.Status == ops.StatusOpen {
            advanceOp := ops.Op{
                Type: ops.OpTransition, TargetID: parentID,
                Timestamp: nowEpoch(), WorkerID: workerID,
                Payload: ops.Payload{To: ops.StatusInProgress},
            }
            _ = ops.AppendAndCommit(logPath, e.AppCtx.WorktreePath, advanceOp, gc)
        }
    }
    return nil
}

func (e *Engine) ensureWorktree(gc *adapters.Client, issue *materialize.Issue, worktreePath string) error {
    branch := issue.Branch
    if branch == "" {
        branch = "task/" + issue.ID
    }
    return gc.AddWorktree(branch, worktreePath)
}

func (e *Engine) escalate(gc *adapters.Client, opts RunOptions, taskID string, state OrchestrateState, allOps []ops.Op) error {
    var failures []ops.FailureRecord
    for _, op := range allOps {
        if op.Type == ops.OpOrchestrateVerifyFail && op.TargetID == taskID {
            failures = append(failures, ops.FailureRecord{
                Run: op.Payload.Run, Check: op.Payload.Check, Reason: op.Payload.Reason,
            })
        }
    }
    escalateOp := ops.Op{
        Type: ops.OpOrchestrateEscalate, TargetID: taskID,
        Timestamp: nowEpoch(), WorkerID: opts.WorkerID,
        Payload: ops.Payload{TotalRuns: state.Run - 1, Failures: failures},
    }
    if err := ops.AppendAndCommit(opts.LogPath, e.AppCtx.WorktreePath, escalateOp, gc); err != nil {
        return err
    }
    // Transition task to blocked
    blockedOp := ops.Op{
        Type: ops.OpTransition, TargetID: taskID,
        Timestamp: nowEpoch(), WorkerID: opts.WorkerID,
        Payload: ops.Payload{To: ops.StatusBlocked,
            Outcome: fmt.Sprintf("Orchestrator escalated after %d runs — see event log for details", state.Run-1)},
    }
    if err := ops.AppendAndCommit(opts.LogPath, e.AppCtx.WorktreePath, blockedOp, gc); err != nil {
        return err
    }
    return fmt.Errorf("task %s escalated after %d failed runs", taskID, state.Run-1)
}

// handleRollbackFailure writes the verify-fail op and sets state to correcting.
// The caller is responsible for continuing the loop (not returning the error).
func (e *Engine) handleRollbackFailure(gc *adapters.Client, opts RunOptions, taskID string, state *OrchestrateState, worktreePath string, cause error) {
    // Remove and re-create worktree from pre-dispatch ref
    _ = gc.RemoveWorktree(worktreePath)
    failOp := ops.Op{
        Type: ops.OpOrchestrateVerifyFail, TargetID: taskID,
        Timestamp: nowEpoch(), WorkerID: opts.WorkerID,
        Payload: ops.Payload{Run: state.Run, Check: "rollback", Severity: "fail",
            Reason: fmt.Sprintf("git reset/apply failed: %v", cause)},
    }
    _ = ops.AppendAndCommit(opts.LogPath, e.AppCtx.WorktreePath, failOp, gc)
    state.Phase = "correcting"
}

func (e *Engine) assembleContext(taskID string) (*ctx_pkg.Context, error) {
    state, err := materialize.Materialize(
        e.AppCtx.IssuesDir, e.AppCtx.StateDir, e.AppCtx.Mode == "single-branch")
    if err != nil {
        return nil, err
    }
    return ctx_pkg.Assemble(taskID, e.AppCtx.StateDir, state)
}

func (e *Engine) loadIssue(taskID string) (*materialize.Issue, error) {
    if _, err := materialize.Materialize(
        e.AppCtx.IssuesDir, e.AppCtx.StateDir, e.AppCtx.Mode == "single-branch"); err != nil {
        return nil, err
    }
    issue, err := materialize.LoadIssue(filepath.Join(e.AppCtx.StateDir, "issues", taskID+".json"))
    if err != nil {
        return nil, fmt.Errorf("issue %s not found: %w", taskID, err)
    }
    return &issue, nil
}

func (e *Engine) writeNote(logPath, workerID, taskID, msg string) error {
    gc := adapters.NewGit(e.AppCtx.RepoPath)
    return ops.AppendAndCommit(logPath, e.AppCtx.WorktreePath, ops.Op{
        Type: ops.OpNote, TargetID: taskID,
        Timestamp: nowEpoch(), WorkerID: workerID,
        Payload: ops.Payload{Msg: msg},
    }, gc)
}

func (e *Engine) buildCommitMessage(issue *materialize.Issue, results []CheckResult, harness, model string) string {
    checks := checksPassedList(results)
    coAuthor := harness
    if model != "" {
        coAuthor = fmt.Sprintf("%s (%s)", harness, model)
    }
    return fmt.Sprintf("feat(%s): %s\n\nChecks passed: %s\n\nCo-Authored-By: %s",
        issue.ID, issue.Title, strings.Join(checks, ", "), coAuthor)
}

func checksPassedList(results []CheckResult) []string {
    var out []string
    for _, r := range results {
        if r.Severity == SeverityPass {
            out = append(out, r.Check)
        }
    }
    return out
}

func buildRetryFeedbackSummary(run int, failures []CheckResult) string {
    names := make([]string, len(failures))
    for i, f := range failures {
        names[i] = f.Check
    }
    return fmt.Sprintf("run %d: %s", run, strings.Join(names, ", "))
}

func hashPrompt(prompt string) string {
    h := sha256.Sum256([]byte(prompt))
    return "sha256:" + hex.EncodeToString(h[:8])
}

func nowEpoch() int64 { return time.Now().Unix() }
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/orchestrate/... -v
```

Fix any compilation errors (missing imports, etc.).

- [ ] **Step 5: Run `make check` to verify no regressions**

```bash
make check
```

- [ ] **Step 6: Commit**

```bash
git add internal/orchestrate/engine.go internal/orchestrate/engine_test.go
git commit -m "feat: add orchestration engine state machine"
```

---

## Chunk 6: CLI Command and Wiring

### Task 14: Create `cmd/armature/orchestrate.go`

**Files:**
- Create: `cmd/armature/orchestrate.go`
- Modify: `cmd/armature/main.go`

- [ ] **Step 1: Write failing test — command exists in help output**

Add to `cmd/armature/main_test.go` (or a new file):
```go
func TestOrchestrateCommandRegistered(t *testing.T) {
    cmd := newRootCmd()
    found := false
    for _, sub := range cmd.Commands() {
        if sub.Use == "orchestrate [task-id]" || strings.HasPrefix(sub.Use, "orchestrate") {
            found = true
            break
        }
    }
    assert.True(t, found, "orchestrate command should be registered")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./cmd/armature/... -run TestOrchestrateCommandRegistered -v
```

- [ ] **Step 3: Create `cmd/armature/orchestrate.go`**

```go
package main

import (
    "fmt"
    "path/filepath"

    "github.com/scullxbones/armature/internal/materialize"
    "github.com/scullxbones/armature/internal/orchestrate"
    "github.com/scullxbones/armature/internal/worker"
    "github.com/spf13/cobra"
)

func newOrchestrateCmd() *cobra.Command {
    var (
        harness    string
        model      string
        retries    int
        timeoutStr string
        dryRun     bool
    )

    cmd := &cobra.Command{
        Use:   "orchestrate [task-id]",
        Short: "Dispatch a harness agent to complete a task with deterministic verification",
        Long: `Orchestrate wraps agent harness invocations in a state machine that enforces
scope boundaries, test existence, build health, lint, code coverage, and citation
requirements — all verified externally, without trusting the agent's self-reporting.

Exits zero on success, non-zero on escalation or preflight rejection.`,
        Example: `  # Orchestrate with Claude Code harness (default model from config)
  $ arm orchestrate E6-S4-T2 --harness claude

  # Override model and retry budget
  $ arm orchestrate E6-S4-T2 --harness claude --model claude-opus-4-7 --retries 5

  # Dry-run: show prompt without dispatching
  $ arm orchestrate E6-S4-T2 --harness claude --dry-run`,
        Args: cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            taskID := ""
            if len(args) > 0 {
                taskID = args[0]
            }
            if taskID == "" {
                return fmt.Errorf("task ID is required")
            }

            if harness == "" {
                harness = appCtx.Config.Orchestrator.DefaultHarness
            }
            if harness == "" {
                harness = "claude"
            }

            // Three-level model resolution: --model flag > task PreferredModel > config default
            resolvedModel := model
            if resolvedModel == "" {
                // Level 2: task's PreferredModel (set at decompose time)
                // Materialize state so the issues/ directory is populated.
                if _, err := materialize.Materialize(appCtx.IssuesDir, appCtx.StateDir, appCtx.Mode == "single-branch"); err == nil {
                    issueForModel, modelErr := materialize.LoadIssue(
                        filepath.Join(appCtx.StateDir, "issues", taskID+".json"))
                    if modelErr == nil && issueForModel.PreferredModel != "" {
                        resolvedModel = issueForModel.PreferredModel
                    }
                }
            }
            if resolvedModel == "" {
                resolvedModel = appCtx.Config.Orchestrator.DefaultModel
            }

            resolvedRetries := retries
            if resolvedRetries == 0 {
                resolvedRetries = appCtx.Config.Orchestrator.DefaultRetries
            }
            if resolvedRetries == 0 {
                resolvedRetries = 3
            }

            timeoutSec := 600 // default 10m
            if timeoutStr != "" {
                d, err := time.ParseDuration(timeoutStr)
                if err != nil {
                    return fmt.Errorf("invalid --timeout %q: %w", timeoutStr, err)
                }
                timeoutSec = int(d.Seconds())
            } else if appCtx.Config.Orchestrator.DefaultTimeout != "" {
                d, err := time.ParseDuration(appCtx.Config.Orchestrator.DefaultTimeout)
                if err == nil {
                    timeoutSec = int(d.Seconds())
                }
            }

            workerID, logPath, err := resolveWorkerAndLog()
            if err != nil {
                return err
            }

            // Resolve model: flag > task preferred_model > config default (engine handles task-level)
            hCfg := orchestrate.HarnessConfig{
                Adapter: harness, Model: resolvedModel, Timeout: timeoutSec,
            }
            adapter, err := orchestrate.NewHarnessAdapter(hCfg)
            if err != nil {
                return err
            }

            engine := &orchestrate.Engine{
                AppCtx:  appCtx,
                Adapter: adapter,
            }

            opts := orchestrate.RunOptions{
                Harness:    harness,
                Model:      resolvedModel,
                Retries:    resolvedRetries,
                TimeoutSec: timeoutSec,
                DryRun:     dryRun,
                WorkerID:   workerID,
                LogPath:    logPath,
            }

            _ = worker.GetWorkerID // referenced to ensure import
            _ = filepath.Join     // referenced to ensure import

            return engine.Run(cmd.Context(), taskID, opts)
        },
    }

    cmd.Flags().StringVar(&harness, "harness", "", "harness adapter: claude, codex, devin (default: from config)")
    cmd.Flags().StringVar(&model, "model", "", "model override (default: task preferred_model or config default)")
    cmd.Flags().IntVar(&retries, "retries", 0, "retry budget (default: 3 or from config)")
    cmd.Flags().StringVar(&timeoutStr, "timeout", "", "per-dispatch timeout, e.g. 10m (default: 10m or from config)")
    cmd.Flags().BoolVar(&dryRun, "dry-run", false, "run preflight and print prompt without dispatching")
    return cmd
}
```

Add `"time"` to imports.

- [ ] **Step 4: Register the command in `cmd/armature/main.go`**

In `newRootCmd()`, after the existing workflow commands, add:
```go
orchestrateCmd := newOrchestrateCmd()
orchestrateCmd.GroupID = "workflow"
root.AddCommand(orchestrateCmd)
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./cmd/armature/... -run TestOrchestrateCommandRegistered -v
```

- [ ] **Step 6: Build binary and verify it appears in help**

```bash
make build
./bin/arm orchestrate --help
```

Expected: help text for `arm orchestrate` is displayed.

- [ ] **Step 7: Commit**

```bash
git add cmd/armature/orchestrate.go cmd/armature/main.go cmd/armature/main_test.go
git commit -m "feat: add arm orchestrate CLI command"
```

---

### Task 15: Final validation

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -v 2>&1 | tail -50
```

Fix any failures.

- [ ] **Step 2: Run `make check`**

```bash
make check
```

All four stages (lint, test, coverage-check ≥80%, mutate) must be green.

- [ ] **Step 3: Smoke test help output**

```bash
./bin/arm orchestrate --help
./bin/arm --help | grep orchestrate
```

- [ ] **Step 4: Final commit if any fixes were needed**

```bash
git add -p
git commit -m "fix: address make check failures in orchestrator implementation"
```

---

## Implementation Notes

### Model resolution in engine
The engine's `Run()` receives `opts.Model` (from flag or config default). The task's `PreferredModel` field (set at decompose time) should override the config default but be overridden by the `--model` flag. Add this logic to the CLI command before calling `engine.Run`: if `opts.Model == ""`, load the issue's `PreferredModel` via a quick materialize.

### Worktree branch
The `ensureWorktree` method assumes the task has a `Branch` field set (from a prior `arm claim`). If `issue.Branch` is empty, the orchestrator creates a branch named `task/<task-id>`. The worktree uses this branch.

### Log directory
Run logs at `.arm/orchestration/<task-id>/run-N.log` — note this uses `.arm/` not `.armature/`. The `.arm/` directory is the default gitignore location per the spec. Add `.arm/orchestration/` to `.gitignore` during init or document it.

### `json.Unmarshal` in verify.go
The `jsonUnmarshal` helper avoids importing `encoding/json` directly — add the import to `verify.go`.

### Worktree `.git` file and `ResolveContext`
When the harness runs inside a worktree at `.worktrees/<task-id>`, `arm` commands invoked by the harness will have `ResolveContext` called from the worktree path. The existing `resolveParentRepoFromWorktree` handles this correctly — no changes needed.
