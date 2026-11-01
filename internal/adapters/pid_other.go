//go:build !unix

package adapters

import "os"

// pidAlive reports whether pid looks live. On non-unix, FindProcess always
// succeeds; treat unknown pids as dead so crash sweeps still reclaim leftovers.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil || p == nil {
		return false
	}
	// Windows FindProcess does not prove liveness. Assume alive so we do not
	// delete a peer's nonce mid-run; leaseIsLive's age backstop (started= or
	// mtime) is the reclaim path so nothing leaks forever.
	return true
}
