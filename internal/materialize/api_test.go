package materialize

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func materializeCold(allOps []ops.Op) (*State, error) {
	state := NewState()
	if err := ApplyOpsSorted(state, allOps); err != nil {
		return nil, err
	}
	return state, nil
}

func materializeIncremental(state *State, allOps []ops.Op) error {
	if state == nil {
		return fmt.Errorf("materializeIncremental: state is nil")
	}
	*state = *NewState()
	return ApplyOpsSorted(state, allOps)
}

func TestMaterializeIncremental_REQ_MATENC_S1_T7(t *testing.T) {
	t.Parallel()

	t.Run("cold matches incremental after reclaim", func(t *testing.T) {
		t.Parallel()
		allOps := []ops.Op{
			{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100, WorkerID: "w1",
				Payload: ops.Payload{Title: "Story", NodeType: "story"}},
			{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101, WorkerID: "w1",
				Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}},
			{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 102, WorkerID: "w1",
				Payload: ops.Payload{To: ops.StatusMerged}},
			{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 103, WorkerID: "w1",
				Payload: ops.Payload{To: ops.StatusOpen}},
			{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 104, WorkerID: "w1",
				Payload: ops.Payload{TTL: 60}},
		}

		seed, err := materializeCold(allOps[:3])
		require.NoError(t, err)
		require.Equal(t, ops.StatusMerged, seed.Issues["story-01"].Status)
		require.Equal(t, ops.StatusOpen, seed.Issues["story-01"].RollupStatusBefore)

		cached := cloneMaterializeState(t, seed)
		require.NoError(t, materializeIncremental(cached, allOps))

		cold, err := materializeCold(allOps)
		require.NoError(t, err)
		require.Equal(t, ops.StatusInProgress, cold.Issues["story-01"].Status)

		assert.Equal(t, cold.Issues["story-01"].Status, cached.Issues["story-01"].Status)
		assert.Equal(t, cold.Issues["task-01"].Status, cached.Issues["task-01"].Status)
		assert.Empty(t, cached.Issues["story-01"].RollupStatusBefore)
	})

	t.Run("incremental full-replays the log not a delta", func(t *testing.T) {
		t.Parallel()
		allOps := []ops.Op{
			{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100, WorkerID: "w1",
				Payload: ops.Payload{Title: "Story", NodeType: "story"}},
			{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101, WorkerID: "w1",
				Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}},
			{Type: ops.OpCreate, TargetID: "task-02", Timestamp: 102, WorkerID: "w1",
				Payload: ops.Payload{Title: "Task B", NodeType: "task", Parent: "story-01"}},
			{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 103, WorkerID: "w1",
				Payload: ops.Payload{To: ops.StatusMerged}},
			{Type: ops.OpTransition, TargetID: "task-02", Timestamp: 104, WorkerID: "w1",
				Payload: ops.Payload{To: ops.StatusMerged}},
		}

		seed, err := materializeCold(allOps)
		require.NoError(t, err)
		cached := cloneMaterializeState(t, seed)
		delete(cached.Issues, "task-02")
		_, stillMissing := cached.Issues["task-02"]
		require.False(t, stillMissing, "precondition: cache omitted task-02")

		require.NoError(t, materializeIncremental(cached, allOps))
		_, recovered := cached.Issues["task-02"]
		assert.True(t, recovered, "full replay must recreate issues absent from the snapshot")
		assert.Equal(t, ops.StatusMerged, cached.Issues["story-01"].Status)
	})

	t.Run("ApplyOpsSorted retracts before replay", func(t *testing.T) {
		t.Parallel()
		seedOps := []ops.Op{
			{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100, WorkerID: "w1",
				Payload: ops.Payload{Title: "Story", NodeType: "story"}},
			{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101, WorkerID: "w1",
				Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}},
			{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 102, WorkerID: "w1",
				Payload: ops.Payload{To: ops.StatusMerged}},
		}
		seed, err := materializeCold(seedOps)
		require.NoError(t, err)
		require.Equal(t, ops.StatusMerged, seed.Issues["story-01"].Status)

		projected := cloneMaterializeState(t, seed)
		err = ApplyOpsSorted(projected, []ops.Op{
			{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 103, WorkerID: "w1",
				Payload: ops.Payload{To: ops.StatusOpen}},
			{Type: ops.OpClaim, TargetID: "task-01", Timestamp: 104, WorkerID: "w1",
				Payload: ops.Payload{TTL: 60}},
		})
		require.NoError(t, err)
		assert.Equal(t, ops.StatusInProgress, projected.Issues["story-01"].Status,
			"retract must run before claim so promoteParentToInProgress can fire")
	})

	t.Run("RollupStatusBefore is retained not Asserted/Derived", func(t *testing.T) {
		t.Parallel()
		issueType := reflect.TypeOf(Issue{})
		_, hasAsserted := issueType.FieldByName("Asserted")
		_, hasDerived := issueType.FieldByName("Derived")
		assert.False(t, hasAsserted)
		assert.False(t, hasDerived)
		_, hasBefore := issueType.FieldByName("RollupStatusBefore")
		assert.True(t, hasBefore)

		cold, err := materializeCold([]ops.Op{
			{Type: ops.OpCreate, TargetID: "story-01", Timestamp: 100, WorkerID: "w1",
				Payload: ops.Payload{Title: "Story", NodeType: "story"}},
			{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 101, WorkerID: "w1",
				Payload: ops.Payload{Title: "Task A", NodeType: "task", Parent: "story-01"}},
			{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 102, WorkerID: "w1",
				Payload: ops.Payload{To: ops.StatusMerged}},
		})
		require.NoError(t, err)
		story := cold.Issues["story-01"]
		require.Equal(t, ops.StatusMerged, story.Status)
		require.Equal(t, ops.StatusOpen, story.RollupStatusBefore)

		raw, err := json.Marshal(story)
		require.NoError(t, err)
		var asMap map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &asMap))
		assert.Contains(t, asMap, "rollup_status_before")
		assert.NotContains(t, asMap, "asserted")
		assert.NotContains(t, asMap, "derived")
	})

	t.Run("second incremental of same log does not duplicate PriorOutcomes", func(t *testing.T) {
		t.Parallel()
		allOps := []ops.Op{
			{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
				Payload: ops.Payload{Title: "Task", NodeType: "task"}},
			{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 101, WorkerID: "w1",
				Payload: ops.Payload{To: ops.StatusDone, Outcome: "first"}},
			{Type: ops.OpTransition, TargetID: "task-01", Timestamp: 102, WorkerID: "w1",
				Payload: ops.Payload{To: ops.StatusOpen}},
		}

		cold, err := materializeCold(allOps)
		require.NoError(t, err)
		require.Equal(t, []string{"first"}, cold.Issues["task-01"].PriorOutcomes)

		cached := cloneMaterializeState(t, cold)
		require.NoError(t, materializeIncremental(cached, allOps))
		require.NoError(t, materializeIncremental(cached, allOps))
		assert.Equal(t, []string{"first"}, cached.Issues["task-01"].PriorOutcomes)
		assert.Equal(t, cold.Issues["task-01"].PriorOutcomes, cached.Issues["task-01"].PriorOutcomes)
		assert.Equal(t, ops.StatusOpen, cached.Issues["task-01"].Status)
		assert.Empty(t, cached.Issues["task-01"].Outcome)
	})

	t.Run("nil state is an error", func(t *testing.T) {
		t.Parallel()
		err := materializeIncremental(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "materializeIncremental: state is nil")

		err = ApplyOpsSorted(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ApplyOpsSorted: state is nil")
	})
}

func cloneMaterializeState(t *testing.T, src *State) *State {
	t.Helper()
	raw, err := json.Marshal(src)
	require.NoError(t, err)
	dst := NewState()
	require.NoError(t, json.Unmarshal(raw, dst))
	return dst
}
