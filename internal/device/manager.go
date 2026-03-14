package device

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/flutterprobe/probe/internal/ios"
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
	adb    *ADB
	simctl *ios.SimCtl
}

// NewManager creates a Manager using the ADB binary found in PATH.
func NewManager() *Manager {
	return &Manager{adb: NewADB(), simctl: ios.New()}
}

// List returns all connected Android emulators/devices and iOS simulators.
func (m *Manager) List(ctx context.Context) ([]Device, error) {
	var all []Device

	// Android devices
	androids, err := m.adb.Devices(ctx)
	if err == nil {
		all = append(all, androids...)
	}

	// iOS simulators
	sims, err := m.simctl.List(ctx)
	if err == nil {
		for _, s := range sims {
			all = append(all, Device{
				ID:       s.UDID,
				Name:     s.Name,
				Platform: PlatformIOS,
				State:    strings.ToLower(s.State),
			})
		}
	}

	return all, nil
}

// Start boots an Android emulator identified by avdName.
func (m *Manager) Start(ctx context.Context, avdName string) (*Device, error) {
	return m.adb.StartEmulator(ctx, avdName)
}

// StartIOS boots an iOS simulator by UDID. If udid is empty, auto-selects one.
func (m *Manager) StartIOS(ctx context.Context, udid string) (*Device, error) {
	if udid == "" {
		sim, err := m.simctl.AutoSelect(ctx)
		if err != nil {
			return nil, err
		}
		udid = sim.UDID
	}

	if err := m.simctl.Boot(ctx, udid); err != nil {
		return nil, err
	}
	if err := m.simctl.WaitForBoot(ctx, udid, 60*time.Second); err != nil {
		return nil, err
	}

	// Resolve name
	sims, _ := m.simctl.List(ctx)
	name := udid
	for _, s := range sims {
		if s.UDID == udid {
			name = s.Name
			break
		}
	}

	return &Device{
		ID:       udid,
		Name:     name,
		Platform: PlatformIOS,
		State:    "booted",
	}, nil
}

// SimCtl returns the underlying SimCtl for direct access.
func (m *Manager) SimCtl() *ios.SimCtl {
	return m.simctl
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

// ReadTokenIOS reads the ProbeAgent token from the iOS simulator's syslog.
func (m *Manager) ReadTokenIOS(ctx context.Context, udid string, timeout time.Duration) (string, error) {
	return m.simctl.ReadToken(ctx, udid, timeout)
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
