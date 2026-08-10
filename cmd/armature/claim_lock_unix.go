//go:build unix

package main

import (
	"os"
	"syscall"
)

// tryFlock takes a non-blocking exclusive advisory lock on f via flock(2).
// Returns (true, nil) if the lock was acquired, (false, nil) if another
// process already holds it, and a non-nil error only for unexpected failures.
func tryFlock(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if err == syscall.EWOULDBLOCK {
		return false, nil
	}
	return false, err
}

// unlockFlock releases the advisory lock previously taken by tryFlock.
func unlockFlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
