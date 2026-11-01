package main

import (
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
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
