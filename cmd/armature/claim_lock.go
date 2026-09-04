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

// Concurrency model for arm claim.
//
// Two mutable substrates are in play, and each is guarded differently:
//
//   - Substrate A — the op log, cross-clone: OPTIMISTIC. Every claim op
//     carries a unique Payload.ClaimToken; compensating ops are conditional
//     (Payload.IfClaimToken) and validated at replay in
//     materialize.applyTransition. Order-independent: no locking, no
//     ordering assumptions across clones or machines.
//   - Substrate B — this clone's filesystem/git worktree state, same-clone
//     only: PESSIMISTIC. A per-issue flock (acquireClaimLock, below) on
//     <git-common-dir>/armature-claim-<issue>.lock, held from before the
//     FIRST read of issue or worktree state through the claim append,
//     provisioning, all destructive cleanup, and rollback.
//
// THE RULE, which any change to claim's read/mutate sequence must satisfy:
// any read whose value influences a filesystem mutation or the rollback
// snapshot must be taken inside the lock; and the lock must be a real lock
// on every shipped platform (see internal/filelock, which is a real lock on
// unix and windows and fails closed everywhere else per I5: deterministic
// gates fail closed). Several review rounds each found the next-widest
// read-decide-mutate window because the guard kept getting placed at the
// narrowest point fixing the named symptom instead of at this rule --
// most recently, the "do I still own this claim?" ownership check itself
// (used by both rollbackClaim and cleanupPartialWorktree below) was written
// out by hand at each call site and drifted out of sync with its sibling
// copy in materialize.applyTransition. That predicate is now defined in
// exactly one place, materialize.Issue.ClaimHeldBy (status must be exactly
// "claimed" and workerID/claimToken must match exactly), and every call
// site -- including materialize's own replay-time IfClaimToken guard --
// delegates to it. Do not reintroduce a second, ad-hoc field comparison
// anywhere in this file; call ClaimHeldBy (via claimStillOwnedBy) instead.
//
// acquireClaimLock takes an exclusive, non-blocking, per-issue advisory lock
// scoped to this clone, serializing concurrent `arm claim` invocations against
// the same issue within a single clone. It closes the destructive-filesystem
// race described on createWorktreeAndBranch's cleanupPartialWorktree: the
// ownership recheck there (stillOwns) ends before the destructive
// `git worktree remove --force` / MoveWorktree call it guards, so a second
// worker's claim landing in that exact window could still have its worktree
// discarded. That race is inherently same-clone-only — a remote claimant
// provisions into its own filesystem entirely and never touches this path —
// so an OS-level file lock scoped to this clone is sufficient to make it
// impossible rather than merely unlikely.
//
// The lock file lives in the MAIN repo's git common dir (resolved the same
// way worktree.ResolveGitDir resolves any other worktree's git dir; for the
// main repo this is simply <repoPath>/.git), so it is shared across every
// linked worktree of this clone and survives worktree creation/removal.
// It is intentionally never deleted: reacquiring it later (a legitimate
// retry after release) just reopens and re-locks the same file.
//
// On success, the returned release func MUST be called (typically via
// `defer`) to drop the lock; the OS also releases it if the process exits
// without calling release. If the lock is already held by another process,
// acquireClaimLock returns a clear, actionable error naming the issue.
func acquireClaimLock(repoPath, issueID string) (release func(), err error) {
	gitDir, err := resolveCommonGitDir(repoPath)
	if err != nil {
		return nil, fmt.Errorf("resolve git dir for claim lock: %w", err)
	}
	lockPath := filepath.Join(gitDir, fmt.Sprintf("armature-claim-%s.lock", issueID))

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // path is built from a validated issue ID, not user-controlled
	if err != nil {
		return nil, fmt.Errorf("open claim lock file: %w", err)
	}

	locked, lockErr := filelock.TryLock(f)
	if lockErr != nil {
		_ = f.Close() //nolint:errcheck // already returning lockErr; a close failure on the abandoned fd is not actionable
		return nil, fmt.Errorf("acquire claim lock: %w", lockErr)
	}
	if !locked {
		_ = f.Close() //nolint:errcheck // already returning the lock-held error; a close failure on the abandoned fd is not actionable
		return nil, fmt.Errorf("another claim for %s is in progress in this clone", issueID)
	}

	return func() {
		_ = filelock.Unlock(f) //nolint:errcheck // best-effort release; process exit also releases OS-level locks
		_ = f.Close()          //nolint:errcheck // best-effort close in a release func with no error return
	}, nil
}

// acquireGitExcludeLock serializes read-modify-write updates to the clone's
// shared .git/info/exclude file. It is repository-wide rather than per issue:
// claims for different issues still mutate the same file and must not lose one
// another's exclusion pattern.
func acquireGitExcludeLock(repoPath string) (release func(), err error) {
	gitDir, err := resolveCommonGitDir(repoPath)
	if err != nil {
		return nil, fmt.Errorf("resolve git dir for exclude lock: %w", err)
	}
	lockPath := filepath.Join(gitDir, "armature-git-exclude.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // fixed lock name in this clone's git dir
	if err != nil {
		return nil, fmt.Errorf("open git exclude lock file: %w", err)
	}
	if err := filelock.Lock(f); err != nil {
		_ = f.Close() //nolint:errcheck // already returning the lock error
		return nil, fmt.Errorf("acquire git exclude lock: %w", err)
	}

	return func() {
		_ = filelock.Unlock(f) //nolint:errcheck // best-effort release
		_ = f.Close()          //nolint:errcheck // best-effort close
	}, nil
}

// resolveCommonGitDir returns the repository's shared Git directory rather
// than a linked worktree's private administrative directory. Shared claim
// locks and .git/info/exclude must converge on this path across every linked
// worktree in the clone.
func resolveCommonGitDir(repoPath string) (string, error) {
	// The main worktree's .git directory is already the common directory. This
	// direct path also preserves updateGitExclude's small filesystem-fixture
	// contract, where callers provide a .git/info directory without a complete
	// repository. Linked worktrees have a .git file and use rev-parse below.
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
