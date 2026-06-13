package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test 1: empty dir → returns zero-value Snapshot (no error)
func TestLoad_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	assert.NotNil(t, snap)
	assert.NotNil(t, snap.State)
	assert.NotNil(t, snap.Index)
	assert.NotNil(t, snap.Issues)
	assert.Equal(t, 0, len(snap.Issues))
	assert.Equal(t, 0, len(snap.Warnings))
}

// Test 2: single issue → Snapshot has 1 issue, State populated
func TestLoad_SingleIssue(t *testing.T) {
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	workerID := "test-worker"
	logPath := filepath.Join(opsDir, workerID+".log")

	// Op format is a JSON array: [type, target_id, timestamp, worker_id, payload]
	opLine := `["create","issue-1",1000,"test-worker",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0644))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	assert.NotNil(t, snap)
	assert.Equal(t, 1, len(snap.Issues))
	assert.NotNil(t, snap.Issues["issue-1"])
	assert.Equal(t, "issue-1", snap.Issues["issue-1"].ID)
	assert.Equal(t, "Test Issue", snap.Issues["issue-1"].Title)
}

// Test 3: worker-ID mismatch warning → Warnings slice contains warning about mismatched worker IDs
func TestLoad_WorkerIDMismatchWarning(t *testing.T) {
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	// File named "alice.log" but ops are from worker "bob"
	logPath := filepath.Join(opsDir, "alice.log")
	opLine := `["create","issue-1",1000,"bob",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0644))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	// The op should be rejected due to worker ID mismatch, resulting in a warning
	assert.NotNil(t, snap)
	assert.Equal(t, 0, len(snap.Issues))
	assert.Greater(t, len(snap.Warnings), 0)
	found := false
	for _, w := range snap.Warnings {
		if containsAny(w, "worker", "mismatch") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected warning about worker ID mismatch, got: %v", snap.Warnings)
}

// Test 4: state+issues agreement → Snapshot.State and Snapshot.Issues are consistent
func TestLoad_StateAndIssuesAgreement(t *testing.T) {
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	workerID := "worker1"
	logPath := filepath.Join(opsDir, workerID+".log")

	op1 := `["create","parent-task",1000,"worker1",{"title":"Parent Task","type":"task","scope":[],"context_files":[]}]`
	op2 := `["create","child-task",1001,"worker1",{"title":"Child Task","parent":"parent-task","type":"task","scope":[],"context_files":[]}]`
	content := op1 + "\n" + op2 + "\n"
	require.NoError(t, adapters.WriteFile(logPath, []byte(content), 0644))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	assert.Equal(t, 2, len(snap.Issues))
	assert.NotNil(t, snap.Issues["parent-task"])
	assert.NotNil(t, snap.Issues["child-task"])

	assert.Equal(t, 2, len(snap.State.Issues))
	assert.NotNil(t, snap.State.Issues["parent-task"])
	assert.NotNil(t, snap.State.Issues["child-task"])

	parent := snap.Issues["parent-task"]
	child := snap.Issues["child-task"]
	assert.Equal(t, "parent-task", child.Parent)
	assert.Contains(t, parent.Children, "child-task")

	stateParent := snap.State.Issues["parent-task"]
	assert.Equal(t, parent.ID, stateParent.ID)
	assert.Equal(t, parent.Title, stateParent.Title)
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
