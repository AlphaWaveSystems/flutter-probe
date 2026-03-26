// Package ios provides wrappers for Apple device management tools.
// DeviceCtl wraps `xcrun devicectl` for physical iOS device operations.
// SimCtl (in simctl.go) wraps `xcrun simctl` for simulator operations.

package ios

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DeviceCtl wraps `xcrun devicectl` for managing physical iOS devices.
// Requires Xcode 15+ and a USB-connected device.
type DeviceCtl struct{}

// NewDeviceCtl creates a DeviceCtl instance.
func NewDeviceCtl() *DeviceCtl {
	return &DeviceCtl{}
}

// Launch starts an app on a physical device by bundle ID.
func (d *DeviceCtl) Launch(ctx context.Context, udid, bundleID string) error {
	cmd := exec.CommandContext(ctx, "xcrun", "devicectl", "device", "process", "launch",
		"--device", udid, bundleID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("devicectl launch: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Terminate stops an app on a physical device by resolving its PID first.
// Returns nil if the app is not running.
func (d *DeviceCtl) Terminate(ctx context.Context, udid, bundleID string) error {
	pid, err := d.FindPID(ctx, udid, bundleID)
	if err != nil || pid == 0 {
		return nil // not running
	}
	cmd := exec.CommandContext(ctx, "xcrun", "devicectl", "device", "process", "terminate",
		"--device", udid, "--pid", strconv.Itoa(pid))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("devicectl terminate (pid %d): %s: %w", pid, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Install installs an .app bundle on a physical device.
func (d *DeviceCtl) Install(ctx context.Context, udid, appPath string) error {
	cmd := exec.CommandContext(ctx, "xcrun", "devicectl", "device", "install", "app",
		"--device", udid, appPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("devicectl install: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// FindPID looks up the process ID of a running app by its bundle ID.
// Returns 0 if the app is not running.
func (d *DeviceCtl) FindPID(ctx context.Context, udid, bundleID string) (int, error) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("devicectl-procs-%s.json", udid[:8]))
	defer os.Remove(tmpFile)

	cmd := exec.CommandContext(ctx, "xcrun", "devicectl", "device", "info", "processes",
		"--device", udid, "--json-output", tmpFile)
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("devicectl info processes: %w", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return 0, fmt.Errorf("reading process list: %w", err)
	}

	// Parse the JSON output to find matching bundle ID
	var result struct {
		Result struct {
			RunningProcesses []struct {
				BundleIdentifier string `json:"bundleIdentifier"`
				ProcessIdentifier int   `json:"processIdentifier"`
				Executable        string `json:"executable"`
			} `json:"runningProcesses"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, fmt.Errorf("parsing process list: %w", err)
	}

	for _, p := range result.Result.RunningProcesses {
		if p.BundleIdentifier == bundleID {
			return p.ProcessIdentifier, nil
		}
	}

	return 0, nil // not found = not running
}

// ListDevices returns UDIDs of connected physical iOS devices via idevice_id.
func ListPhysicalDevices(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "idevice_id", "-l").Output()
	if err != nil {
		return nil, err
	}
	var udids []string
	for _, line := range strings.Split(string(out), "\n") {
		udid := strings.TrimSpace(line)
		if udid != "" {
			udids = append(udids, udid)
		}
	}
	return udids, nil
}
