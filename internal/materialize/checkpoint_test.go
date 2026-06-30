package materialize

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.json")

	cp := Checkpoint{
		LastCommitSHA: "abc123",
		ByteOffsets:   map[string]int64{"worker-a1.log": 1024, "worker-b2.log": 512},
	}

	require.NoError(t, WriteCheckpoint(path, cp))

	loaded, err := LoadCheckpoint(path)
	require.NoError(t, err)
	assert.Equal(t, "abc123", loaded.LastCommitSHA)
	assert.Equal(t, int64(1024), loaded.ByteOffsets["worker-a1.log"])
}

func TestLoadCheckpoint_Missing(t *testing.T) {
	cp, err := LoadCheckpoint("/nonexistent/checkpoint.json")
	require.NoError(t, err) // missing checkpoint = fresh start
	assert.Equal(t, "", cp.LastCommitSHA)
	assert.NotNil(t, cp.ByteOffsets)
}
