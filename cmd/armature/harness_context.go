package main

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/config"
	armcontext "github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/materialize"
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

	allOps, offsets, err := readAllOpsFromDirWithOffsets(filepath.Join(appCtx.IssuesDir, "ops"))
	if err != nil {
		return "", fmt.Errorf("read ops: %w", err)
	}
	if _, err := materialize.Materialize(stateDir, allOps, appCtx.Mode == "single-branch", offsets); err != nil {
		return "", fmt.Errorf("materialize: %w", err)
	}
	state, err := loadStateFromStateDir(stateDir)
	if err != nil {
		return "", fmt.Errorf("load state: %w", err)
	}
	assembled, err := armcontext.Assemble(issueID, stateDir, state)
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
