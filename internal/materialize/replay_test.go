package materialize

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/require"
)

type appendPhase int

const (
	appendPhaseComplete appendPhase = iota
	appendPhaseTornRecord
	appendPhasePendingMarker
)

type appendCrash struct {
	phase        appendPhase
	logPrefix    []byte
	pending      []byte
	pendingStart int64
}

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

	baseline, _, _ := replayState(t, appendCrash{phase: appendPhaseComplete}, create, transition)
	for interruptedAt := 0; interruptedAt <= len(encodedCreate); interruptedAt++ {
		t.Run("write-point-"+strconv.Itoa(interruptedAt), func(t *testing.T) {
			t.Parallel()

			torn := append([]byte{}, encodedCreate[:interruptedAt]...)
			actual, logBytes, opCount := replayState(t, appendCrash{
				phase:     appendPhaseTornRecord,
				logPrefix: torn,
			}, create, transition)
			assertAppendOnlyTail(t, torn, logBytes)
			require.Equal(t, 2, opCount)
			require.Equal(t, baseline, actual)
		})
	}
}

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

	baseline, _, _ := replayState(t, appendCrash{phase: appendPhaseComplete}, create, rename, transition)
	encodedCreate, err := ops.MarshalOp(create)
	require.NoError(t, err)
	encodedRename, err := ops.MarshalOp(rename)
	require.NoError(t, err)

	delimiterCrashPrefix := append(append(append([]byte{}, encodedCreate...), '\n'), encodedRename...)
	delimiterCrashPrefix = append(delimiterCrashPrefix, '\n')
	pendingRename := append(append([]byte{}, encodedRename...), '\n')
	actual, logBytes, opCount := replayState(t, appendCrash{
		phase:        appendPhasePendingMarker,
		logPrefix:    delimiterCrashPrefix,
		pending:      pendingRename,
		pendingStart: int64(len(encodedCreate) + 1),
	}, rename, transition)

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

func replayState(t *testing.T, crash appendCrash, recovered ...ops.Op) (*Issue, []byte, int) {
	t.Helper()
	root := t.TempDir()
	logPath := filepath.Join(root, "ops", "worker-a.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o750))
	if len(crash.logPrefix) > 0 {
		require.NoError(t, os.WriteFile(logPath, crash.logPrefix, 0o600))
	}
	if crash.phase == appendPhasePendingMarker {
		require.NoError(t, adapters.SimulatePendingMarker(logPath, crash.pendingStart, crash.pending))
	}
	for _, op := range recovered {
		require.NoError(t, ops.AppendOp(logPath, op))
	}
	allOps, err := ops.ReadLog(logPath)
	require.NoError(t, err)
	state, _, err := materializeQuiet(filepath.Join(root, "state"), allOps, map[string]int64{"worker-a.log": 1})
	require.NoError(t, err)
	issue := state.Issues["REPLAY-001"]
	require.NotNil(t, issue)
	logBytes, err := os.ReadFile(logPath)
	require.NoError(t, err)
	return issue, logBytes, len(allOps)
}
