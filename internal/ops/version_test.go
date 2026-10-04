package ops

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpsVersionRejectsNewer_REQ_TOPTIER_S6_T2(t *testing.T) {
	t.Parallel()

	newer := CurrentSchemaVersion + 1
	line := fmt.Sprintf(
		`["create","FUTURE-1",1000,"worker-a",{"title":"future","type":"task"},%d]`,
		newer,
	)
	_, err := ParseLine([]byte(line))
	require.Error(t, err, "older binary must fail loudly on newer ops")
	assert.Contains(t, err.Error(), "newer than supported")
	assert.Contains(t, err.Error(), fmt.Sprintf("%d", newer))
	assert.Error(t, CheckSchemaVersion(newer))

	legacy := []byte(`["create","LEGACY-1",1000,"worker-a",{"title":"legacy","type":"task"}]`)
	op, err := ParseLine(legacy)
	require.NoError(t, err, "pre-version 5-element ops are v1")
	assert.Equal(t, LegacySchemaVersion, op.SchemaVersion)

	current := fmt.Sprintf(
		`["create","V1-1",1000,"worker-a",{"title":"v1","type":"task"},%d]`,
		CurrentSchemaVersion,
	)
	op, err = ParseLine([]byte(current))
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, op.SchemaVersion)

	marshaled, err := MarshalOp(Op{
		Type: OpCreate, TargetID: "V1-2", Timestamp: 1001, WorkerID: "worker-a",
		Payload: Payload{Title: "written", NodeType: "task"},
	})
	require.NoError(t, err)
	assert.Contains(t, string(marshaled), fmt.Sprintf(",%d]", CurrentSchemaVersion))
	roundTrip, err := ParseLine(marshaled)
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, roundTrip.SchemaVersion)

	assert.Equal(t, CurrentSchemaVersion, EffectiveSchemaVersion(0))
	assert.Equal(t, 2, EffectiveSchemaVersion(2))
	assert.Error(t, CheckSchemaVersion(0))
	assert.NoError(t, CheckSchemaVersion(CurrentSchemaVersion))
}
