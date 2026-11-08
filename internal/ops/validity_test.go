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
	assert.False(t, AffectsValidity(OpLogQuarantine))
	affects, classified := ClassifiedValidity("not-a-real-op")
	assert.False(t, classified)
	assert.False(t, affects)
	assert.True(t, AffectsValidity("not-a-real-op"), "unclassified op types must fail-closed as validity-affecting")
	require.NotEmpty(t, classifiedValidity)
}

func TestExternalRefNotValidityAffecting_REQ_HOSTREF_S1_T1(t *testing.T) {
	t.Parallel()

	assert.False(t, AffectsValidity(OpExternalRef), "external-ref must be classified beside assign as not validity-affecting")
	affects, classified := ClassifiedValidity(OpExternalRef)
	assert.True(t, classified, "external-ref must be present in classifiedValidity; unclassified types fail closed as affecting")
	assert.False(t, affects)

	assignAffects, assignClassified := ClassifiedValidity(OpAssign)
	assert.True(t, assignClassified)
	assert.False(t, assignAffects)
	assert.Equal(t, assignAffects, affects, "external-ref must sit beside assign: both not validity-affecting")

	schema := GenerateSchema()
	assert.Contains(t, schema, OpExternalRef)
	assert.Contains(t, schema, "external_ref")
	assert.Contains(t, schema, "external_role")
	assert.Contains(t, schema, "withdraw_external_ref")
	assert.NotContains(t, schema, "approval")
	assert.NotContains(t, schema, " body")
}
