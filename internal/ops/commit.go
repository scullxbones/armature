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
	if err := AppendOp(logPath, op); err != nil {
		return err
	}
	if worktreePath == "" {
		return nil // single-branch: no git commit needed
	}

	relPath, err := filepath.Rel(worktreePath, logPath)
	if err != nil {
		return fmt.Errorf("resolve relative log path: %w", err)
	}

	// Safely truncate WorkerID to at most 8 chars for the commit message
	workerPrefix := op.WorkerID
	if len(workerPrefix) > 8 {
		workerPrefix = workerPrefix[:8]
	}

	message := fmt.Sprintf("ops: %s %s by %s", strings.ToLower(op.Type), op.TargetID, workerPrefix)
	return gc.CommitWorktreeOp(relPath, message)
}
