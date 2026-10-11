//go:build !windows

package sysdialog

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group, so killing it also
// takes down the xcodebuild/test-runner processes it spawns.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup SIGKILLs the child's whole process group.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
