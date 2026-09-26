//go:build !unix && !windows

package filelock

import (
	"fmt"
	"os"
	"runtime"
)

func lock(_ *os.File) error {
	return fmt.Errorf("file locking is not implemented on %s; concurrent access safety cannot be guaranteed", runtime.GOOS)
}

func tryLock(_ *os.File) (bool, error) {
	return false, fmt.Errorf("file locking is not implemented on %s; concurrent access safety cannot be guaranteed", runtime.GOOS)
}

func unlock(_ *os.File) error { return nil }
