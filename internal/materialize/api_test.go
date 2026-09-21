package materialize

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

		seed, err := MaterializeCold(allOps[:3])
		require.NoError(t, err)
		require.Equal(t, ops.StatusMerged, seed.Issues["story-01"].Status)
		require.Equal(t, ops.StatusOpen, seed.Issues["story-01"].RollupStatusBefore)

		cached := cloneMaterializeState(t, seed)
		require.NoError(t, MaterializeIncremental(cached, allOps))

		cold, err := MaterializeCold(allOps)
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

		seed, err := MaterializeCold(allOps)
		require.NoError(t, err)
		cached := cloneMaterializeState(t, seed)
		delete(cached.Issues, "task-02")
		_, stillMissing := cached.Issues["task-02"]
		require.False(t, stillMissing, "precondition: cache omitted task-02")

		require.NoError(t, MaterializeIncremental(cached, allOps))
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
		seed, err := MaterializeCold(seedOps)
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

		cold, err := MaterializeCold([]ops.Op{
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

	t.Run("nil state is an error", func(t *testing.T) {
		t.Parallel()
		err := MaterializeIncremental(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MaterializeIncremental: state is nil")

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
