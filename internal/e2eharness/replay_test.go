package e2eharness_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/require"
)

// TestMaterializationConvergesAfterInterruptedAppend_REQ_TOPTIER_S3_T2 proves
// that every possible torn-write point in an op append is delimited by the
// production append path. Once the writer retries the complete op, replay
// converges to the same state as an uninterrupted append without truncating the
// append-only log tail.
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

	baseline, _, _ := replayState(t, nil, create, transition)
	for interruptedAt := 0; interruptedAt <= len(encodedCreate); interruptedAt++ {
		t.Run("write-point-"+strconv.Itoa(interruptedAt), func(t *testing.T) {
			t.Parallel()

			// Write the exact bytes a killed append can leave behind: there is no
			// synthetic newline. AppendOp must delimit that tail before retrying.
			torn := append([]byte{}, encodedCreate[:interruptedAt]...)
			actual, logBytes, opCount := replayState(t, torn, create, transition)
			assertAppendOnlyTail(t, torn, logBytes)
			// A complete JSON value without its delimiter is already durable. Its
			// retry must be recognized so non-idempotent ops cannot be duplicated.
			require.Equal(t, 2, opCount)
			require.Equal(t, baseline, actual)
		})
	}
}

// TestMaterializationConvergesAfterDelimiterCrash_REQ_TOPTIER_S3_T2 proves
// that retrying after recovery has written a delimiter does not append a second
// copy of the operation. Scope rename is deliberately non-idempotent for this
// input: replaying it twice would turn "src" into "src22".
func TestMaterializationConvergesAfterDelimiterCrash_REQ_TOPTIER_S3_T2(t *testing.T) {
	t.Parallel()

	create := ops.Op{
		Type: ops.OpCreate, TargetID: "REPLAY-001", Timestamp: 100, WorkerID: "worker-a",
		Payload: ops.Payload{
			NodeType: "task", Title: "replay scenario", DefinitionOfDone: "recover",
			Scope: []string{"src"},
		},
	}
	rename := ops.Op{
		Type: ops.OpScopeRename, TargetID: "REPLAY-001", Timestamp: 101, WorkerID: "worker-a",
		Payload: ops.Payload{OldPath: "src", NewPath: "src2"},
	}
	transition := ops.Op{
		Type: ops.OpTransition, TargetID: "REPLAY-001", Timestamp: 102, WorkerID: "worker-a",
		Payload: ops.Payload{To: ops.StatusDone, Outcome: "recovered after delimiter crash"},
	}

	baseline, _, _ := replayState(t, nil, create, rename, transition)
	encodedCreate, err := ops.MarshalOp(create)
	require.NoError(t, err)
	encodedRename, err := ops.MarshalOp(rename)
	require.NoError(t, err)

	// This is the durable state if the first recovery delimits the complete
	// rename tail and then crashes before it can return. The next retry must
	// recognize that newline-terminated final record as an exact retry.
	delimiterCrashPrefix := append(append(append([]byte{}, encodedCreate...), '\n'), encodedRename...)
	delimiterCrashPrefix = append(delimiterCrashPrefix, '\n')
	actual, logBytes, opCount := replayState(t, delimiterCrashPrefix, rename, transition)

	require.True(t, bytes.HasPrefix(logBytes, delimiterCrashPrefix), "recovery must preserve the delimiter-crash tail")
	require.Equal(t, 3, opCount, "retry must not append a second scope rename")
	require.Equal(t, baseline, actual)
	require.Equal(t, []string{"src2"}, actual.Scope, "a duplicate rename would yield src22")
}

func assertAppendOnlyTail(t *testing.T, torn, logBytes []byte) {
	t.Helper()
	require.True(t, bytes.HasPrefix(logBytes, torn), "recovery must preserve the interrupted tail")
	if len(torn) > 0 {
		require.Equal(t, byte('\n'), logBytes[len(torn)], "recovery must delimit rather than join the retry")
	}
}

func replayState(t *testing.T, tornPrefix []byte, recovered ...ops.Op) (*materialize.Issue, []byte, int) {
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
	logBytes, err := os.ReadFile(logPath)
	require.NoError(t, err)
	return issue, logBytes, len(allOps)
}
