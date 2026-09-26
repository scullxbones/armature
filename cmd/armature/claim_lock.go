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

type gitDirFlock struct {
	release func()
}

func (f gitDirFlock) Release() {
	if f.release != nil {
		f.release()
	}
}

func tryAcquirePessimisticCloneClaimFlock(repoPath, issueID string) (gitDirFlock, error) {
	f, err := openGitDirLock(repoPath, fmt.Sprintf("armature-claim-%s.lock", issueID),
		"resolve git dir for claim lock", "open claim lock file")
	if err != nil {
		return gitDirFlock{}, err
	}

	locked, lockErr := filelock.TryLock(f)
	if lockErr != nil {
		bestEffortClose(f)
		return gitDirFlock{}, fmt.Errorf("acquire claim lock: %w", lockErr)
	}
	if !locked {
		bestEffortClose(f)
		return gitDirFlock{}, fmt.Errorf("another claim for %s is in progress in this clone", issueID)
	}
	return holdGitDirFlock(f), nil
}

func acquireBlockingGitExcludeFlock(repoPath string) (gitDirFlock, error) {
	f, err := openGitDirLock(repoPath, "armature-git-exclude.lock",
		"resolve git dir for exclude lock", "open git exclude lock file")
	if err != nil {
		return gitDirFlock{}, err
	}
	if err := filelock.Lock(f); err != nil {
		bestEffortClose(f)
		return gitDirFlock{}, fmt.Errorf("acquire git exclude lock: %w", err)
	}
	return holdGitDirFlock(f), nil
}

func openGitDirLock(repoPath, lockName, resolveMsg, openMsg string) (*os.File, error) {
	gitDir, err := resolveCommonGitDir(repoPath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", resolveMsg, err)
	}
	lockPath := filepath.Join(gitDir, lockName)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // lock path is under this clone's git dir
	if err != nil {
		return nil, fmt.Errorf("%s: %w", openMsg, err)
	}
	return f, nil
}

func holdGitDirFlock(f *os.File) gitDirFlock {
	return gitDirFlock{release: func() {
		swallowErr(filelock.Unlock(f))
		bestEffortClose(f)
	}}
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
