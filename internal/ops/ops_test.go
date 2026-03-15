package ops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
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

func TestPropOpRoundTrip(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 200

	properties := gopter.NewProperties(params)

	opTypes := gen.OneConstOf(OpCreate, OpClaim, OpHeartbeat, OpTransition,
		OpNote, OpLink, OpSourceLink, OpSourceFingerprint, OpDAGTransition, OpDecision)

	properties.Property("marshal then parse preserves type, target, timestamp, worker", prop.ForAll(
		func(opType string, targetID string, ts int64, workerID string) bool {
			if targetID == "" || workerID == "" {
				return true // skip empty — not valid ops
			}
			op := Op{
				Type:      opType,
				TargetID:  targetID,
				Timestamp: ts,
				WorkerID:  workerID,
			}
			data, err := MarshalOp(op)
			if err != nil {
				return false
			}
			parsed, err := ParseLine(data)
			if err != nil {
				return false
			}
			return parsed.Type == op.Type &&
				parsed.TargetID == op.TargetID &&
				parsed.Timestamp == op.Timestamp &&
				parsed.WorkerID == op.WorkerID
		},
		opTypes,
		gen.AlphaString(),
		gen.Int64(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

func TestLogAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "Test task", NodeType: "task"}}
	op2 := Op{Type: OpClaim, TargetID: "task-01", Timestamp: 101, WorkerID: "worker-a1",
		Payload: Payload{TTL: 60}}

	require.NoError(t, AppendOp(logPath, op1))
	require.NoError(t, AppendOp(logPath, op2))

	ops, err := ReadLog(logPath)
	require.NoError(t, err)
	assert.Len(t, ops, 2)
	assert.Equal(t, OpCreate, ops[0].Type)
	assert.Equal(t, OpClaim, ops[1].Type)
}

func TestReadLogFromOffset(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "First", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op1))

	// Get current offset
	info, _ := os.Stat(logPath)
	offset := info.Size()

	op2 := Op{Type: OpNote, TargetID: "task-01", Timestamp: 200, WorkerID: "worker-a1",
		Payload: Payload{Msg: "Second"}}
	require.NoError(t, AppendOp(logPath, op2))

	ops, err := ReadLogFromOffset(logPath, offset)
	require.NoError(t, err)
	assert.Len(t, ops, 1)
	assert.Equal(t, "Second", ops[0].Payload.Msg)
}

func TestValidateWorkerIDInLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	// Op with wrong worker ID
	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b2",
		Payload: Payload{Title: "Bad", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op))

	ops, err := ReadLogValidated(logPath, "worker-a1")
	require.NoError(t, err)
	assert.Len(t, ops, 0) // rejected — worker ID mismatch
}
