package main

import (
	"fmt"
	"time"

	"github.com/scullxbones/trellis/internal/git"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/worker"
)

func resolveWorkerAndLog() (string, string, error) {
	workerID, err := worker.GetWorkerID(appCtx.RepoPath)
	if err != nil {
		return "", "", fmt.Errorf("worker not initialized: %w", err)
	}
	logPath := fmt.Sprintf("%s/ops/%s.log", appCtx.IssuesDir, workerID)
	return workerID, logPath, nil
}

func nowEpoch() int64 {
	return time.Now().Unix()
}

// appendOp appends an op to the log and, in dual-branch mode, commits it to the worktree branch.
func appendOp(logPath string, op ops.Op) error {
	var gc ops.GitCommitter
	if appCtx.WorktreePath != "" {
		gc = git.New(appCtx.WorktreePath)
	}
	return ops.AppendAndCommit(logPath, appCtx.WorktreePath, op, gc)
}
