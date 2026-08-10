//go:build windows

package filelock

import (
	"os"

	"golang.org/x/sys/windows"
)

// lock takes a BLOCKING exclusive advisory lock on f via LockFileEx, matched
// to unlock below (same byte range, same handle semantics).
func lock(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0, 1, 0, ol,
	)
}

// tryLock takes a non-blocking exclusive advisory lock on f via LockFileEx,
// matched to unlock below (same byte range, same handle semantics). Returns
// (true, nil) if the lock was acquired, (false, nil) if another process
// already holds it, and a non-nil error only for unexpected failures.
func tryLock(f *os.File) (bool, error) {
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, ol,
	)
	if err == nil {
		return true, nil
	}
	if err == windows.ERROR_LOCK_VIOLATION || err == windows.ERROR_IO_PENDING {
		return false, nil
	}
	return false, err
}

// unlock releases the advisory lock previously taken by lock or tryLock. The
// range arguments (1, 0) must match the LockFileEx calls above exactly.
func unlock(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
