//go:build windows

package sysdialog

import "os/exec"

// The iOS driver needs macOS and Xcode, so on Windows these are only here to
// keep the package compiling (the CLI and Studio are built for Windows).

func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
