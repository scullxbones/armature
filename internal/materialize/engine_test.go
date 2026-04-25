package materialize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/scullxbones/armature/internal/ops"
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

	result, err := Materialize(dir, filepath.Join(dir, "state"), true)
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

func TestApplyCreateOp_DraftConfidence_Propagated(t *testing.T) {
	state := NewState()
	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-draft", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Draft task", NodeType: "task", Confidence: "draft"},
	}
	require.NoError(t, state.ApplyOp(op))
	assert.Equal(t, "draft", state.Issues["task-draft"].Provenance.Confidence)
}

func TestApplyCreateOp_NoConfidence_DefaultsToVerified(t *testing.T) {
	state := NewState()
	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-legacy", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Legacy task", NodeType: "task"},
	}
	require.NoError(t, state.ApplyOp(op))
	assert.Equal(t, "verified", state.Issues["task-legacy"].Provenance.Confidence)
}

func TestApplyCreateOp_VerifiedConfidence_Propagated(t *testing.T) {
	state := NewState()
	op := ops.Op{
		Type: ops.OpCreate, TargetID: "task-verified", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Verified task", NodeType: "task", Confidence: "verified"},
	}
	require.NoError(t, state.ApplyOp(op))
	assert.Equal(t, "verified", state.Issues["task-verified"].Provenance.Confidence)
}

func TestApplyDagTransitionOp_PromotesDraftSubtreeToVerified(t *testing.T) {
	state := NewState()
	// Create a root epic with two draft children; one is outside the subtree
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "epic-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Epic", NodeType: "epic", Confidence: "draft"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story", Parent: "epic-01", Confidence: "draft"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task under story", NodeType: "task", Parent: "story-01", Confidence: "draft"}}))
	// outside the subtree
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-outside", Timestamp: 103,
		WorkerID: "w1", Payload: ops.Payload{Title: "Outside task", NodeType: "task", Confidence: "draft"}}))

	// Apply dag-transition with IssueID="epic-01"
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpDAGTransition, TargetID: "epic-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{IssueID: "epic-01"},
	}))

	assert.Equal(t, "verified", state.Issues["epic-01"].Provenance.Confidence)
	assert.Equal(t, "verified", state.Issues["story-01"].Provenance.Confidence)
	assert.Equal(t, "verified", state.Issues["task-01"].Provenance.Confidence)
	// outside the subtree is unaffected
	assert.Equal(t, "draft", state.Issues["task-outside"].Provenance.Confidence)
}

func TestApplyDagTransitionOp_CustomTargetConfidence(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task", Confidence: "draft"}}))

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpDAGTransition, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{IssueID: "task-01", To: "verified"},
	}))

	assert.Equal(t, "verified", state.Issues["task-01"].Provenance.Confidence)
}

func TestApplyDagTransitionOp_NodesOutsideSubtreeUnaffected(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "epic-A", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Epic A", NodeType: "epic", Confidence: "draft"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "epic-B", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Epic B", NodeType: "epic", Confidence: "draft"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-A1", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task A1", NodeType: "task", Parent: "epic-A", Confidence: "draft"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-B1", Timestamp: 103,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task B1", NodeType: "task", Parent: "epic-B", Confidence: "draft"}}))

	// Promote only epic-A subtree
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpDAGTransition, TargetID: "epic-A", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{IssueID: "epic-A"},
	}))

	assert.Equal(t, "verified", state.Issues["epic-A"].Provenance.Confidence)
	assert.Equal(t, "verified", state.Issues["task-A1"].Provenance.Confidence)
	assert.Equal(t, "draft", state.Issues["epic-B"].Provenance.Confidence)
	assert.Equal(t, "draft", state.Issues["task-B1"].Provenance.Confidence)
}

func TestApplyDagTransitionOp_BackwardCompatExistingConfirmedBehavior(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	assert.False(t, state.Issues["task-01"].Provenance.DAGConfirmed)

	// Old-style op (no IssueID) still sets DAGConfirmed
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpDAGTransition, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Confirmed: true}}))
	assert.True(t, state.Issues["task-01"].Provenance.DAGConfirmed)
}

func TestApplySourceLinkOp(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpSourceLink, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{SourceID: "entry-42", SourceURL: "https://example.com/doc", Title: "Ref Doc"}}))
	issue := state.Issues["task-01"]
	require.Len(t, issue.SourceLinks, 1)
	assert.Equal(t, "entry-42", issue.SourceLinks[0].SourceEntryID)
	assert.Equal(t, "https://example.com/doc", issue.SourceLinks[0].SourceURL)
	assert.Equal(t, "Ref Doc", issue.SourceLinks[0].Title)
}

func TestApplyDAGTransitionOp(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}}))
	assert.False(t, state.Issues["task-01"].Provenance.DAGConfirmed)
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpDAGTransition, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Confirmed: true}}))
	assert.True(t, state.Issues["task-01"].Provenance.DAGConfirmed)
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

	state, result, err := MaterializeAndReturn(dir, filepath.Join(dir, "state"), true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.IssueCount)
	require.NotNil(t, state)
	assert.Contains(t, state.Issues, "task-01")
	assert.Equal(t, "My Task", state.Issues["task-01"].Title)
}

func TestMaterializeAndReturn_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	// No ops dir — should return empty state
	state, result, err := MaterializeAndReturn(dir, filepath.Join(dir, "state"), false)
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

func TestRunRollup_CascadesToEpic(t *testing.T) {
	// epic-01 → story-01 → task-01; when task-01 is merged, both story and epic should cascade-merge.
	// This exercises the parent-decrement path at engine.go:371-380.
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "epic-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "Epic", NodeType: "epic"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "Story", NodeType: "story", Parent: "epic-01"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 102,
		WorkerID: "w1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "story-01"}}))

	// Mark task merged directly (simulating single-branch done → merged)
	state.Issues["task-01"].Status = ops.StatusMerged

	state.RunRollup()
	assert.Equal(t, "merged", state.Issues["story-01"].Status, "story should cascade-merge when all tasks merged")
	assert.Equal(t, "merged", state.Issues["epic-01"].Status, "epic should cascade-merge when all stories merged")
}

func TestApplyUnlinkOp_BlockedByRel(t *testing.T) {
	// Create two linked tasks then unlink them — exercises applyUnlink (engine.go:184, 445)
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "A", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "B", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpLink, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "blocked_by"}}))
	require.Contains(t, state.Issues["task-01"].BlockedBy, "task-02")

	// Unlink: task-01 is no longer blocked_by task-02
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpUnlink, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "blocked_by"}}))
	assert.NotContains(t, state.Issues["task-01"].BlockedBy, "task-02")
	assert.NotContains(t, state.Issues["task-02"].Blocks, "task-01")
}

func TestApplyUnlinkOp_NonBlockedByRel_NoOp(t *testing.T) {
	// Unlink with a rel other than "blocked_by" should be a no-op (engine.go:184 negation path)
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "A", NodeType: "task"}}))
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "B", NodeType: "task"}}))
	// Link with "blocked_by" first
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpLink, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "blocked_by"}}))
	// Unlink with a different rel — should not remove the blocked_by relationship
	require.NoError(t, state.ApplyOp(ops.Op{Type: ops.OpUnlink, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "relates-to"}}))
	assert.Contains(t, state.Issues["task-01"].BlockedBy, "task-02", "blocked_by not removed for non-blocked_by unlink")
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

func TestApplyAmendOp_PatchesType(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "S1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Story", NodeType: "story"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "S1", Timestamp: 200, WorkerID: "w2",
		Payload: ops.Payload{NodeType: "epic"},
	}))
	assert.Equal(t, "epic", state.Issues["S1"].Type)
}

func TestApplyAmendOp_PatchesAcceptance(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Task", NodeType: "task"},
	}))
	acceptance := json.RawMessage(`[{"type":"test_passes","cmd":"make check"}]`)
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "T1", Timestamp: 200, WorkerID: "w2",
		Payload: ops.Payload{Acceptance: acceptance},
	}))
	assert.NotEmpty(t, state.Issues["T1"].Acceptance)
	assert.Equal(t, string(acceptance), string(state.Issues["T1"].Acceptance))
}

func TestApplyAmendOp_PatchesScope(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "T1", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "Task", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "T1", Timestamp: 200, WorkerID: "w2",
		Payload: ops.Payload{Scope: []string{"internal/**"}},
	}))
	assert.Equal(t, []string{"internal/**"}, state.Issues["T1"].Scope)
}

func TestApplyAmendOp_UnknownIssue_NoError(t *testing.T) {
	state := NewState()
	err := state.ApplyOp(ops.Op{
		Type: ops.OpAmend, TargetID: "NONEXISTENT", Timestamp: 100,
		Payload: ops.Payload{NodeType: "epic"},
	})
	assert.NoError(t, err)
}

func TestApplyCitationAccepted(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCitationAccepted, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{ConfirmedNoninteractively: true},
	}))
	issue := state.Issues["task-01"]
	require.Len(t, issue.CitationAcceptances, 1)
	assert.Equal(t, "w1", issue.CitationAcceptances[0].WorkerID)
	assert.Equal(t, int64(200), issue.CitationAcceptances[0].Timestamp)
	assert.True(t, issue.CitationAcceptances[0].ConfirmedNoninteractively)
	assert.Equal(t, int64(200), issue.Updated)
}

func TestApplyCitationAccepted_UnknownIssue_NoError(t *testing.T) {
	state := NewState()
	err := state.ApplyOp(ops.Op{
		Type: ops.OpCitationAccepted, TargetID: "NONEXISTENT", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{ConfirmedNoninteractively: false},
	})
	assert.NoError(t, err)
}

func TestToTraceabilityRefs_PopulatesCitationAcceptanceCount(t *testing.T) {
	issues := map[string]*Issue{
		"task-01": {
			ID: "task-01",
			CitationAcceptances: []CitationAcceptance{
				{WorkerID: "w1", Timestamp: 100},
				{WorkerID: "w2", Timestamp: 200},
			},
		},
		"task-02": {
			ID:                  "task-02",
			CitationAcceptances: nil,
		},
	}

	refs := toTraceabilityRefs(issues)

	refsByID := make(map[string]interface{})
	for _, r := range refs {
		refsByID[r.ID] = r
	}

	require.Len(t, refs, 2)

	for _, r := range refs {
		switch r.ID {
		case "task-01":
			assert.Equal(t, 2, r.CitationAcceptanceCount)
		case "task-02":
			assert.Equal(t, 0, r.CitationAcceptanceCount)
		}
	}
	_ = refsByID
}

// BenchmarkRunRollup_10kIssues benchmarks the rollup operation on a large hierarchy.
// This test demonstrates that RunRollup should complete in O(n) time.
// With the previous O(n²) implementation, 10k issues would take too long.
func BenchmarkRunRollup_10kIssues(b *testing.B) {
	state := NewState()
	state.SingleBranchMode = true

	// Create a 3-level hierarchy: 1 epic -> 100 stories -> 100 tasks per story
	// Total: ~10,101 issues
	timestamp := int64(100)

	// Create epic
	epicID := "epic-0"
	require.NoError(b, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: epicID, Timestamp: timestamp, WorkerID: "w1",
		Payload: ops.Payload{Title: "Epic", NodeType: "epic"},
	}))
	timestamp++

	// Create stories under epic
	storyIDs := make([]string, 100)
	for i := 0; i < 100; i++ {
		storyID := "story-" + string(rune('0'+i/10)) + string(rune('0'+i%10))
		storyIDs[i] = storyID
		require.NoError(b, state.ApplyOp(ops.Op{
			Type: ops.OpCreate, TargetID: storyID, Timestamp: timestamp, WorkerID: "w1",
			Payload: ops.Payload{Title: "Story " + string(rune('0'+i/10)) + string(rune('0'+i%10)), NodeType: "story", Parent: epicID},
		}))
		timestamp++
	}

	// Create tasks under each story
	taskIDs := make([][]string, 100)
	for si := 0; si < 100; si++ {
		taskIDs[si] = make([]string, 100)
		for ti := 0; ti < 100; ti++ {
			taskID := "task-" + string(rune('0'+si/10)) + string(rune('0'+si%10)) + "-" + string(rune('0'+ti/10)) + string(rune('0'+ti%10))
			taskIDs[si][ti] = taskID
			require.NoError(b, state.ApplyOp(ops.Op{
				Type: ops.OpCreate, TargetID: taskID, Timestamp: timestamp, WorkerID: "w1",
				Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: storyIDs[si]},
			}))
			timestamp++
		}
	}

	// Mark all tasks as done, which becomes merged in single branch mode
	for si := 0; si < 100; si++ {
		for ti := 0; ti < 100; ti++ {
			taskID := taskIDs[si][ti]
			require.NoError(b, state.ApplyOp(ops.Op{
				Type: ops.OpClaim, TargetID: taskID, Timestamp: timestamp, WorkerID: "w1",
				Payload: ops.Payload{TTL: 60},
			}))
			timestamp++
			require.NoError(b, state.ApplyOp(ops.Op{
				Type: ops.OpTransition, TargetID: taskID, Timestamp: timestamp, WorkerID: "w1",
				Payload: ops.Payload{To: "done"},
			}))
			timestamp++
		}
	}

	// Now run the benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.RunRollup()
	}
	b.StopTimer()

	// Verify that the epic is merged (all children promoted)
	if state.Issues[epicID].Status != "merged" {
		b.Fatalf("epic should be merged after rollup, got %s", state.Issues[epicID].Status)
	}
}
