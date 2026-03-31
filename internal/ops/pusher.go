package ops

import (
	"fmt"
	"time"
)

// GitPusher is the interface for pushing a branch to origin with fetch+rebase support.
type GitPusher interface {
	Push(branch string) error
	FetchAndRebase(branch string) error
}

// Pusher is the interface used by AppendCommitAndPush to push after appending ops.
type Pusher interface {
	Push(logPath, worktreePath string, op Op, gc GitCommitter) error
}

// NoPusher is a no-op Pusher for single-branch mode or when push is disabled.
type NoPusher struct{}

func (NoPusher) Push(logPath, worktreePath string, op Op, gc GitCommitter) error {
	return AppendAndCommit(logPath, worktreePath, op, gc)
}

// AppendCommitAndPush appends an op, commits it (in dual-branch mode), and pushes
// to origin with up to 4 attempts using exponential backoff (1s, 2s, 4s between retries).
// It uses FetchAndRebase to recover from non-fast-forward rejections.
type AppendCommitAndPush struct {
	Pusher  GitPusher
	Branch  string
	Backoff []time.Duration // override for testing; defaults to [1s, 2s, 4s]
}

func (a *AppendCommitAndPush) Push(logPath, worktreePath string, op Op, gc GitCommitter) error {
	if err := AppendAndCommit(logPath, worktreePath, op, gc); err != nil {
		return err
	}

	// Only push in dual-branch mode (worktreePath set)
	if worktreePath == "" {
		return nil
	}

	backoff := a.Backoff
	if backoff == nil {
		backoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	}

	// First attempt — no backoff needed.
	var lastErr error
	if err := a.Pusher.Push(a.Branch); err == nil {
		return nil
	} else {
		lastErr = err
	}

	// Retries: fetch+rebase, sleep, push.
	for attempt, delay := range backoff {
		if err := a.Pusher.FetchAndRebase(a.Branch); err != nil {
			return fmt.Errorf("fetch+rebase before push attempt %d: %w", attempt+2, err)
		}
		time.Sleep(delay)
		if err := a.Pusher.Push(a.Branch); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	return fmt.Errorf("push failed after %d attempts: %w", len(backoff)+1, lastErr)
}
