package worker

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/uuid"
)

const gitConfigKey = "armature.worker-id"

// InitWorker generates a new worker UUID and stores it in local git config.
func InitWorker(repoPath string) (string, error) {
	id := uuid.New().String()
	cmd := exec.Command("git", "-C", repoPath, "config", "--local", gitConfigKey, id)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to set worker ID: %s: %w", out, err)
	}
	return id, nil
}

// GetWorkerID reads the worker UUID from local git config.
func GetWorkerID(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "config", "--local", gitConfigKey)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("worker ID not configured — run 'trls worker-init': %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CheckWorkerID returns whether a worker ID is configured, and if so, what it is.
func CheckWorkerID(repoPath string) (bool, string) {
	id, err := GetWorkerID(repoPath)
	if err != nil {
		return false, ""
	}
	return true, id
}
