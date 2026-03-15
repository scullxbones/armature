package ops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCreateOp(t *testing.T) {
	line := `["create","task-01",1740700800,"worker-a1",{"title":"Fix auth","parent":"epic-1","type":"task","scope":["src/auth/**"],"acceptance":[]}]`

	op, err := ParseLine([]byte(line))
	require.NoError(t, err)
	assert.Equal(t, OpCreate, op.Type)
	assert.Equal(t, "task-01", op.TargetID)
	assert.Equal(t, int64(1740700800), op.Timestamp)
	assert.Equal(t, "worker-a1", op.WorkerID)
	assert.Equal(t, "Fix auth", op.Payload.Title)
	assert.Equal(t, "epic-1", op.Payload.Parent)
}

func TestParseClaimOp(t *testing.T) {
	line := `["claim","task-01",1740700801,"worker-a1",{"ttl":60}]`

	op, err := ParseLine([]byte(line))
	require.NoError(t, err)
	assert.Equal(t, OpClaim, op.Type)
	assert.Equal(t, 60, op.Payload.TTL)
}

func TestMarshalOp(t *testing.T) {
	op := Op{
		Type:      OpCreate,
		TargetID:  "task-01",
		Timestamp: 1740700800,
		WorkerID:  "worker-a1",
		Payload: Payload{
			Title:    "Fix auth",
			Parent:   "epic-1",
			NodeType: "task",
			Scope:    []string{"src/auth/**"},
		},
	}

	line, err := MarshalOp(op)
	require.NoError(t, err)

	// Round-trip
	parsed, err := ParseLine(line)
	require.NoError(t, err)
	assert.Equal(t, op.Type, parsed.Type)
	assert.Equal(t, op.TargetID, parsed.TargetID)
	assert.Equal(t, op.Payload.Title, parsed.Payload.Title)
}

func TestParseInvalidLine(t *testing.T) {
	_, err := ParseLine([]byte(`not json`))
	assert.Error(t, err)

	_, err = ParseLine([]byte(`["unknown","x",0,"w",{}]`))
	assert.Error(t, err)
}
