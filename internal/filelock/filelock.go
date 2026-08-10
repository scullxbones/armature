// Package filelock provides a cross-platform, advisory, whole-file exclusive
// lock over *os.File, used to serialize access to shared state (op logs,
// per-issue claim locks) within a single machine/clone.
//
// Three operations are exposed:
//
//   - Lock: a BLOCKING exclusive lock — the caller waits until it can be
//     acquired.
//   - TryLock: a NON-BLOCKING exclusive lock — returns immediately with
//     (false, nil) if another process already holds it, rather than
//     blocking or erroring.
//   - Unlock: releases a lock previously taken by Lock or TryLock.
//
// The implementation is build-tagged per platform:
//
//   - filelock_unix.go (unix): flock(2).
//   - filelock_windows.go (windows): LockFileEx/UnlockFileEx.
//   - filelock_other.go (neither): fails closed on every operation — see
//     that file for the rationale (I5: deterministic gates fail closed).
//
// Every shipped platform (linux, darwin, windows — see .goreleaser.yaml and
// docs/design/architecture.md's supported platform matrix) has a real
// locking implementation; the "other" fallback exists purely so a build
// targeting an unsupported GOOS still compiles, and is not expected to run
// in a shipped binary.
package filelock

import "os"

// Lock takes a BLOCKING exclusive advisory lock on f, waiting until it can
// be acquired.
func Lock(f *os.File) error {
	return lock(f)
}

// TryLock takes a NON-BLOCKING exclusive advisory lock on f. It returns
// (true, nil) if the lock was acquired, (false, nil) if another process
// already holds it, and a non-nil error only for unexpected failures.
func TryLock(f *os.File) (bool, error) {
	return tryLock(f)
}

// Unlock releases the advisory lock previously taken by Lock or TryLock.
func Unlock(f *os.File) error {
	return unlock(f)
}
