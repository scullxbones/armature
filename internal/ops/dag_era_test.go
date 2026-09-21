package ops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDAGEra_REQ_MATENC_S1_T6(t *testing.T) {
	t.Parallel()

	t.Run("legacy confirm payload is DAGEraLegacy", func(t *testing.T) {
		t.Parallel()
		op := Op{
			Type: OpDAGTransition, TargetID: "task-01",
			Payload: Payload{Confirmed: true},
		}
		got := DecodeDAGMode(op)
		assert.Equal(t, DAGEraLegacy, got.Era)
		assert.Equal(t, "task-01", got.RootID)
		assert.True(t, got.Confirmed)
		assert.Empty(t, got.Confidence)
	})

	t.Run("IssueID classifies as DAGEraCanonical", func(t *testing.T) {
		t.Parallel()
		op := Op{
			Type: OpDAGTransition, TargetID: "epic-01",
			Payload: Payload{IssueID: "epic-01", To: "verified"},
		}
		got := DecodeDAGMode(op)
		assert.Equal(t, DAGEraCanonical, got.Era)
		assert.Equal(t, "epic-01", got.RootID)
		assert.Equal(t, "verified", got.Confidence)
		assert.False(t, got.Confirmed)
	})

	t.Run("canonical empty To defaults to verified at decode", func(t *testing.T) {
		t.Parallel()
		got := DecodeDAGMode(Op{
			Type: OpDAGTransition, TargetID: "epic-01",
			Payload: Payload{IssueID: "epic-01"},
		})
		assert.Equal(t, DAGEraCanonical, got.Era)
		assert.Equal(t, "verified", got.Confidence)
	})

	t.Run("canonical To draft is preserved", func(t *testing.T) {
		t.Parallel()
		got := DecodeDAGMode(Op{
			Type: OpDAGTransition, TargetID: "epic-01",
			Payload: Payload{IssueID: "epic-01", To: "draft"},
		})
		assert.Equal(t, DAGEraCanonical, got.Era)
		assert.Equal(t, "draft", got.Confidence)
	})

	t.Run("To is not the era discriminator", func(t *testing.T) {
		t.Parallel()
		got := DecodeDAGMode(Op{
			Type: OpDAGTransition, TargetID: "task-01",
			Payload: Payload{To: "verified"},
		})
		assert.Equal(t, DAGEraLegacy, got.Era, "payload.To without IssueID is still arm-confirm era")
		assert.Equal(t, "task-01", got.RootID)
		assert.Empty(t, got.Confidence)
	})

	t.Run("IssueID wins when Confirmed is also set", func(t *testing.T) {
		t.Parallel()
		got := DecodeDAGMode(Op{
			Type: OpDAGTransition, TargetID: "task-01",
			Payload: Payload{IssueID: "task-01", Confirmed: true, To: "verified"},
		})
		assert.Equal(t, DAGEraCanonical, got.Era)
		assert.False(t, got.Confirmed, "canonical era does not carry the confirm flag")
		assert.Equal(t, "verified", got.Confidence)
	})

	t.Run("canonical RootID is IssueID not TargetID", func(t *testing.T) {
		t.Parallel()
		got := DecodeDAGMode(Op{
			Type: OpDAGTransition, TargetID: "task-01",
			Payload: Payload{IssueID: "epic-01"},
		})
		assert.Equal(t, DAGEraCanonical, got.Era)
		assert.Equal(t, "epic-01", got.RootID)
	})

	t.Run("ParseLine does not mutate payload or rewrite JSONL", func(t *testing.T) {
		t.Parallel()
		line := []byte(`["dag-transition","task-01",1,"w1",{"confirmed":true}]`)
		op, err := ParseLine(line)
		require.NoError(t, err)
		assert.True(t, op.Payload.Confirmed)
		assert.Empty(t, op.Payload.IssueID)

		got := DecodeDAGMode(op)
		assert.Equal(t, DAGEraLegacy, got.Era)
		assert.True(t, op.Payload.Confirmed)
		assert.Empty(t, op.Payload.IssueID)

		marshaled, err := MarshalOp(op)
		require.NoError(t, err)
		roundTrip, err := ParseLine(marshaled)
		require.NoError(t, err)
		assert.Equal(t, op.Payload, roundTrip.Payload)
		assert.True(t, roundTrip.Payload.Confirmed)
		assert.Empty(t, roundTrip.Payload.IssueID)
	})
}
