package ops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendAndReadGateEvidence_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker.log")

	ev := GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Start:   10,
		End:     12,
		Exit:    0,
	}
	require.NoError(t, AppendGateEvidence(logPath, "worker-1", ev))

	got, err := ReadGateEvidence(logPath)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, ev, got[0])
	assert.True(t, got[0].Citable())
}

func TestIsAuditOnly_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	assert.True(t, IsAuditOnly(OpSourceFingerprint))
	assert.True(t, IsAuditOnly(OpGateEvidence))
	assert.False(t, IsAuditOnly(OpNote))
	assert.False(t, IsAuditOnly(OpCreate))
}

func TestGateEvidenceDirtyNotCitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	ev := GateEvidence{Profile: "full", Exit: 0, Uncommitted: true}
	assert.False(t, ev.Citable())
}

type fakeCommitter struct {
	rel     string
	message string
	err     error
}

func (f *fakeCommitter) CommitWorktreeOp(relPath, message string) error {
	f.rel = relPath
	f.message = message
	return f.err
}

func testEvidence() GateEvidence {
	return GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Start:   1,
		End:     2,
		Exit:    0,
	}
}

func TestAppendGateEvidenceAndCommit_NoWorktreeSkipsCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker.log")
	fc := &fakeCommitter{}
	require.NoError(t, AppendGateEvidenceAndCommit(logPath, "", "worker-1", testEvidence(), fc))
	assert.Empty(t, fc.message)
	got, err := ReadGateEvidence(logPath)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestAppendGateEvidenceAndCommit_TruncatesLongWorkerID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	worktree := filepath.Join(dir, "ops-wt")
	logPath := filepath.Join(worktree, "ops", "worker.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	fc := &fakeCommitter{}
	require.NoError(t, AppendGateEvidenceAndCommit(logPath, worktree, "abcdefghij", testEvidence(), fc))
	assert.Contains(t, fc.message, "abcdefgh")
	assert.NotContains(t, fc.message, "abcdefghij")
	assert.Equal(t, filepath.Join("ops", "worker.log"), fc.rel)
}

func TestAppendGateEvidenceAndCommit_ShortWorkerID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	worktree := filepath.Join(dir, "ops-wt")
	logPath := filepath.Join(worktree, "ops", "w.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	fc := &fakeCommitter{}
	require.NoError(t, AppendGateEvidenceAndCommit(logPath, worktree, "abcd", testEvidence(), fc))
	assert.Contains(t, fc.message, "abcd")
}

func TestAppendGateEvidenceAndCommit_AppendError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := AppendGateEvidenceAndCommit(dir, "", "worker-1", testEvidence(), nil)
	require.Error(t, err)
}

func TestReadAllGateEvidence_Success(t *testing.T) {
	t.Parallel()
	opsDir := t.TempDir()
	require.NoError(t, AppendGateEvidence(filepath.Join(opsDir, "a.log"), "w1", testEvidence()))
	got, err := ReadAllGateEvidence(opsDir)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "full", got[0].Profile)
}

func TestReadAllGateEvidence_ListError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
	_, err := ReadAllGateEvidence(path)
	require.Error(t, err)
}

func TestReadGateEvidence_InvalidPayloadErrors_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker.log")
	require.NoError(t, os.WriteFile(logPath, []byte(`["gate-evidence","full",1,"w","not-an-object"]`+"\n"), 0o644))
	_, err := ReadGateEvidence(logPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestReadGateEvidence_UnrelatedCorruptSkipped_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker.log")
	require.NoError(t, os.WriteFile(logPath, []byte("not-json\n"), 0o644))
	require.NoError(t, AppendGateEvidence(logPath, "w1", testEvidence()))
	got, err := ReadGateEvidence(logPath)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestAppendGateEvidenceConcurrent_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker.log")

	errCh := make(chan error, 2)
	for i := range 2 {
		go func(i int) {
			ev := testEvidence()
			ev.Start = int64(i + 1)
			ev.End = int64(i + 2)
			errCh <- AppendGateEvidence(logPath, "worker-1", ev)
		}(i)
	}
	for range 2 {
		require.NoError(t, <-errCh)
	}
	got, err := ReadGateEvidence(logPath)
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestReadAllGateEvidence_ReadError(t *testing.T) {
	t.Parallel()
	opsDir := t.TempDir()
	logPath := filepath.Join(opsDir, "blocked.log")
	require.NoError(t, os.WriteFile(logPath, []byte("x"), 0o000))
	_, err := ReadAllGateEvidence(opsDir)
	require.Error(t, err)
}
