//go:build !unix

package adapters

import "os/exec"

// configureNetworkCmd sets WaitDelay so a cancelled network git does not hang
// on stuck pipes. Process-group kill is unix-only (see network_unix.go).
func configureNetworkCmd(cmd *exec.Cmd) {
	cmd.WaitDelay = gitNetworkWaitDelay
}
