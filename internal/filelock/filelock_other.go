//go:build !unix && !windows

package filelock

import (
	"fmt"
	"os"
	"runtime"
)

// lock, tryLock, and unlock are the fallback for platforms that are neither
// unix (filelock_unix.go) nor windows (filelock_windows.go) — this repo
// ships linux, darwin, and windows binaries only (see .goreleaser.yaml and
// docs/design/architecture.md's supported platform matrix). Anything else is
// unsupported, and per I5 ("deterministic gates fail closed"), an
// unsupported platform must refuse to lock rather than silently reporting a
// lock as acquired: a no-op here would let callers race with no mutual
// exclusion at all — including, for acquireClaimLock's caller, the
// destructive `git worktree remove --force` / MoveWorktree path a claim lock
// exists to make impossible, and, for lockLog's caller, concurrent op-log
// appends racing each other. This file exists purely so a build targeting an
// unsupported GOOS still compiles; it is never expected to run in a shipped
// binary.
func lock(_ *os.File) error {
	return fmt.Errorf("file locking is not implemented on %s; concurrent access safety cannot be guaranteed", runtime.GOOS)
}

// tryLock is the non-blocking counterpart to lock above; it fails closed the
// same way, returning (false, non-nil error) unconditionally rather than
// (true, nil).
func tryLock(_ *os.File) (bool, error) {
	return false, fmt.Errorf("file locking is not implemented on %s; concurrent access safety cannot be guaranteed", runtime.GOOS)
}

// unlock is the counterpart to lock/tryLock above. Since neither ever
// reports success on this platform, unlock is never reached in practice by a
// caller that only unlocks what it locked; it returns nil rather than
// duplicating the fail-closed error, so a best-effort release in a defer
// (e.g. `defer func() { _ = filelock.Unlock(f) }()`) does not itself fail.
func unlock(_ *os.File) error { return nil }
