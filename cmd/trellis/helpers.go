package main

import (
	"fmt"
	"time"

	"github.com/scullxbones/trellis/internal/worker"
)

func resolveWorkerAndLog(repoPath string) (string, string, error) {
	workerID, err := worker.GetWorkerID(repoPath)
	if err != nil {
		return "", "", fmt.Errorf("worker not initialized: %w", err)
	}
	logPath := fmt.Sprintf("%s/.issues/ops/%s.log", repoPath, workerID)
	return workerID, logPath, nil
}

func nowEpoch() int64 {
	return time.Now().Unix()
}
