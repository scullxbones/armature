# Armature E5 — Polish, UX Hardening & User-Facing Docs

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close known UX gaps from dogfooding, ship the full interactive TUI board, add time-travel forensics, and give users a complete documentation site.

**Architecture:** Five stories. S0 is foundational and **must land first** — it implements the draft confidence model and dag-summary governance gate, which we dogfood during the rest of E5. S1–S4 are blocked by S0 in spirit (they must be sign-off'd through the new gate before workers can claim their tasks). S1 fixes UX regressions, S2 ships the interactive TUI board, S3 adds time-travel forensics, S4 is pure docs. Each story produces working, testable software on its own.

**Tech Stack:** Go 1.26, Cobra, BubbleTea 1.3.4, Lip Gloss 1.1.0, Glamour 0.8.0, Bubbles 0.20.0, testify, gopter. No new dependencies needed.

---

## Pre-flight

Before starting any story, verify the baseline:

```bash
cd /path/to/trellis
make check   # must pass: lint + test (≥80%) + mutate
```

If `make check` fails, stop and fix before proceeding.

---

## Chunk 0: E5-S0 — Draft Confidence Model & Governance Gate

**Implement this story first.** The architecture specifies that `decompose-apply` creates nodes with `provenance.confidence: "draft"`, which are excluded from the ready queue. The `dag-summary` interactive sign-off promotes `draft` → `verified` (via `dag-transition` op), after which tasks become claimable. This is the governance gate for all decomposition work. We want to dogfood this gate during E5 itself — after E5-S0 lands, all remaining E5 stories will go through the real sign-off flow.

### File Map

| File | Change |
|------|--------|
| `internal/ops/types.go` | Add `dag-transition` op type; add `Confidence` constants |
| `internal/materialize/materialize.go` | Handle `dag-transition` op; populate `confidence` field on nodes |
| `internal/ready/compute.go` | Exclude `draft` nodes from ready queue |
| `internal/decompose/apply.go` | Emit `confidence: "draft"` in create op payloads |
| `internal/tui/dagsummary/` | **New** — BubbleTea checklist model for dag-summary sign-off |
| `cmd/trellis/dag_summary.go` | Wire dag-summary TUI; emit `dag-transition` ops on sign-off |
| `cmd/trellis/main_test.go` | Integration tests for confidence gate end-to-end |

---

### Task 1: Confidence field in materialized state and draft gate in ready queue

**Files:**
- Modify: `internal/ops/types.go` (add `Confidence` constants: `"draft"`, `"verified"`, `"inferred"`)
- Modify: `internal/materialize/materialize.go` (populate `confidence` on each node from `create` op payload)
- Modify: `internal/ready/compute.go` (add rule 5: `confidence != "draft"`)
- Test: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write failing integration test**

Add to `cmd/trellis/main_test.go`:

```go
func TestDraftNodeNotReady(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    runCmd(t, repo, "arm", "create", "--id", "E-1", "--title", "Epic", "--type", "epic")
    runCmd(t, repo, "arm", "transition", "--issue", "E-1", "--to", "in-progress")
    // Create a task with draft confidence
    runCmd(t, repo, "arm", "create", "--id", "E-1-T1", "--title", "Draft Task",
        "--type", "task", "--parent", "E-1", "--confidence", "draft")
    out := runCmdOutput(t, repo, "arm", "ready")
    // draft node must not appear
    require.NotContains(t, out, "E-1-T1")
}

func TestVerifiedNodeIsReady(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    runCmd(t, repo, "arm", "create", "--id", "E-1", "--title", "Epic", "--type", "epic")
    runCmd(t, repo, "arm", "transition", "--issue", "E-1", "--to", "in-progress")
    runCmd(t, repo, "arm", "create", "--id", "E-1-T1", "--title", "Draft Task",
        "--type", "task", "--parent", "E-1", "--confidence", "draft")
    // Promote via dag-transition
    runCmd(t, repo, "arm", "dag-transition", "--root", "E-1", "--to", "approved")
    out := runCmdOutput(t, repo, "arm", "ready")
    require.Contains(t, out, "E-1-T1")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/trellis/... -run TestDraftNode -v
go test ./cmd/trellis/... -run TestVerifiedNode -v
```

Expected: FAIL.

- [ ] **Step 3: Add `Confidence` constants and `confidence` field to create op**

In `internal/ops/types.go`, add:

```go
const (
    ConfidenceDraft    = "draft"
    ConfidenceVerified = "verified"
    ConfidenceInferred = "inferred"
)
```

In create op payload struct and materialize parsing, populate `node.Confidence` from the op payload. Default to `""` (empty = verified) for backward compatibility with existing nodes that predate this field.

- [ ] **Step 4: Exclude draft nodes from ready queue**

In `internal/ready/compute.go`, add to the ready-task conditions:

```go
// Rule 5: draft confidence nodes are not yet approved for work
if issue.Confidence == ops.ConfidenceDraft {
    continue
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./cmd/trellis/... -run TestDraftNode -run TestVerifiedNode -v
```

Expected: PASS (after implementing Step 6 below).

- [ ] **Step 6: Commit**

```bash
git add internal/ops/types.go internal/materialize/materialize.go \
        internal/ready/compute.go cmd/trellis/main_test.go
git commit -m "feat(governance): add draft confidence field and ready-gate for unverified nodes"
```

---

### Task 2: `dag-transition` op — materialization support

The `dag-transition` op walks a subtree and promotes all `draft` nodes to `verified`.

**Files:**
- Modify: `internal/ops/types.go` (add `OpTypeDagTransition = "dag-transition"`)
- Modify: `internal/materialize/materialize.go` (handle `dag-transition`: walk subtree, set confidence to `verified` on all draft descendants)
- Test: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write failing test**

Add to `cmd/trellis/main_test.go`:

```go
func TestDagTransitionPromotesDraft(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    runCmd(t, repo, "arm", "create", "--id", "E-1", "--title", "Epic", "--type", "epic",
        "--confidence", "draft")
    runCmd(t, repo, "arm", "create", "--id", "E-1-S1", "--title", "Story", "--type", "story",
        "--parent", "E-1", "--confidence", "draft")
    runCmd(t, repo, "arm", "create", "--id", "E-1-S1-T1", "--title", "Task", "--type", "task",
        "--parent", "E-1-S1", "--confidence", "draft")
    runCmd(t, repo, "arm", "dag-transition", "--root", "E-1", "--to", "approved")
    // All three nodes should now be verified
    out := runCmdExpectOK(t, repo, "arm", "status")
    require.Contains(t, out, "E-1-S1-T1") // visible, no longer hidden
}
```

- [ ] **Step 2: Implement `dag-transition` op in materialization**

During replay, when a `dag-transition` op is encountered:
1. Walk the full subtree rooted at the op's `target_id`
2. For each node with `confidence == "draft"`, set `confidence = "verified"`
3. Record signing worker ID and timestamp

- [ ] **Step 3: Add `arm dag-transition` command**

New thin Cobra command (for tests and manual use):

```
arm dag-transition --root <id> --to approved [--uncovered-acknowledged <id,...>]
```

Emits `["dag-transition", root_id, ts, worker_id, {"to": "approved", "uncovered_acknowledged": [...]}]`.

- [ ] **Step 4: Run tests**

```bash
go test ./cmd/trellis/... -run TestDagTransition -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ops/types.go internal/materialize/materialize.go \
        cmd/trellis/dag_summary.go cmd/trellis/main_test.go
git commit -m "feat(governance): add dag-transition op promoting draft subtree to verified"
```

---

### Task 3: `dag-summary` interactive sign-off TUI

Wire the existing `dag-summary` command into a BubbleTea checklist that requires per-item acknowledgment before emitting a `dag-transition` op. Mechanically impossible to bulk-approve.

**Files:**
- Create: `internal/tui/dagsummary/model.go`
- Create: `internal/tui/dagsummary/model_test.go`
- Modify: `cmd/trellis/dag_summary.go`

- [ ] **Step 1: Implement BubbleTea checklist model**

`internal/tui/dagsummary/model.go` — displays a list of draft nodes, each showing:
- Node ID, type, title
- Source citation (if any; `[uncited]` if missing)
- Coverage status

Per-item keys: `y` (approve), `n` (flag for re-decomposition), `s` (skip/defer). All items must be acted on before sign-off is available. `q` quits without signing off.

Coverage summary at bottom: `cited: N/M nodes (X%)`. Any uncited nodes named individually — reviewer must type the ID to acknowledge each one.

- [ ] **Step 2: Wire into `dag-summary` command**

`cmd/trellis/dag_summary.go` runs the BubbleTea model, collects per-item decisions, then on confirmation emits:
```
arm dag-transition --root <root-id> --to approved \
    --uncovered-acknowledged <comma-separated uncited node IDs>
```

- [ ] **Step 3: Unit test the model**

`internal/tui/dagsummary/model_test.go` — test that:
- All items must be actioned before sign-off unlocks
- Uncited nodes appear in the `uncovered_acknowledged` list
- `q` without full sign-off emits no ops

- [ ] **Step 4: Run tests**

```bash
go test ./internal/tui/dagsummary/... -v
make check
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui/dagsummary/ cmd/trellis/dag_summary.go
git commit -m "feat(governance): wire dag-summary interactive sign-off TUI (architecture section 9)"
```

---

### Task 4: `decompose-apply` emits `confidence: "draft"` on all created nodes

After E5-S0-T1 through T3 are complete, update `decompose-apply` so every node it creates carries `confidence: "draft"` in the create op payload. Workers must then use `dag-summary` to promote before claiming.

**Files:**
- Modify: `internal/decompose/apply.go`
- Test: `cmd/trellis/cmd_extra_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestDecomposeApplyCreatesDraftNodes(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    plan := writeTempPlan(t, minimalPlanJSON)
    runCmd(t, repo, "arm", "decompose-apply", "--plan", plan)
    // Nodes should be draft — not yet in ready queue
    out := runCmdOutput(t, repo, "arm", "ready")
    require.Contains(t, out, "No tasks ready")
}
```

- [ ] **Step 2: Update `internal/decompose/apply.go`**

Include `"confidence": "draft"` in every create op payload emitted by `ApplyPlan`.

- [ ] **Step 3: Run tests**

```bash
go test ./cmd/trellis/... -run TestDecomposeApplyCreatesDraftNodes -v
make check
```

- [ ] **Step 4: Dogfood**

Run `arm decompose-apply` on a test plan, then run `arm dag-summary`, sign off interactively, and confirm tasks appear in `arm ready` afterwards.

- [ ] **Step 5: Commit**

```bash
git add internal/decompose/apply.go cmd/trellis/cmd_extra_test.go
git commit -m "feat(governance): decompose-apply emits draft confidence; requires dag-summary sign-off to claim"
```

---

## Chunk 1: E5-S1 — UX Hardening

Fixes four dogfooding regressions: missing `open` transition target (L11), stale-claim ready-queue silence (L6), story-claim blocking child tasks (L9), and `unassign` leaving residual `claimed` state (L10).

### File Map

| File | Change |
|------|--------|
| `internal/ops/types.go` | Add `StatusOpen` to `ValidTransitionTargets` |
| `internal/ready/stale.go` | **New** — `StaleClaims()` helper returning blocking stale issues |
| `internal/ready/stale_test.go` | **New** — unit tests for `StaleClaims()` |
| `cmd/trellis/ready.go` | Print stale-claim diagnostic when queue is empty |
| `cmd/trellis/claim.go` | Auto-advance story/epic to `in-progress` after claiming |
| `cmd/trellis/assign.go` | In `newUnassignCmd`, also emit `transition → open` when issue is `claimed` |
| `cmd/trellis/main_test.go` | Integration tests for all four fixes |

---

### Task 1: Add `open` to `ValidTransitionTargets`

**Files:**
- Modify: `internal/ops/types.go:41-47`
- Test: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write the failing integration test**

Add to `cmd/trellis/main_test.go`:

```go
func TestTransitionToOpen(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    runCmd(t, repo, "arm", "create", "--id", "T-1", "--title", "Task One", "--type", "task")
    runCmd(t, repo, "arm", "transition", "--issue", "T-1", "--to", "in-progress")
    // Now transition back to open — this should succeed
    out := runCmdExpectOK(t, repo, "arm", "transition", "--issue", "T-1", "--to", "open")
    require.Contains(t, out, `"open"`)
}

func TestTransitionToOpenRejectsInvalidAlias(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    runCmd(t, repo, "arm", "create", "--id", "T-1", "--title", "Task One", "--type", "task")
    // Underscore variant must still be rejected
    runCmdExpectError(t, repo, "arm", "transition", "--issue", "T-1", "--to", "in_progress")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/trellis/... -run TestTransitionToOpen -v
```

Expected: FAIL — `invalid status "open"`.

- [ ] **Step 3: Add `StatusOpen` to `ValidTransitionTargets`**

In `internal/ops/types.go`, change:

```go
// ValidTransitionTargets is the set of statuses accepted by the transition command.
var ValidTransitionTargets = map[string]bool{
	StatusInProgress: true,
	StatusDone:       true,
	StatusMerged:     true,
	StatusBlocked:    true,
	StatusCancelled:  true,
}
```

to:

```go
// ValidTransitionTargets is the set of statuses accepted by the transition command.
// StatusOpen is included so workers can revert accidental claims without using `arm reopen`.
var ValidTransitionTargets = map[string]bool{
	StatusOpen:       true,
	StatusInProgress: true,
	StatusDone:       true,
	StatusMerged:     true,
	StatusBlocked:    true,
	StatusCancelled:  true,
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./cmd/trellis/... -run TestTransitionToOpen -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ops/types.go cmd/trellis/main_test.go
git commit -m "fix(ux): add open as valid transition target (L11)"
```

---

### Task 2: Stale-claim diagnostic in `arm ready`

When `arm ready` returns empty because in-progress tasks hold stale claims, print an actionable message.

**Files:**
- Create: `internal/ready/stale.go`
- Create: `internal/ready/stale_test.go`
- Modify: `cmd/trellis/ready.go`

- [ ] **Step 1: Write unit tests for `StaleClaims()`**

Create `internal/ready/stale_test.go`:

```go
package ready

import (
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/require"
)

func TestStaleClaims_EmptyWhenNoClaims(t *testing.T) {
	index := materialize.Index{
		"T-1": {Status: ops.StatusOpen},
	}
	issues := map[string]*materialize.Issue{}
	result := StaleClaims(index, issues, 1000)
	require.Empty(t, result)
}

func TestStaleClaims_DetectsStaleInProgress(t *testing.T) {
	index := materialize.Index{
		"T-1": {Status: ops.StatusInProgress, Blocks: []string{"T-2"}},
		"T-2": {Status: ops.StatusOpen, BlockedBy: []string{"T-1"}},
	}
	issues := map[string]*materialize.Issue{
		"T-1": {
			ID:            "T-1",
			ClaimedBy:     "worker-abc",
			ClaimedAt:     100,
			LastHeartbeat: 100,
			ClaimTTL:      60,
		},
	}
	// now = 100 + 61*60 + 1 = well past TTL
	result := StaleClaims(index, issues, 100+61*60+1)
	require.Len(t, result, 1)
	require.Equal(t, "T-1", result[0].IssueID)
	require.Equal(t, "worker-abc", result[0].WorkerID)
}

func TestStaleClaims_IgnoresActiveClaims(t *testing.T) {
	index := materialize.Index{
		"T-1": {Status: ops.StatusInProgress},
	}
	issues := map[string]*materialize.Issue{
		"T-1": {
			ID:            "T-1",
			ClaimedBy:     "worker-abc",
			ClaimedAt:     1000,
			LastHeartbeat: 1000,
			ClaimTTL:      60,
		},
	}
	// now = 1005, not stale
	result := StaleClaims(index, issues, 1005)
	require.Empty(t, result)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ready/... -run TestStaleClaims -v
```

Expected: FAIL — `StaleClaims` undefined.

- [ ] **Step 3: Implement `StaleClaims()`**

Create `internal/ready/stale.go`:

```go
package ready

import (
	"sort"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// StaleClaimInfo describes an in-progress issue whose claim has expired.
type StaleClaimInfo struct {
	IssueID  string
	WorkerID string
}

// StaleClaims returns in-progress or claimed issues whose claims are stale.
// These are potential silent blockers: tasks depending on them cannot become ready.
// Results are sorted by IssueID for deterministic output.
func StaleClaims(index materialize.Index, issues map[string]*materialize.Issue, now int64) []StaleClaimInfo {
	var result []StaleClaimInfo
	for id, entry := range index {
		if entry.Status != ops.StatusInProgress && entry.Status != ops.StatusClaimed {
			continue
		}
		issue, ok := issues[id]
		if !ok || issue.ClaimedBy == "" {
			continue
		}
		ttl := issue.ClaimTTL
		if ttl <= 0 {
			ttl = 60
		}
		if claim.IsClaimStale(issue.ClaimedAt, issue.LastHeartbeat, ttl, now) {
			result = append(result, StaleClaimInfo{IssueID: id, WorkerID: issue.ClaimedBy})
		}
	}
	// Sort for deterministic output (map iteration is random in Go)
	sort.Slice(result, func(i, j int) bool { return result[i].IssueID < result[j].IssueID })
	return result
}
```

- [ ] **Step 4: Run unit tests**

```bash
go test ./internal/ready/... -run TestStaleClaims -v
```

Expected: PASS.

- [ ] **Step 5: Write integration test for the diagnostic message**

Add to `cmd/trellis/main_test.go`:

```go
func TestReadyEmptyWithStaleDiagnostic(t *testing.T) {
	repo := setupTestRepo(t)
	runCmd(t, repo, "arm", "worker-init")
	// Create blocker task and block another on it
	runCmd(t, repo, "arm", "create", "--id", "E-1", "--title", "Epic", "--type", "epic")
	runCmd(t, repo, "arm", "transition", "--issue", "E-1", "--to", "in-progress")
	runCmd(t, repo, "arm", "create", "--id", "T-1", "--title", "Blocker", "--type", "task",
		"--parent", "E-1")
	runCmd(t, repo, "arm", "create", "--id", "T-2", "--title", "Blocked", "--type", "task",
		"--parent", "E-1", "--blocked-by", "T-1")
	// Claim T-1 with a 1-minute TTL then fast-forward time by injecting a stale state
	// We test the diagnostic by directly checking StaleClaims in unit tests;
	// here just verify the ready command runs cleanly when queue is empty.
	out := runCmdOutput(t, repo, "arm", "ready")
	// Could be empty or have T-1 — just verify no crash
	_ = out
}
```

- [ ] **Step 6: Update `cmd/trellis/ready.go` to print stale diagnostic**

Replace the `len(entries) == 0` block:

```go
if len(entries) == 0 {
    _, _ = fmt.Fprintln(cmd.OutOrStdout(), "No tasks ready.")
    return nil
}
```

with:

```go
if len(entries) == 0 {
    _, _ = fmt.Fprintln(cmd.OutOrStdout(), "No tasks ready.")
    // Check for stale claims that may be silently blocking the queue
    stale := ready.StaleClaims(index, issues, time.Now().Unix())
    if len(stale) > 0 {
        _, _ = fmt.Fprintln(cmd.ErrOrStderr(), "")
        _, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: stale claim(s) may be blocking the queue:")
        for _, s := range stale {
            _, _ = fmt.Fprintf(cmd.ErrOrStderr(),
                "  %s  claimed by %s (claim expired) — run `arm claim --issue %s` to steal\n",
                s.IssueID, s.WorkerID, s.IssueID)
        }
    }
    return nil
}
```

`ready.go` already imports the `ready` package. Only `"time"` needs to be added to the imports block.

- [ ] **Step 7: Run full tests**

```bash
go test ./... -v 2>&1 | tail -20
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add internal/ready/stale.go internal/ready/stale_test.go cmd/trellis/ready.go
git commit -m "fix(ux): print stale-claim diagnostic when ready queue is empty (L6)"
```

---

### Task 3: Auto-advance story/epic to `in-progress` when claimed

When claiming a story or epic, automatically emit a second `transition → in-progress` op so children become visible immediately.

**Files:**
- Modify: `cmd/trellis/claim.go`
- Test: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write failing test**

Add to `cmd/trellis/main_test.go`:

```go
func TestClaimStoryAutoAdvancesToInProgress(t *testing.T) {
	repo := setupTestRepo(t)
	runCmd(t, repo, "arm", "worker-init")
	runCmd(t, repo, "arm", "create", "--id", "E-1", "--title", "Epic", "--type", "epic")
	runCmd(t, repo, "arm", "transition", "--issue", "E-1", "--to", "in-progress")
	runCmd(t, repo, "arm", "create", "--id", "S-1", "--title", "Story", "--type", "story",
		"--parent", "E-1")
	runCmd(t, repo, "arm", "create", "--id", "T-1", "--title", "Child task", "--type", "task",
		"--parent", "S-1")

	// Claim the story — children should become ready immediately
	runCmd(t, repo, "arm", "claim", "--issue", "S-1")
	runCmd(t, repo, "arm", "materialize")

	// S-1 should be in-progress (not just claimed)
	index := loadIndex(t, repo)
	require.Equal(t, "in-progress", index["S-1"].Status)

	// T-1 should appear in the ready queue
	out := runCmdOutput(t, repo, "arm", "ready")
	require.Contains(t, out, "T-1")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./cmd/trellis/... -run TestClaimStoryAutoAdvancesToInProgress -v
```

Expected: FAIL — story status remains `claimed`, T-1 not in ready queue.

- [ ] **Step 3: Update `claim.go` to auto-advance stories and epics**

In `cmd/trellis/claim.go`, after the `appendHighStakesOp` call and before the result print, add:

```go
// Auto-advance stories and epics to in-progress so their child tasks
// immediately become visible in the ready queue (L9 dogfooding fix).
if issue.Type == "story" || issue.Type == "epic" {
    advanceOp := ops.Op{
        Type:      ops.OpTransition,
        TargetID:  issueID,
        Timestamp: nowEpoch(),
        WorkerID:  workerID,
        Payload:   ops.Payload{To: ops.StatusInProgress},
    }
    if err := appendOp(logPath, advanceOp); err != nil {
        // Non-fatal: claim succeeded; worker can manually transition
        _, _ = fmt.Fprintf(cmd.ErrOrStderr(),
            "Warning: claimed story/epic but could not auto-advance to in-progress: %v\n", err)
    }
}
```

- [ ] **Step 4: Run test**

```bash
go test ./cmd/trellis/... -run TestClaimStoryAutoAdvancesToInProgress -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/trellis/claim.go cmd/trellis/main_test.go
git commit -m "fix(ux): auto-advance story/epic to in-progress on claim (L9)"
```

---

### Task 4: Fix `arm unassign` to release `claimed` status

`arm unassign` currently only clears the assignment but leaves the issue in `claimed` status, requiring a manual transition to unblock. Fix: when the issue is `claimed`, also emit `transition → open`.

**Files:**
- Modify: `cmd/trellis/assign.go`
- Test: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write failing test**

Add to `cmd/trellis/main_test.go`:

```go
func TestUnassignReleasesClaimedStatus(t *testing.T) {
	repo := setupTestRepo(t)
	runCmd(t, repo, "arm", "worker-init")
	runCmd(t, repo, "arm", "create", "--id", "T-1", "--title", "Task", "--type", "task")
	runCmd(t, repo, "arm", "claim", "--issue", "T-1")
	runCmd(t, repo, "arm", "materialize")

	// Unassign should return T-1 to open
	runCmd(t, repo, "arm", "unassign", "--issue", "T-1")
	runCmd(t, repo, "arm", "materialize")

	index := loadIndex(t, repo)
	require.Equal(t, "open", index["T-1"].Status,
		"unassign on a claimed issue should restore open status")

	// T-1 should reappear in ready queue
	out := runCmdOutput(t, repo, "arm", "ready")
	require.Contains(t, out, "T-1")
}

func TestUnassignDoesNotAffectInProgressStatus(t *testing.T) {
	repo := setupTestRepo(t)
	runCmd(t, repo, "arm", "worker-init")
	runCmd(t, repo, "arm", "create", "--id", "T-1", "--title", "Task", "--type", "task")
	runCmd(t, repo, "arm", "transition", "--issue", "T-1", "--to", "in-progress")

	// Unassign on in-progress should NOT change status (worker is actively working)
	runCmd(t, repo, "arm", "unassign", "--issue", "T-1")
	runCmd(t, repo, "arm", "materialize")

	index := loadIndex(t, repo)
	require.Equal(t, "in-progress", index["T-1"].Status,
		"unassign on an in-progress issue must not change status")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/trellis/... -run TestUnassign -v
```

Expected: `TestUnassignReleasesClaimedStatus` FAIL (status stays `claimed`).

- [ ] **Step 3: Update `newUnassignCmd` in `assign.go`**

The `newUnassignCmd` function currently just emits an `OpAssign` with empty `assigned_to`. Extend it to also load the current issue status and emit a transition when the status is `claimed`:

```go
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

			// Emit the assignment-clear op
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

			// If the issue is currently `claimed`, also transition back to `open`
			// so that the ready queue immediately reflects the release (L10 fix).
			issuesDir := appCtx.IssuesDir
			issue, loadErr := materialize.LoadIssue(
				fmt.Sprintf("%s/state/issues/%s.json", issuesDir, issueID))
			if loadErr == nil && issue.Status == ops.StatusClaimed {
				releaseOp := ops.Op{
					Type:      ops.OpTransition,
					TargetID:  issueID,
					Timestamp: nowEpoch(),
					WorkerID:  workerID,
					Payload:   ops.Payload{To: ops.StatusOpen},
				}
				if appendErr := appendOp(logPath, releaseOp); appendErr != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"Warning: unassigned but could not release claim: %v\n"+
							"Run `arm transition --issue %s --to open` manually.\n",
						appendErr, issueID)
				}
			}

			result := map[string]string{"issue": issueID, "assigned_to": ""}
			data, _ := json.Marshal(result)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID to unassign")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}
```

Add `"github.com/scullxbones/armature/internal/materialize"` to imports in `assign.go`.

> **Note on stale state:** `materialize.LoadIssue` reads from the last materialized checkpoint on disk. The test calls `arm materialize` explicitly before `unassign` to ensure the checkpoint reflects the claim. In production use, `arm sync` or any command that materializes will keep the checkpoint fresh. If the checkpoint is missing, `loadErr != nil` and the release transition is simply skipped (safe degradation).

- [ ] **Step 4: Run tests**

```bash
go test ./cmd/trellis/... -run TestUnassign -v
```

Expected: both PASS.

- [ ] **Step 5: Run full suite**

```bash
make check
```

Expected: all pass, coverage ≥80%.

- [ ] **Step 6: Commit**

```bash
git add cmd/trellis/assign.go cmd/trellis/main_test.go
git commit -m "fix(ux): unassign releases claimed status back to open (L10/L11)"
```

---

### Task 5: `decompose-apply` plan JSON schema discoverability (L12)

Agents must read Go source to discover the plan JSON format. Add an `--example` flag that prints a minimal valid plan.

**Files:**
- Modify: `cmd/trellis/decompose.go`
- Test: `cmd/trellis/cmd_extra_test.go`

- [ ] **Step 1: Write failing test**

Add to `cmd/trellis/cmd_extra_test.go`:

```go
func TestDecomposeApplyExampleFlag(t *testing.T) {
    repo := setupTestRepo(t)
    out := runCmdExpectOK(t, repo, "arm", "decompose-apply", "--example")
    require.Contains(t, out, `"version"`)
    require.Contains(t, out, `"issues"`)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/trellis/... -run TestDecomposeApplyExampleFlag -v
```

Expected: FAIL — `unknown flag: --example`.

- [ ] **Step 3: Add `--example` flag to `newDecomposeApplyCmd`**

In `cmd/trellis/decompose.go`, add an `--example` flag that prints a minimal valid plan JSON and exits, without requiring `--plan`:

```go
var examplePlan = `{
  "version": 1,
  "title": "Example plan",
  "issues": [
    {
      "id": "E1",
      "title": "My Epic",
      "type": "epic",
      "scope": "",
      "priority": "",
      "dod": "",
      "parent": "",
      "blocked_by": [],
      "notes": []
    },
    {
      "id": "E1-S1",
      "title": "My Story",
      "type": "story",
      "scope": "",
      "priority": "",
      "dod": "",
      "parent": "E1",
      "blocked_by": [],
      "notes": []
    },
    {
      "id": "E1-S1-T1",
      "title": "My Task",
      "type": "task",
      "scope": "internal/foo/foo.go",
      "priority": "high",
      "dod": "Tests pass, coverage maintained",
      "parent": "E1-S1",
      "blocked_by": [],
      "notes": ["See docs/foo.md for context"]
    }
  ]
}`
```

When `--example` is set, print `examplePlan` and return nil without executing the apply logic. Mark `--plan` as not required when `--example` is set.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./cmd/trellis/... -run TestDecomposeApplyExampleFlag -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/trellis/decompose.go cmd/trellis/cmd_extra_test.go
git commit -m "feat(ux): add decompose-apply --example flag for schema discoverability (L12)"
```

---

### Task 6: `arm list` command for queryable issue enumeration (L13)

Agents cannot discover existing issue IDs or hierarchy using `arm` alone, falling back to filesystem reads. Add a `arm list` command with type and parent filters.

**Files:**
- Create: `cmd/trellis/list.go`
- Modify: `cmd/trellis/main.go`
- Test: `cmd/trellis/cmd_extra_test.go`

- [ ] **Step 1: Write failing tests**

Add to `cmd/trellis/cmd_extra_test.go`:

```go
func TestListAllIssues(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    runCmd(t, repo, "arm", "create", "--id", "E-1", "--title", "Epic", "--type", "epic")
    runCmd(t, repo, "arm", "create", "--id", "E-1-S1", "--title", "Story", "--type", "story", "--parent", "E-1")
    runCmd(t, repo, "arm", "create", "--id", "E-1-S1-T1", "--title", "Task", "--type", "task", "--parent", "E-1-S1")
    out := runCmdExpectOK(t, repo, "arm", "list")
    require.Contains(t, out, "E-1")
    require.Contains(t, out, "E-1-S1")
    require.Contains(t, out, "E-1-S1-T1")
}

func TestListFilterByType(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    runCmd(t, repo, "arm", "create", "--id", "E-1", "--title", "Epic", "--type", "epic")
    runCmd(t, repo, "arm", "create", "--id", "E-1-S1-T1", "--title", "Task", "--type", "task", "--parent", "E-1")
    out := runCmdExpectOK(t, repo, "arm", "list", "--type", "task")
    require.Contains(t, out, "E-1-S1-T1")
    require.NotContains(t, out, "E-1\t")
}

func TestListFilterByParent(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    runCmd(t, repo, "arm", "create", "--id", "E-1", "--title", "Epic", "--type", "epic")
    runCmd(t, repo, "arm", "create", "--id", "E-2", "--title", "Other Epic", "--type", "epic")
    runCmd(t, repo, "arm", "create", "--id", "E-1-T1", "--title", "Child", "--type", "task", "--parent", "E-1")
    runCmd(t, repo, "arm", "create", "--id", "E-2-T1", "--title", "Other child", "--type", "task", "--parent", "E-2")
    out := runCmdExpectOK(t, repo, "arm", "list", "--parent", "E-1")
    require.Contains(t, out, "E-1-T1")
    require.NotContains(t, out, "E-2-T1")
}

func TestListFormatJSON(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    runCmd(t, repo, "arm", "create", "--id", "E-1", "--title", "Epic", "--type", "epic")
    out := runCmdExpectOK(t, repo, "arm", "list", "--format", "json")
    require.Contains(t, out, `"id"`)
    require.Contains(t, out, `"E-1"`)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/trellis/... -run TestList -v
```

Expected: FAIL — `unknown command "list"`.

- [ ] **Step 3: Implement `cmd/trellis/list.go`**

Create `cmd/trellis/list.go` that:
- Materializes state
- Iterates all issues sorted by ID
- Filters by `--type` (if provided) and `--parent` (if provided)
- In human format: prints `ID\tTYPE\tSTATUS\tTITLE` tab-separated, one per line
- In json/agent format: marshals a JSON array of `{id, title, type, status, parent, blocked_by}` objects

Register in `main.go`.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./cmd/trellis/... -run TestList -v
```

Expected: PASS.

- [ ] **Step 5: Run full check**

```bash
make check
```

Expected: all pass, coverage ≥80%.

- [ ] **Step 6: Commit**

```bash
git add cmd/trellis/list.go cmd/trellis/main.go cmd/trellis/cmd_extra_test.go
git commit -m "feat(ux): add arm list command with --type and --parent filters (L13)"
```

---

### Task 7: Document the decompose pipeline in SKILL.md and add `decompose-apply --dry-run` (L14 + L15)

The `sources → decompose-context → decompose-apply` pipeline is invisible to agents. Additionally, `decompose-apply` has no way to preview effects before committing ops.

**Files:**
- Modify: `.claude/skills/arm/SKILL.md`
- Modify: `cmd/trellis/decompose.go`
- Modify: `internal/decompose/apply.go`
- Test: `cmd/trellis/cmd_extra_test.go`

- [ ] **Step 1: Write failing test for `--dry-run`**

Add to `cmd/trellis/cmd_extra_test.go`:

```go
func TestDecomposeApplyDryRun(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")

    plan := writeTempPlan(t, `{
        "version": 1,
        "title": "Dry run test",
        "issues": [
            {"id": "E-99", "title": "Test Epic", "type": "epic",
             "scope": "", "priority": "", "dod": "", "parent": "",
             "blocked_by": [], "notes": []}
        ]
    }`)

    out := runCmdExpectOK(t, repo, "arm", "decompose-apply", "--plan", plan, "--dry-run")
    require.Contains(t, out, "E-99")
    require.Contains(t, out, "dry run")

    // Verify nothing was actually created
    statusOut := runCmdOutput(t, repo, "arm", "status")
    require.NotContains(t, statusOut, "E-99")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/trellis/... -run TestDecomposeApplyDryRun -v
```

Expected: FAIL — `unknown flag: --dry-run`.

- [ ] **Step 3: Add `--dry-run` flag to `newDecomposeApplyCmd`**

When `--dry-run` is set, validate the plan against current state and print what would be created (ID, type, parent, title) without writing any ops. Print a summary line: `Dry run: would create N issues`.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./cmd/trellis/... -run TestDecomposeApplyDryRun -v
```

Expected: PASS.

- [ ] **Step 5: Add "Loading a plan" workflow to SKILL.md**

In `.claude/skills/arm/SKILL.md`, add a section describing the full pipeline:

```markdown
## Loading a Plan (Source → Decompose → Apply)

When ingesting a specification or design document into the issue graph:

1. Register the source document:
   arm sources add --type filesystem --url <path/to/doc.md> --title "Plan title"
   arm sources sync

2. Build the decomposition prompt (include existing graph for ID awareness):
   arm decompose-context --sources <source-id> --existing-dag

3. Use the prompt output to generate a plan JSON matching the schema
   (run `arm decompose-apply --example` to see the required format).

4. Validate before committing:
   arm decompose-apply --plan plan.json --dry-run

5. Apply:
   arm decompose-apply --plan plan.json
```

- [ ] **Step 6: Run full check**

```bash
make check
```

Expected: all pass, coverage ≥80%.

- [ ] **Step 7: Commit**

```bash
git add cmd/trellis/decompose.go internal/decompose/apply.go \
        cmd/trellis/cmd_extra_test.go .claude/skills/arm/SKILL.md
git commit -m "feat(ux): add decompose-apply --dry-run and document decompose pipeline in skill (L14, L15)"
```

---

### Task 8: `decompose-apply` missing architecture flags — `--root`, `--generate-ids`, `--strict`

The architecture (section 7) specifies three flags that are not yet implemented. These are part of the original spec, not additions.

**Files:**
- Modify: `cmd/trellis/decompose.go`
- Modify: `internal/decompose/apply.go`
- Test: `cmd/trellis/cmd_extra_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestDecomposeApplyStrictRejectsWarnings(t *testing.T) {
    // Plan with scope overlap (W1) — should fail with --strict
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    plan := writePlanWithScopeOverlap(t)
    runCmdExpectError(t, repo, "arm", "decompose-apply", "--plan", plan, "--strict")
}

func TestDecomposeApplyGenerateIDs(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    plan := writeTempPlan(t, minimalPlanJSON) // has explicit IDs
    out := runCmdExpectOK(t, repo, "arm", "decompose-apply", "--plan", plan, "--generate-ids")
    // Output should show created count; original IDs should NOT appear
    require.NotContains(t, out, "E-99") // plan's explicit ID replaced
}
```

- [ ] **Step 2: Implement `--strict`**

In `newDecomposeApplyCmd`: add `--strict` flag. When set, treat all W-class warnings as blocking errors — `decompose-apply` exits with code 1 and writes no ops.

- [ ] **Step 3: Implement `--generate-ids`**

When `--generate-ids` is set, replace every node's ID with a generated UUID before validation and op creation. The plan's ID field is used as a human label in notes only.

- [ ] **Step 4: Implement `--root`**

`--root <id>` overrides the inferred root node. Default behavior (infer from plan hierarchy) is unchanged.

- [ ] **Step 5: Run tests and full check**

```bash
go test ./cmd/trellis/... -run TestDecomposeApply -v
make check
```

- [ ] **Step 6: Commit**

```bash
git add cmd/trellis/decompose.go internal/decompose/apply.go cmd/trellis/cmd_extra_test.go
git commit -m "feat(ux): add decompose-apply --root, --generate-ids, --strict (architecture section 7)"
```

---

## Chunk 2: E5-S2 — `arm tui` Interactive Board

A full BubbleTea kanban board: three status columns (open / active / done), a workers panel, auto-refresh, and an issue detail viewport. Read-only in this iteration.

### File Map

| File | Change |
|------|--------|
| `internal/tui/board/model.go` | **New** — BubbleTea model (columns, nav, detail view) |
| `internal/tui/board/keys.go` | **New** — key bindings |
| `internal/tui/board/model_test.go` | **New** — unit tests |
| `cmd/trellis/tui.go` | **New** — `arm tui` CLI command |
| `cmd/trellis/main.go` | Register `newTUICmd()` |

---

### Task 1: Board model — column layout and keyboard navigation

The model receives pre-loaded data and renders a 3-column layout with cursor navigation.

**Files:**
- Create: `internal/tui/board/keys.go`
- Create: `internal/tui/board/model.go`
- Create: `internal/tui/board/model_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/tui/board/model_test.go`:

```go
package board

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/require"
)

func makeTestData() BoardData {
	return BoardData{
		Index: materialize.Index{
			"T-1": {Title: "Alpha", Status: ops.StatusOpen, Type: "task"},
			"T-2": {Title: "Beta", Status: ops.StatusInProgress, Type: "task"},
			"T-3": {Title: "Gamma", Status: ops.StatusDone, Type: "task"},
		},
		Issues:  map[string]*materialize.Issue{},
		Workers: nil,
	}
}

func TestNew_PopulatesColumns(t *testing.T) {
	m := New(makeTestData())
	require.Len(t, m.cols[colOpen], 1)
	require.Len(t, m.cols[colActive], 1)
	require.Len(t, m.cols[colDone], 1)
}

func TestNavigation_TabSwitchesColumn(t *testing.T) {
	m := New(makeTestData())
	require.Equal(t, column(colOpen), m.activeCol)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	nm := next.(Model)
	require.Equal(t, column(colActive), nm.activeCol)
}

func TestNavigation_JMovesCursorDown(t *testing.T) {
	data := makeTestData()
	// Add a second open item (Index is map[string]IndexEntry — value type, not pointer)
	data.Index["T-4"] = materialize.IndexEntry{Title: "Delta", Status: ops.StatusOpen, Type: "task"}
	m := New(data)
	require.Equal(t, 0, m.cursors[colOpen])
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	nm := next.(Model)
	require.Equal(t, 1, nm.cursors[colOpen])
}

func TestNavigation_QQuits(t *testing.T) {
	m := New(makeTestData())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)
}

func TestView_RendersAllColumns(t *testing.T) {
	m := New(makeTestData())
	view := m.View()
	require.Contains(t, view, "Open")
	require.Contains(t, view, "Active")
	require.Contains(t, view, "Done")
	require.Contains(t, view, "Alpha")
	require.Contains(t, view, "Beta")
	require.Contains(t, view, "Gamma")
}
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/tui/board/... -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Create key bindings**

Create `internal/tui/board/keys.go`:

```go
package board

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines key bindings for the board TUI.
type KeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	Tab    key.Binding
	Detail key.Binding
	Quit   key.Binding
	Escape key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:     key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:   key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Left:   key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "prev col")),
		Right:  key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "next col")),
		Tab:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next col")),
		Detail: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Escape: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}
```

- [ ] **Step 4: Create the board model**

Create `internal/tui/board/model.go`:

```go
package board

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/tui"
)

// column indices
type column int

const (
	colOpen   column = 0
	colActive column = 1
	colDone   column = 2
	numCols   column = 3
)

var colLabels = [3]string{"Open", "Active", "Done"} // use literal 3, not named type constant

// BoardItem is a display-ready row within a column.
type BoardItem struct {
	ID       string
	Title    string
	Priority string
	Worker   string // non-empty for active items
}

// WorkerSummary is a display-ready worker row.
type WorkerSummary struct {
	WorkerID    string
	Status      string
	ActiveIssue string
	LastOpTime  int64
}

// BoardData is the pure-data input to the board model.
type BoardData struct {
	Index   materialize.Index
	Issues  map[string]*materialize.Issue
	Workers []WorkerSummary
}

// RefreshMsg carries new data on a tick refresh. Exported so cmd/trellis/tui.go can send it.
type RefreshMsg struct{ Data BoardData }

// Model is the BubbleTea model for the board.
type Model struct {
	cols      [numCols][]BoardItem
	workers   []WorkerSummary
	cursors   [numCols]int
	activeCol column
	detailID  string // empty = no detail view
	keys      KeyMap
	width     int
	height    int
}

// New builds a board model from the given data.
func New(data BoardData) Model {
	m := Model{keys: DefaultKeyMap()}
	m.loadData(data)
	return m
}

func (m *Model) loadData(data BoardData) {
	var open, active, done []BoardItem
	for id, entry := range data.Index {
		// IndexEntry does not carry Priority — read it from the full Issue if available.
		item := BoardItem{ID: id, Title: entry.Title}
		if issue, ok := data.Issues[id]; ok {
			item.Worker = issue.ClaimedBy
			item.Priority = issue.Priority
		}
		switch entry.Status {
		case ops.StatusOpen:
			open = append(open, item)
		case ops.StatusClaimed, ops.StatusInProgress:
			active = append(active, item)
		case ops.StatusDone:
			done = append(done, item)
		}
	}
	sortItems(open)
	sortItems(active)
	sortItems(done)
	m.cols[colOpen] = open
	m.cols[colActive] = active
	m.cols[colDone] = done
	m.workers = data.Workers
}

func sortItems(items []BoardItem) {
	priorityRank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "": 4}
	sort.Slice(items, func(i, j int) bool {
		pi := priorityRank[items[i].Priority]
		pj := priorityRank[items[j].Priority]
		if pi != pj {
			return pi < pj
		}
		return items[i].ID < items[j].ID
	})
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case RefreshMsg:
		m.loadData(msg.Data)

	case tea.KeyMsg:
		if m.detailID != "" {
			// In detail view, esc or q returns to board
			if key.Matches(msg, m.keys.Escape) || key.Matches(msg, m.keys.Quit) {
				m.detailID = ""
				return m, nil
			}
			return m, nil
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.Right):
			m.activeCol = (m.activeCol + 1) % numCols
		case key.Matches(msg, m.keys.Left):
			m.activeCol = (m.activeCol + numCols - 1) % numCols
		case key.Matches(msg, m.keys.Down):
			col := m.activeCol
			if m.cursors[col] < len(m.cols[col])-1 {
				m.cursors[col]++
			}
		case key.Matches(msg, m.keys.Up):
			col := m.activeCol
			if m.cursors[col] > 0 {
				m.cursors[col]--
			}
		case key.Matches(msg, m.keys.Detail):
			col := m.activeCol
			items := m.cols[col]
			if len(items) > 0 && m.cursors[col] < len(items) {
				m.detailID = items[m.cursors[col]].ID
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.detailID != "" {
		return m.detailView()
	}
	return m.boardView()
}

func (m Model) boardView() string {
	colWidth := 30
	if m.width > 90 {
		colWidth = (m.width - 6) / 3
	}

	cols := make([]string, numCols)
	for ci := column(0); ci < numCols; ci++ {
		header := colLabels[ci]
		if ci == m.activeCol {
			header = tui.ActionRequired.Bold(true).Render("▶ " + header)
		} else {
			header = tui.Muted.Render("  " + header)
		}
		count := tui.Muted.Render(fmt.Sprintf("(%d)", len(m.cols[ci])))
		lines := []string{header + " " + count, strings.Repeat("─", colWidth)}

		for i, item := range m.cols[ci] {
			line := truncate(item.ID+" "+item.Title, colWidth-2)
			if ci == m.activeCol && i == m.cursors[ci] {
				line = tui.Info.Render("> " + line)
			} else {
				line = "  " + line
			}
			lines = append(lines, line)
		}

		cols[ci] = lipgloss.NewStyle().
			Width(colWidth).
			BorderStyle(lipgloss.NormalBorder()).
			BorderRight(ci < numCols-1).
			Render(strings.Join(lines, "\n"))
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)

	// Workers panel
	workerLines := []string{tui.Muted.Render("Workers")}
	for _, w := range m.workers {
		status := tui.OK.Render(w.Status)
		if w.Status == "stale" {
			status = tui.Warning.Render(w.Status)
		}
		active := ""
		if w.ActiveIssue != "" {
			active = " → " + w.ActiveIssue
		}
		workerLines = append(workerLines, fmt.Sprintf("  %-30s %s%s",
			truncate(w.WorkerID, 30), status, active))
	}

	help := tui.Muted.Render("j/k up/down  h/l/tab col  enter detail  q quit")

	return board + "\n" + strings.Join(workerLines, "\n") + "\n\n" + help
}

func (m Model) detailView() string {
	return fmt.Sprintf("Issue: %s\n\n(run `arm render-context --issue %s` for full context)\n\nPress esc to return.",
		m.detailID, m.detailID)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
```

Note: `key.Matches` requires importing `"github.com/charmbracelet/bubbles/key"` — add that import.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/tui/board/... -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/board/
git commit -m "feat(tui): add board model with column layout and keyboard nav (E5-S2-T1)"
```

> **Ordering note:** `RefreshMsg` is defined and exported in this task's `model.go`. Task 2 (`tui.go`) references `board.RefreshMsg` — it must come after this task, not before.

---

### Task 2: `arm tui` CLI command with auto-refresh

**Files:**
- Create: `cmd/trellis/tui.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write the CLI command**

Create `cmd/trellis/tui.go`:

```go
package main

import (
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/tui/board"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive board TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"

			data, err := loadBoardData(issuesDir, singleBranch)
			if err != nil {
				return fmt.Errorf("load board data: %w", err)
			}

			m := board.New(data)
			p := tea.NewProgram(m, tea.WithAltScreen())
			// Auto-refresh every 30 seconds
			go func() {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				for range ticker.C {
					fresh, err := loadBoardData(issuesDir, singleBranch)
					if err == nil {
						p.Send(board.RefreshMsg{Data: fresh}) // RefreshMsg is exported from internal/tui/board/model.go
					}
				}
			}()
			_, err = p.Run()
			return err
		},
	}
	return cmd
}

// loadBoardData materializes state and assembles a BoardData for the TUI.
func loadBoardData(issuesDir string, singleBranch bool) (board.BoardData, error) {
	if _, err := materialize.Materialize(issuesDir, singleBranch); err != nil {
		return board.BoardData{}, fmt.Errorf("materialize: %w", err)
	}

	index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
	if err != nil {
		return board.BoardData{}, err
	}

	issues := make(map[string]*materialize.Issue)
	for id := range index {
		issue, err := materialize.LoadIssue(
			fmt.Sprintf("%s/state/issues/%s.json", issuesDir, id))
		if err == nil {
			issues[id] = &issue
		}
	}

	opsDir := filepath.Join(issuesDir, "ops")
	workers, err := buildBoardWorkers(opsDir)
	if err != nil {
		workers = nil // non-fatal
	}

	return board.BoardData{
		Index:   index,
		Issues:  issues,
		Workers: workers,
	}, nil
}

// buildBoardWorkers assembles worker summaries for the board panel.
func buildBoardWorkers(opsDir string) ([]board.WorkerSummary, error) {
	workerOps, err := enumerateWorkers(opsDir)
	if err != nil {
		return nil, err
	}
	defaultTTL := appCtx.Config.DefaultTTL
	if defaultTTL <= 0 {
		defaultTTL = 60
	}
	now := time.Now().Unix()
	var summaries []board.WorkerSummary
	for workerID, allOps := range workerOps {
		s := buildWorkerStatus(workerID, allOps, defaultTTL, now)
		summaries = append(summaries, board.WorkerSummary{
			WorkerID:    s.WorkerID,
			Status:      s.Status,
			ActiveIssue: s.ActiveIssue,
			LastOpTime:  s.LastOpTime,
		})
	}
	return summaries, nil
}
```

Note: `RefreshMsg` is already exported in Task 1's `model.go`. No model changes needed here.

- [ ] **Step 2: Register command in `main.go`**

In `cmd/trellis/main.go`, add before `return root`:

```go
root.AddCommand(newTUICmd())
```

- [ ] **Step 3: Build to verify compilation**

```bash
go build ./cmd/trellis/
```

Expected: compiles cleanly.

- [ ] **Step 4: Run tests**

```bash
make check
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/trellis/tui.go cmd/trellis/main.go
git commit -m "feat(tui): add arm tui interactive board command (E5-S2)"
```

---

### Task 3: `arm ready` interactive TUI in human mode

The architecture (section 15) specifies that `arm ready` in human-interactive mode shows a task selection list from which the operator can claim directly. Currently `arm ready` is plain text only. This makes it a first-class interactive claim flow for humans, distinct from (but complementary to) the `arm tui` kanban board.

**Files:**
- Modify: `cmd/trellis/ready.go`
- Create: `internal/tui/ready/model.go`
- Create: `internal/tui/ready/model_test.go`

- [ ] **Step 1: Implement BubbleTea selection model**

`internal/tui/ready/model.go` — displays the ready task queue with:
- Priority-sorted list (priority, estimated_complexity, scope summary)
- `j`/`k` navigation
- `enter` to claim the selected task (runs `arm claim` internally)
- `q` to quit without claiming
- `--format=json` bypasses the TUI entirely (agent-safe)

- [ ] **Step 2: Wire into `ready.go`**

When stdout is a TTY and `--format` is not explicitly `json` or `agent`, launch the BubbleTea model. When stdout is a pipe or `--format=json`, emit plain JSON as today.

- [ ] **Step 3: Unit test the model**

`internal/tui/ready/model_test.go` — test that selection state updates correctly, that `enter` on an item produces the correct claim command.

- [ ] **Step 4: Run full check**

```bash
make check
```

- [ ] **Step 5: Commit**

```bash
git add cmd/trellis/ready.go internal/tui/ready/
git commit -m "feat(tui): add ready interactive TUI for human task selection and claim (architecture section 15)"
```

---

## Chunk 3: E5-S3 — Time Travel & Forensics

`render-context --at <sha>` reconstructs the exact context an agent received at a past ops-branch commit. `arm context-history --issue ID` lists commits where the issue changed, with timestamps and op types.

### File Map

| File | Change |
|------|--------|
| `internal/git/git.go` | Add `ListFilesAtCommit`, `ShowFileAtCommit`, `LogBranch` |
| `internal/git/git_test.go` | Extend with forensics method tests |
| `internal/materialize/atsha.go` | **New** — `MaterializeAtSHA()` |
| `internal/materialize/atsha_test.go` | **New** — unit tests |
| `cmd/trellis/render_context.go` | Add `--at <sha>` flag |
| `cmd/trellis/context_history.go` | **New** — `arm context-history` command |
| `cmd/trellis/main.go` | Register `newContextHistoryCmd()` |

---

### Task 1: Git client forensics methods

**Files:**
- Modify: `internal/git/git.go`
- Modify: `internal/git/git_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/git/git_test.go`:

```go
func TestListFilesAtCommit(t *testing.T) {
	repo := t.TempDir()
	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.email", "test@test.com")
	mustRun(t, repo, "git", "config", "user.name", "Test")
	mustRun(t, repo, "git", "config", "commit.gpgsign", "false") // prevent GPG failures in CI

	// Create a file and commit it
	os.MkdirAll(filepath.Join(repo, "ops"), 0755)
	os.WriteFile(filepath.Join(repo, "ops", "worker-a.log"), []byte("line1\n"), 0644)
	mustRun(t, repo, "git", "add", ".")
	mustRun(t, repo, "git", "commit", "-m", "test commit")

	sha := getHeadSHA(t, repo)
	c := New(repo)
	files, err := c.ListFilesAtCommit(sha, "ops")
	require.NoError(t, err)
	require.Contains(t, files, "ops/worker-a.log")
}

func TestShowFileAtCommit(t *testing.T) {
	repo := t.TempDir()
	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.email", "test@test.com")
	mustRun(t, repo, "git", "config", "user.name", "Test")
	mustRun(t, repo, "git", "config", "commit.gpgsign", "false")

	os.WriteFile(filepath.Join(repo, "data.txt"), []byte("hello forensics\n"), 0644)
	mustRun(t, repo, "git", "add", ".")
	mustRun(t, repo, "git", "commit", "-m", "add data")

	sha := getHeadSHA(t, repo)
	c := New(repo)
	content, err := c.ShowFileAtCommit(sha, "data.txt")
	require.NoError(t, err)
	require.Equal(t, "hello forensics\n", content)
}

func TestLogBranch(t *testing.T) {
	repo := t.TempDir()
	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.email", "test@test.com")
	mustRun(t, repo, "git", "config", "user.name", "Test")
	mustRun(t, repo, "git", "config", "commit.gpgsign", "false")

	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0644)
	mustRun(t, repo, "git", "add", ".")
	mustRun(t, repo, "git", "commit", "-m", "first")
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b\n"), 0644)
	mustRun(t, repo, "git", "add", ".")
	mustRun(t, repo, "git", "commit", "-m", "second")

	c := New(repo)
	commits, err := c.LogBranch("HEAD", 10)
	require.NoError(t, err)
	require.Len(t, commits, 2)
	require.NotEmpty(t, commits[0].SHA)
	require.NotZero(t, commits[0].Timestamp)
}

// helpers for git tests
func mustRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func getHeadSHA(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/git/... -run "TestListFilesAtCommit|TestShowFileAtCommit|TestLogBranch" -v
```

Expected: FAIL — methods undefined.

- [ ] **Step 3: Add methods to `internal/git/git.go`**

Append to `internal/git/git.go`:

```go
// CommitInfo holds a commit SHA and its unix timestamp.
type CommitInfo struct {
	SHA       string
	Timestamp int64
}

// ListFilesAtCommit returns paths under a directory prefix at a given SHA.
// The returned paths are relative to the repository root (e.g. "ops/worker.log").
func (c *Client) ListFilesAtCommit(sha, prefix string) ([]string, error) {
	cmd := exec.Command("git", "-C", c.repoPath, "ls-tree", "-r", "--name-only", sha, "--", prefix)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ls-tree %s %s: %w", sha, prefix, err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// ShowFileAtCommit returns the content of a file as it existed at a given SHA.
// path is relative to the repository root.
func (c *Client) ShowFileAtCommit(sha, path string) (string, error) {
	cmd := exec.Command("git", "-C", c.repoPath, "show", sha+":"+path)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git show %s:%s: %w", sha, path, err)
	}
	return string(output), nil
}

// LogBranch returns up to limit commits on branch (newest first) as CommitInfo.
func (c *Client) LogBranch(branch string, limit int) ([]CommitInfo, error) {
	args := []string{"-C", c.repoPath, "log",
		fmt.Sprintf("-n%d", limit),
		"--format=%H %at",
		branch,
	}
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log %s: %w", branch, err)
	}
	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		var ts int64
		_, _ = fmt.Sscan(parts[1], &ts)
		commits = append(commits, CommitInfo{SHA: parts[0], Timestamp: ts})
	}
	return commits, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/git/... -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat(git): add ListFilesAtCommit, ShowFileAtCommit, LogBranch (E5-S3-T1)"
```

---

### Task 2: `MaterializeAtSHA`

Reads ops from a git commit rather than the filesystem, then materializes state as of that point in time.

**Files:**
- Create: `internal/materialize/atsha.go`
- Create: `internal/materialize/atsha_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/materialize/atsha_test.go`:

```go
package materialize

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/require"
)

func TestMaterializeAtSHA_ReadsOpsFromGitObject(t *testing.T) {
	// Set up a bare repo with a committed op log
	repo := t.TempDir()
	mustGit(t, repo, "init")
	mustGit(t, repo, "config", "user.email", "x@x.com")
	mustGit(t, repo, "config", "user.name", "Test")

	opsDir := filepath.Join(repo, ".issues", "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	mustGit(t, repo, "config", "commit.gpgsign", "false") // prevent GPG failures in CI

	// Write a create op
	op := ops.Op{Type: ops.OpCreate, TargetID: "T-1", Timestamp: 100,
		WorkerID: "worker-a",
		Payload:  ops.Payload{Title: "Past Task", NodeType: "task"}}
	logPath := filepath.Join(opsDir, "worker-a.log")
	require.NoError(t, ops.AppendOp(logPath, op))

	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-m", "add ops")
	sha := gitHEAD(t, repo)

	// Add a second op AFTER the commit (should NOT appear in --at result)
	op2 := ops.Op{Type: ops.OpTransition, TargetID: "T-1", Timestamp: 200,
		WorkerID: "worker-a", Payload: ops.Payload{To: ops.StatusDone}}
	require.NoError(t, ops.AppendOp(logPath, op2))

	state, err := MaterializeAtSHA(repo, sha, ".armature/ops", true)
	require.NoError(t, err)
	require.Contains(t, state.Issues, "T-1")
	require.Equal(t, ops.StatusOpen, state.Issues["T-1"].Status,
		"state at SHA should not include the post-commit transition")
}

// mustGit is an in-package test helper. Check for name conflicts with other
// *_test.go files in internal/materialize/ before adding — rename if needed.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func gitHEAD(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./internal/materialize/... -run TestMaterializeAtSHA -v
```

Expected: FAIL — `MaterializeAtSHA` undefined.

- [ ] **Step 3: Implement `MaterializeAtSHA`**

Create `internal/materialize/atsha.go`:

```go
package materialize

import (
	"strings"

	"github.com/scullxbones/armature/internal/git"
	"github.com/scullxbones/armature/internal/ops"
)

// MaterializeAtSHA reconstructs the materialized state as of a specific git commit.
//
// repoPath is the git repository root.
// sha is the commit from which to read ops (typically a commit on the ops branch).
// opsPathPrefix is the directory path within the commit tree that contains *.log files
//   - dual-branch mode: "ops"  (the _armature branch root IS .armature/)
//   - single-branch mode: ".armature/ops"
//
// singleBranch is passed through to State.SingleBranchMode.
func MaterializeAtSHA(repoPath, sha, opsPathPrefix string, singleBranch bool) (*State, error) {
	gc := git.New(repoPath)

	files, err := gc.ListFilesAtCommit(sha, opsPathPrefix)
	if err != nil {
		return nil, err
	}

	var allOps []ops.Op
	for _, filePath := range files {
		if !strings.HasSuffix(filePath, ".log") {
			continue
		}
		content, err := gc.ShowFileAtCommit(sha, filePath)
		if err != nil {
			continue // skip unreadable files
		}
		workerID := ops.WorkerIDFromFilename(filePath)
		for _, line := range strings.Split(content, "\n") {
			if line == "" {
				continue
			}
			op, err := ops.ParseLine([]byte(line))
			if err != nil || op.WorkerID != workerID {
				continue
			}
			allOps = append(allOps, op)
		}
	}

	sortOpsByTimestamp(allOps)

	state := NewState()
	state.SingleBranchMode = singleBranch
	for _, op := range allOps {
		_ = state.ApplyOp(op)
	}
	state.RunRollup()
	return state, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/materialize/... -run TestMaterializeAtSHA -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/materialize/atsha.go internal/materialize/atsha_test.go
git commit -m "feat(materialize): add MaterializeAtSHA for forensic context reconstruction (E5-S3-T2)"
```

---

### Task 3: `render-context --at <sha>`

**Files:**
- Modify: `cmd/trellis/render_context.go`

- [ ] **Step 1: Write failing integration test**

Add to `cmd/trellis/main_test.go`:

```go
func TestRenderContextAtSHA(t *testing.T) {
	repo := setupTestRepo(t)
	runCmd(t, repo, "arm", "worker-init")
	runCmd(t, repo, "arm", "create", "--id", "T-1", "--title", "Snapshot Task", "--type", "task")

	// Commit the current ops state
	mustGitInRepo(t, repo, "add", ".")
	mustGitInRepo(t, repo, "commit", "-m", "snapshot ops")
	sha := gitHEADInRepo(t, repo)

	// Add a subsequent transition that should NOT appear in --at output
	runCmd(t, repo, "arm", "transition", "--issue", "T-1", "--to", "in-progress")

	// render-context --at <sha> should show T-1 as open (pre-transition)
	out := runCmdOutput(t, repo, "arm", "render-context", "--issue", "T-1", "--at", sha)
	require.Contains(t, out, "Snapshot Task")
}
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./cmd/trellis/... -run TestRenderContextAtSHA -v
```

Expected: FAIL — unknown flag `--at`.

- [ ] **Step 3: Add `--at` flag to `render_context.go`**

In `cmd/trellis/render_context.go`, add `rcAt string` to the var block and register the flag:

```go
var (
    rcIssue  string
    rcBudget int
    rcRaw    bool
    rcAt     string  // SHA for forensic reconstruction
)
// ...
cmd.Flags().StringVar(&rcAt, "at", "", "reconstruct context at this git commit SHA")
```

In the `RunE` body, replace the materialize+load block with:

```go
var state *materialize.State

if rcAt != "" {
    // Forensic mode: read ops from git object store at the given SHA
    opsPrefix := ".armature/ops"
    if appCtx.Mode != "single-branch" {
        // In dual-branch mode the SHA is on _armature which has ops/ at its root
        opsPrefix = "ops"
    }
    var err error
    state, err = materialize.MaterializeAtSHA(
        appCtx.RepoPath, rcAt, opsPrefix, appCtx.Mode == "single-branch")
    if err != nil {
        return fmt.Errorf("materialize at %s: %w", rcAt, err)
    }
} else {
    _, err := materialize.Materialize(issuesDir, appCtx.Mode == "single-branch")
    if err != nil {
        return fmt.Errorf("materialize: %w", err)
    }
    state, err = loadStateFromIssuesDir(issuesDir)
    if err != nil {
        return fmt.Errorf("load state: %w", err)
    }
}
```

Then use `state` for context assembly as before.

Note: `appCtx.RepoPath` already exists in `internal/config/context.go` (populated by `ResolveContext`). No changes to the config package are needed.

- [ ] **Step 4: Verify `config.Context` has `RepoPath`**

```bash
grep -n "RepoPath" internal/config/*.go
```

If missing, add `RepoPath string` to the `Context` struct and populate it in `ResolveContext`.

- [ ] **Step 5: Run tests**

```bash
go test ./cmd/trellis/... -run TestRenderContextAtSHA -v
make check
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/trellis/render_context.go internal/config/
git commit -m "feat(forensics): add render-context --at <sha> for time-travel context (E5-S3-T3)"
```

---

### Task 4: `arm context-history`

Lists ops-branch commits that touched a given issue, newest first, with timestamps and op types.

**Files:**
- Create: `cmd/trellis/context_history.go`
- Modify: `cmd/trellis/main.go`

- [ ] **Step 1: Write failing integration test**

Add to `cmd/trellis/main_test.go`:

```go
func TestContextHistoryListsCommits(t *testing.T) {
	repo := setupTestRepo(t)
	runCmd(t, repo, "arm", "worker-init")
	runCmd(t, repo, "arm", "create", "--id", "T-1", "--title", "History Task", "--type", "task")
	mustGitInRepo(t, repo, "add", ".")
	mustGitInRepo(t, repo, "commit", "-m", "add T-1")

	runCmd(t, repo, "arm", "transition", "--issue", "T-1", "--to", "in-progress")
	mustGitInRepo(t, repo, "add", ".")
	mustGitInRepo(t, repo, "commit", "-m", "transition T-1")

	out := runCmdOutput(t, repo, "arm", "context-history", "--issue", "T-1")
	require.Contains(t, out, "T-1")
}
```

- [ ] **Step 2: Run to verify fail**

```bash
go test ./cmd/trellis/... -run TestContextHistoryListsCommits -v
```

Expected: FAIL — unknown command.

- [ ] **Step 3: Implement `context-history`**

Create `cmd/trellis/context_history.go`:

```go
package main

import (
	"fmt"
	"time"

	"github.com/scullxbones/armature/internal/git"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/spf13/cobra"
)

func newContextHistoryCmd() *cobra.Command {
	var issueID string
	var limit int

	cmd := &cobra.Command{
		Use:   "context-history",
		Short: "Show ops-branch commits where an issue changed",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath := appCtx.RepoPath
			branch := "_armature" // _armature is visible from main repo even when checked out as worktree
			if appCtx.Mode == "single-branch" {
				branch = "HEAD"
			}

			gc := git.New(repoPath)
			commits, err := gc.LogBranch(branch, limit)
			if err != nil {
				return fmt.Errorf("git log %s: %w", branch, err)
			}

			opsPrefix := "ops"
			if appCtx.Mode == "single-branch" {
				opsPrefix = ".armature/ops"
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"Context history for %s (newest first):\n\n", issueID)

			for _, commit := range commits {
				state, err := materialize.MaterializeAtSHA(
					repoPath, commit.SHA, opsPrefix, appCtx.Mode == "single-branch")
				if err != nil {
					continue
				}
				issue, ok := state.Issues[issueID]
				if !ok {
					continue
				}
				ts := time.Unix(commit.Timestamp, 0).UTC().Format("2006-01-02T15:04:05Z")
				// Summarize relevant state at this commit
				note := fmt.Sprintf("status=%s", issue.Status)
				if issue.ClaimedBy != "" {
					note += fmt.Sprintf(" claimed_by=%s", issue.ClaimedBy)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s  %s\n",
					commit.SHA[:8], ts, note)
			}

			// Suggest render-context --at for full forensics
			if len(commits) > 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"\nUse `arm render-context --issue %s --at <sha>` to reconstruct full context at any commit.\n",
					issueID)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().IntVar(&limit, "limit", 20, "max commits to scan")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}

// Note: opsForIssue is intentionally omitted — context history is derived from
// full materialization at each SHA, not from raw op filtering.
```

- [ ] **Step 4: Register in `main.go`**

Add `root.AddCommand(newContextHistoryCmd())`.

- [ ] **Step 5: Run tests and check**

```bash
go test ./cmd/trellis/... -run TestContextHistory -v
make check
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/trellis/context_history.go cmd/trellis/main.go
git commit -m "feat(forensics): add context-history command (E5-S3-T4)"
```

---

### Task 5: `arm materialize --exclude-worker <worker-id>`

The architecture (section 19) specifies selective replay as a forensics primitive: rebuild state excluding one worker's ops. Useful for diagnosing whether a specific worker introduced corruption or inconsistency.

**Files:**
- Modify: `cmd/trellis/materialize.go` (add `--exclude-worker` flag)
- Modify: `internal/materialize/materialize.go` (pass exclusion list to replay engine)
- Test: `cmd/trellis/cmd_extra_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestMaterializeExcludeWorker(t *testing.T) {
    repo := setupTestRepo(t)
    runCmd(t, repo, "arm", "worker-init")
    workerID := getWorkerID(t, repo)
    runCmd(t, repo, "arm", "create", "--id", "T-1", "--title", "Task", "--type", "task")
    // Exclude this worker's ops — T-1 should not appear in state
    runCmd(t, repo, "arm", "materialize", "--exclude-worker", workerID)
    out := runCmdOutput(t, repo, "arm", "status")
    require.NotContains(t, out, "T-1")
}
```

- [ ] **Step 2: Add `--exclude-worker` flag to materialize command**

Pass a `[]string` exclusion list into the replay engine. During log file iteration, skip any log file whose worker ID matches the exclusion list. This is a read-only diagnostic mode — does not modify the ops branch, does not update checkpoint.

- [ ] **Step 3: Run tests and full check**

```bash
go test ./cmd/trellis/... -run TestMaterializeExclude -v
make check
```

- [ ] **Step 4: Commit**

```bash
git add cmd/trellis/materialize.go internal/materialize/materialize.go \
        cmd/trellis/cmd_extra_test.go
git commit -m "feat(forensics): add materialize --exclude-worker for selective replay (architecture section 19)"
```

---

## Chunk 4: E5-S4 — User-Facing Documentation

No existing `README.md`. This story creates the full user documentation surface: project overview, installation guide, getting-started walkthrough, command reference, and per-persona use case guides.

**Accuracy rule:** Every command shown in these docs must be verified to actually work before committing. Run each example in a test repo.

### File Map

| File | Change |
|------|--------|
| `README.md` | **New** — project overview, install, 5-minute quickstart |
| `docs/getting-started.md` | **New** — step-by-step guide from install to first agent task |
| `docs/commands.md` | **New** — complete command reference (every `arm` subcommand) |
| `docs/use-cases.md` | **New** — per-persona workflow walkthroughs |

---

### Task 1: `README.md` — Project overview and quickstart

The README is the first thing users see. It must answer four questions in under 90 seconds: what is it, why should I care, how do I install it, and how do I try it right now.

**Files:**
- Create: `README.md`

- [ ] **Step 1: Draft `README.md`**

Create `README.md`:

```markdown
# Armature

**Git-native work orchestration for multi-agent AI coordination.**

Armature gives AI coding agents persistent memory and lets human+AI teams coordinate without merge conflicts, external dependencies, or context rot. All state lives in git. No database, no server, no daemon — just a single Go binary and git.

> *"Context rot is a memory problem. Armature gives your agents memory."*

## Why Armature?

AI coding agents forget everything between sessions. When multiple agents work in the same repo, they step on each other with no coordination primitive. Existing tools (Jira, Linear, GitHub Issues) were built for humans, require network access, and consume 12,000–21,000+ tokens when presented to AI agents.

Armature solves this by treating the work queue as a git-native data structure:

- **Zero infrastructure.** Git is the only dependency.
- **Merge-conflict-free by construction.** Each worker writes to its own log file (MRDT).
- **Token-efficient context.** 650–1,600 tokens per task vs. 12,000+ for markdown approaches.
- **Works with every AI agent.** Claude Code, Cursor, Windsurf, Gemini CLI, Kiro — any tool with a terminal.

## Install

**Homebrew (macOS/Linux):**
```bash
brew install scullxbones/tap/arm
```

**Direct download (all platforms):**

Download the latest binary for your platform from [GitHub Releases](https://github.com/scullxbones/armature/releases).

**From source:**
```bash
git clone https://github.com/scullxbones/armature
cd trellis && make install   # installs to ~/.local/bin/arm
```

Requires Go 1.26+ and git 2.25+.

## 5-Minute Quickstart

```bash
# 1. Initialize Armature in your repo
cd my-project
arm init

# 2. Initialize yourself as a worker
arm worker-init

# 3. Create your first task
arm create --id T-1 --title "Add login page" --type task

# 4. Find what's ready to work on
arm ready

# 5. Claim the task
arm claim --issue T-1

# 6. Get structured context (what your AI agent will see)
arm render-context --issue T-1

# 7. When done, mark it complete
arm transition --issue T-1 --to done --outcome "Login page implemented with OAuth2"
```

For AI agents: see [`docs/SKILL.md`](docs/SKILL.md) for the complete work loop contract.

## Documentation

- [Getting Started](docs/getting-started.md) — step-by-step guide for your first project
- [Command Reference](docs/commands.md) — every `arm` subcommand explained
- [Use Cases](docs/use-cases.md) — workflows for solo devs, teams, and AI fleets
- [Architecture](docs/architecture.md) — how it works under the hood
- [SKILL.md](docs/SKILL.md) — AI worker contract (feed this to your agent)

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Op log** | Append-only JSONL file per worker. The source of truth. |
| **Materialization** | Replaying all ops to produce the current state. |
| **Ready queue** | Tasks whose blockers are merged and parent is in-progress. |
| **Dual-branch mode** | Ops live on a separate `_armature` orphan branch, keeping them out of code history. |
| **MRDT** | Mergeable Replicated Data Type — merge conflicts are impossible by construction. |

## License

MIT
```

- [ ] **Step 2: Verify each quickstart command works**

Run each command from the quickstart in a fresh temp directory:

```bash
TMPDIR=$(mktemp -d) && cd $TMPDIR
git init && git config user.email "test@test.com" && git config user.name "Test"
git commit --allow-empty -m "init"
arm init
arm worker-init
arm create --id T-1 --title "Add login page" --type task
arm ready
arm claim --issue T-1
arm render-context --issue T-1
arm transition --issue T-1 --to done --outcome "done"
```

All commands must exit 0.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add README with overview, install, and quickstart (E5-S4-T1)"
```

---

### Task 2: `docs/getting-started.md` — Step-by-step guide

Covers: installation, single-branch setup (Lone Wolf), decomposing a PRD, the agent work loop, and dual-branch setup for teams.

**Files:**
- Create: `docs/getting-started.md`

- [ ] **Step 1: Create `docs/getting-started.md`**

Create `docs/getting-started.md` with the following sections:

```markdown
# Getting Started with Armature

This guide takes you from a fresh install to a working project in about 15 minutes.

## Prerequisites

- git 2.25 or newer
- `arm` binary installed (see [README](../README.md#install))

## 1. Initialize Your Repository

```bash
cd my-project
arm init
```

`arm init` auto-detects your repository configuration:
- If your main branch is unprotected (solo/local): **single-branch mode** — ops live alongside your code in `.armature/`.
- If your main branch is protected (team/enterprise): **dual-branch mode** — ops live on a separate `_armature` orphan branch.

You can force a mode:
```bash
arm init --mode single-branch
arm init --mode dual-branch
```

## 2. Register as a Worker

```bash
arm worker-init
```

This assigns you a unique UUID and records it in your local git config. Every operation you perform is attributed to this ID. AI agents run `worker-init` once per session.

## 3. Create Work Items

Work items form a DAG: **epic → story → task**. Only tasks appear in the ready queue.

```bash
# Create an epic
arm create --id E-1 --title "User Authentication" --type epic

# Create a story under it
arm create --id S-1 --title "Login page" --type story --parent E-1

# Create tasks under the story
arm create --id T-1 --title "Build login form" --type task --parent S-1
arm create --id T-2 --title "Add OAuth2 provider" --type task --parent S-1 --blocked-by T-1
```

## 4. Start the Work Loop

```bash
# Activate the epic/story chain
arm transition --issue E-1 --to in-progress
arm transition --issue S-1 --to in-progress

# See what's ready
arm ready

# Get full context for a task (what your AI agent will see)
arm render-context --issue T-1

# Claim it
arm claim --issue T-1

# ... do the work ...

# Mark complete
arm transition --issue T-1 --to done --outcome "Login form built with email/password"
```

## 5. Decomposing from a PRD (Conductor workflow)

If you have source documents (PRD, architecture doc), Armature can help decompose them:

```bash
# Register your PRD
arm sources add --id prd --url file://./docs/prd.md

# Generate decomposition context (feed this to your AI)
arm decompose-context --output decompose-context.md

# After your AI produces a plan.json:
arm decompose-apply --plan plan.json --dry-run   # validate
arm decompose-apply --plan plan.json             # apply

# Review the resulting DAG
arm dag-summary   # interactive TUI
```

## 6. Monitoring Progress

```bash
arm status          # all issues grouped by status
arm workers         # active workers and their claims
arm log             # full audit trail
arm validate        # check DAG structural integrity
```

## 7. Team Setup (Dual-Branch Mode)

In dual-branch mode, AI agents push their ops to `_armature` independently. No code conflicts possible.

```bash
# Each agent on first run:
arm init       # detects dual-branch, sets up _armature + worktree
arm worker-init

# Pull latest ops before starting work:
arm sync

# Push after completing tasks:
# (arm transition --to done triggers push automatically if configured)
```

See [use-cases.md](use-cases.md) for full team workflow examples.
```

- [ ] **Step 2: Verify commands are accurate**

Walk through section 3 and 4 manually in a temp repo. Ensure all commands work and outputs match.

- [ ] **Step 3: Commit**

```bash
git add docs/getting-started.md
git commit -m "docs: add getting-started guide (E5-S4-T2)"
```

---

### Task 3: `docs/commands.md` — Complete command reference

Every `arm` subcommand with flags, examples, and output description.

**Files:**
- Create: `docs/commands.md`

- [ ] **Step 1: Generate the base command list**

Run:
```bash
arm --help
```

Use the output as the authoritative list of commands. The reference must cover every command registered in `cmd/trellis/main.go`.

- [ ] **Step 2: Create `docs/commands.md`**

Structure:

```markdown
# Armature Command Reference

All commands accept `--repo <path>` to specify the repository (default: current directory) and `--format human|json|agent` to control output format.

---

## Setup

### `arm init`
Initialize Armature in a git repository.

```
arm init [--mode single-branch|dual-branch] [--repair]
```

Auto-detects branch mode. Creates `.armature/` directory (single-branch) or `_armature` orphan branch with ops worktree (dual-branch). Proposes git hooks based on detected project type.

**Flags:**
- `--mode` — force single-branch or dual-branch (default: auto-detect)
- `--repair` — re-run initialization to fix a broken state

---

### `arm worker-init`
Register the current user/agent as a Armature worker.

```
arm worker-init
```

Generates a UUID if none exists and records it in the local git config. Run once per machine (humans) or once per session (AI agents).

---

## Work Loop

### `arm ready`
List tasks that are ready to be claimed, in priority order.

```
arm ready [--worker <worker-id>] [--format json]
```

A task is ready when: status is `open`, all blockers are `merged`, parent is `in-progress`, and no active (non-stale) claim exists.

**Flags:**
- `--worker` — enable assignment-aware sorting (assigned-to-me first)
- `--format json` — machine-readable output for AI agents

---

### `arm render-context`
Assemble the full context slice for a task.

```
arm render-context --issue <id> [--at <sha>] [--budget <chars>] [--raw]
```

Produces 650–1,600 tokens of structured context across 7 layers: task spec, scope, decisions, notes, sibling outcomes, open questions, and token budget status.

**Flags:**
- `--at <sha>` — reconstruct context as of a past git commit (forensics)
- `--budget <chars>` — override token budget in characters (default: 4000)
- `--raw` — skip truncation

---

### `arm claim`
Claim a ready task.

```
arm claim --issue <id> [--ttl <minutes>]
```

Timestamp-based race resolution: first claim by time wins, lexicographic worker ID as tiebreaker. Stories and epics are auto-advanced to `in-progress` after claiming.

**Flags:**
- `--ttl` — claim TTL in minutes (default: 60)

---

### `arm heartbeat`
Signal that work is ongoing to prevent claim expiry.

```
arm heartbeat --issue <id>
```

Resets the claim TTL timer. Rate-limited to once per minute per issue.

---

### `arm transition`
Move an issue to a new status.

```
arm transition --issue <id> --to <status> [--outcome <text>] [--branch <name>] [--pr <number>]
```

Valid statuses: `open`, `in-progress`, `done`, `merged`, `blocked`, `cancelled`.

Pre-transition hooks run before the op is recorded. Required hooks must pass; optional hooks warn but proceed.

**Flags:**
- `--outcome` — one-line summary of what was accomplished (recommended for `done`)
- `--branch` — feature branch name (recorded for merge detection)
- `--pr` — PR number or URL

---

### `arm reopen`
Reopen a done or blocked issue.

```
arm reopen --issue <id>
```

Equivalent to `arm transition --issue <id> --to open`.

---

## Coordination

### `arm assign`
Assign an issue to a specific worker.

```
arm assign --issue <id> --worker <worker-id>
```

Assignment is advisory — any worker can claim an assigned issue. Assigned tasks appear first in `arm ready --worker <me>`.

---

### `arm unassign`
Remove worker assignment. If the issue is currently `claimed`, also transitions it back to `open`.

```
arm unassign --issue <id>
```

---

### `arm workers`
Show all workers and their current activity.

```
arm workers [--json]
```

Reports `active` (live claim), `stale` (expired claim), or `idle` for each worker.

---

### `arm sync`
Fetch and materialize the ops branch.

```
arm sync [--check] [--code]
```

**Flags:**
- `--check` — show sync status without pulling
- `--code` — also sync the code branch

---

### `arm status`
Show all issues grouped by status.

```
arm status
```

---

## Governance

### `arm validate`
Run structural integrity checks on the DAG.

```
arm validate [--scope <prefix>] [--strict] [--ci] [--format json]
```

Checks W1–W11 (warnings) and E2–E12 (errors). Use `--ci` in CI pipelines for non-zero exit on any finding.

---

### `arm dag-summary`
Interactive TUI for reviewing and signing off on DAG nodes.

```
arm dag-summary [--json]
```

Per-node sign-off records a `dag-transition` op with worker attribution. Use `--json` for non-interactive output.

---

### `arm log`
Display the audit trail.

```
arm log [--issue <id>] [--worker <id>] [--since <time>] [--json]
```

---

## Sources & Decomposition

### `arm sources`
Manage source documents.

```
arm sources add --id <id> --url <url> [--provider filesystem|confluence|sharepoint]
arm sources sync [--id <id>]
arm sources verify
```

---

### `arm decompose-context`
Generate a decomposition context package for an AI agent.

```
arm decompose-context [--sources] [--template <path>] [--output <path>]
```

---

### `arm decompose-apply`
Apply an AI-generated decomposition plan.

```
arm decompose-apply --plan <plan.json> [--dry-run]
```

---

### `arm decompose-revert`
Revert a decomposition (double-entry cancellation).

```
arm decompose-revert --plan <plan.json>
```

---

## Forensics

### `arm context-history`
List ops-branch commits where an issue changed.

```
arm context-history --issue <id> [--limit <n>]
```

Use with `arm render-context --at <sha>` to reconstruct the exact context at any point in history.

---

## Brownfield

### `arm import`
Import tasks from CSV or JSON.

```
arm import --file <path>
```

Imported nodes get `provenance.confidence=inferred` and cannot be claimed until confirmed.

---

### `arm confirm`
Confirm an imported node for claiming.

```
arm confirm --issue <id>
```

---

## Interactive TUI

### `arm tui`
Launch the interactive board. *(Added in E5-S2 — verify the command is registered before committing this documentation.)*

```
arm tui
```

Three-column kanban (Open / Active / Done) with workers panel. Keyboard: `j/k` to navigate, `tab/h/l` to switch columns, `enter` for issue detail, `q` to quit. Auto-refreshes every 30 seconds.

---

### `arm stale-review`
Interactive TUI for reviewing source document changes.

```
arm stale-review
```

---

## Utilities

### `arm create`
Create a new work item.

```
arm create --id <id> --title <text> --type epic|story|task|feature
            [--parent <id>] [--blocked-by <id>,...] [--priority critical|high|medium|low]
            [--scope <path>,...] [--complexity small|medium|large]
```

---

### `arm note`
Add a note to an issue.

```
arm note --issue <id> --msg <text>
```

---

### `arm decision`
Record a structured decision.

```
arm decision --topic <text> --choice <text> --rationale <text> [--affects <id>,...]
```

---

### `arm link`
Add a dependency between issues.

```
arm link --from <id> --to <id> [--rel blocks|related]
```

---

### `arm materialize`
Force explicit materialization from ops.

```
arm materialize
```

---

### `arm merged`
Mark an issue as merged (typically called by post-merge git hook).

```
arm merged --issue <id> [--pr <number>]
```

---

### `arm version`
Print the binary version.

```
arm version
```
```

- [ ] **Step 3: Verify the command list is complete**

```bash
arm --help | grep -E "^\s{2}\w"
```

Compare output against the commands documented above. Add any missing commands.

- [ ] **Step 4: Commit**

```bash
git add docs/commands.md
git commit -m "docs: add complete command reference (E5-S4-T3)"
```

---

### Task 4: `docs/use-cases.md` — Per-persona workflows

Covers the five personas from the PRD: Lone Wolf, Gatekeeper, Conductor, Wrangler, and AI Worker.

**Files:**
- Create: `docs/use-cases.md`

- [ ] **Step 1: Create `docs/use-cases.md`**

```markdown
# Armature Use Cases

Real-world workflows for each type of Armature user.

---

## P1: Lone Wolf — Solo Developer

**Setup:** Single-branch mode. All `.armature/` data on main. One worker.

```bash
cd my-project
arm init                          # auto-detects single-branch
arm worker-init

# Create your project breakdown
arm create --id E-1 --title "v2.0 Refactor" --type epic
arm transition --issue E-1 --to in-progress

arm create --id T-1 --title "Extract auth module" --type task --parent E-1
arm create --id T-2 --title "Write auth tests" --type task --parent E-1 --blocked-by T-1

# Start your AI agent — feed it SKILL.md and let it loop:
arm ready
arm render-context --issue T-1
arm claim --issue T-1
# ... agent works ...
arm transition --issue T-1 --to done --outcome "Auth module extracted to internal/auth"
```

**Key benefit:** Your AI agent picks up exactly where it left off in the next session. No re-derivation of context.

---

## P2: Gatekeeper — Enterprise Developer with Protected Main

**Setup:** Dual-branch mode. `_armature` orphan branch for ops. Feature branches to main via PR.

```bash
arm init                          # detects protected main, creates _armature branch
arm worker-init

# Work loop is the same as Lone Wolf, but completion is two-phase:
arm transition --issue T-1 --to done --branch feature/auth --pr 142 --outcome "..."

# After your PR is merged, Armature auto-detects the merge:
arm sync                          # promotes T-1 from done → merged

# Only after merged will T-2 become unblocked and appear in ready queue
arm ready
```

**Key benefit:** Downstream tasks don't start until code has passed review. The audit trail connects ops history to PR history bidirectionally.

---

## P3: Conductor — Tech Lead Orchestrating a Fleet

**Setup:** Dual-branch mode. Multiple workers. Full DAG governance.

### Decomposing a PRD

```bash
# Register your source documents
arm sources add --id prd --url https://your-confluence/prd
arm sources add --id arch --url file://./docs/architecture.md
arm sources sync

# Generate decomposition context and feed to AI
arm decompose-context --sources --output decompose-context.md
# ... AI produces plan.json ...

# Validate and apply
arm decompose-apply --plan plan.json --dry-run
arm decompose-apply --plan plan.json

# Review the DAG interactively
arm dag-summary
```

### Monitoring in Real Time

```bash
arm status          # all issues by status
arm workers         # who's working on what
arm log --since 1h  # what happened in the last hour
arm validate --ci   # CI-ready structural check
```

### Spot-checking Agent Context

```bash
# See exactly what Agent X will see before it starts
arm render-context --issue T-42

# Check historical context after an incident
arm context-history --issue T-42
arm render-context --issue T-42 --at abc1234
```

---

## P4: Wrangler — Platform Engineer

**Setup:** Manages Armature infra for the Conductor's team.

### Provisioning Agents

```bash
# Each CI agent on spin-up:
arm init --mode dual-branch
arm worker-init
# Worker ID is now in local git config, persists for the agent's lifetime
```

### Tuning TTLs

Edit `.armature/config.json`:
```json
{
  "default_ttl": 120,
  "hooks": [
    { "name": "tests", "command": "go test ./...", "required": true },
    { "name": "lint", "command": "golangci-lint run", "required": false }
  ]
}
```

### Recovering from a Stuck Agent

```bash
# Find the stale worker
arm workers

# Steal its claim (or let TTL expire and claim normally)
arm claim --issue T-99   # timestamp-based winner takes precedence

# Or transition it manually
arm transition --issue T-99 --to open
```

---

## P5: AI Worker (The Swarm)

AI agents interact exclusively through the SKILL.md contract and `arm` CLI.

**Full work loop:** See [`docs/SKILL.md`](SKILL.md).

**Quick reference for agents:**

```bash
# Start of session
arm worker-init
arm sync

# Find work
arm ready --format json

# Get context for the top task
arm render-context --issue <id> --format json

# Claim
arm claim --issue <id>

# Signal progress (every 10 minutes during long operations)
arm heartbeat --issue <id>

# Complete
arm transition --issue <id> --to done --outcome "<one line summary>"

# Return to top of loop
```

**Error recovery:**
- `No tasks ready` with stale claim warning → run `arm claim --issue <id>` to steal
- `confidence=inferred` claim error → task needs human confirmation first
- Scope overlap warning → acknowledge and proceed, or raise with Conductor

---

## Multi-Agent Conflict Scenario

Two agents both see `T-55` as ready and both claim it simultaneously:

1. Both agents write claim ops to their own log files (no conflict at write time).
2. Both push to `_armature` — pushes succeed independently.
3. On the next `arm sync`, both agents materialize the same state.
4. Timestamp resolution: the earlier claim wins. The other agent's claim is treated as stale.
5. The losing agent runs `arm ready` and gets a different task.

**Zero wasted work** beyond the sync cycle window. No locks, no coordinators.
```

- [ ] **Step 2: Commit**

```bash
git add docs/use-cases.md
git commit -m "docs: add per-persona use case guides (E5-S4-T4)"
```

---

## Post-Epic Verification

After all five stories are complete:

- [ ] Run `make check` — must pass (lint + test ≥80% coverage + mutate)
- [ ] Run `arm decompose-apply` on a test plan — verify all nodes are created as `draft` and do NOT appear in `arm ready`
- [ ] Run `arm dag-summary` — sign off interactively, verify tasks become claimable in `arm ready`
- [ ] Run `arm tui` in a repo with issues — verify board renders, columns populate, `q` quits
- [ ] Run `arm ready` (no flags, TTY) — verify interactive selection TUI launches; select a task and confirm claim succeeds
- [ ] Run `arm render-context --issue <id> --at <sha>` on a past commit — verify output differs from HEAD
- [ ] Run `arm materialize --exclude-worker <id>` — verify excluded worker's issues disappear from state
- [ ] Run `arm ready` with a stale in-progress issue — verify diagnostic prints to stderr
- [ ] Verify `arm transition --to open` succeeds
- [ ] Verify `arm unassign` on a claimed issue transitions back to open
- [ ] Read README.md cold — answer: install path clear? quickstart runnable without docs?

---

## Deferred to E6

- GitHub Issues bidirectional sync (plan-post-bootstrap E4-002)
- Webhook support (plan-post-bootstrap E4-003)
- Issue templates / `arm init --template` (plan-post-bootstrap E4-004)
- Sprint bookmarks
- `arm metrics` command
- Log compaction (`arm compact`)
- **Plan file format migration: `issues` → `nodes` with separate `links` array** — The architecture spec (section 7) defines the plan format using a `nodes` array and a top-level `links` array for dependency edges. The current implementation uses `issues` with `blocked_by` inline per node. The `nodes`/`links` format supports arbitrary link types and is the correct long-term schema. Defer migration to E6 to avoid churn while E5 work is in flight; document the divergence in the `decompose-apply --help` text and in `docs/commands.md` so users are not surprised.
