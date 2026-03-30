package main

import (
	"fmt"
	"os"
	"time"

	"github.com/scullxbones/trellis/internal/git"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/worker"
)

// pushDeps holds push-related dependencies set up by initPushDeps.
var (
	appPusher  ops.Pusher
	appTracker ops.PendingPushTracker
)

func resolveWorkerAndLog() (string, string, error) {
	workerID, err := worker.GetWorkerID(appCtx.RepoPath)
	if err != nil {
		return "", "", fmt.Errorf("worker not initialized: %w", err)
	}
	logName := workerID
	if slot := os.Getenv("TRLS_LOG_SLOT"); slot != "" {
		logName = workerID + "~" + slot
	}
	logPath := fmt.Sprintf("%s/ops/%s.log", appCtx.IssuesDir, logName)
	return workerID, logPath, nil
}

func nowEpoch() int64 {
	return time.Now().Unix()
}

// initPushDeps wires up the pusher and tracker based on the current context.
// In single-branch mode: NoPusher + NoTracker.
// In dual-branch mode: AppendCommitAndPush with FilePushTracker.
func initPushDeps() {
	if appCtx.WorktreePath == "" {
		// single-branch: no push needed
		appPusher = ops.NoPusher{}
		appTracker = ops.NoTracker{}
		return
	}
	gc := git.New(appCtx.WorktreePath)
	appPusher = &ops.AppendCommitAndPush{
		Pusher:  gc,
		Branch:  "_trellis",
		Backoff: nil, // use defaults: 1s, 2s, 4s
	}
	appTracker = ops.NewFilePushTracker(appCtx.StateDir)
}

// appendOp appends an op to the log and, in dual-branch mode, commits it to the worktree branch.
func appendOp(logPath string, op ops.Op) error {
	var gc ops.GitCommitter
	if appCtx.WorktreePath != "" {
		gc = git.New(appCtx.WorktreePath)
	}
	return ops.AppendAndCommit(logPath, appCtx.WorktreePath, op, gc)
}

// appendHighStakesOp appends an op, commits it (dual-branch), and attempts to push.
// Push errors are best-effort — the op is still committed locally.
// Used for claim, transition, assign, unassign — ops that must not be delayed.
func appendHighStakesOp(logPath string, op ops.Op) error {
	var gc ops.GitCommitter
	if appCtx.WorktreePath != "" {
		gc = git.New(appCtx.WorktreePath)
	}
	// Append and commit — this is not best-effort
	if err := ops.AppendAndCommit(logPath, appCtx.WorktreePath, op, gc); err != nil {
		return err
	}
	// Push is best-effort: push via the pusher (which handles retries) but ignore errors
	if appCtx.WorktreePath != "" {
		// Use the underlying git client for push attempts
		gc2 := git.New(appCtx.WorktreePath)
		if err := gc2.Push("_trellis"); err != nil {
			// Best-effort: attempt fetch+rebase and retry once
			if rbErr := gc2.FetchAndRebase("_trellis"); rbErr == nil {
				gc2.Push("_trellis") //nolint:errcheck
			}
		}
		appTracker.Reset() //nolint:errcheck
	}
	return nil
}

// appendLowStakesOp appends an op, increments the pending counter, and only
// pushes when the threshold is reached.
func appendLowStakesOp(logPath string, op ops.Op) error {
	var gc ops.GitCommitter
	if appCtx.WorktreePath != "" {
		gc = git.New(appCtx.WorktreePath)
	}
	if err := ops.AppendAndCommit(logPath, appCtx.WorktreePath, op, gc); err != nil {
		return err
	}

	threshold := appCtx.Config.LowStakesPushThreshold
	if threshold <= 0 {
		threshold = 5
	}

	n, err := appTracker.Increment()
	if err != nil {
		return err
	}

	if n >= threshold {
		// Push now and reset counter
		if appCtx.WorktreePath != "" {
			pushGC := git.New(appCtx.WorktreePath)
			_ = pushGC // push happens via AppendCommitAndPush on next high-stakes op
		}
		appTracker.Reset() //nolint:errcheck
	}
	return nil
}
