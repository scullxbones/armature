package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCreateOp(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	line := `["claim","task-01",1740700801,"worker-a1",{"ttl":60}]`

	op, err := ParseLine([]byte(line))
	require.NoError(t, err)
	assert.Equal(t, OpClaim, op.Type)
	assert.Equal(t, 60, op.Payload.TTL)
}

func TestMarshalOp(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	_, err := ParseLine([]byte(`not json`))
	assert.Error(t, err)

	// Unknown op types are now allowed at parse time; the engine validates them.
	_, err = ParseLine([]byte(`["unknown","x",0,"w",{}]`))
	assert.NoError(t, err)
}

func TestPropOpRoundTrip(t *testing.T) {
	t.Parallel()
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 200

	properties := gopter.NewProperties(params)

	opTypes := gen.OneConstOf(OpCreate, OpClaim, OpHeartbeat, OpTransition,
		OpNote, OpLink, OpSourceLink, OpSourceFingerprint, OpDAGTransition, OpDecision, OpAssign)

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
	t.Parallel()
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
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	op1 := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-a1",
		Payload: Payload{Title: "First", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op1))

	// Get current offset
	info, err := os.Stat(logPath)
	require.NoError(t, err)
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
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-a1.log")

	// Op with wrong worker ID
	op := Op{Type: OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "worker-b2",
		Payload: Payload{Title: "Bad", NodeType: "task"}}
	require.NoError(t, AppendOp(logPath, op))

	stream := NewValidatedOpStream()
	stream.AddFile(logPath, "worker-a1")
	items, warnings, err := stream.Load()
	require.NoError(t, err)
	assert.Len(t, items, 0) // rejected — worker ID mismatch
	assert.NotEmpty(t, warnings)
}

func TestGenerateSchema(t *testing.T) {
	t.Parallel()
	schema := GenerateSchema()
	assert.Contains(t, schema, "op_type")
	assert.Contains(t, schema, "target_id")
	assert.Contains(t, schema, "timestamp")
	assert.Contains(t, schema, "worker_id")
	assert.Contains(t, schema, "payload")
}

func TestGenerateSchema_DocumentsEveryRegisteredOpType(t *testing.T) {
	t.Parallel()
	schema := GenerateSchema()
	documentedTypes := SchemaDocumentedOpTypes()

	// Convert to a set for fast lookup
	documentedSet := make(map[string]bool, len(documentedTypes))
	for _, opType := range documentedTypes {
		documentedSet[opType] = true
	}

	// All registered op types must be documented in the schema
	requiredOpTypes := []string{
		OpCreate,
		OpClaim,
		OpHeartbeat,
		OpTransition,
		OpNote,
		OpNoteDelete,
		OpLink,
		OpUnlink,
		OpSourceLink,
		OpSourceFingerprint,
		OpDAGTransition,
		OpDecision,
		OpAssign,
		OpAmend,
		OpCitationAccepted,
		OpScopeRename,
		OpScopeDelete,
		OpReparent,
		OpAssessmentAttested,
		OpGateEvidence,
	}

	for _, opType := range requiredOpTypes {
		assert.True(t, documentedSet[opType], "op type %q must be documented in schema", opType)
		assert.Contains(t, schema, opType, "op type %q must appear in generated schema", opType)
	}
}

func TestGenerateSchema_DocumentsClaimFields_REQ_LNGHZN_S5_T9(t *testing.T) {
	t.Parallel()
	schema := GenerateSchema()

	claimHeaderPrefix := "#   " + OpClaim + ":"
	transitionHeaderPrefix := "#   " + OpTransition + ":"

	var claimLine string
	inTransitionBlock := false
	transitionHasIfClaimToken := false

	for _, line := range strings.Split(schema, "\n") {
		switch {
		case strings.HasPrefix(line, claimHeaderPrefix):
			claimLine = line
			inTransitionBlock = false
		case strings.HasPrefix(line, transitionHeaderPrefix):
			inTransitionBlock = true
		case strings.HasPrefix(line, "#   ") && strings.Contains(line, ":"):
			// A different op's header line ends the transition block.
			inTransitionBlock = false
		}
		if inTransitionBlock && strings.Contains(line, "if_claim_token") {
			transitionHasIfClaimToken = true
		}
	}

	assert.Contains(t, claimLine, "claim_token", "claim op's schema line must document the claim_token field")
	assert.True(t, transitionHasIfClaimToken, "transition op must document the if_claim_token field")
}

func TestHeartbeatRateLimiter(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter()

	// First heartbeat should be allowed
	assert.True(t, rl.AllowHeartbeat("task-01", 1000))

	// Heartbeat within 60 seconds should be rejected
	assert.False(t, rl.AllowHeartbeat("task-01", 1030))

	// Heartbeat after 60 seconds should be allowed
	assert.True(t, rl.AllowHeartbeat("task-01", 1061))

	// Different task should be independent
	assert.True(t, rl.AllowHeartbeat("task-02", 1030))
}

func TestCreateRateLimiter(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter()

	for i := 0; i < 500; i++ {
		assert.True(t, rl.AllowCreate())
	}
	assert.False(t, rl.AllowCreate()) // 501st should fail

	rl.ResetCreateCount() // simulate commit boundary
	assert.True(t, rl.AllowCreate())
}

// TestReadLogFromOffset_ManyOps verifies that ReadLogFromOffset correctly reads
// a log containing many ops. This also exercises the pre-allocated slice path.
func TestReadLogFromOffset_ManyOps(t *testing.T) {
	t.Parallel()
	const count = 200
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker-many.log")

	for i := 0; i < count; i++ {
		op := Op{
			Type:      OpNote,
			TargetID:  "task-01",
			Timestamp: int64(i + 1),
			WorkerID:  "worker-many",
			Payload:   Payload{Msg: "note"},
		}
		require.NoError(t, AppendOp(logPath, op))
	}

	result, err := ReadLogFromOffset(logPath, 0)
	require.NoError(t, err)
	assert.Len(t, result, count)
}
