package materialize

import (
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeHistory struct {
	files    []string
	contents map[string][]byte
}

func (f fakeHistory) ListFilesAtCommit(string) ([]string, error) {
	return f.files, nil
}

func (f fakeHistory) ShowFileAtCommit(_ string, path string) ([]byte, error) {
	return f.contents[path], nil
}

func TestMaterializeAtSHAUsesHistoryPortInsteadOfConcreteAdapter(t *testing.T) {
	t.Parallel()
	line, err := ops.MarshalOp(ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "TASK-001",
		Timestamp: 1700000000,
		WorkerID:  "worker-a",
		Payload: ops.Payload{
			Title:    "T",
			NodeType: "task",
		},
	})
	require.NoError(t, err)

	history := fakeHistory{
		files: []string{".armature/ops/worker-a.log"},
		contents: map[string][]byte{
			".armature/ops/worker-a.log": append(line, '\n'),
		},
	}

	state, err := MaterializeAtSHA(history, "abc123", ".armature/ops")

	require.NoError(t, err)
	assert.Contains(t, state.BuildIndex(), "TASK-001")
}
