//go:build windows

package mcp

import "os/exec"

// Windows has no process groups in the Unix sense; these keep the package
// compiling (probe-mcp ships for Windows) and stop the direct child.

func setProcessGroup(cmd *exec.Cmd) {}

func terminateProcessGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
