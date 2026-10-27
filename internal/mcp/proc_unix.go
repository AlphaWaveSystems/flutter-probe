//go:build !windows

package mcp

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group, so signalling it
// also reaches the adb/simctl/test-runner processes it spawns.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateProcessGroup asks the child's whole process group to stop (SIGTERM).
func terminateProcessGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}

// killProcessGroup SIGKILLs the child's whole process group.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
