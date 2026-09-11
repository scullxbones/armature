package ops

import (
	"fmt"
	"path/filepath"
	"strings"
)

// GitCommitter is the interface for committing a file change in a worktree.
type GitCommitter interface {
	CommitWorktreeOp(relPath, message string) error
}

// AppendAndCommit appends op to logPath and, if worktreePath is non-empty,
// commits the log file to the worktree's branch via gc.
// Pass worktreePath="" (and gc=nil) for single-branch mode — commit is skipped.
func AppendAndCommit(logPath, worktreePath string, op Op, gc GitCommitter) error {
	_, err := AppendAndCommitIf(logPath, worktreePath, op, gc, nil)
	return err
}

// AppendAndCommitIf appends op unless proceed returns false while the
// per-log lock is held. A skipped append does not commit. A nil proceed
// always writes.
func AppendAndCommitIf(logPath, worktreePath string, op Op, gc GitCommitter, proceed func() (bool, error)) (bool, error) {
	wrote, err := AppendOpIf(logPath, op, proceed)
	if err != nil || !wrote {
		return wrote, err
	}
	if worktreePath == "" {
		return true, nil // single-branch: no git commit needed
	}

	relPath, err := filepath.Rel(worktreePath, logPath)
	if err != nil {
		return false, fmt.Errorf("resolve relative log path: %w", err)
	}

	// Safely truncate WorkerID to at most 8 chars for the commit message
	workerPrefix := op.WorkerID
	if len(workerPrefix) > 8 {
		workerPrefix = workerPrefix[:8]
	}

	message := fmt.Sprintf("ops: %s %s by %s", strings.ToLower(op.Type), op.TargetID, workerPrefix)
	if err := gc.CommitWorktreeOp(relPath, message); err != nil {
		return false, err
	}
	return true, nil
}
