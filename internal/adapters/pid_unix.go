//go:build unix

package adapters

import (
	"errors"
	"os"
	"syscall"
)

// pidAlive reports whether pid is a live process. Signal 0 does not deliver a
// signal; ESRCH means the process does not exist. EPERM means the process
// exists but we lack permission to signal it (e.g. pid 1) — treat as alive so
// a privileged peer's lease is never swept as stale.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) || errors.Is(err, os.ErrPermission) {
		return true
	}
	return false
}
