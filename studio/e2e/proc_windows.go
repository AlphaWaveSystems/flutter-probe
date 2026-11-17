//go:build windows

package e2e

import "os/exec"

// The driver controls the macOS app, so it only runs on a Mac; these keep the package compiling (and vetted) on Windows.
func ownProcessGroup(*exec.Cmd) {}

func killGroup(int, bool) {}
