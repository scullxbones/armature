//go:build unix

package filelock

import (
	"os"
	"syscall"
)

// lock takes a BLOCKING exclusive advisory lock on f via flock(2).
func lock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// tryLock takes a non-blocking exclusive advisory lock on f via flock(2).
// Returns (true, nil) if the lock was acquired, (false, nil) if another
// process already holds it, and a non-nil error only for unexpected failures.
func tryLock(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if err == syscall.EWOULDBLOCK {
		return false, nil
	}
	return false, err
}

// unlock releases the advisory lock previously taken by lock or tryLock.
func unlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
