package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeedsPromotionEnrichment_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	assert.True(t, needsPromotionEnrichment(promotion.Node{Status: ops.StatusDone}))
	assert.True(t, needsPromotionEnrichment(promotion.Node{Status: ops.StatusClaimed}))
	assert.True(t, needsPromotionEnrichment(promotion.Node{Status: ops.StatusInProgress}))
	assert.True(t, needsPromotionEnrichment(promotion.Node{Status: ops.StatusBlocked}))
	assert.False(t, needsPromotionEnrichment(promotion.Node{Status: ops.StatusOpen}))
	assert.False(t, needsPromotionEnrichment(promotion.Node{Status: ops.StatusMerged}))
	assert.True(t, needsPromotionEnrichment(promotion.Node{Status: ops.StatusOpen, ClaimedBy: "w"}))
}

// TestNodeFromMaterializeIssue_LastActivityMax_REQ_coverage_determinism locks the
// LastHeartbeat / LastClaimingWorkerActivity max branches. Without a dedicated
// caller, parallel suite scheduling left promotion_facts.go:60 (LastHeartbeat >
// ClaimedAt) covered on some check-core runs and uncovered on others → cmd
// coverage flipped 83.76% ↔ 83.78% on an identical tree (#426/#432 quote flakes).
func TestNodeFromMaterializeIssue_LastActivityMax_REQ_coverage_determinism(t *testing.T) {
	t.Parallel()
	issue := &materialize.Issue{
		Status:                     ops.StatusClaimed,
		ClaimedAt:                  1000,
		LastHeartbeat:              1500,
		LastClaimingWorkerActivity: 1600,
		ClaimedBy:                  "w1",
		ClaimToken:                 "tok",
		ClaimTTL:                   60,
		Parent:                     "P",
		Children:                   []string{"C"},
		Base:                       "b",
		Tip:                        "t",
	}
	n := nodeFromMaterializeIssue(issue)
	assert.Equal(t, int64(1600), n.LastActivity,
		"LastActivity must be max(ClaimedAt, LastHeartbeat, LastClaimingWorkerActivity)")
	assert.Equal(t, "w1", n.ClaimedBy)
	assert.Equal(t, "tok", n.ClaimToken)
	assert.Equal(t, 60, n.TTLMinutes)

	issue2 := *issue
	issue2.LastClaimingWorkerActivity = 1400
	n2 := nodeFromMaterializeIssue(&issue2)
	assert.Equal(t, int64(1500), n2.LastActivity)

	issue3 := *issue
	issue3.LastHeartbeat = 900
	issue3.LastClaimingWorkerActivity = 800
	n3 := nodeFromMaterializeIssue(&issue3)
	assert.Equal(t, int64(1000), n3.LastActivity)
}

func TestPromotionGraphCache_Memoizes_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	index := materialize.Index{
		"A": {Status: ops.StatusOpen},
		"B": {Status: ops.StatusDone, Parent: "A"},
	}
	c := newPromotionGraphCache(nil, index)
	g1, err1 := c.Graph()
	require.NoError(t, err1)
	g2, err2 := c.Graph()
	require.NoError(t, err2)
	assert.Equal(t, g1, g2)
	assert.Equal(t, ops.StatusDone, g1["B"].Status)
}

func TestPromotionGraphFromStore_NilStore_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	index := materialize.Index{"X": {Status: ops.StatusOpen}}
	g, err := promotionGraphFromStore(nil, index)
	require.NoError(t, err)
	assert.Equal(t, ops.StatusOpen, g["X"].Status)
}

func TestPromotionGraphCache_NilCache_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	var c *promotionGraphCache
	_, err := c.Graph()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil cache")
}

func TestPromotionGraphFromStore_ReadIssueError_FailClosed_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	stateDir := t.TempDir()
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))
	// needsPromotionEnrichment(done)==true → ReadIssue called
	index := materialize.Index{
		"P": {Status: ops.StatusMerged, Type: "story", Children: []string{"T"}},
		"T": {Status: ops.StatusDone, Type: "task", Parent: "P"},
	}
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "T.json"), []byte("{not-json"), 0o644))
	store := snapshot.NewStore(t.TempDir() /* unused ops */, stateDir)
	g, err := promotionGraphFromStore(store, index)
	require.Error(t, err)
	assert.Nil(t, g, "fail-closed: must not return a partial/empty graph")
	assert.Contains(t, err.Error(), "promotion graph: read issue T:")

	c := newPromotionGraphCache(store, index)
	_, err1 := c.Graph()
	_, err2 := c.Graph()
	require.Error(t, err1)
	assert.Equal(t, err1.Error(), err2.Error(), "memoized error")
}
