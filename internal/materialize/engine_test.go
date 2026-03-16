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
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}})
	issue := state.Issues["task-01"]
	assert.Equal(t, "claimed", issue.Status)
	assert.Equal(t, "w1", issue.ClaimedBy)
	assert.Equal(t, int64(200), issue.ClaimedAt)
}

func TestApplyTransitionOp(t *testing.T) {
	state := NewState()
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}})
	state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "Fixed it"}})
	issue := state.Issues["task-01"]
	assert.Equal(t, "done", issue.Status)
	assert.Equal(t, "Fixed it", issue.Outcome)
}

func TestApplyNoteOp(t *testing.T) {
	state := NewState()
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpNote, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Msg: "Found edge case"}})
	assert.Len(t, state.Issues["task-01"].Notes, 1)
	assert.Equal(t, "Found edge case", state.Issues["task-01"].Notes[0].Msg)
}

func TestApplyLinkOp(t *testing.T) {
	state := NewState()
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "A", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 101,
		WorkerID: "w1", Payload: ops.Payload{Title: "B", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpLink, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Dep: "task-02", Rel: "blocked_by"}})
	assert.Contains(t, state.Issues["task-01"].BlockedBy, "task-02")
	assert.Contains(t, state.Issues["task-02"].Blocks, "task-01")
}

func TestApplyDecisionOp_LastWriteWins(t *testing.T) {
	state := NewState()
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpDecision, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{Topic: "db", Choice: "postgres", Rationale: "mature"}})
	state.ApplyOp(ops.Op{Type: ops.OpDecision, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w2", Payload: ops.Payload{Topic: "db", Choice: "sqlite", Rationale: "simpler"}})
	decisions := state.Issues["task-01"].Decisions
	active := activeDecisionForTopic(decisions, "db")
	assert.Equal(t, "sqlite", active.Choice)
}

func TestSingleBranchAutoMerge(t *testing.T) {
	state := NewState()
	state.SingleBranchMode = true
	state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100,
		WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})
	state.ApplyOp(ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "w1", Payload: ops.Payload{TTL: 60}})
	state.ApplyOp(ops.Op{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 300,
		WorkerID: "w1", Payload: ops.Payload{To: "done", Outcome: "Done"}})
	assert.Equal(t, "merged", state.Issues["task-01"].Status)
}

func TestMaterializePipeline(t *testing.T) {
	dir := t.TempDir()
	opsDir := filepath.Join(dir, "ops")
	stateDir := filepath.Join(dir, "state")
	issuesDir := filepath.Join(stateDir, "issues")
	os.MkdirAll(opsDir, 0755)
	os.MkdirAll(issuesDir, 0755)

	logPath := filepath.Join(opsDir, "worker-a1.log")
	ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "epic-01", Timestamp: 100,
		WorkerID: "worker-a1", Payload: ops.Payload{Title: "Epic", NodeType: "epic"}})
	ops.AppendOp(logPath, ops.Op{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101,
		WorkerID: "worker-a1", Payload: ops.Payload{Title: "Task", NodeType: "task", Parent: "epic-01"}})
	ops.AppendOp(logPath, ops.Op{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 200,
		WorkerID: "worker-a1", Payload: ops.Payload{TTL: 60}})

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

			state.ApplyOp(ops.Op{Type: ops.OpCreate, TargetID: targetID, Timestamp: ts,
				WorkerID: "w1", Payload: ops.Payload{Title: "T", NodeType: "task"}})

			state.ApplyOp(ops.Op{Type: opType, TargetID: targetID, Timestamp: ts + 1,
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

			state.ApplyOp(op)
			state.ApplyOp(op)

			return len(state.Issues) == 1 && state.Issues[id].Title == "T"
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
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
