# E3 Collaboration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement E3 multi-worker collaboration — remote sync push/pull layer, audit log viewer, worker presence command, and advisory issue assignment — shipping all four as one atomic release.

**Architecture:** Each op type is classified as high-stakes (eager push after commit) or low-stakes (count-batched push); both flow through a new `Pusher` interface that degrades to a no-op in single-branch mode. Audit log reading walks raw JSONL op files rather than materialized state. Worker presence and assignment derive from materialized index state.

**Tech Stack:** Go, Cobra CLI, git exec subprocess, testify assert/require, file-backed push counter

---

## Chunk 1: E3-001 — Push/Sync Layer

### Task 1: Add `LowStakesPushThreshold` to Config

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go` (note: file is `package config`, no import prefix needed):
```go
func TestDefaultConfig_LowStakesPushThreshold(t *testing.T) {
    cfg := DefaultConfig("go")
    assert.Equal(t, 5, cfg.LowStakesPushThreshold)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/... -run TestDefaultConfig_LowStakesPushThreshold -v
```
Expected: FAIL — field does not exist yet.

- [ ] **Step 3: Add the field and update DefaultConfig**

In `internal/config/config.go`, add `LowStakesPushThreshold int` to `Config` and set its default to 5 in `DefaultConfig`:

```go
type Config struct {
    Mode                    string       `json:"mode"`
    ProjectType             string       `json:"project_type"`
    DefaultTTL              int          `json:"default_ttl"`
    TokenBudget             int          `json:"token_budget"`
    LowStakesPushThreshold  int          `json:"low_stakes_push_threshold,omitempty"`
    Hooks                   []HookConfig `json:"hooks"`
}

// In DefaultConfig:
func DefaultConfig(projectType string) Config {
    return Config{
        Mode:                   "single-branch",
        ProjectType:            projectType,
        DefaultTTL:             60,
        TokenBudget:            1600,
        LowStakesPushThreshold: 5,
        Hooks:                  []HookConfig{},
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/config/... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add LowStakesPushThreshold field (default 5)"
```

---

### Task 2: Add `Push` and `FetchAndRebase` to `git.Client`

**Files:**
- Modify: `internal/git/git.go`
- Modify: `internal/git/git_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/git/git_test.go`:

```go
func TestPush_FastForward(t *testing.T) {
    remote := t.TempDir()
    gitRun := func(dir string, args ...string) {
        cmd := exec.Command("git", args...)
        cmd.Dir = dir
        out, err := cmd.CombinedOutput()
        require.NoError(t, err, "git %v: %s", args, out)
    }
    gitRun(remote, "init", "--bare")

    worker := t.TempDir()
    gitRun(worker, "clone", remote, ".")
    gitRun(worker, "config", "user.email", "test@test.com")
    gitRun(worker, "config", "user.name", "Test")
    gitRun(worker, "config", "commit.gpgsign", "false")
    gitRun(worker, "checkout", "--orphan", "_trellis")
    gitRun(worker, "commit", "--allow-empty", "-m", "init: _trellis")

    c := git.New(worker)
    err := c.Push("_trellis")
    require.NoError(t, err)

    // Verify remote has the branch
    verifyCmd := exec.Command("git", "-C", remote, "rev-parse", "--verify", "_trellis")
    assert.NoError(t, verifyCmd.Run())
}

func TestPush_Rejected(t *testing.T) {
    remote := t.TempDir()
    gitRun := func(dir string, args ...string) string {
        cmd := exec.Command("git", args...)
        cmd.Dir = dir
        out, err := cmd.CombinedOutput()
        require.NoError(t, err, "git %v: %s", args, out)
        return strings.TrimSpace(string(out))
    }
    gitRun(remote, "init", "--bare")

    workerA := t.TempDir()
    workerB := t.TempDir()
    for _, w := range []string{workerA, workerB} {
        gitRun(w, "clone", remote, ".")
        gitRun(w, "config", "user.email", "test@test.com")
        gitRun(w, "config", "user.name", "Test")
        gitRun(w, "config", "commit.gpgsign", "false")
    }

    // Both workers start from the same base: A creates branch and pushes
    gitRun(workerA, "checkout", "--orphan", "_trellis")
    gitRun(workerA, "commit", "--allow-empty", "-m", "init")
    gitRun(workerA, "push", "-u", "origin", "_trellis")

    // B fetches and starts from the same tip as A
    gitRun(workerB, "fetch", "origin")
    gitRun(workerB, "checkout", "-b", "_trellis", "origin/_trellis")
    gitRun(workerB, "branch", "--set-upstream-to=origin/_trellis", "_trellis")
    // B makes a local commit (but doesn't push yet)
    gitRun(workerB, "commit", "--allow-empty", "-m", "worker-b op")

    // A makes another commit and pushes first — advancing the remote tip
    gitRun(workerA, "commit", "--allow-empty", "-m", "worker-a second")
    gitRun(workerA, "push", "origin", "_trellis")

    // Now B's push is rejected — remote tip has moved past B's base
    cB := git.New(workerB)
    err := cB.Push("_trellis")
    assert.Error(t, err, "expected push rejection")
}

func TestFetchAndRebase_MakesPushPossible(t *testing.T) {
    remote := t.TempDir()
    gitRun := func(dir string, args ...string) {
        cmd := exec.Command("git", args...)
        cmd.Dir = dir
        out, err := cmd.CombinedOutput()
        require.NoError(t, err, "git %v: %s", args, out)
    }
    gitRun(remote, "init", "--bare")

    workerA, workerB := t.TempDir(), t.TempDir()
    for _, w := range []string{workerA, workerB} {
        gitRun(w, "clone", remote, ".")
        gitRun(w, "config", "user.email", "test@test.com")
        gitRun(w, "config", "user.name", "Test")
        gitRun(w, "config", "commit.gpgsign", "false")
    }

    // A creates shared base and pushes
    gitRun(workerA, "checkout", "--orphan", "_trellis")
    gitRun(workerA, "commit", "--allow-empty", "-m", "init")
    gitRun(workerA, "push", "-u", "origin", "_trellis")

    // B fetches and tracks the branch, then makes a local commit
    gitRun(workerB, "fetch", "origin")
    gitRun(workerB, "checkout", "-b", "_trellis", "origin/_trellis")
    gitRun(workerB, "branch", "--set-upstream-to=origin/_trellis", "_trellis")
    gitRun(workerB, "commit", "--allow-empty", "-m", "worker-b op")

    // A advances remote
    gitRun(workerA, "commit", "--allow-empty", "-m", "worker-a op")
    gitRun(workerA, "push", "origin", "_trellis")

    // B's push would fail; FetchAndRebase then Push should succeed
    cB := git.New(workerB)
    require.NoError(t, cB.FetchAndRebase("_trellis"))
    require.NoError(t, cB.Push("_trellis"))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/git/... -run "TestPush|TestFetchAndRebase" -v
```
Expected: compile error — `Push` and `FetchAndRebase` methods don't exist yet.

- [ ] **Step 3: Add `Push` and `FetchAndRebase` to `internal/git/git.go`**

```go
// Push pushes the local branch to origin. Returns an error if the push is rejected.
func (c *Client) Push(branch string) error {
    cmd := exec.Command("git", "-C", c.repoPath, "push", "origin", branch)
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("git push origin %s: %w\n%s", branch, err, out)
    }
    return nil
}

// FetchAndRebase fetches origin and rebases the local branch onto origin/<branch>.
// Used to resolve push rejection after another worker has advanced the remote tip.
func (c *Client) FetchAndRebase(branch string) error {
    fetch := exec.Command("git", "-C", c.repoPath, "fetch", "origin", branch)
    if out, err := fetch.CombinedOutput(); err != nil {
        return fmt.Errorf("git fetch origin %s: %w\n%s", branch, err, out)
    }
    rebase := exec.Command("git", "-C", c.repoPath, "rebase", "origin/"+branch)
    if out, err := rebase.CombinedOutput(); err != nil {
        return fmt.Errorf("git rebase origin/%s: %w\n%s", branch, err, out)
    }
    return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/git/... -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat(git): add Push and FetchAndRebase methods to Client"
```

---

### Task 3: `Pusher` Interface + `NoPusher` + `AppendCommitAndPush`

**Files:**
- Create: `internal/ops/push.go`
- Create: `internal/ops/push_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/ops/push_test.go`:

```go
package ops_test

import (
    "testing"

    "github.com/scullxbones/trellis/internal/ops"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "os"
    "path/filepath"
)

// fakePusher records push/rebase calls and can simulate failure.
type fakePusher struct {
    pushCalls   int
    rebaseCalls int
    pushErr     error
    rebaseErr   error
}

func (f *fakePusher) Push(branch string) error {
    f.pushCalls++
    return f.pushErr
}

func (f *fakePusher) FetchAndRebase(branch string) error {
    f.rebaseCalls++
    return f.rebaseErr
}

// fakeTracker tracks Reset calls.
type fakeTracker struct {
    resets    int
    threshold int
    count     int
}

func (f *fakeTracker) Increment() bool {
    f.count++
    return f.count >= f.threshold
}

func (f *fakeTracker) Reset() {
    f.resets++
    f.count = 0
}

func TestAppendCommitAndPush_SingleBranch_NoPush(t *testing.T) {
    dir := t.TempDir()
    logPath := filepath.Join(dir, "ops", "abc.log")
    require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

    fp := &fakePusher{}
    fc := &fakeCommitter{}
    ft := &fakeTracker{}
    op := ops.Op{Type: ops.OpClaim, TargetID: "T1", Timestamp: 1000, WorkerID: "abc",
        Payload: ops.Payload{TTL: 60}}

    err := ops.AppendCommitAndPush(logPath, "", "_trellis", op, fc, fp, ft)
    require.NoError(t, err)

    // In single-branch mode: no commit, no push, no fetch
    assert.Equal(t, 0, fc.calls)
    assert.Equal(t, 0, fp.pushCalls)
    assert.Equal(t, 0, fp.rebaseCalls)
}

func TestAppendCommitAndPush_DualBranch_PushSucceeds(t *testing.T) {
    dir := t.TempDir()
    worktreePath := filepath.Join(dir, ".trellis")
    logPath := filepath.Join(worktreePath, ".issues", "ops", "abc.log")
    require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

    fp := &fakePusher{}  // push succeeds on first try
    fc := &fakeCommitter{}
    ft := &fakeTracker{}
    op := ops.Op{Type: ops.OpClaim, TargetID: "T1", Timestamp: 1000, WorkerID: "abc-def-ghi",
        Payload: ops.Payload{TTL: 60}}

    err := ops.AppendCommitAndPush(logPath, worktreePath, "_trellis", op, fc, fp, ft)
    require.NoError(t, err)

    assert.Equal(t, 1, fc.calls)     // committed once
    assert.Equal(t, 1, fp.rebaseCalls)
    assert.Equal(t, 1, fp.pushCalls)
    assert.Equal(t, 1, ft.resets)   // tracker reset after successful push
}

func TestAppendCommitAndPush_RetryOnRejection(t *testing.T) {
    dir := t.TempDir()
    worktreePath := filepath.Join(dir, ".trellis")
    logPath := filepath.Join(worktreePath, ".issues", "ops", "abc.log")
    require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

    attempt := 0
    fp := &fakePusher{pushErr: fmt.Errorf("rejected")}
    // Make push succeed on third attempt
    // Override Push to fail twice then succeed
    // We'll test via a different approach: use pushErr=nil for simple success test
    // This test verifies the retry calls FetchAndRebase before each retry
    fc := &fakeCommitter{}
    ft := &fakeTracker{}
    op := ops.Op{Type: ops.OpClaim, TargetID: "T1", Timestamp: 1000, WorkerID: "abc"}

    _ = attempt // for coverage
    // When all retries fail, error is returned
    err := ops.AppendCommitAndPush(logPath, worktreePath, "_trellis", op, fc, fp, ft)
    assert.Error(t, err)

    // Should have attempted push 4 times total (1 + 3 retries)
    assert.Equal(t, 4, fp.pushCalls)
    // Should have done FetchAndRebase before each retry (3 times, not the initial attempt)
    assert.GreaterOrEqual(t, fp.rebaseCalls, 3)
    // Tracker should NOT be reset since push failed
    assert.Equal(t, 0, ft.resets)
}

func TestNoPusher_IsNoop(t *testing.T) {
    p := ops.NoPusher{}
    assert.NoError(t, p.Push("_trellis"))
    assert.NoError(t, p.FetchAndRebase("_trellis"))
}
```

Note: `fakeCommitter` is already defined in `commit_test.go` in the `ops_test` package; add a local `fmt` import as needed.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ops/... -run "TestAppendCommitAndPush|TestNoPusher" -v
```
Expected: compile error — `AppendCommitAndPush`, `NoPusher` don't exist yet.

- [ ] **Step 3: Create `internal/ops/push.go`**

```go
package ops

import (
    "fmt"
    "path/filepath"
    "strings"
    "time"
)

// Pusher handles remote sync for the _trellis branch.
// In single-branch mode, both methods are no-ops returning nil.
type Pusher interface {
    FetchAndRebase(branch string) error
    Push(branch string) error
}

// NoPusher is a no-op Pusher used in single-branch mode.
type NoPusher struct{}

func (NoPusher) FetchAndRebase(_ string) error { return nil }
func (NoPusher) Push(_ string) error            { return nil }

// AppendCommitAndPush writes op to the log, commits it to the worktree, then
// pushes to origin. In single-branch mode (worktreePath == ""), the commit and
// push steps are skipped. Retries push up to 3 times with exponential backoff
// (1s, 2s, 4s), re-fetching and rebasing before each retry.
func AppendCommitAndPush(logPath, worktreePath, branch string, op Op, gc GitCommitter, p Pusher, t PendingPushTracker) error {
    if err := AppendOp(logPath, op); err != nil {
        return err
    }
    if worktreePath == "" {
        return nil // single-branch: skip commit, fetch, push
    }

    relPath, err := filepath.Rel(worktreePath, logPath)
    if err != nil {
        return fmt.Errorf("resolve relative log path: %w", err)
    }
    workerPrefix := op.WorkerID
    if len(workerPrefix) > 8 {
        workerPrefix = workerPrefix[:8]
    }
    message := fmt.Sprintf("ops: %s %s by %s", strings.ToLower(op.Type), op.TargetID, workerPrefix)
    if err := gc.CommitWorktreeOp(relPath, message); err != nil {
        return err
    }

    if err := p.FetchAndRebase(branch); err != nil {
        return fmt.Errorf("fetch and rebase before push: %w", err)
    }

    delays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
    var pushErr error
    if pushErr = p.Push(branch); pushErr == nil {
        t.Reset()
        return nil
    }
    for _, delay := range delays {
        time.Sleep(delay)
        if err := p.FetchAndRebase(branch); err != nil {
            return fmt.Errorf("rebase on push retry: %w", err)
        }
        if pushErr = p.Push(branch); pushErr == nil {
            t.Reset()
            return nil
        }
    }
    return fmt.Errorf("push failed after %d attempts: %w", 1+len(delays), pushErr)
}
```

- [ ] **Step 4: Fix test — add missing import in push_test.go**

The `TestAppendCommitAndPush_RetryOnRejection` test uses `fmt.Errorf`. Add `"fmt"` to imports in `push_test.go`.

Also, `fakeCommitter` is already defined in `commit_test.go` in the same `ops_test` package — no redeclaration needed. Verify by running:

```bash
go build ./internal/ops/...
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/ops/... -run "TestAppendCommitAndPush|TestNoPusher" -v
```
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ops/push.go internal/ops/push_test.go
git commit -m "feat(ops): add Pusher interface, NoPusher, and AppendCommitAndPush"
```

---

### Task 4: `PendingPushTracker` + `FilePushTracker` + `NoTracker`

**Files:**
- Create: `internal/ops/tracker.go`
- Create: `internal/ops/tracker_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/ops/tracker_test.go`:

```go
package ops_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/scullxbones/trellis/internal/ops"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestFilePushTracker_IncrementAndReset(t *testing.T) {
    dir := t.TempDir()
    stateDir := filepath.Join(dir, "state")
    require.NoError(t, os.MkdirAll(stateDir, 0755))

    tracker := ops.NewFilePushTracker(dir, 3)

    assert.False(t, tracker.Increment()) // count=1, threshold=3
    assert.False(t, tracker.Increment()) // count=2
    assert.True(t, tracker.Increment())  // count=3, threshold reached

    tracker.Reset()
    assert.False(t, tracker.Increment()) // count=1 after reset
}

func TestFilePushTracker_PersistsAcrossInstances(t *testing.T) {
    dir := t.TempDir()
    stateDir := filepath.Join(dir, "state")
    require.NoError(t, os.MkdirAll(stateDir, 0755))

    t1 := ops.NewFilePushTracker(dir, 5)
    t1.Increment()
    t1.Increment()

    // New instance reads persisted count
    t2 := ops.NewFilePushTracker(dir, 5)
    assert.False(t2.Increment()) // count=3
    assert.False(t2.Increment()) // count=4
    assert.True(t2.Increment())  // count=5, threshold reached
}

func TestFilePushTracker_DefaultThreshold(t *testing.T) {
    dir := t.TempDir()
    stateDir := filepath.Join(dir, "state")
    require.NoError(t, os.MkdirAll(stateDir, 0755))

    // threshold=0 means use default of 5
    tracker := ops.NewFilePushTracker(dir, 0)
    for i := 0; i < 4; i++ {
        assert.False(t, tracker.Increment())
    }
    assert.True(t, tracker.Increment()) // 5th triggers flush
}

func TestNoTracker_NeverFlushes(t *testing.T) {
    tracker := ops.NoTracker{}
    for i := 0; i < 100; i++ {
        assert.False(t, tracker.Increment())
    }
    tracker.Reset() // no-op, no panic
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ops/... -run "TestFilePushTracker|TestNoTracker" -v
```
Expected: compile error — types don't exist yet.

- [ ] **Step 3: Create `internal/ops/tracker.go`**

```go
package ops

import (
    "os"
    "path/filepath"
    "strconv"
    "strings"
)

// PendingPushTracker counts locally-committed-but-unpushed low-stakes ops.
// Returns true from Increment when the threshold is reached (flush needed).
type PendingPushTracker interface {
    Increment() (shouldFlush bool)
    Reset()
}

// FilePushTracker persists the pending count to .issues/state/pending-push-count.
// The file is never committed to git — it is local process state only.
type FilePushTracker struct {
    path      string
    threshold int
}

// NewFilePushTracker creates a FilePushTracker. threshold=0 uses the default of 5.
func NewFilePushTracker(issuesDir string, threshold int) *FilePushTracker {
    if threshold <= 0 {
        threshold = 5
    }
    return &FilePushTracker{
        path:      filepath.Join(issuesDir, "state", "pending-push-count"),
        threshold: threshold,
    }
}

func (f *FilePushTracker) Increment() bool {
    count := f.read() + 1
    f.write(count)
    return count >= f.threshold
}

func (f *FilePushTracker) Reset() {
    f.write(0)
}

func (f *FilePushTracker) read() int {
    data, err := os.ReadFile(f.path)
    if err != nil {
        return 0
    }
    n, err := strconv.Atoi(strings.TrimSpace(string(data)))
    if err != nil {
        return 0
    }
    return n
}

func (f *FilePushTracker) write(n int) {
    _ = os.WriteFile(f.path, []byte(strconv.Itoa(n)), 0644)
}

// NoTracker is a no-op PendingPushTracker used in single-branch mode.
type NoTracker struct{}

func (NoTracker) Increment() bool { return false }
func (NoTracker) Reset()          {}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ops/... -run "TestFilePushTracker|TestNoTracker" -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ops/tracker.go internal/ops/tracker_test.go
git commit -m "feat(ops): add PendingPushTracker, FilePushTracker, NoTracker"
```

---

### Task 5: Wire Push Dependencies through Command Helpers

**Files:**
- Modify: `cmd/trellis/helpers.go`
- Modify: `cmd/trellis/main.go`
- Modify: `cmd/trellis/claim.go`
- Modify: `cmd/trellis/create.go`
- Modify: `cmd/trellis/transition.go`
- Modify: `cmd/trellis/reopen.go`
- Modify: `cmd/trellis/decision.go`
- Modify: `cmd/trellis/link.go`
- Modify: `cmd/trellis/note.go`
- Modify: `cmd/trellis/heartbeat.go`

This task has no dedicated unit tests (the integration path is covered by git_test.go); verify by running the full test suite.

- [ ] **Step 1: Add push deps to `cmd/trellis/helpers.go`**

Replace the existing `appendOp` function and add new helpers. The final file should look like:

```go
package main

import (
    "fmt"
    "time"

    "github.com/scullxbones/trellis/internal/git"
    "github.com/scullxbones/trellis/internal/ops"
    "github.com/scullxbones/trellis/internal/worker"
)

var appPusher  ops.Pusher
var appTracker ops.PendingPushTracker

func resolveWorkerAndLog() (string, string, error) {
    workerID, err := worker.GetWorkerID(appCtx.RepoPath)
    if err != nil {
        return "", "", fmt.Errorf("worker not initialized: %w", err)
    }
    logPath := fmt.Sprintf("%s/ops/%s.log", appCtx.IssuesDir, workerID)
    return workerID, logPath, nil
}

func nowEpoch() int64 {
    return time.Now().Unix()
}

// initPushDeps initialises the push/tracker singletons based on the resolved context.
// Must be called after appCtx is set (i.e., from PersistentPreRunE).
func initPushDeps() {
    if appCtx.WorktreePath != "" {
        gc := git.New(appCtx.WorktreePath)
        appPusher = gc
        appTracker = ops.NewFilePushTracker(appCtx.IssuesDir, appCtx.Config.LowStakesPushThreshold)
    } else {
        appPusher = ops.NoPusher{}
        appTracker = ops.NoTracker{}
    }
}

// appendHighStakesOp appends a high-stakes op, commits it, and immediately pushes.
func appendHighStakesOp(logPath string, op ops.Op) error {
    var gc ops.GitCommitter
    if appCtx.WorktreePath != "" {
        gc = git.New(appCtx.WorktreePath)
    }
    return ops.AppendCommitAndPush(logPath, appCtx.WorktreePath, "_trellis", op, gc, appPusher, appTracker)
}

// appendLowStakesOp appends a low-stakes op, commits it locally, and flushes
// when the pending count reaches the configured threshold.
func appendLowStakesOp(logPath string, op ops.Op) error {
    var gc ops.GitCommitter
    if appCtx.WorktreePath != "" {
        gc = git.New(appCtx.WorktreePath)
    }
    if err := ops.AppendAndCommit(logPath, appCtx.WorktreePath, op, gc); err != nil {
        return err
    }
    if shouldFlush := appTracker.Increment(); shouldFlush {
        if err := appPusher.FetchAndRebase("_trellis"); err != nil {
            return fmt.Errorf("low-stakes flush rebase: %w", err)
        }
        if err := appPusher.Push("_trellis"); err != nil {
            return fmt.Errorf("low-stakes flush push: %w", err)
        }
        appTracker.Reset()
    }
    return nil
}

// appendOp is kept for backward compatibility during migration. New code should
// use appendHighStakesOp or appendLowStakesOp.
func appendOp(logPath string, op ops.Op) error {
    var gc ops.GitCommitter
    if appCtx.WorktreePath != "" {
        gc = git.New(appCtx.WorktreePath)
    }
    return ops.AppendAndCommit(logPath, appCtx.WorktreePath, op, gc)
}
```

- [ ] **Step 2: Call `initPushDeps()` from `PersistentPreRunE` in `cmd/trellis/main.go`**

In `newRootCmd`, update the `PersistentPreRunE` block:

```go
PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
    repoPath, _ := cmd.Flags().GetString("repo")
    if repoPath == "" {
        repoPath = "."
    }
    ctx, err := config.ResolveContext(repoPath)
    if err != nil {
        return err
    }
    appCtx = ctx
    initPushDeps()  // <-- add this line
    return nil
},
```

- [ ] **Step 3: Update high-stakes commands to use `appendHighStakesOp`**

For each of these files, find the call to `appendOp(logPath, op)` for the **primary op** (not the scope-overlap notes, which remain low-stakes) and change it to `appendHighStakesOp(logPath, op)`:

**`cmd/trellis/claim.go`** — the final `appendOp(logPath, op)` (the claim op itself):
```go
// Before:
if err := appendOp(logPath, op); err != nil {
// After:
if err := appendHighStakesOp(logPath, op); err != nil {
```
Keep the two `appendOp` calls for the scope-overlap notes — those are informational (low-stakes).

**`cmd/trellis/create.go`** — the create op:
```go
// Change: appendOp(logPath, op) → appendHighStakesOp(logPath, op)
```

**`cmd/trellis/transition.go`** — the transition op:
```go
// Change: appendOp(logPath, op) → appendHighStakesOp(logPath, op)
```

**`cmd/trellis/reopen.go`** — the reopen/transition op:
```go
// Change: appendOp(logPath, op) → appendHighStakesOp(logPath, op)
```

**`cmd/trellis/decision.go`** — the decision op:
```go
// Change: appendOp(logPath, op) → appendHighStakesOp(logPath, op)
```

**`cmd/trellis/link.go`** — the link op:
```go
// Change: appendOp(logPath, op) → appendHighStakesOp(logPath, op)
```

- [ ] **Step 4: Update low-stakes commands to use `appendLowStakesOp`**

**`cmd/trellis/note.go`** — the note op:
```go
// Change: appendOp(logPath, op) → appendLowStakesOp(logPath, op)
```

**`cmd/trellis/heartbeat.go`** — the heartbeat op:
```go
// Change: appendOp(logPath, op) → appendLowStakesOp(logPath, op)
```

- [ ] **Step 5: Build and run all tests**

```bash
go build ./...
go test ./...
```
Expected: all PASS, no compile errors.

- [ ] **Step 6: Commit**

```bash
git add cmd/trellis/helpers.go cmd/trellis/main.go \
        cmd/trellis/claim.go cmd/trellis/create.go \
        cmd/trellis/transition.go cmd/trellis/reopen.go \
        cmd/trellis/decision.go cmd/trellis/link.go \
        cmd/trellis/note.go cmd/trellis/heartbeat.go
git commit -m "feat(cmd): wire high-stakes/low-stakes push through command helpers"
```

---

## Chunk 2: E3-003 — Audit Log Viewer

> This chunk has no dependency on Chunk 1 and can be implemented in parallel with Tasks 2–5.

### Task 6: `internal/audit/audit.go`

**Files:**
- Create: `internal/audit/audit.go`
- Create: `internal/audit/audit_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/audit/audit_test.go`:

```go
package audit_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/scullxbones/trellis/internal/audit"
    "github.com/scullxbones/trellis/internal/ops"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func writeLog(t *testing.T, dir, workerID string, logOps []ops.Op) {
    t.Helper()
    opsDir := filepath.Join(dir, "ops")
    require.NoError(t, os.MkdirAll(opsDir, 0755))
    logPath := filepath.Join(opsDir, workerID+".log")
    for _, op := range logOps {
        require.NoError(t, ops.AppendOp(logPath, op))
    }
}

func TestLoad_ReturnsAllOps(t *testing.T) {
    dir := t.TempDir()
    writeLog(t, dir, "worker-a", []ops.Op{
        {Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a", Payload: ops.Payload{Title: "Task 1", NodeType: "task"}},
        {Type: ops.OpClaim, TargetID: "T1", Timestamp: 200, WorkerID: "worker-a", Payload: ops.Payload{TTL: 60}},
    })
    writeLog(t, dir, "worker-b", []ops.Op{
        {Type: ops.OpNote, TargetID: "T1", Timestamp: 150, WorkerID: "worker-b", Payload: ops.Payload{Msg: "hello"}},
    })

    entries, err := audit.Load(filepath.Join(dir, "ops"), audit.Filter{})
    require.NoError(t, err)
    assert.Len(t, entries, 3)
    // Should be sorted by timestamp
    assert.Equal(t, int64(100), entries[0].Op.Timestamp)
    assert.Equal(t, int64(150), entries[1].Op.Timestamp)
    assert.Equal(t, int64(200), entries[2].Op.Timestamp)
}

func TestLoad_FilterByIssue(t *testing.T) {
    dir := t.TempDir()
    writeLog(t, dir, "worker-a", []ops.Op{
        {Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a"},
        {Type: ops.OpCreate, TargetID: "T2", Timestamp: 101, WorkerID: "worker-a"},
    })

    entries, err := audit.Load(filepath.Join(dir, "ops"), audit.Filter{IssueID: "T1"})
    require.NoError(t, err)
    assert.Len(t, entries, 1)
    assert.Equal(t, "T1", entries[0].Op.TargetID)
}

func TestLoad_FilterByWorker(t *testing.T) {
    dir := t.TempDir()
    writeLog(t, dir, "worker-a", []ops.Op{
        {Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a"},
    })
    writeLog(t, dir, "worker-b", []ops.Op{
        {Type: ops.OpNote, TargetID: "T1", Timestamp: 101, WorkerID: "worker-b"},
    })

    entries, err := audit.Load(filepath.Join(dir, "ops"), audit.Filter{WorkerID: "worker-a"})
    require.NoError(t, err)
    assert.Len(t, entries, 1)
    assert.Equal(t, "worker-a", entries[0].Op.WorkerID)
}

func TestLoad_FilterBySince(t *testing.T) {
    dir := t.TempDir()
    writeLog(t, dir, "worker-a", []ops.Op{
        {Type: ops.OpNote, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a"},
        {Type: ops.OpNote, TargetID: "T1", Timestamp: 200, WorkerID: "worker-a"},
    })

    entries, err := audit.Load(filepath.Join(dir, "ops"), audit.Filter{Since: 150})
    require.NoError(t, err)
    assert.Len(t, entries, 1)
    assert.Equal(t, int64(200), entries[0].Op.Timestamp)
}

func TestLoad_MarkLostRaceClaims(t *testing.T) {
    dir := t.TempDir()
    // Worker A claims at t=100, Worker B claims at t=200 — B loses
    writeLog(t, dir, "worker-a", []ops.Op{
        {Type: ops.OpClaim, TargetID: "T1", Timestamp: 100, WorkerID: "worker-a", Payload: ops.Payload{TTL: 60}},
    })
    writeLog(t, dir, "worker-b", []ops.Op{
        {Type: ops.OpClaim, TargetID: "T1", Timestamp: 200, WorkerID: "worker-b", Payload: ops.Payload{TTL: 60}},
    })

    entries, err := audit.Load(filepath.Join(dir, "ops"), audit.Filter{})
    require.NoError(t, err)
    require.Len(t, entries, 2)

    // Worker A wins (earlier timestamp); Worker B's claim is marked as lost
    var aEntry, bEntry audit.AuditEntry
    for _, e := range entries {
        if e.Op.WorkerID == "worker-a" {
            aEntry = e
        } else {
            bEntry = e
        }
    }
    assert.False(t, aEntry.LostRace)
    assert.True(t, bEntry.LostRace)
}

func TestLoad_EmptyDir(t *testing.T) {
    dir := t.TempDir()
    opsDir := filepath.Join(dir, "ops")
    require.NoError(t, os.MkdirAll(opsDir, 0755))

    entries, err := audit.Load(opsDir, audit.Filter{})
    require.NoError(t, err)
    assert.Empty(t, entries)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/audit/... -v
```
Expected: compile error — package doesn't exist yet.

- [ ] **Step 3: Create `internal/audit/audit.go`**

```go
package audit

import (
    "os"
    "path/filepath"
    "sort"
    "strings"

    "github.com/scullxbones/trellis/internal/claim"
    "github.com/scullxbones/trellis/internal/ops"
)

// Filter controls which ops are returned by Load.
type Filter struct {
    IssueID  string // empty = no filter
    WorkerID string // empty = no filter
    Since    int64  // Unix epoch; 0 = no filter
}

// AuditEntry is a single op with its derived metadata.
type AuditEntry struct {
    Op       ops.Op
    LostRace bool // true for claim ops that lost the claim race
}

// Load reads all *.log files from opsDir, merges them by timestamp,
// applies the filter, and marks losing claim ops.
func Load(opsDir string, f Filter) ([]AuditEntry, error) {
    entries, err := os.ReadDir(opsDir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil
        }
        return nil, err
    }

    // Collect all ops from all worker log files.
    var allOps []ops.Op
    for _, e := range entries {
        if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
            continue
        }
        workerID := strings.TrimSuffix(e.Name(), ".log")
        logPath := filepath.Join(opsDir, e.Name())
        workerOps, err := ops.ReadLogValidated(logPath, workerID)
        if err != nil {
            return nil, err
        }
        allOps = append(allOps, workerOps...)
    }

    // Sort by timestamp, then by worker ID for deterministic order.
    sort.SliceStable(allOps, func(i, j int) bool {
        if allOps[i].Timestamp != allOps[j].Timestamp {
            return allOps[i].Timestamp < allOps[j].Timestamp
        }
        return allOps[i].WorkerID < allOps[j].WorkerID
    })

    // Identify losing claims per issue.
    // Group claims by target ID, resolve winner, mark losers.
    claimsByIssue := make(map[string][]ops.Op)
    for _, op := range allOps {
        if op.Type == ops.OpClaim {
            claimsByIssue[op.TargetID] = append(claimsByIssue[op.TargetID], op)
        }
    }
    type claimKey struct{ issueID, workerID string }
    lostRace := make(map[claimKey]bool)
    for issueID, claims := range claimsByIssue {
        if len(claims) <= 1 {
            continue
        }
        winner := claim.ResolveClaim(claims)
        for _, c := range claims {
            if c.WorkerID != winner.WorkerID {
                lostRace[claimKey{issueID, c.WorkerID}] = true
            }
        }
    }

    // Apply filters and build result.
    var result []AuditEntry
    for _, op := range allOps {
        if f.IssueID != "" && op.TargetID != f.IssueID {
            continue
        }
        if f.WorkerID != "" && op.WorkerID != f.WorkerID {
            continue
        }
        if f.Since != 0 && op.Timestamp < f.Since {
            continue
        }
        lost := lostRace[claimKey{op.TargetID, op.WorkerID}] && op.Type == ops.OpClaim
        result = append(result, AuditEntry{Op: op, LostRace: lost})
    }
    return result, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/audit/... -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/audit/audit.go internal/audit/audit_test.go
git commit -m "feat(audit): add audit.Load — reads, merges, filters, annotates op logs"
```

---

### Task 7: `cmd/trellis/log.go` — `trls log` command

**Files:**
- Create: `cmd/trellis/log.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write the failing test**

Add to a new `cmd/trellis/log_test.go` (or verify integration manually after wiring). Since the `cmd` package uses a global `appCtx`, integration testing requires a real repo. Run a smoke-test after wiring instead.

- [ ] **Step 2: Create `cmd/trellis/log.go`**

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
    "strconv"
    "strings"
    "time"

    "github.com/scullxbones/trellis/internal/audit"
    "github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
    var issueID, workerID, sinceStr string

    cmd := &cobra.Command{
        Use:   "log",
        Short: "Show audit log of ops",
        RunE: func(cmd *cobra.Command, args []string) error {
            opsDir := appCtx.IssuesDir + "/ops"

            var since int64
            if sinceStr != "" {
                // Try Unix epoch first, then RFC3339
                if n, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
                    since = n
                } else if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
                    since = t.Unix()
                } else {
                    return fmt.Errorf("--since: cannot parse %q as Unix epoch or RFC3339", sinceStr)
                }
            }

            filter := audit.Filter{
                IssueID:  issueID,
                WorkerID: workerID,
                Since:    since,
            }

            entries, err := audit.Load(opsDir, filter)
            if err != nil {
                return err
            }

            format, _ := cmd.Flags().GetString("format")
            if format == "json" || format == "agent" {
                return printLogJSON(cmd, entries)
            }
            return printLogHuman(cmd, entries)
        },
    }

    cmd.Flags().StringVar(&issueID, "issue", "", "filter by issue ID")
    cmd.Flags().StringVar(&workerID, "worker", "", "filter by worker ID")
    cmd.Flags().StringVar(&sinceStr, "since", "", "filter ops at or after TIME (Unix epoch or RFC3339)")
    return cmd
}

func printLogHuman(cmd *cobra.Command, entries []audit.AuditEntry) error {
    for _, e := range entries {
        ts := time.Unix(e.Op.Timestamp, 0).UTC().Format("2006-01-02T15:04:05")
        lost := ""
        if e.LostRace {
            lost = "  [lost race]"
        }
        extra := opSummary(e.Op)
        fmt.Fprintf(cmd.OutOrStdout(), "%s  %-12s  %-10s  %-20s%s%s\n",
            ts, e.Op.Type, e.Op.TargetID, e.Op.WorkerID, extra, lost)
    }
    return nil
}

func opSummary(op interface{ GetPayload() interface{} }) string {
    return ""
}

// opSummary returns a short inline summary of op-specific payload fields.
func printLogJSON(cmd *cobra.Command, entries []audit.AuditEntry) error {
    enc := json.NewEncoder(cmd.OutOrStdout())
    for _, e := range entries {
        record := map[string]interface{}{
            "type":      e.Op.Type,
            "target_id": e.Op.TargetID,
            "timestamp": e.Op.Timestamp,
            "worker_id": e.Op.WorkerID,
            "payload":   e.Op.Payload,
        }
        if e.LostRace {
            record["_lost_race"] = true
        }
        if err := enc.Encode(record); err != nil {
            return err
        }
    }
    return nil
}
```

Note: The human output helper `opSummary` above is a placeholder — replace the two conflicting `opSummary` declarations with a proper inline approach. Here is the corrected final version of the file:

```go
package main

import (
    "encoding/json"
    "fmt"
    "strconv"
    "time"

    "github.com/scullxbones/trellis/internal/audit"
    "github.com/scullxbones/trellis/internal/ops"
    "github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
    var issueID, workerID, sinceStr string

    cmd := &cobra.Command{
        Use:   "log",
        Short: "Show audit log of ops (read raw op logs, not materialized state)",
        RunE: func(cmd *cobra.Command, args []string) error {
            opsDir := appCtx.IssuesDir + "/ops"

            var since int64
            if sinceStr != "" {
                if n, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
                    since = n
                } else if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
                    since = t.Unix()
                } else {
                    return fmt.Errorf("--since: cannot parse %q as Unix epoch or RFC3339", sinceStr)
                }
            }

            entries, err := audit.Load(opsDir, audit.Filter{
                IssueID:  issueID,
                WorkerID: workerID,
                Since:    since,
            })
            if err != nil {
                return err
            }

            format, _ := cmd.Flags().GetString("format")
            if format == "json" || format == "agent" {
                enc := json.NewEncoder(cmd.OutOrStdout())
                for _, e := range entries {
                    rec := map[string]interface{}{
                        "type":      e.Op.Type,
                        "target_id": e.Op.TargetID,
                        "timestamp": e.Op.Timestamp,
                        "worker_id": e.Op.WorkerID,
                        "payload":   e.Op.Payload,
                    }
                    if e.LostRace {
                        rec["_lost_race"] = true
                    }
                    if err := enc.Encode(rec); err != nil {
                        return err
                    }
                }
                return nil
            }

            // Human-readable
            for _, e := range entries {
                ts := time.Unix(e.Op.Timestamp, 0).UTC().Format("2006-01-02T15:04:05")
                extra := logPayloadSummary(e.Op)
                lost := ""
                if e.LostRace {
                    lost = "  [lost race]"
                }
                fmt.Fprintf(cmd.OutOrStdout(), "%s  %-14s  %-12s  %s%s%s\n",
                    ts, e.Op.Type, e.Op.TargetID, e.Op.WorkerID, extra, lost)
            }
            return nil
        },
    }

    cmd.Flags().StringVar(&issueID, "issue", "", "filter by issue ID")
    cmd.Flags().StringVar(&workerID, "worker", "", "filter by worker ID")
    cmd.Flags().StringVar(&sinceStr, "since", "", "filter ops at or after TIME (Unix epoch or RFC3339)")
    return cmd
}

// logPayloadSummary returns a short inline annotation for human-readable output.
// Note: the ops.OpAssign case is added in Task 8 after OpAssign is defined.
func logPayloadSummary(op ops.Op) string {
    switch op.Type {
    case ops.OpClaim:
        return fmt.Sprintf("  ttl=%dm", op.Payload.TTL)
    case ops.OpTransition:
        return fmt.Sprintf("  → %s", op.Payload.To)
    case ops.OpNote:
        msg := op.Payload.Msg
        if len(msg) > 40 {
            msg = msg[:37] + "..."
        }
        return fmt.Sprintf("  %q", msg)
    }
    return ""
}
```

- [ ] **Step 3: Register `newLogCmd` in `main.go`**

In `cmd/trellis/main.go`, add inside `newRootCmd`:
```go
root.AddCommand(newLogCmd())
```

- [ ] **Step 4: Build to verify compilation**

```bash
go build ./cmd/trellis/...
```
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add cmd/trellis/log.go cmd/trellis/main.go
git commit -m "feat(cmd): add trls log command for audit log viewing"
```

---

## Chunk 3: E3-004 & E3-002 — Assignment & Workers

### Task 8: Register `OpAssign` — Types, ValidOpTypes, Payload, Schema

**Files:**
- Modify: `internal/ops/types.go`
- Modify: `internal/ops/schema.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/ops/ops_test.go`:

```go
func TestParseAssignOp(t *testing.T) {
    line := `["assign","T1-001",1742042531,"worker-a1",{"assigned_to":"worker-b2"}]`

    op, err := ParseLine([]byte(line))
    require.NoError(t, err)
    assert.Equal(t, ops.OpAssign, op.Type)
    assert.Equal(t, "T1-001", op.TargetID)
    assert.Equal(t, "worker-b2", op.Payload.AssignedTo)
}

func TestParseUnassignOp(t *testing.T) {
    // unassign is assign with assigned_to: ""
    line := `["assign","T1-001",1742042531,"worker-a1",{}]`

    op, err := ParseLine([]byte(line))
    require.NoError(t, err)
    assert.Equal(t, ops.OpAssign, op.Type)
    assert.Equal(t, "", op.Payload.AssignedTo)
}

func TestGenerateSchema_ContainsAssign(t *testing.T) {
    schema := ops.GenerateSchema()
    assert.Contains(t, schema, "assign")
    assert.Contains(t, schema, "assigned_to")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ops/... -run "TestParseAssignOp|TestParseUnassignOp|TestGenerateSchema_ContainsAssign" -v
```
Expected: FAIL — `OpAssign` constant and `AssignedTo` field don't exist yet.

- [ ] **Step 3: Update `internal/ops/types.go`**

Add the constant:
```go
const (
    // ... existing constants ...
    OpAssign = "assign"
)
```

Add to `ValidOpTypes`:
```go
var ValidOpTypes = map[string]bool{
    OpCreate: true, OpClaim: true, OpHeartbeat: true,
    OpTransition: true, OpNote: true, OpLink: true,
    OpSourceLink: true, OpSourceFingerprint: true,
    OpDAGTransition: true, OpDecision: true,
    OpAssign: true,  // E3-004
}
```

Add `AssignedTo` to `Payload`:
```go
// assign
AssignedTo string `json:"assigned_to,omitempty"`
```
Add it after the `decision` block at the end of the Payload struct.

- [ ] **Step 4: Update `internal/ops/schema.go`**

In `GenerateSchema()`, update the op_type comment and add the payload section:
```go
// Position 0: op_type (string) — one of: create, claim, heartbeat, transition,
//             note, link, source-link, source-fingerprint, dag-transition, decision,
//             assign
```

Add to the payload section:
```go
#   assign:             assigned_to
```

- [ ] **Step 5: Add the `OpAssign` case to `logPayloadSummary` in `cmd/trellis/log.go`**

Now that `OpAssign` is defined, add the case to `logPayloadSummary`:

```go
case ops.OpAssign:
    if op.Payload.AssignedTo == "" {
        return "  (unassign)"
    }
    return fmt.Sprintf("  → %s", op.Payload.AssignedTo)
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/ops/... -v
go build ./cmd/trellis/...
```
Expected: all PASS, binary builds without errors.

- [ ] **Step 7: Commit**

```bash
git add internal/ops/types.go internal/ops/schema.go cmd/trellis/log.go
git commit -m "feat(ops): add OpAssign type, AssignedTo payload field, schema update"
```

---

### Task 9: Materialization — `AssignedWorker` + `applyAssign`

**Files:**
- Modify: `internal/materialize/state.go`
- Modify: `internal/materialize/engine.go`
- Modify: `internal/materialize/engine_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/materialize/engine_test.go`:

```go
func TestApplyAssign_SetsAssignedWorker(t *testing.T) {
    s := materialize.NewState()
    // Create an issue first
    createOp := ops.Op{Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
        Payload: ops.Payload{Title: "Task 1", NodeType: "task"}}
    require.NoError(t, s.ApplyOp(createOp))

    // Assign it
    assignOp := ops.Op{Type: ops.OpAssign, TargetID: "T1", Timestamp: 200, WorkerID: "w1",
        Payload: ops.Payload{AssignedTo: "worker-b"}}
    require.NoError(t, s.ApplyOp(assignOp))

    assert.Equal(t, "worker-b", s.Issues["T1"].AssignedWorker)
}

func TestApplyAssign_Unassign(t *testing.T) {
    s := materialize.NewState()
    createOp := ops.Op{Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
        Payload: ops.Payload{Title: "Task 1", NodeType: "task"}}
    require.NoError(t, s.ApplyOp(createOp))
    require.NoError(t, s.ApplyOp(ops.Op{Type: ops.OpAssign, TargetID: "T1", Timestamp: 200, WorkerID: "w1",
        Payload: ops.Payload{AssignedTo: "worker-b"}}))

    // Unassign (assigned_to == "")
    unassignOp := ops.Op{Type: ops.OpAssign, TargetID: "T1", Timestamp: 300, WorkerID: "w1",
        Payload: ops.Payload{AssignedTo: ""}}
    require.NoError(t, s.ApplyOp(unassignOp))

    assert.Equal(t, "", s.Issues["T1"].AssignedWorker)
}

func TestBuildIndex_IncludesAssignedWorker(t *testing.T) {
    s := materialize.NewState()
    require.NoError(t, s.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
        Payload: ops.Payload{Title: "Task 1", NodeType: "task"}}))
    require.NoError(t, s.ApplyOp(ops.Op{Type: ops.OpAssign, TargetID: "T1", Timestamp: 200, WorkerID: "w1",
        Payload: ops.Payload{AssignedTo: "worker-b"}}))

    index := s.BuildIndex()
    assert.Equal(t, "worker-b", index["T1"].AssignedWorker)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/materialize/... -run "TestApplyAssign|TestBuildIndex_IncludesAssignedWorker" -v
```
Expected: FAIL — `AssignedWorker` field and `applyAssign` don't exist yet.

- [ ] **Step 3: Add `AssignedWorker` to `Issue` and `IndexEntry` in `state.go`**

In the `Issue` struct, add after the `PR` field:
```go
AssignedWorker string `json:"assigned_worker,omitempty"`
```

In the `IndexEntry` struct, add after the `PR` field:
```go
AssignedWorker string `json:"assigned_worker,omitempty"`
```

- [ ] **Step 4: Add `applyAssign` to `engine.go` and update `ApplyOp` switch + `BuildIndex`**

In `ApplyOp`, add the assign case AND extend the no-op passthrough with `OpAssign` as a forward-compatibility measure (per spec, pre-E3 binaries that encounter `assign` ops must not hard-fail):

```go
// In ApplyOp switch:
case ops.OpAssign:
    return s.applyAssign(op)
// OpAssign is also listed here so that binaries compiled without applyAssign
// (e.g. a pre-E3 binary patched only with this forward-compat shim) tolerate it.
case ops.OpSourceLink, ops.OpSourceFingerprint, ops.OpDAGTransition:
    return nil
```

> Note: In the E3 binary, the `case ops.OpAssign` branch above will always be reached first. The no-op passthrough only matters for a hypothetical pre-E3 shim patch — the spec requires it for defense-in-depth.

Add the `applyAssign` method at the end of the file (before `appendUnique`):

```go
func (s *State) applyAssign(op ops.Op) error {
    issue, ok := s.Issues[op.TargetID]
    if !ok {
        return nil // tolerate assign ops for unknown issues (forward compat)
    }
    issue.AssignedWorker = op.Payload.AssignedTo
    issue.Updated = op.Timestamp
    return nil
}
```

In `BuildIndex`, add `AssignedWorker` mapping:

```go
func (s *State) BuildIndex() Index {
    index := make(Index, len(s.Issues))
    for id, issue := range s.Issues {
        index[id] = IndexEntry{
            // ... existing fields ...
            AssignedWorker: issue.AssignedWorker,  // add this
        }
    }
    return index
}
```

- [ ] **Step 5: Run all materialize tests**

```bash
go test ./internal/materialize/... -v
```
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/materialize/state.go internal/materialize/engine.go internal/materialize/engine_test.go
git commit -m "feat(materialize): add AssignedWorker field and applyAssign handler"
```

---

### Task 10: `cmd/trellis/assign.go` — `trls assign` / `trls unassign`

**Files:**
- Create: `cmd/trellis/assign.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Create `cmd/trellis/assign.go`**

```go
package main

import (
    "encoding/json"
    "fmt"

    "github.com/scullxbones/trellis/internal/ops"
    "github.com/spf13/cobra"
)

func newAssignCmd() *cobra.Command {
    var issueID, targetWorker string

    cmd := &cobra.Command{
        Use:   "assign",
        Short: "Assign an issue to a worker (advisory)",
        RunE: func(cmd *cobra.Command, args []string) error {
            workerID, logPath, err := resolveWorkerAndLog()
            if err != nil {
                return err
            }

            op := ops.Op{
                Type:     ops.OpAssign,
                TargetID: issueID,
                Timestamp: nowEpoch(),
                WorkerID: workerID,
                Payload:  ops.Payload{AssignedTo: targetWorker},
            }
            if err := appendHighStakesOp(logPath, op); err != nil {
                return err
            }

            result := map[string]interface{}{"issue": issueID, "assigned_to": targetWorker, "by": workerID}
            data, _ := json.Marshal(result)
            fmt.Fprintln(cmd.OutOrStdout(), string(data))
            return nil
        },
    }

    cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to assign")
    cmd.Flags().StringVar(&targetWorker, "worker", "", "worker ID to assign the issue to")
    cmd.MarkFlagRequired("issue")
    cmd.MarkFlagRequired("worker")
    return cmd
}

func newUnassignCmd() *cobra.Command {
    var issueID string

    cmd := &cobra.Command{
        Use:   "unassign",
        Short: "Remove worker assignment from an issue",
        RunE: func(cmd *cobra.Command, args []string) error {
            workerID, logPath, err := resolveWorkerAndLog()
            if err != nil {
                return err
            }

            op := ops.Op{
                Type:      ops.OpAssign,
                TargetID:  issueID,
                Timestamp: nowEpoch(),
                WorkerID:  workerID,
                Payload:   ops.Payload{AssignedTo: ""},
            }
            if err := appendHighStakesOp(logPath, op); err != nil {
                return err
            }

            result := map[string]interface{}{"issue": issueID, "unassigned": true, "by": workerID}
            data, _ := json.Marshal(result)
            fmt.Fprintln(cmd.OutOrStdout(), string(data))
            return nil
        },
    }

    cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to unassign")
    cmd.MarkFlagRequired("issue")
    return cmd
}
```

- [ ] **Step 2: Register commands in `main.go`**

```go
root.AddCommand(newAssignCmd())
root.AddCommand(newUnassignCmd())
```

- [ ] **Step 3: Build to verify compilation**

```bash
go build ./cmd/trellis/...
```
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add cmd/trellis/assign.go cmd/trellis/main.go
git commit -m "feat(cmd): add trls assign and trls unassign commands"
```

---

### Task 11: Assignment-Aware Ready Sort

**Files:**
- Modify: `internal/ready/compute.go`
- Modify: `cmd/trellis/ready.go`
- Modify: `internal/ready/compute_test.go` (if it exists, else create it)

- [ ] **Step 1: Write the failing tests**

Check if `internal/ready/compute_test.go` exists:
```bash
ls internal/ready/
```

Add tests (create or append to existing test file):

```go
package ready_test

import (
    "testing"

    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/ops"
    "github.com/scullxbones/trellis/internal/ready"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func makeReadyIndex(ids []string) materialize.Index {
    idx := make(materialize.Index)
    for _, id := range ids {
        idx[id] = materialize.IndexEntry{
            Status: ops.StatusOpen,
            Type:   "task",
        }
    }
    return idx
}

func makeReadyIssues(ids []string) map[string]*materialize.Issue {
    m := make(map[string]*materialize.Issue)
    for _, id := range ids {
        m[id] = &materialize.Issue{ID: id, Status: ops.StatusOpen}
    }
    return m
}

func TestComputeReady_AssignedToWorkerSortsFirst(t *testing.T) {
    index := materialize.Index{
        "T1": {Status: ops.StatusOpen, Type: "task", AssignedWorker: "worker-b"},
        "T2": {Status: ops.StatusOpen, Type: "task"},
        "T3": {Status: ops.StatusOpen, Type: "task", AssignedWorker: "worker-a"},
    }
    issues := map[string]*materialize.Issue{
        "T1": {ID: "T1", Status: ops.StatusOpen},
        "T2": {ID: "T2", Status: ops.StatusOpen},
        "T3": {ID: "T3", Status: ops.StatusOpen},
    }

    entries := ready.ComputeReady(index, issues, "worker-a")
    require.Len(t, entries, 3)

    // T3 (assigned to worker-a) sorts first
    assert.Equal(t, "T3", entries[0].Issue)
    // T2 (unassigned) comes before T1 (assigned to another worker)
    assert.Equal(t, "T2", entries[1].Issue)
    assert.Equal(t, "T1", entries[2].Issue)
}

func TestComputeReady_NoWorker_OtherAssignedSortsLast(t *testing.T) {
    index := materialize.Index{
        "T1": {Status: ops.StatusOpen, Type: "task", AssignedWorker: "worker-b"},
        "T2": {Status: ops.StatusOpen, Type: "task"},
    }
    issues := map[string]*materialize.Issue{
        "T1": {ID: "T1", Status: ops.StatusOpen},
        "T2": {ID: "T2", Status: ops.StatusOpen},
    }

    entries := ready.ComputeReady(index, issues, "") // no worker
    require.Len(t, entries, 2)

    // Unassigned T2 before other's-assigned T1
    assert.Equal(t, "T2", entries[0].Issue)
    assert.Equal(t, "T1", entries[1].Issue)
}

func TestComputeReady_BackwardsCompat_NoWorkerArg(t *testing.T) {
    // Callers that pass only index and issues (no workerID) should still work
    // after the signature change — workerID defaults to ""
    index := materialize.Index{
        "T1": {Status: ops.StatusOpen, Type: "task"},
    }
    issues := map[string]*materialize.Issue{
        "T1": {ID: "T1", Status: ops.StatusOpen},
    }
    entries := ready.ComputeReady(index, issues, "")
    assert.Len(t, entries, 1)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ready/... -run "TestComputeReady_Assigned" -v
```
Expected: FAIL — `ComputeReady` signature doesn't accept `workerID` yet.

- [ ] **Step 3: Update `internal/ready/compute.go`**

Change `ComputeReady` signature to accept `workerID string` as the third argument (before the variadic `now`):

```go
func ComputeReady(index materialize.Index, issues map[string]*materialize.Issue, workerID string, now ...int64) []ReadyEntry {
```

Update the call to `sortReady` at the end of `ComputeReady`:
```go
sortReady(ready, index, workerID)
```

Update `sortReady` to accept and use `workerID`:

```go
func sortReady(entries []ReadyEntry, index materialize.Index, workerID string) {
    sort.SliceStable(entries, func(i, j int) bool {
        ei := index[entries[i].Issue]
        ej := index[entries[j].Issue]

        // Issues assigned to the calling worker sort first.
        if workerID != "" {
            assignedToMe_i := ei.AssignedWorker == workerID
            assignedToMe_j := ej.AssignedWorker == workerID
            if assignedToMe_i != assignedToMe_j {
                return assignedToMe_i
            }
        }

        // Issues assigned to any other worker sort last.
        otherAssigned_i := ei.AssignedWorker != "" && ei.AssignedWorker != workerID
        otherAssigned_j := ej.AssignedWorker != "" && ej.AssignedWorker != workerID
        if otherAssigned_i != otherAssigned_j {
            return !otherAssigned_i // unassigned before other-assigned
        }

        // Existing tiebreakers.
        pi := priorityOrder[entries[i].Priority]
        pj := priorityOrder[entries[j].Priority]
        if pi != pj {
            return pi < pj
        }
        di := depth(entries[i].Issue, index)
        dj := depth(entries[j].Issue, index)
        if di != dj {
            return di > dj
        }
        bi := len(index[entries[i].Issue].Blocks)
        bj := len(index[entries[j].Issue].Blocks)
        if bi != bj {
            return bi > bj
        }
        return entries[i].Issue < entries[j].Issue
    })
}
```

- [ ] **Step 4: Update `cmd/trellis/ready.go` to pass `workerID`**

Add `--worker` flag and pass it to `ComputeReady`:

```go
func newReadyCmd() *cobra.Command {
    var workerID string

    cmd := &cobra.Command{
        Use:   "ready",
        Short: "Show tasks ready to be claimed",
        RunE: func(cmd *cobra.Command, args []string) error {
            // ... existing materialize + load code ...

            entries := ready.ComputeReady(index, issues, workerID)

            // ... existing format output code ...
        },
    }

    cmd.Flags().StringVar(&workerID, "worker", "", "show assignment-aware ordering for this worker ID")
    return cmd
}
```

- [ ] **Step 5: Fix any other callers of `ComputeReady`**

Search for all callers:
```bash
grep -r "ComputeReady" --include="*.go" .
```

Update each caller to pass `""` as the `workerID` argument (or a real workerID where appropriate).

- [ ] **Step 6: Run all tests**

```bash
go test ./internal/ready/... ./cmd/trellis/... -v
```
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/ready/compute.go cmd/trellis/ready.go
git commit -m "feat(ready): assignment-aware sort — assigned issues first, other-assigned last"
```

---

### Task 12: `cmd/trellis/workers.go` — `trls workers` command

**Files:**
- Create: `cmd/trellis/workers.go`
- Modify: `cmd/trellis/main.go`

The workers command reads materialized state (index + issues) to determine claim status, and scans op log filenames to enumerate all known workers.

> **Dependency:** This task requires Task 9's `AssignedWorker` field to be present in `IndexEntry`. Implement Task 9 before creating this file.

- [ ] **Step 1: Create `cmd/trellis/workers.go`**

```go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
    "sort"
    "strings"
    "time"

    claimPkg "github.com/scullxbones/trellis/internal/claim"
    "github.com/scullxbones/trellis/internal/materialize"
    "github.com/scullxbones/trellis/internal/ops"
    "github.com/spf13/cobra"
)

type workerStatus struct {
    WorkerID            string   `json:"worker_id"`
    Status              string   `json:"status"`
    ClaimedIssue        string   `json:"claimed_issue,omitempty"`
    TTLRemainingSeconds int64    `json:"ttl_remaining_seconds,omitempty"`
    LastHeartbeatEpoch  int64    `json:"last_heartbeat_epoch,omitempty"`
    AssignedIssues      []string `json:"assigned_issues"`
}

func newWorkersCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "workers",
        Short: "Show worker presence and claim status",
        RunE: func(cmd *cobra.Command, args []string) error {
            issuesDir := appCtx.IssuesDir

            if _, err := materialize.Materialize(issuesDir, appCtx.Mode == "single-branch"); err != nil {
                return fmt.Errorf("materialize: %w", err)
            }

            index, err := materialize.LoadIndex(issuesDir + "/state/index.json")
            if err != nil {
                return err
            }

            issues := make(map[string]*materialize.Issue)
            for id := range index {
                issue, errL := materialize.LoadIssue(fmt.Sprintf("%s/state/issues/%s.json", issuesDir, id))
                if errL == nil {
                    issues[id] = &issue
                }
            }

            now := time.Now().Unix()
            inactivitySecs := int64(appCtx.Config.DefaultTTL) * 2 * 60

            workerIDs := enumerateWorkers(issuesDir + "/ops")
            lastOpTimes := make(map[string]int64, len(workerIDs))
            for _, wid := range workerIDs {
                logPath := fmt.Sprintf("%s/ops/%s.log", issuesDir, wid)
                lastOpTimes[wid] = lastOpTimestampFromLog(logPath)
            }

            // Assigned issues per worker (from index.AssignedWorker).
            assignedByWorker := make(map[string][]string)
            for id, entry := range index {
                if entry.AssignedWorker != "" {
                    assignedByWorker[entry.AssignedWorker] = append(assignedByWorker[entry.AssignedWorker], id)
                }
            }

            var active, idle, stale []workerStatus
            for _, workerID := range workerIDs {
                ws := buildWorkerStatus(workerID, index, issues, assignedByWorker[workerID],
                    lastOpTimes[workerID], now, inactivitySecs)
                if ws == nil {
                    continue
                }
                switch ws.Status {
                case "active":
                    active = append(active, *ws)
                case "idle":
                    idle = append(idle, *ws)
                default:
                    stale = append(stale, *ws)
                }
            }

            format, _ := cmd.Flags().GetString("format")
            if format == "json" || format == "agent" {
                all := append(append(active, idle...), stale...)
                data, _ := json.MarshalIndent(all, "", "  ")
                fmt.Fprintln(cmd.OutOrStdout(), string(data))
                return nil
            }

            printWorkerSection(cmd, "active", active, now)
            printWorkerSection(cmd, "idle", idle, now)
            printWorkerSection(cmd, "stale", stale, now)
            return nil
        },
    }
    return cmd
}

func enumerateWorkers(opsDir string) []string {
    entries, err := os.ReadDir(opsDir)
    if err != nil {
        return nil
    }
    var workers []string
    for _, e := range entries {
        if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
            workers = append(workers, strings.TrimSuffix(e.Name(), ".log"))
        }
    }
    sort.Strings(workers)
    return workers
}

// lastOpTimestampFromLog reads the last JSONL line of a log file and extracts
// the timestamp (position 2 in the array). Returns 0 on any error.
func lastOpTimestampFromLog(logPath string) int64 {
    f, err := os.Open(logPath)
    if err != nil {
        return 0
    }
    defer f.Close()

    var lastLine []byte
    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        if b := scanner.Bytes(); len(b) > 0 {
            lastLine = append([]byte(nil), b...)
        }
    }
    if len(lastLine) == 0 {
        return 0
    }
    op, err := ops.ParseLine(lastLine)
    if err != nil {
        return 0
    }
    return op.Timestamp
}

func buildWorkerStatus(workerID string, index materialize.Index,
    issues map[string]*materialize.Issue, assignedIDs []string,
    lastOpTime, now, inactivitySecs int64) *workerStatus {

    if assignedIDs == nil {
        assignedIDs = []string{}
    }

    // Find live and stale claims for this worker.
    var liveClaim, staleClaim *materialize.Issue
    for id, entry := range index {
        if entry.Assignee != workerID {
            continue
        }
        issue, ok := issues[id]
        if !ok {
            continue
        }
        if !claimPkg.IsClaimStale(issue.ClaimedAt, issue.LastHeartbeat, issue.ClaimTTL, now) {
            liveClaim = issue
            break
        }
        if staleClaim == nil {
            staleClaim = issue
        }
    }

    if liveClaim != nil {
        ws := &workerStatus{
            WorkerID:           workerID,
            Status:             "active",
            ClaimedIssue:       liveClaim.ID,
            LastHeartbeatEpoch: liveClaim.LastHeartbeat,
            AssignedIssues:     assignedIDs,
        }
        if liveClaim.ClaimTTL > 0 {
            lastActivity := liveClaim.ClaimedAt
            if liveClaim.LastHeartbeat > lastActivity {
                lastActivity = liveClaim.LastHeartbeat
            }
            ws.TTLRemainingSeconds = (lastActivity + int64(liveClaim.ClaimTTL)*60) - now
        }
        return ws
    }

    if staleClaim != nil {
        return &workerStatus{
            WorkerID:           workerID,
            Status:             "stale",
            ClaimedIssue:       staleClaim.ID,
            LastHeartbeatEpoch: staleClaim.LastHeartbeat,
            AssignedIssues:     assignedIDs,
        }
    }

    // No claims. Use lastOpTime for idle/gone determination.
    baseline := lastOpTime
    if baseline == 0 || now-baseline > inactivitySecs {
        return nil // beyond window — omit
    }
    return &workerStatus{
        WorkerID:       workerID,
        Status:         "idle",
        AssignedIssues: assignedIDs,
    }
}

func printWorkerSection(cmd *cobra.Command, heading string, workers []workerStatus, now int64) {
    if len(workers) == 0 {
        return
    }
    fmt.Fprintf(cmd.OutOrStdout(), "\n=== %s ===\n", heading)
    for _, w := range workers {
        switch w.Status {
        case "active":
            hbAgo := (now - w.LastHeartbeatEpoch) / 60
            ttlLine := ""
            if w.TTLRemainingSeconds > 0 {
                ttlLine = fmt.Sprintf(", TTL %dm remaining", w.TTLRemainingSeconds/60)
            }
            fmt.Fprintf(cmd.OutOrStdout(), "  %-20s  %s (heartbeat %dm ago%s)\n",
                w.WorkerID, w.ClaimedIssue, hbAgo, ttlLine)
        case "stale":
            hbAgo := (now - w.LastHeartbeatEpoch) / 60
            fmt.Fprintf(cmd.OutOrStdout(), "  %-20s  %s (stale — heartbeat %dm ago)\n",
                w.WorkerID, w.ClaimedIssue, hbAgo)
        case "idle":
            assignedStr := ""
            if len(w.AssignedIssues) > 0 {
                assignedStr = fmt.Sprintf("\n%s  assigned: %s (unclaimed)",
                    strings.Repeat(" ", 22), strings.Join(w.AssignedIssues, ", "))
            }
            fmt.Fprintf(cmd.OutOrStdout(), "  %-20s  (no active claim)%s\n",
                w.WorkerID, assignedStr)
        }
    }
}
```

- [ ] **Step 2: Register `newWorkersCmd` in `main.go`**

```go
root.AddCommand(newWorkersCmd())
```

- [ ] **Step 3: Build to verify compilation**

```bash
go build ./cmd/trellis/...
```
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add cmd/trellis/workers.go cmd/trellis/main.go
git commit -m "feat(cmd): add trls workers command for worker presence visibility"
```

---

### Task 13: Final Integration — Build, Test, and Register All Commands

**Files:**
- Modify: `cmd/trellis/main.go` (verify all commands registered)

- [ ] **Step 1: Verify all new commands are registered in `main.go`**

The following should all appear in `newRootCmd`:
```go
root.AddCommand(newLogCmd())      // E3-003
root.AddCommand(newAssignCmd())   // E3-004
root.AddCommand(newUnassignCmd()) // E3-004
root.AddCommand(newWorkersCmd())  // E3-002
```

- [ ] **Step 2: Run the full test suite**

```bash
go test ./... -v 2>&1 | tail -40
```
Expected: all PASS

- [ ] **Step 3: Build the binary**

```bash
go build -o bin/trls ./cmd/trellis/
```
Expected: binary produced with no errors.

- [ ] **Step 4: Smoke test — help output shows all new commands**

```bash
./bin/trls --help
./bin/trls log --help
./bin/trls assign --help
./bin/trls unassign --help
./bin/trls workers --help
```
Expected: all commands visible with correct flags.

- [ ] **Step 5: Commit**

```bash
git add cmd/trellis/main.go
git commit -m "feat(e3): complete E3 integration — all commands registered and tested"
```

---

## Implementation Order Summary

The four E3 issues must ship atomically. Recommended parallel execution:

| Stream A (E3-001) | Stream B (E3-003) |
|---|---|
| Task 1: Config | Task 6: audit.go (no dependency on Task 1–5) |
| Task 2: git Push/FetchAndRebase | Task 7: trls log (without OpAssign case — added in Task 8) |
| Task 3: Pusher + AppendCommitAndPush | |
| Task 4: FilePushTracker | |
| Task 5: Wire helpers + commands | |

After both streams complete:

| Stream (E3-002 + E3-004) |
|---|
| Task 8: OpAssign type registration |
| Task 9: Materialization AssignedWorker |
| Task 10: trls assign/unassign |
| Task 11: Assignment-aware ready sort |
| Task 12: trls workers |
| Task 13: Final integration |
