package ops

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendAndReadPromotionStop_REQ_482(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker.log")

	stop := PromotionStop{
		Action:   PromotionStopTrip,
		PR:       "99",
		IssueID:  "task-01",
		TreeRoot: "epic-01",
		Step:     "land:missing-assessment",
		Kind:     "missing-assessment",
		AndonURL: "https://github.com/scullxbones/armature/issues/1000",
		At:       42,
	}
	require.NoError(t, AppendPromotionStop(logPath, "worker-1", stop))

	got, err := readPromotionStops(logPath)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, stop, got[0])

	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)
	var arr []json.RawMessage
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(raw), &arr))
	require.GreaterOrEqual(t, len(arr), 6)
	var version int
	require.NoError(t, json.Unmarshal(arr[5], &version))
	assert.Equal(t, CurrentSchemaVersion, version)

	parsed, err := ParseLine(bytes.TrimSpace(raw))
	require.NoError(t, err)
	assert.Equal(t, OpPromotionStop, parsed.Type)
	assert.Equal(t, "epic-01", parsed.TargetID)
}

func TestIsAuditOnly_PromotionStop_REQ_482(t *testing.T) {
	t.Parallel()
	assert.True(t, IsAuditOnly(OpPromotionStop))
	assert.False(t, AffectsValidity(OpPromotionStop))
}
