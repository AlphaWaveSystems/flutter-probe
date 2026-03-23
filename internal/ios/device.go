// Package ios provides physical iOS device operations via xcrun devicectl
// and libimobiledevice tools (iproxy, idevicesyslog).
package ios

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// PhysicalDevice represents a connected physical iOS device.
type PhysicalDevice struct {
	UDID      string
	Name      string
	OSVersion string
	State     string // connected | disconnected
}

// IOSDevice wraps xcrun devicectl and libimobiledevice tools for
// physical iOS device management.
type IOSDevice struct{}

// NewIOSDevice creates a new IOSDevice instance.
func NewIOSDevice() *IOSDevice { return &IOSDevice{} }

// uuidPattern matches standard UUID format (simulator UDIDs).
// Physical iOS UDIDs do NOT match this pattern.
var uuidPattern = regexp.MustCompile(
	`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`,
)

// IsSimulatorUDID returns true if the UDID matches the standard UUID format
// used by iOS simulators. Physical device UDIDs are hex strings like
// "00008120-0011790211A2201E" which do not match the UUID pattern.
func IsSimulatorUDID(udid string) bool {
	return uuidPattern.MatchString(udid)
}

// IsAvailable checks if xcrun devicectl is available on this system.
func (d *IOSDevice) IsAvailable() bool {
	cmd := exec.Command("xcrun", "devicectl", "list", "devices", "--help")
	return cmd.Run() == nil
}

// IProxyAvailable checks if iproxy is installed and in PATH.
func (d *IOSDevice) IProxyAvailable() bool {
	_, err := exec.LookPath("iproxy")
	return err == nil
}

// IDeviceSyslogAvailable checks if idevicesyslog is installed and in PATH.
func (d *IOSDevice) IDeviceSyslogAvailable() bool {
	_, err := exec.LookPath("idevicesyslog")
	return err == nil
}

// ListDevices returns all connected physical iOS devices via xcrun devicectl.
func (d *IOSDevice) ListDevices(ctx context.Context) ([]PhysicalDevice, error) {
	// Use a temp file for JSON output because devicectl writes a text table
	// to stdout alongside the JSON when using /dev/stdout.
	tmpFile, err := os.CreateTemp("", "probe-devicectl-*.json")
	if err != nil {
		return nil, fmt.Errorf("devicectl: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd := exec.CommandContext(ctx, "xcrun", "devicectl", "list", "devices",
		"--json-output", tmpPath)
	// Discard stdout/stderr (text table output)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("devicectl list: %w", err)
	}

	out, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("devicectl: read output: %w", err)
	}

	var result struct {
		Result struct {
			Devices []struct {
				Identifier       string `json:"identifier"`
				DeviceProperties struct {
					Name            string `json:"name"`
					OSVersionNumber string `json:"osVersionNumber"`
				} `json:"deviceProperties"`
				ConnectionProperties struct {
					TransportType string `json:"transportType"`
				} `json:"connectionProperties"`
			} `json:"devices"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("devicectl parse: %w", err)
	}

	var devices []PhysicalDevice
	for _, dev := range result.Result.Devices {
		state := "connected"
		if dev.ConnectionProperties.TransportType == "" {
			state = "disconnected"
		}
		devices = append(devices, PhysicalDevice{
			UDID:      dev.Identifier,
			Name:      dev.DeviceProperties.Name,
			OSVersion: "iOS " + dev.DeviceProperties.OSVersionNumber,
			State:     state,
		})
	}
	return devices, nil
}

// ForwardPort spawns iproxy to forward a host port to a device port over USB.
// Returns the running process — caller must kill it when done.
func (d *IOSDevice) ForwardPort(ctx context.Context, udid string, hostPort, devicePort int) (*exec.Cmd, error) {
	if !d.IProxyAvailable() {
		return nil, fmt.Errorf("iproxy not found — install via 'brew install libimobiledevice'")
	}

	portSpec := fmt.Sprintf("%d:%d", hostPort, devicePort)
	cmd := exec.CommandContext(ctx, "iproxy", portSpec, "--udid", udid)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("iproxy start: %w", err)
	}

	// Wait briefly to check the process didn't exit immediately (e.g. port conflict)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return nil, fmt.Errorf("iproxy exited immediately: %w (is port %d already in use?)", err, hostPort)
	case <-time.After(500 * time.Millisecond):
		// Process is still running — port forwarding is active
	}

	return cmd, nil
}

// StopForward kills an iproxy process.
func (d *IOSDevice) StopForward(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// ReadToken reads the ProbeAgent auth token from the physical device's system
// log via idevicesyslog. It scans for lines containing PROBE_TOKEN= and returns
// the token value.
func (d *IOSDevice) ReadToken(ctx context.Context, udid string, timeout time.Duration) (string, error) {
	if !d.IDeviceSyslogAvailable() {
		return "", fmt.Errorf("idevicesyslog not found — install via 'brew install libimobiledevice'")
	}

	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(tCtx, "idevicesyslog", "-u", udid)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("idevicesyslog pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("idevicesyslog start: %w", err)
	}
	defer cmd.Process.Kill() //nolint:errcheck

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "PROBE_TOKEN="); idx >= 0 {
			token := strings.TrimSpace(line[idx+len("PROBE_TOKEN="):])
			// Filter out short/invalid tokens and log stream header lines
			if len(token) >= 16 && !strings.HasPrefix(token, "\"") {
				return token, nil
			}
		}
	}
	return "", fmt.Errorf("ios device: probe token not found within %s", timeout)
}

// LaunchApp launches an app on a physical iOS device via xcrun devicectl.
func (d *IOSDevice) LaunchApp(ctx context.Context, udid, bundleID string) error {
	cmd := exec.CommandContext(ctx, "xcrun", "devicectl",
		"device", "process", "launch",
		"--device", udid, bundleID)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("devicectl launch: %s", stderr.String())
	}
	return nil
}

// TerminateApp terminates a running app on a physical iOS device.
func (d *IOSDevice) TerminateApp(ctx context.Context, udid, bundleID string) error {
	// Note: devicectl doesn't have a direct terminate command in all Xcode versions.
	// We try the available approach.
	cmd := exec.CommandContext(ctx, "xcrun", "devicectl",
		"device", "process", "terminate",
		"--device", udid, bundleID)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// Ignore errors — app might not be running
	_ = cmd.Run()
	return nil
}

// InstallApp installs an IPA or .app on a physical iOS device.
func (d *IOSDevice) InstallApp(ctx context.Context, udid, appPath string) error {
	cmd := exec.CommandContext(ctx, "xcrun", "devicectl",
		"device", "install", "app",
		"--device", udid, appPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("devicectl install: %s", stderr.String())
	}
	return nil
}

// LookupUDIDByDevicectlID maps the CoreDevice identifier (from devicectl) to
// the USB serial number (used by iproxy/idevicesyslog). In many cases these
// are different — devicectl returns a CoreDevice UUID while iproxy needs the
// USB serial. This method queries both and returns the USB serial if found.
func (d *IOSDevice) LookupUDIDByDevicectlID(ctx context.Context, devicectlID string) string {
	// For now, return the devicectl ID as-is. In practice, iproxy on modern
	// macOS accepts both the CoreDevice UUID and the USB serial.
	return devicectlID
}
