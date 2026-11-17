//go:build !windows

package e2e

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup puts the child in its own process group so Close can stop the helpers Studio spawns (iproxy etc.).
func ownProcessGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// killGroup signals the child's whole process group: SIGTERM, or SIGKILL when force is set.
func killGroup(pid int, force bool) {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-pid, sig)
}
