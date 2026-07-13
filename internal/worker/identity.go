// Package worker resolves and persists the current worker's identity, used to attribute claims and commits.
package worker

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/scullxbones/armature/internal/platform"
)

const gitConfigKey = "armature.worker-id"

// InitWorker generates a new worker UUID and stores it in local git config.
func InitWorker(repoPath string) (string, error) {
	return InitWorkerWithPort(platform.NewGitConfigPort(repoPath))
}

// InitWorkerWithPort generates a new worker UUID and stores it through the
// provided git config port.
func InitWorkerWithPort(port platform.GitConfigPort) (string, error) {
	id := uuid.New().String()
	if err := port.Set(gitConfigKey, id); err != nil {
		return "", fmt.Errorf("failed to set worker ID: %w", err)
	}
	return id, nil
}

// GetWorkerID reads the worker UUID from local git config.
func GetWorkerID(repoPath string) (string, error) {
	return GetWorkerIDWithPort(platform.NewGitConfigPort(repoPath))
}

// GetWorkerIDWithPort reads the worker UUID through the provided git config port.
func GetWorkerIDWithPort(port platform.GitConfigPort) (string, error) {
	id, err := port.Get(gitConfigKey)
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
