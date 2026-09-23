package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/filelock"
)

// pessimisticCloneClaimFlock is the same-clone flock (TryLock). Op-log claim
// ownership is optimistic (Issue.HeldByExactWorkerAndClaimToken / Payload.IfClaimToken).
type pessimisticCloneClaimFlock struct {
	release func()
}

func (f pessimisticCloneClaimFlock) Release() {
	if f.release != nil {
		f.release()
	}
}

func tryAcquirePessimisticCloneClaimFlock(repoPath, issueID string) (pessimisticCloneClaimFlock, error) {
	gitDir, err := resolveCommonGitDir(repoPath)
	if err != nil {
		return pessimisticCloneClaimFlock{}, fmt.Errorf("resolve git dir for claim lock: %w", err)
	}
	lockPath := filepath.Join(gitDir, fmt.Sprintf("armature-claim-%s.lock", issueID))

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // path is built from a validated issue ID, not user-controlled
	if err != nil {
		return pessimisticCloneClaimFlock{}, fmt.Errorf("open claim lock file: %w", err)
	}

	locked, lockErr := filelock.TryLock(f)
	if lockErr != nil {
		bestEffortClose(f)
		return pessimisticCloneClaimFlock{}, fmt.Errorf("acquire claim lock: %w", lockErr)
	}
	if !locked {
		bestEffortClose(f)
		return pessimisticCloneClaimFlock{}, fmt.Errorf("another claim for %s is in progress in this clone", issueID)
	}

	return pessimisticCloneClaimFlock{release: func() {
		swallowErr(filelock.Unlock(f))
		bestEffortClose(f)
	}}, nil
}

// blockingGitExcludeFlock is the clone-wide git exclude lock (blocking Lock).
// Distinct from pessimisticCloneClaimFlock, which is per-issue TryLock.
type blockingGitExcludeFlock struct {
	release func()
}

func (f blockingGitExcludeFlock) Release() {
	if f.release != nil {
		f.release()
	}
}

func acquireBlockingGitExcludeFlock(repoPath string) (blockingGitExcludeFlock, error) {
	gitDir, err := resolveCommonGitDir(repoPath)
	if err != nil {
		return blockingGitExcludeFlock{}, fmt.Errorf("resolve git dir for exclude lock: %w", err)
	}
	lockPath := filepath.Join(gitDir, "armature-git-exclude.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // fixed lock name in this clone's git dir
	if err != nil {
		return blockingGitExcludeFlock{}, fmt.Errorf("open git exclude lock file: %w", err)
	}
	if err := filelock.Lock(f); err != nil {
		bestEffortClose(f)
		return blockingGitExcludeFlock{}, fmt.Errorf("acquire git exclude lock: %w", err)
	}

	return blockingGitExcludeFlock{release: func() {
		swallowErr(filelock.Unlock(f))
		bestEffortClose(f)
	}}, nil
}

func resolveCommonGitDir(repoPath string) (string, error) {
	if info, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil && info.IsDir() {
		gitDir, absErr := filepath.Abs(filepath.Join(repoPath, ".git"))
		if absErr != nil {
			return "", fmt.Errorf("resolve git common dir path: %w", absErr)
		}
		if resolved, evalErr := filepath.EvalSymlinks(gitDir); evalErr == nil {
			gitDir = resolved
		}
		return filepath.Clean(gitDir), nil
	}
	// #nosec G204 - git binary and arguments are controlled by Armature.
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "rev-parse", "--git-common-dir")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve git common dir: %w", err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", fmt.Errorf("resolve git common dir: git returned an empty path")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoPath, dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve git common dir path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
}
