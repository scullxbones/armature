package main

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/config"
	armcontext "github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/scullxbones/armature/internal/worker"
)

func buildHarnessStructuredContext(appCtx *config.Context, issueID string) (string, error) {
	stateDir := appCtx.StateDir
	if stateDir == "" {
		workerID, err := worker.GetWorkerID(appCtx.RepoPath)
		if err != nil || workerID == "" {
			workerID = "default"
		}
		stateDir = stateDirFor(appCtx, workerID)
	}

	snap, err := snapshot.Load(filepath.Join(appCtx.IssuesDir, "ops"), stateDir, appCtx.Mode == "single-branch")
	if err != nil {
		return "", fmt.Errorf("load snapshot: %w", err)
	}
	// Create an OSFileReader for file access
	reader := &armcontext.OSFileReader{Root: appCtx.RepoPath}
	assembled, err := armcontext.Assemble(issueID, snap.State, reader)
	if err != nil {
		return "", fmt.Errorf("assemble context: %w", err)
	}
	if appCtx.Config.TokenBudget > 0 {
		assembled = armcontext.Truncate(assembled, appCtx.Config.TokenBudget)
	}
	rendered, err := armcontext.RenderAgent(assembled)
	if err != nil {
		return "", fmt.Errorf("render context: %w", err)
	}
	return rendered, nil
}
