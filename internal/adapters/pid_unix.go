//go:build unix

package adapters

import (
	"os"
	"syscall"
)

// pidAlive reports whether pid is a live process. Signal 0 does not deliver a
// signal; ESRCH means the process does not exist.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil
}
