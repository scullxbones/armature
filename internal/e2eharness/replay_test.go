package e2eharness_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/require"
)

// TestMaterializationConvergesAfterInterruptedAppend_REQ_TOPTIER_S3_T2 proves
// that every possible torn-write point in an op append is ignored on recovery;
// once the writer retries the complete op, replay converges to the same state as
// an uninterrupted append.
func TestMaterializationConvergesAfterInterruptedAppend_REQ_TOPTIER_S3_T2(t *testing.T) {
	t.Parallel()

	create := ops.Op{
		Type: ops.OpCreate, TargetID: "REPLAY-001", Timestamp: 100, WorkerID: "worker-a",
		Payload: ops.Payload{NodeType: "task", Title: "replay scenario", DefinitionOfDone: "recover"},
	}
	transition := ops.Op{
		Type: ops.OpTransition, TargetID: "REPLAY-001", Timestamp: 101, WorkerID: "worker-a",
		Payload: ops.Payload{To: ops.StatusDone, Outcome: "recovered after interruption"},
	}
	encodedCreate, err := ops.MarshalOp(create)
	require.NoError(t, err)

	baseline := replayState(t, []byte{}, create, transition)
	for interruptedAt := 0; interruptedAt < len(encodedCreate); interruptedAt++ {
		t.Run("write-point-"+strconv.Itoa(interruptedAt), func(t *testing.T) {
			t.Parallel()

			// A recovery newline delimits the torn record, so the parser rejects only
			// that record instead of joining it to the writer's retry.
			torn := append(append([]byte{}, encodedCreate[:interruptedAt]...), '\n')
			actual := replayState(t, torn, create, transition)
			require.Equal(t, baseline, actual)
		})
	}
}

func replayState(t *testing.T, tornPrefix []byte, recovered ...ops.Op) *materialize.Issue {
	t.Helper()
	root := t.TempDir()
	logPath := filepath.Join(root, "ops", "worker-a.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o750))
	if len(tornPrefix) > 0 {
		require.NoError(t, os.WriteFile(logPath, tornPrefix, 0o600))
	}
	for _, op := range recovered {
		require.NoError(t, ops.AppendOp(logPath, op))
	}
	allOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	state, _, err := materialize.MaterializeAndReturnQuiet(filepath.Join(root, "state"), allOps, map[string]int64{"worker-a.log": 1})
	require.NoError(t, err)
	issue := state.Issues["REPLAY-001"]
	require.NotNil(t, issue)
	return issue
}
