//go:build unix

package adapters

import (
	"os/exec"
	"syscall"
)

// configureNetworkCmd puts the git child in its own process group and kills that
// group on context cancel so helpers like git-remote-http die with the parent.
// WaitDelay bounds stuck pipe wait after Cancel.
func configureNetworkCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative PID signals the process group (Setpgid), not only the leader.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = gitNetworkWaitDelay
}
