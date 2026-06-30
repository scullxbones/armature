package ops_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

// RunGitCommitterContract validates that an implementation of GitCommitter
// correctly handles committing worktree operations.
// Call this from any test that provides a real or test GitCommitter implementation.
func RunGitCommitterContract(t *testing.T, committer ops.GitCommitter) {
	t.Run("CommitWorktreeOp_ReturnsNoErrorOnSuccess", func(t *testing.T) {
		t.Parallel()
		err := committer.CommitWorktreeOp("path/to/file.txt", "test commit message")
		assert.NoError(t, err)
	})

	t.Run("CommitWorktreeOp_AcceptsMultiplePaths", func(t *testing.T) {
		t.Parallel()
		err1 := committer.CommitWorktreeOp("file1.txt", "first commit")
		err2 := committer.CommitWorktreeOp("file2.txt", "second commit")
		assert.NoError(t, err1)
		assert.NoError(t, err2)
	})

	t.Run("CommitWorktreeOp_AcceptsVaryingMessageFormats", func(t *testing.T) {
		t.Parallel()
		messages := []string{
			"ops: claim T1 by abc",
			"ops: transition T2 by def",
			"ops: note T3 by ghi",
		}
		for _, msg := range messages {
			err := committer.CommitWorktreeOp("test.log", msg)
			assert.NoError(t, err)
		}
	})
}

// FakeCommitter is a test fake that implements GitCommitter.
// It records all calls for inspection in tests.
type FakeCommitter struct {
	Calls []struct {
		RelPath string
		Message string
	}
	Err error
}

func (f *FakeCommitter) CommitWorktreeOp(relPath, message string) error {
	f.Calls = append(f.Calls, struct {
		RelPath string
		Message string
	}{relPath, message})
	return f.Err
}

// TestFakeCommitter_SatisfiesContract ensures that FakeCommitter
// correctly implements the GitCommitter interface and satisfies its contract.
func TestFakeCommitter_SatisfiesContract(t *testing.T) {
	t.Parallel()
	fc := &FakeCommitter{}
	RunGitCommitterContract(t, fc)
}
