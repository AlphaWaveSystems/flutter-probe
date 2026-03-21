package device

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ios"
)

// Platform indicates the target platform.
type Platform string

const (
	PlatformAndroid Platform = "android"
	PlatformIOS     Platform = "ios"
)

// Device represents a connected emulator or simulator.
type Device struct {
	ID        string   // ADB serial or iOS UDID
	Name      string   // display name
	Platform  Platform
	State     string // online | offline | booting
	OSVersion string // e.g. "iOS 18.6" or "Android 14"
}

// ToolPaths holds configurable paths to external tools.
// Empty strings mean "use the tool name from PATH".
type ToolPaths struct {
	ADB     string // path to adb binary
	Flutter string // path to flutter binary
}

// Manager handles device discovery and lifecycle.
type Manager struct {
	adb    *ADB
	simctl *ios.SimCtl
	tools  ToolPaths
}

// NewManager creates a Manager using tools found in PATH.
func NewManager() *Manager {
	return &Manager{adb: NewADB(), simctl: ios.New()}
}

// NewManagerWithPaths creates a Manager with configurable tool paths.
// This is useful for CI/CD environments or when tools are not in PATH.
func NewManagerWithPaths(paths ToolPaths) *Manager {
	return &Manager{
		adb:    NewADBWithPath(paths.ADB),
		simctl: ios.New(),
		tools:  paths,
	}
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
				ID:        s.UDID,
				Name:      s.Name,
				Platform:  PlatformIOS,
				State:     strings.ToLower(s.State),
				OSVersion: s.HumanRuntime(),
			})
		}
	}

	return all, nil
}

// Start boots an Android emulator identified by avdName.
// bootTimeout and pollInterval control startup behavior (0 = use defaults).
func (m *Manager) Start(ctx context.Context, avdName string, bootTimeout, pollInterval time.Duration) (*Device, error) {
	return m.adb.StartEmulator(ctx, avdName, bootTimeout, pollInterval)
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
	if err := m.simctl.WaitForBoot(ctx, udid, 60*time.Second, 0); err != nil {
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

// ADB returns the underlying ADB wrapper for direct access.
func (m *Manager) ADB() *ADB {
	return m.adb
}

// InstallAndLaunchApp installs an app and launches it on the specified device.
// For Android: installs APK, optionally grants all permissions, then launches.
// For iOS: installs .app bundle, optionally grants all privacy services, then launches.
func (m *Manager) InstallAndLaunchApp(ctx context.Context, serial string, platform Platform, appPath, appID string, grantPerms bool) error {
	switch platform {
	case PlatformAndroid:
		fmt.Printf("  Installing %s on %s...\n", appPath, serial)
		if err := m.adb.Install(ctx, serial, appPath); err != nil {
			return fmt.Errorf("install: %w", err)
		}
		if grantPerms {
			fmt.Printf("  Granting all permissions for %s...\n", appID)
			for _, perms := range AndroidPermissions {
				for _, perm := range perms {
					_ = m.adb.GrantPermission(ctx, serial, appID, perm)
				}
			}
		}
		fmt.Printf("  Launching %s...\n", appID)
		if err := m.adb.LaunchApp(ctx, serial, appID); err != nil {
			return fmt.Errorf("launch: %w", err)
		}

	case PlatformIOS:
		fmt.Printf("  Installing %s on %s...\n", appPath, serial)
		if err := m.simctl.Install(ctx, serial, appPath); err != nil {
			return fmt.Errorf("install: %w", err)
		}
		if grantPerms {
			fmt.Printf("  Granting all permissions for %s...\n", appID)
			for _, svc := range IOSPrivacyServices {
				_ = m.simctl.GrantPrivacy(ctx, serial, appID, svc)
			}
		}
		fmt.Printf("  Launching %s...\n", appID)
		if err := m.simctl.Launch(ctx, serial, appID); err != nil {
			return fmt.Errorf("launch: %w", err)
		}

	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}
	return nil
}

// WaitForBoot polls until a device is online or the context is cancelled.
// pollInterval controls how often to check (0 = default 2s).
func (m *Manager) WaitForBoot(ctx context.Context, serial string, pollInterval time.Duration) error {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
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

// ReadTokenIOS reads the ProbeAgent token from the iOS simulator.
// bundleID is optional — if provided, it checks the app container's token file first.
func (m *Manager) ReadTokenIOS(ctx context.Context, udid string, timeout time.Duration, bundleID ...string) (string, error) {
	return m.simctl.ReadToken(ctx, udid, timeout, bundleID...)
}

// ReadToken scans the device logcat output for the ProbeAgent one-time token.
// The agent prints a line of the form:  PROBE_TOKEN=<token>
func (m *Manager) ReadToken(ctx context.Context, serial string, timeout time.Duration) (string, error) {
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(tCtx, m.adb.bin, "-s", serial, "logcat", "-s", "flutter:I")
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
	flutterBin := "flutter"
	if m.tools.Flutter != "" {
		flutterBin = m.tools.Flutter
	}
	args := []string{
		"run",
		"--debug",
		"-d", serial,
		"--dart-define=PROBE_AGENT=true",
	}
	cmd := exec.CommandContext(ctx, flutterBin, args...)
	cmd.Dir = projectDir
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("device: flutter run: %w", err)
	}
	return cmd, nil
}
