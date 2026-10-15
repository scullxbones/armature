package ops_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
)

func RunGitCommitterContract(t *testing.T, newCommitter func() ops.GitCommitter) {
	t.Helper()
	t.Run("CommitWorktreeOp_ReturnsNoErrorOnSuccess", func(t *testing.T) {
		t.Parallel()
		err := newCommitter().CommitWorktreeOp("path/to/file.txt", "test commit message")
		assert.NoError(t, err)
	})

	t.Run("CommitWorktreeOp_AcceptsMultiplePaths", func(t *testing.T) {
		t.Parallel()
		c := newCommitter()
		err1 := c.CommitWorktreeOp("file1.txt", "first commit")
		err2 := c.CommitWorktreeOp("file2.txt", "second commit")
		assert.NoError(t, err1)
		assert.NoError(t, err2)
	})

	t.Run("CommitWorktreeOp_AcceptsVaryingMessageFormats", func(t *testing.T) {
		t.Parallel()
		c := newCommitter()
		messages := []string{
			"ops: claim T1 by abc",
			"ops: transition T2 by def",
			"ops: note T3 by ghi",
		}
		for _, msg := range messages {
			err := c.CommitWorktreeOp("test.log", msg)
			assert.NoError(t, err)
		}
	})
}

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

func TestFakeCommitter_SatisfiesContract(t *testing.T) {
	t.Parallel()
	RunGitCommitterContract(t, func() ops.GitCommitter { return &FakeCommitter{} })
}
