//go:build !unix && !windows

package filelock

import (
	"fmt"
	"os"
	"runtime"
)

type unsupportedLockPlatformError struct {
	goos string
}

func (e unsupportedLockPlatformError) Error() string {
	return fmt.Sprintf("file locking is not implemented on %s; concurrent access safety cannot be guaranteed", e.goos)
}

func lock(_ *os.File) error {
	return unsupportedLockPlatformError{runtime.GOOS}
}

func tryLock(_ *os.File) (bool, error) {
	return false, unsupportedLockPlatformError{runtime.GOOS}
}

func unlock(_ *os.File) error { return nil }
