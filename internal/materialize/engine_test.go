package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyCreateOp(t *testing.T) {
	state := NewState()
	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Fix auth", Parent: "story-01", NodeType: "task",
			Scope: []string{"src/auth/**"}, DefinitionOfDone: "Tests pass"},
	}
	require.NoError(t, state.ApplyOp(op))
	issue := state.Issues["task-01"]
	assert.Equal(t, "task-01", issue.ID)
	assert.Equal(t, "open", issue.Status)
	assert.Equal(t, "Fix auth", issue.Title)
	assert.Equal(t, "story-01", issue.Parent)
}

func TestApplyClaimOp(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	issue := state.Issues["task-01"]
	assert.Equal(t, "claimed", issue.Status)
	assert.Equal(t, "w1", issue.ClaimedBy)
	assert.Equal(t, int64(200), issue.ClaimedAt)
}

func TestApplyTransitionOp(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "Fixed it"}}))
	issue := state.Issues["task-01"]
	assert.Equal(t, "done", issue.Status)
	assert.Equal(t, "Fixed it", issue.Outcome)
}

func TestApplyNoteOp(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpNote, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Msg: "Found edge case"}}))
	assert.Len(t, state.Issues["task-01"].Notes, 1)
	assert.Equal(t, "Found edge case", state.Issues["task-01"].Notes[0].Msg)
}

func TestApplyLinkOp(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "A", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "B", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpLink, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "blocked_by"}}))
	assert.Contains(t, state.Issues["task-01"].BlockedBy, "task-02")
	assert.Contains(t, state.Issues["task-02"].Blocks, "task-01")
}

func TestApplyDecisionOp_LastWriteWins(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpDecision, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Topic: "db", Choice: "postgres", Rationale: "mature"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpDecision, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w2", Payload: ops.Payload{Topic: "db", Choice: "sqlite", Rationale: "simpler"}}))
	decisions := state.Issues["task-01"].Decisions
	active := activeDecisionForTopic(decisions, "db")
	assert.Equal(t, "sqlite", active.Choice)
}

func TestSingleBranchAutoMerge(t *testing.T) {
	state := NewState()
	state.SingleBranchMode = true
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "Done"}}))
	assert.Equal(t, "merged", state.Issues["task-01"].Status)
}

func TestMaterializePipeline(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	logPath := filepath.Join(opsDir, "worker-a1.log")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "epic-01", Timestamp: 100,
		WorkerID: "worker-a1", Payload: ops.Payload{Title: "Epic", NodeType: "epic"}}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "worker-a1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "epic-01"}}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "worker-a1", Payload: ops.Payload{TTL: 60}}))

	result, err := Materialize(dir, true)
	require.NoError(t, err)
	assert.Equal(t, 2, result.IssueCount)

	assert.FileExists(t, filepath.Join(stateDir, "index.json"))
	assert.FileExists(t, filepath.Join(issuesDir, "task-01.json"))
	assert.FileExists(t, filepath.Join(stateDir, "checkpoint.json"))
}

func TestPropRandomOpsNeverCrash(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 500

	properties := gopter.NewProperties(params)

	opTypeGen := gen.OneConstOf(ops.OpCreate, ops.OpClaim, ops.OpHeartbeat,
		ops.OpTransition, ops.OpNote, ops.OpLink, ops.OpDecision)

	properties.Property("random op sequences never panic", prop.ForAll(
		func(opType string, targetID string, ts int64) bool {
			if targetID == "" {
				return true
			}
			state := NewState()
			state.SingleBranchMode = true

			_ = state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: targetID, Timestamp: ts,
				WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})

			_ = state.ApplyOp(ops.Op{Type: opType, TargetID: targetID, Timestamp: ts + 1,
				WorkerID: "w1", Payload: ops.Payload{TTL: 60, To: "done", Msg: "test",
					Dep: "other", Rel: "blocked_by", Topic: "t", Choice: "c"}})

			return true
		},
		opTypeGen,
		gen.AlphaString(),
		gen.Int64Range(0, 1<<50),
	))

	properties.TestingRun(t)
}

func TestPropCreateIdempotent(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	properties.Property("duplicate creates are idempotent", prop.ForAll(
		func(id string) bool {
			if id == "" {
				return true
			}
			state := NewState()
			op := ops.Op{Type: ops.OpCreate, TargetID: id, Timestamp: 100,
				WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}

			_ = state.ApplyOp(op)
			_ = state.ApplyOp(op)

			return len(state.Issues) == 1 && state.Issues[id].Title == "T"
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

func TestApplyAssignOp(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpAssign, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{AssignedTo: "worker-x"}}))
	assert.Equal(t, "worker-x", state.Issues["task-01"].AssignedWorker)
}

func TestApplyUnassignOp(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpAssign, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{AssignedTo: "worker-x"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpAssign, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{AssignedTo: ""}}))
	assert.Equal(t, "", state.Issues["task-01"].AssignedWorker)
}

func TestApplyAssignOp_ToleratesUnknownIssue(t *testing.T) {
	state := NewState()
	// No create op — assign should not error
	err := state.ApplyOp(ops.Op{Type: ops.OpAssign, TargetID: "unknown-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{AssignedTo: "worker-x"}})
	assert.NoError(t, err)
}

func TestBuildIndex_IncludesAssignedWorker(t *testing.T) {
	s := NewState()
	s.Issues["T-001"] = &Issue{
		ID: "T-001", Type: "task", Status: "open", Title: "task",
		AssignedWorker: "worker-x",
		Children:       []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	index := s.BuildIndex()
	entry := index["T-001"]
	assert.Equal(t, "worker-x", entry.AssignedWorker)
}

func TestBuildIndex_IncludesBranchAndPR(t *testing.T) {
	s := NewState()
	s.Issues["T-001"] = &Issue{
		ID: "T-001", Type: "task", Status: "done",
		Title: "some task", Branch: "feature/my-work", PR: "42",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}

	index := s.BuildIndex()
	entry, ok := index["T-001"]
	require.True(t, ok)
	assert.Equal(t, "feature/my-work", entry.Branch)
	assert.Equal(t, "42", entry.PR)
}

func TestMaterializeAndReturn_BasicPipeline(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	logPath := filepath.Join(opsDir, "worker-b1.log")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "worker-b1", Payload: ops.Payload{Title: "My Task", NodeType: "task"}}))

	state, result, err := MaterializeAndReturn(dir, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.IssueCount)
	require.NotNil(t, state)
	assert.Contains(t, state.Issues, "task-01")
	assert.Equal(t, "My Task", state.Issues["task-01"].Title)
}

func TestMaterializeAndReturn_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	// No ops dir — should return empty state
	state, result, err := MaterializeAndReturn(dir, false)
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, 0, result.IssueCount)
	assert.Equal(t, 0, result.OpsProcessed)
}

func TestAppendUnique_AddsNew(t *testing.T) {
	slice := []string{"a", "b"}
	result := appendUnique(slice, "c")
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestAppendUnique_SkipsDuplicate(t *testing.T) {
	slice := []string{"a", "b", "c"}
	result := appendUnique(slice, "b")
	assert.Equal(t, []string{"a", "b", "c"}, result)
	assert.Len(t, result, 3, "duplicate should not be added")
}

func TestRunRollup_PromotesStoryWhenAllChildrenMerged(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "story-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))

	// In single branch mode, done → merged
	state.SingleBranchMode = true
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "done"}}))

	state.RunRollup()
	assert.Equal(t, "merged", state.Issues["story-01"].Status)
}

func TestRunRollup_DoesNotPromoteWithUnmergedChild(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task B", NodeType: "task", Parent: "story-01"}}))

	state.SingleBranchMode = true
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done"}}))

	state.RunRollup()
	assert.NotEqual(t, "merged", state.Issues["story-01"].Status, "story should not be merged with open task-02")
}

func TestApplyTransition_ReopenClearsPriorOutcome(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "First attempt done"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 400,
		WorkerID: "w1", Payload: ops.Payload{To: "open"}}))

	issue := state.Issues["task-01"]
	assert.Equal(t, "open", issue.Status)
	assert.Empty(t, issue.Outcome, "outcome should be cleared on reopen")
	assert.Contains(t, issue.PriorOutcomes, "First attempt done")
}

func TestPromoteParentToInProgress_SkipsAlreadyInProgress(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task", Parent: "story-01"}}))

	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}}))
	assert.Equal(t, "in-progress", state.Issues["story-01"].Status)

	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w2", Payload: ops.Payload{TTL: 60}}))
	assert.Equal(t, "in-progress", state.Issues["story-01"].Status)
}

func TestSortOpsByTimestamp(t *testing.T) {
	allOps := []ops.Op{
		{Timestamp: 300, WorkerID: "w1"},
		{Timestamp: 100, WorkerID: "w1"},
		{Timestamp: 200, WorkerID: "w1"},
	}
	sortOpsByTimestamp(allOps)
	assert.Equal(t, int64(100), allOps[0].Timestamp)
	assert.Equal(t, int64(200), allOps[1].Timestamp)
	assert.Equal(t, int64(300), allOps[2].Timestamp)
}

func TestSortOpsByTimestamp_StableOnEqualTimestamp(t *testing.T) {
	allOps := []ops.Op{
		{Timestamp: 100, WorkerID: "w2", Type: "first"},
		{Timestamp: 100, WorkerID: "w1", Type: "second"},
	}
	sortOpsByTimestamp(allOps)
	assert.Equal(t, "first", allOps[0].Type)
	assert.Equal(t, "second", allOps[1].Type)
}
