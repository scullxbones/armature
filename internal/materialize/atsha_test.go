package materialize

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/git"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initAtSHATestRepo creates a temp dir with a git repo configured for testing.
func initAtSHATestRepo(t *testing.T) (string, *git.Client) {
	t.Helper()
	dir := t.TempDir()
	gitRun := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
		return strings.TrimSpace(string(out))
	}
	gitRun("init")
	gitRun("config", "user.email", "test@test.com")
	gitRun("config", "user.name", "Test")
	gitRun("config", "commit.gpgsign", "false")
	gitRun("commit", "--allow-empty", "-m", "init")
	return dir, git.New(dir)
}

// captureHEAD returns the current HEAD SHA of the repo at dir.
func captureHEAD(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

// writeAndCommitOp writes a JSONL op file to opsPrefix/<workerID>.log and commits it.
func writeAndCommitOp(t *testing.T, dir, opsPrefix string, op ops.Op) string {
	t.Helper()
	opsDir := filepath.Join(dir, opsPrefix)
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	line, err := ops.MarshalOp(op)
	require.NoError(t, err)

	logFile := filepath.Join(opsDir, op.WorkerID+".log")
	content := append(line, '\n')
	require.NoError(t, os.WriteFile(logFile, content, 0644))

	gitRun := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("add", opsPrefix+"/"+op.WorkerID+".log")
	gitRun("commit", "-m", "add op for "+op.TargetID)

	return captureHEAD(t, dir)
}

// TestMaterializeAtSHA_CreateOp verifies that a single create op is materialized correctly.
func TestMaterializeAtSHA_CreateOp(t *testing.T) {
	dir, gc := initAtSHATestRepo(t)

	op := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "E1-T1",
		Timestamp: 1000,
		WorkerID:  "test-worker-id",
		Payload: ops.Payload{
			NodeType: "task",
			Title:    "Test task",
		},
	}
	sha := writeAndCommitOp(t, dir, "ops", op)

	state, err := MaterializeAtSHA(gc, sha, "ops")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Len(t, state.Issues, 1)
	issue, ok := state.Issues["E1-T1"]
	require.True(t, ok, "expected issue E1-T1 in state")
	assert.Equal(t, "Test task", issue.Title)
	assert.Equal(t, "task", issue.Type)
}

// TestMaterializeAtSHA_BeforeOpsAdded verifies that materializing at the init commit
// (before any ops) returns an empty state.
func TestMaterializeAtSHA_BeforeOpsAdded(t *testing.T) {
	dir, gc := initAtSHATestRepo(t)
	// Capture the init SHA before adding any ops.
	initSHA := captureHEAD(t, dir)

	// Now add an op in a subsequent commit.
	op := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "E1-T1",
		Timestamp: 1000,
		WorkerID:  "test-worker-id",
		Payload: ops.Payload{
			NodeType: "task",
			Title:    "Test task",
		},
	}
	writeAndCommitOp(t, dir, "ops", op)

	// Materializing at initSHA (before op was committed) should give empty state.
	state, err := MaterializeAtSHA(gc, initSHA, "ops")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Empty(t, state.Issues)
}

// TestMaterializeAtSHA_InvalidSHA verifies that an invalid SHA returns an error.
func TestMaterializeAtSHA_InvalidSHA(t *testing.T) {
	_, gc := initAtSHATestRepo(t)

	_, err := MaterializeAtSHA(gc, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "ops")
	assert.Error(t, err)
}

// TestMaterializeAtSHA_MultipleWorkers verifies that log files from multiple workers
// are all merged correctly into the final state.
func TestMaterializeAtSHA_MultipleWorkers(t *testing.T) {
	dir, gc := initAtSHATestRepo(t)

	opsDir := filepath.Join(dir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	op1 := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "E1-T1",
		Timestamp: 1000,
		WorkerID:  "worker-alpha",
		Payload:   ops.Payload{NodeType: "task", Title: "Alpha task"},
	}
	op2 := ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "E1-T2",
		Timestamp: 2000,
		WorkerID:  "worker-beta",
		Payload:   ops.Payload{NodeType: "task", Title: "Beta task"},
	}

	line1, err := ops.MarshalOp(op1)
	require.NoError(t, err)
	line2, err := ops.MarshalOp(op2)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(opsDir, "worker-alpha.log"), append(line1, '\n'), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(opsDir, "worker-beta.log"), append(line2, '\n'), 0644))

	gitRun := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("add", "ops/worker-alpha.log", "ops/worker-beta.log")
	gitRun("commit", "-m", "add two worker ops")
	sha := captureHEAD(t, dir)

	state, err := MaterializeAtSHA(gc, sha, "ops")
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Len(t, state.Issues, 2)
	assert.Contains(t, state.Issues, "E1-T1")
	assert.Contains(t, state.Issues, "E1-T2")
}
