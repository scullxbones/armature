package ops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAffectsValidity_KnownTypes(t *testing.T) {
	t.Parallel()
	assert.True(t, AffectsValidity(OpCreate))
	assert.True(t, AffectsValidity(OpAmend))
	assert.True(t, AffectsValidity(OpLink))
	assert.True(t, AffectsValidity(OpDecision))
	assert.True(t, AffectsValidity(OpTransition))
	assert.False(t, AffectsValidity(OpClaim))
	assert.False(t, AffectsValidity(OpHeartbeat))
	assert.False(t, AffectsValidity(OpNote))
	assert.False(t, AffectsValidity(OpGateEvidence))
	affects, classified := ClassifiedValidity("not-a-real-op")
	assert.False(t, classified)
	assert.False(t, affects)
}

func TestClassifiedOpTypes_NonEmpty(t *testing.T) {
	t.Parallel()
	require.NotEmpty(t, ClassifiedOpTypes())
}
