package main

import (
	"fmt"
	"time"

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
