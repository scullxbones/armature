package main

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/config"
	armcontext "github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/dag"
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
	// Construct graph from state.Issues
	nodeIndex := make(map[string]*dag.Node)
	for id, issue := range snap.State.Issues {
		nodeIndex[id] = &dag.Node{
			ID:        issue.ID,
			Title:     issue.Title,
			Type:      issue.Type,
			Parent:    issue.Parent,
			Children:  issue.Children,
			BlockedBy: issue.BlockedBy,
			Blocks:    issue.Blocks,
		}
	}
	graph := dag.GraphFromState(nodeIndex)
	assembled, err := armcontext.Assemble(issueID, stateDir, snap.State, graph)
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
