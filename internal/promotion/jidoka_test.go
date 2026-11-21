package promotion_test

import (
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActiveStopsTripAndClear_REQ_482(t *testing.T) {
	t.Parallel()
	history := []ops.PromotionStop{
		{Action: ops.PromotionStopTrip, TreeRoot: "epic-01", IssueID: "task-01", Kind: "missing-assessment", PR: "1", Step: "land:missing-assessment", At: 1, Seq: 0},
		{Action: ops.PromotionStopClear, TreeRoot: "epic-01", NoteID: "n1", At: 2, Seq: 1},
		{Action: ops.PromotionStopTrip, TreeRoot: "other", IssueID: "x", Kind: "check-failed", PR: "2", Step: "land:check-failed", At: 3, Seq: 2},
	}
	active := promotion.ActiveStops(history)
	require.Len(t, active, 1)
	assert.Equal(t, "other", active[0].TreeRoot)
}

func TestActiveStops_OrdersByAtNotFilename_REQ_482(t *testing.T) {
	t.Parallel()

	t.Run("clear@200 earlier-named log beats trip@100 later-named", func(t *testing.T) {
		t.Parallel()
		history := []ops.PromotionStop{
			{Action: ops.PromotionStopClear, TreeRoot: "t1", At: 200, Seq: 0},
			{Action: ops.PromotionStopTrip, TreeRoot: "t1", IssueID: "t1", Kind: "missing-assessment", At: 100, Seq: 1},
		}
		assert.Empty(t, promotion.ActiveStops(history))
	})

	t.Run("clear@200 later Seq still beats trip@100 earlier Seq", func(t *testing.T) {
		t.Parallel()
		history := []ops.PromotionStop{
			{Action: ops.PromotionStopTrip, TreeRoot: "t1", IssueID: "t1", Kind: "missing-assessment", At: 100, Seq: 0},
			{Action: ops.PromotionStopClear, TreeRoot: "t1", At: 200, Seq: 1},
		}
		assert.Empty(t, promotion.ActiveStops(history))
	})

	t.Run("same-second tie uses Seq", func(t *testing.T) {
		t.Parallel()
		cleared := []ops.PromotionStop{
			{Action: ops.PromotionStopTrip, TreeRoot: "t1", IssueID: "t1", Kind: "k", At: 50, Seq: 0},
			{Action: ops.PromotionStopClear, TreeRoot: "t1", At: 50, Seq: 1},
		}
		assert.Empty(t, promotion.ActiveStops(cleared))

		armed := []ops.PromotionStop{
			{Action: ops.PromotionStopClear, TreeRoot: "t1", At: 50, Seq: 0},
			{Action: ops.PromotionStopTrip, TreeRoot: "t1", IssueID: "t1", Kind: "k", At: 50, Seq: 1},
		}
		active := promotion.ActiveStops(armed)
		require.Len(t, active, 1)
		assert.Equal(t, "t1", active[0].TreeRoot)
	})
}

func TestCheckReady_JidokaStopTreeOnly_REQ_482(t *testing.T) {
	t.Parallel()
	base, tip := "base1", "tip1"
	g := promotion.Graph{
		"epic-01":   {Children: []string{"task-01"}},
		"task-01":   {Parent: "epic-01"},
		"unrelated": {},
	}
	stops := promotion.ActiveStops([]ops.PromotionStop{{
		Action:   ops.PromotionStopTrip,
		TreeRoot: "epic-01",
		IssueID:  "task-01",
		PR:       "99",
		Step:     "land:missing-assessment",
		Kind:     "missing-assessment",
	}})
	issue := promotion.ReadyIssue{
		DefinitionOfDone: "ship",
		Acceptance:       json.RawMessage(`["ok"]`),
		Assessments:      []promotion.AssessmentFact{{BaseSHA: base, HeadSHA: tip}},
	}
	err := promotion.CheckReady(promotion.ReadyInput{
		BaseSHA:  base,
		HeadSHA:  tip,
		IssueIDs: []string{"task-01"},
		Issues: map[string]promotion.ReadyIssue{
			"task-01": issue,
		},
		GateEvidence: []ops.GateEvidence{ciFullEvidence(tip)},
		Stops:        stops,
		Graph:        g,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jidoka stop")
	assert.Contains(t, err.Error(), "epic-01")

	err = promotion.CheckReady(promotion.ReadyInput{
		BaseSHA:  base,
		HeadSHA:  tip,
		IssueIDs: []string{"unrelated"},
		Issues: map[string]promotion.ReadyIssue{
			"unrelated": issue,
		},
		GateEvidence: []ops.GateEvidence{ciFullEvidence(tip)},
		Stops:        stops,
		Graph:        g,
	})
	require.NoError(t, err, "unrelated tree must still pass")
}

func TestPlanTrip_AndonThenDogfood_REQ_482(t *testing.T) {
	t.Parallel()
	g := promotion.Graph{"task-01": {}}
	first := promotion.PlanTrip(g, nil, "task-01", "missing-assessment")
	assert.True(t, first.OpenAndon)
	assert.False(t, first.OpenDogfood)

	history := []ops.PromotionStop{{
		Action:   ops.PromotionStopTrip,
		TreeRoot: "task-01",
		Kind:     "missing-assessment",
	}}
	second := promotion.PlanTrip(g, history, "task-01", "missing-assessment")
	assert.False(t, second.OpenAndon)
	assert.True(t, second.OpenDogfood)
}
