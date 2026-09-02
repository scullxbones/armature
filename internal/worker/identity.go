// Package worker resolves and persists the current worker's identity, used to attribute claims and commits.
package worker

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/scullxbones/armature/internal/adapters"
)

const gitConfigKey = "armature.worker-id"

// InitWorker generates a new worker UUID and stores it in local git config.
func InitWorker(repoPath string) (string, error) {
	id := uuid.New().String()
	if err := adapters.New(repoPath).SetGitConfig(gitConfigKey, id); err != nil {
		return "", fmt.Errorf("failed to set worker ID: %w", err)
	}
	return id, nil
}

// GetWorkerID reads the worker UUID from local git config.
func GetWorkerID(repoPath string) (string, error) {
	id, err := adapters.New(repoPath).ReadGitConfig(gitConfigKey)
	if err != nil {
		return "", fmt.Errorf("worker ID not configured — run 'trls worker-init': %w", err)
	}
	return id, nil
}

// CheckWorkerID returns whether a worker ID is configured, and if so, what it is.
func CheckWorkerID(repoPath string) (bool, string) {
	id, err := GetWorkerID(repoPath)
	if err != nil {
		return false, ""
	}
	return true, id
}
