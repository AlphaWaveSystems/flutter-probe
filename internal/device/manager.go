package device

import (
	"bufio"
	"context"
	"fmt"
	"net"
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
	adb       *ADB
	simctl    *ios.SimCtl
	devicectl *ios.DeviceCtl
	tools     ToolPaths
}

// NewManager creates a Manager using tools found in PATH.
func NewManager() *Manager {
	return &Manager{adb: NewADB(), simctl: ios.New(), devicectl: ios.NewDeviceCtl()}
}

// NewManagerWithPaths creates a Manager with configurable tool paths.
// This is useful for CI/CD environments or when tools are not in PATH.
func NewManagerWithPaths(paths ToolPaths) *Manager {
	return &Manager{
		adb:       NewADBWithPath(paths.ADB),
		simctl:    ios.New(),
		devicectl: ios.NewDeviceCtl(),
		tools:     paths,
	}
}

// List returns all connected Android devices/emulators, iOS simulators, and physical iOS devices.
func (m *Manager) List(ctx context.Context) ([]Device, error) {
	var all []Device

	// Android devices (both emulators and physical)
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

	// Physical iOS devices (via libimobiledevice)
	physicalUDIDs, err := ios.ListPhysicalDevices(ctx)
	if err == nil {
		simUDIDs := make(map[string]bool)
		for _, s := range sims {
			simUDIDs[s.UDID] = true
		}
		for _, udid := range physicalUDIDs {
			if !simUDIDs[udid] { // avoid duplicates if somehow listed in both
				all = append(all, Device{
					ID:       udid,
					Name:     "Physical iOS Device",
					Platform: PlatformIOS,
					State:    "online",
				})
			}
		}
	}

	return all, nil
}

// DeviceCtl returns the DeviceCtl instance for physical iOS device operations.
func (m *Manager) DeviceCtl() *ios.DeviceCtl {
	return m.devicectl
}

// IsPhysicalIOS returns true if the given UDID is a physical iOS device
// (not found in the simulator list).
func (m *Manager) IsPhysicalIOS(ctx context.Context, udid string) bool {
	sims, err := m.simctl.List(ctx)
	if err != nil {
		return true // assume physical if simctl fails
	}
	for _, s := range sims {
		if s.UDID == udid {
			return false
		}
	}
	return true
}

// IsPhysicalAndroid returns true if the given serial is a physical Android device
// (not an emulator).
func (m *Manager) IsPhysicalAndroid(ctx context.Context, serial string) bool {
	if strings.HasPrefix(serial, "emulator-") {
		return false
	}
	out, err := m.adb.Shell(ctx, serial, "getprop", "ro.hardware")
	if err != nil {
		return !strings.HasPrefix(serial, "emulator-")
	}
	hw := strings.TrimSpace(string(out))
	return hw != "ranchu" && hw != "goldfish"
}

// EnsureADB checks that the ADB binary is available and can communicate with
// the specified device. It also cleans up any stale port forwards for the given port.
func (m *Manager) EnsureADB(ctx context.Context, serial string, hostPort int) error {
	// Check ADB is installed
	if _, err := exec.LookPath(m.adb.Bin()); err != nil {
		return fmt.Errorf("adb not found at %q — install Android SDK platform-tools or set tools.adb in probe.yaml", m.adb.Bin())
	}

	// Verify the device is reachable
	devices, err := m.adb.Devices(ctx)
	if err != nil {
		return fmt.Errorf("adb devices: %w", err)
	}
	found := false
	for _, d := range devices {
		if d.ID == serial && d.State == "device" {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("adb: device %s not found or not online — check USB connection", serial)
	}

	// Clean up stale port forwards for this port to avoid conflicts
	out, err := m.adb.Run(ctx, serial, "forward", "--list")
	if err == nil {
		rule := fmt.Sprintf("tcp:%d", hostPort)
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, serial) && strings.Contains(line, rule) {
				_ = m.adb.RemoveForward(ctx, serial, hostPort)
				break
			}
		}
	}

	return nil
}

// EnsureIProxy checks that iproxy is installed, kills any stale iproxy processes
// for the given UDID, and starts a fresh iproxy forwarding hostPort to devicePort.
// Returns a cleanup function that kills the iproxy process.
func (m *Manager) EnsureIProxy(ctx context.Context, udid string, hostPort, devicePort int) (cleanup func(), err error) {
	// Check iproxy is installed
	if _, lookErr := exec.LookPath("iproxy"); lookErr != nil {
		return nil, fmt.Errorf("iproxy not found — install via: brew install libimobiledevice")
	}

	// Kill stale iproxy processes for this UDID
	KillStaleIProxy(udid)

	// Start a fresh iproxy
	cmd := exec.CommandContext(ctx, "iproxy",
		fmt.Sprintf("%d", hostPort),
		fmt.Sprintf("%d", devicePort),
		"--udid", udid,
	)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("iproxy start: %w", err)
	}

	cleanup = func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}

	// Verify the tunnel is actually forwarding TCP. iproxy can be alive as a
	// process while the tunnel is dead (USB stack stuck, port not yet bound,
	// device disconnected mid-spawn). Probe the host port directly so a dead
	// tunnel surfaces here rather than as a 30s WebSocket handshake timeout.
	if err := waitIProxyReady(ctx, hostPort, 3*time.Second); err != nil {
		cleanup()
		return nil, fmt.Errorf("iproxy tunnel not forwarding on 127.0.0.1:%d: %w", hostPort, err)
	}

	return cleanup, nil
}

// waitIProxyReady polls 127.0.0.1:port with short TCP connects until either a
// connection succeeds or the deadline expires. It does not exchange any data;
// the connect itself is the readiness signal.
func waitIProxyReady(ctx context.Context, port int, total time.Duration) error {
	deadline := time.Now().Add(total)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s: %w", total, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// KillStaleIProxy kills all iproxy processes that match the given UDID.
// This prevents port conflicts from leftover processes after flutter run crashes.
func KillStaleIProxy(udid string) {
	// Find all iproxy PIDs
	out, err := exec.Command("pgrep", "-f", "iproxy").Output()
	if err != nil {
		return // no iproxy processes
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid := strings.TrimSpace(line)
		if pid == "" {
			continue
		}
		// Check if this PID's command line contains our UDID
		cmdline, err := exec.Command("ps", "-p", pid, "-o", "args=").Output()
		if err != nil {
			continue
		}
		if strings.Contains(string(cmdline), udid) {
			exec.Command("kill", pid).Run()
		}
	}
	// Brief pause to let ports be released
	time.Sleep(500 * time.Millisecond)
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

// ReadTokenIOS reads the ProbeAgent token from an iOS device.
// For simulators, reads the token file via simctl.
// For physical devices, reads from idevicesyslog.
// bundleID is optional — if provided, it checks the app container's token file first.
func (m *Manager) ReadTokenIOS(ctx context.Context, udid string, timeout time.Duration, bundleID ...string) (string, error) {
	// Try simulator path first (fast, file-based)
	token, err := m.simctl.ReadToken(ctx, udid, timeout, bundleID...)
	if err == nil {
		return token, nil
	}

	// Fallback: physical device via idevicesyslog
	return m.readTokenPhysicalIOS(ctx, udid, timeout)
}

// readTokenPhysicalIOS reads the token from a physical iOS device using idevicesyslog.
func (m *Manager) readTokenPhysicalIOS(ctx context.Context, udid string, timeout time.Duration) (string, error) {
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(tCtx, "idevicesyslog", "-u", udid, "--match", "PROBE_TOKEN")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("ios physical: syslog pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("ios physical: idevicesyslog not found — install via: brew install libimobiledevice")
	}
	defer cmd.Process.Kill()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "PROBE_TOKEN="); idx >= 0 {
			token := strings.TrimSpace(line[idx+len("PROBE_TOKEN="):])
			if len(token) >= 16 {
				return token, nil
			}
		}
	}
	return "", fmt.Errorf("ios physical: token not found within %s — is the app running with probe_agent?", timeout)
}

// ReadToken reads the ProbeAgent token from the Android device.
// It tries multiple sources: app cache file (via run-as), /data/local/tmp,
// and logcat streaming as fallback.
func (m *Manager) ReadToken(ctx context.Context, serial string, timeout time.Duration) (string, error) {
	return m.ReadTokenAndroid(ctx, serial, timeout, "")
}

// ReadTokenAndroid reads the token with an optional app ID for run-as access.
func (m *Manager) ReadTokenAndroid(ctx context.Context, serial string, timeout time.Duration, appID string) (string, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Try 1: read from app cache via run-as (most reliable)
		if appID != "" {
			out, err := m.adb.Shell(ctx, serial, "run-as", appID, "cat", "cache/probe/token")
			if err == nil {
				token := strings.TrimSpace(string(out))
				if len(token) >= 16 {
					return token, nil
				}
			}
		}

		// Try 2: read from /data/local/tmp (world-readable, works on some devices)
		out, err := m.adb.Shell(ctx, serial, "cat", "/data/local/tmp/probe/token")
		if err == nil {
			token := strings.TrimSpace(string(out))
			if len(token) >= 16 {
				return token, nil
			}
		}

		// Try 3: scan logcat dump for PROBE_TOKEN=
		logOut, err := m.adb.Shell(ctx, serial, "logcat", "-d", "-s", "flutter:I")
		if err == nil {
			for _, line := range strings.Split(string(logOut), "\n") {
				if idx := strings.Index(line, "PROBE_TOKEN="); idx >= 0 {
					token := strings.TrimSpace(string(line[idx+len("PROBE_TOKEN="):]))
					if len(token) >= 16 {
						return token, nil
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	return "", fmt.Errorf("android: probe token not found within %s — is the app running with probe_agent?", timeout)
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
