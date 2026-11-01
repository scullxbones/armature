package adapters

import "time"

// SetGitNetworkTimeoutForTest shrinks (or restores) the shared ops-backup
// network deadline. Tests only — production code must leave the default.
func SetGitNetworkTimeoutForTest(d time.Duration) (restore func()) {
	prev := gitNetworkTimeout
	gitNetworkTimeout = d
	return func() { gitNetworkTimeout = prev }
}
