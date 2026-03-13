package device

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Platform indicates the target platform.
type Platform string

const (
	PlatformAndroid Platform = "android"
	PlatformIOS     Platform = "ios"
)

// Device represents a connected emulator or simulator.
type Device struct {
	ID       string   // ADB serial or iOS UDID
	Name     string   // display name
	Platform Platform
	State    string // online | offline | booting
}

// Manager handles device discovery and lifecycle.
type Manager struct {
	adb *ADB
}

// NewManager creates a Manager using the ADB binary found in PATH.
func NewManager() *Manager {
	return &Manager{adb: NewADB()}
}

// List returns all connected Android emulators/devices.
func (m *Manager) List(ctx context.Context) ([]Device, error) {
	return m.adb.Devices(ctx)
}

// Start boots an Android emulator identified by avdName.
func (m *Manager) Start(ctx context.Context, avdName string) (*Device, error) {
	return m.adb.StartEmulator(ctx, avdName)
}

// WaitForBoot polls until a device is online or the context is cancelled.
func (m *Manager) WaitForBoot(ctx context.Context, serial string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			devices, err := m.adb.Devices(ctx)
			if err != nil {
				continue
			}
			for _, d := range devices {
				if d.ID == serial && d.State == "online" {
					return nil
				}
			}
		}
	}
}

// ForwardPort sets up adb forward so the host can reach localhost:agentPort on the device.
func (m *Manager) ForwardPort(ctx context.Context, serial string, hostPort, devicePort int) error {
	return m.adb.Forward(ctx, serial, hostPort, devicePort)
}

// RemoveForward cleans up an adb port forward.
func (m *Manager) RemoveForward(ctx context.Context, serial string, hostPort int) error {
	return m.adb.RemoveForward(ctx, serial, hostPort)
}

// ReadToken scans the device logcat output for the ProbeAgent one-time token.
// The agent prints a line of the form:  PROBE_TOKEN=<token>
func (m *Manager) ReadToken(ctx context.Context, serial string, timeout time.Duration) (string, error) {
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(tCtx, "adb", "-s", serial, "logcat", "-s", "ProbeAgent:I")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("device: logcat pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("device: logcat start: %w", err)
	}
	defer cmd.Process.Kill() //nolint:errcheck

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "PROBE_TOKEN="); idx >= 0 {
			token := strings.TrimSpace(line[idx+len("PROBE_TOKEN="):])
			return token, nil
		}
	}
	return "", fmt.Errorf("device: token not found within %s", timeout)
}

// RunFlutter launches flutter run with ProbeAgent in debug mode.
func (m *Manager) RunFlutter(ctx context.Context, projectDir, serial string) (*exec.Cmd, error) {
	args := []string{
		"run",
		"--debug",
		"-d", serial,
		"--dart-define=PROBE_AGENT=true",
	}
	cmd := exec.CommandContext(ctx, "flutter", args...)
	cmd.Dir = projectDir
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("device: flutter run: %w", err)
	}
	return cmd, nil
}
