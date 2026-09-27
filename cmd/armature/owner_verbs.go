package main

import (
	"errors"
	"fmt"

	claimpkg "github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/config"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/oporder"
)

func locatePublishedOps(ctx *config.Context) ([]oporder.LocatedOp, error) {
	if ctx == nil || ctx.WorktreePath == "" {
		return nil, fmt.Errorf("published Owner: ops worktree path is required")
	}
	located, err := oporder.LocateOps(oporder.LocateInput{
		OpsWorktree:  ctx.WorktreePath,
		PublishedRef: oporder.DefaultPublishedRef,
	})
	if err != nil {
		return nil, fmt.Errorf("published Owner: locate ops: %w", err)
	}
	return located, nil
}

func requirePublishedOwner(ctx *config.Context, issueID, workerID string) error {
	located, err := locatePublishedOps(ctx)
	if err != nil {
		return err
	}
	return oporder.RequireOwner(located, issueID, workerID)
}

func leaseStatusAllowsOwnerGate(status string) bool {
	return status == ops.StatusClaimed || status == ops.StatusInProgress
}

func wrapNotClaimOwner(code string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, claimpkg.ErrNotClaimOwner) {
		return armerrors.Wrap(code, claimpkg.ErrNotClaimOwner.Error(), []string{
			"arm claim --worktree",
			"arm show",
		}, err)
	}
	return err
}

func mapHeartbeatError(err error) error {
	if err == nil {
		return nil
	}
	var cf *armerrors.CommandFailure
	if errors.As(err, &cf) {
		return cf
	}
	if wrapped := wrapNotClaimOwner("HEARTBEAT-1", err); wrapped != err {
		return wrapped
	}
	if isLocalArmatureTipPublishError(err) {
		return wrapOpsPublishFailure("HEARTBEAT-1", err)
	}
	return armerrors.Wrap("HEARTBEAT-1", err.Error(), []string{"arm doctor", "arm show"}, err)
}
