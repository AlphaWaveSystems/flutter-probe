package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// ConfirmFunc is called before destructive operations. It receives a description
// of what will happen and returns true if the user approves. In CI/CD mode,
// this is bypassed by setting AllowClearData.
type ConfirmFunc func(message string) bool

// DeviceContext holds device-level information needed for platform operations
// that bypass the Dart agent (restart, clear data). These operations kill
// the running app, so they handle reconnection transparently.
type DeviceContext struct {
	Manager                 *device.Manager
	Serial                  string          // ADB serial or iOS UDID
	Platform                device.Platform
	AppID                   string          // bundle ID / package name
	Port                    int             // host-side agent port (default 48686)
	DevicePort              int             // on-device agent port (default: same as Port)
	IsPhysical              bool            // true for physical devices (vs emulator/simulator)
	UseHTTP                 bool            // if true, use HTTP POST instead of WebSocket for reconnection
	AgentHost               string          // agent host IP (default "127.0.0.1"; set to device IP for WiFi)
	AllowClearData          bool            // if true, skip confirmation for clear app data (CI/CD mode)
	Confirm                 ConfirmFunc     // interactive confirmation callback (nil = deny destructive ops unless AllowClearData)
	GrantPermissionsOnClear bool            // if true, auto-grant all permissions after clearing data
	ReconnectDelay          time.Duration   // delay after app restart before reconnecting WebSocket (default 2s)
	RestartDelay            time.Duration   // delay after force-stop before relaunching (default 500ms)
	TokenReadTimeout        time.Duration   // max time to wait for agent token during reconnect (default 30s)
	DialTimeout             time.Duration   // max time to establish WebSocket connection (default 30s)
	CLIVersion              string          // running probe binary's version, sent during the reconnect handshake
}

// agentHost returns the configured agent host or the default.
func (dc *DeviceContext) agentHost() string {
	if dc.AgentHost != "" {
		return dc.AgentHost
	}
	return "127.0.0.1"
}

// reconnectDelay returns the configured reconnect delay or the default.
func (dc *DeviceContext) reconnectDelay() time.Duration {
	if dc.ReconnectDelay > 0 {
		return dc.ReconnectDelay
	}
	// Android emulators need more time to boot the Flutter engine after restart
	if dc.Platform == device.PlatformAndroid {
		return 5 * time.Second
	}
	return 2 * time.Second
}

// restartDelay returns the configured restart delay or the default.
func (dc *DeviceContext) restartDelay() time.Duration {
	if dc.RestartDelay > 0 {
		return dc.RestartDelay
	}
	return 500 * time.Millisecond
}

// tokenReadTimeout returns the configured token read timeout or the default.
func (dc *DeviceContext) tokenTimeout() time.Duration {
	if dc.TokenReadTimeout > 0 {
		return dc.TokenReadTimeout
	}
	return 30 * time.Second
}

// dialTimeoutVal returns the configured dial timeout or the default.
func (dc *DeviceContext) dialTimeoutVal() time.Duration {
	if dc.DialTimeout > 0 {
		return dc.DialTimeout
	}
	return 30 * time.Second
}

// RestartApp force-stops the app and relaunches it. This preserves app data.
func (dc *DeviceContext) RestartApp(ctx context.Context) error {
	fmt.Printf("    \033[33m↻\033[0m  Force-stopping %s on %s...\n", dc.AppID, dc.Serial)

	switch dc.Platform {
	case device.PlatformAndroid:
		if _, err := dc.Manager.ADB().Shell(ctx, dc.Serial,
			"am", "force-stop", dc.AppID); err != nil {
			return fmt.Errorf("restart: force-stop: %w", err)
		}
		time.Sleep(dc.restartDelay())
		fmt.Printf("    \033[33m↻\033[0m  Relaunching %s...\n", dc.AppID)
		// Launch via am start with the standard Flutter MainActivity
		if _, err := dc.Manager.ADB().Shell(ctx, dc.Serial,
			"am", "start", "-n", dc.Manager.ADB().ResolveLauncherActivity(ctx, dc.Serial, dc.AppID)); err != nil {
			return fmt.Errorf("restart: launch: %w", err)
		}

	case device.PlatformIOS:
		if dc.IsPhysical {
			dctl := dc.Manager.DeviceCtl()
			_ = dctl.Terminate(ctx, dc.Serial, dc.AppID)
			time.Sleep(dc.restartDelay())
			fmt.Printf("    \033[33m↻\033[0m  Relaunching %s...\n", dc.AppID)
			if err := dctl.Launch(ctx, dc.Serial, dc.AppID); err != nil {
				return fmt.Errorf("restart: launch: %w", err)
			}
		} else {
			simctl := dc.Manager.SimCtl()
			_ = simctl.Terminate(ctx, dc.Serial, dc.AppID)
			time.Sleep(dc.restartDelay())
			fmt.Printf("    \033[33m↻\033[0m  Relaunching %s...\n", dc.AppID)
			if err := simctl.Launch(ctx, dc.Serial, dc.AppID); err != nil {
				return fmt.Errorf("restart: launch: %w", err)
			}
		}
	}
	return nil
}

// ClearAppData clears all app storage and relaunches. This is a destructive
// operation that wipes SharedPreferences, databases, and all local files.
// It requires explicit opt-in via --allow-clear-data flag or interactive confirmation.
func (dc *DeviceContext) ClearAppData(ctx context.Context) error {
	// Physical iOS: skip immediately — no filesystem access, and attempting
	// clear would kill the app/agent before we can recover.
	if dc.Platform == device.PlatformIOS && dc.IsPhysical {
		fmt.Println("    \033[33m⚠\033[0m  clear app data is not supported on physical iOS devices — skipping")
		fmt.Println("       Workaround: uninstall and reinstall the app manually")
		return nil
	}

	// Gate: require explicit permission for destructive data wipe
	if !dc.AllowClearData {
		if dc.Confirm == nil {
			return fmt.Errorf("clear app data: this is a destructive operation that wipes all app data for %s. "+
				"Use --allow-clear-data flag to permit this, or run interactively", dc.AppID)
		}
		msg := fmt.Sprintf("This will permanently delete ALL app data for %s on %s.\n"+
			"  SharedPreferences, databases, and files will be wiped. Continue?", dc.AppID, dc.Serial)
		if !dc.Confirm(msg) {
			return fmt.Errorf("clear app data: cancelled by user")
		}
	}

	fmt.Printf("    \033[31m⚠\033[0m  Clearing all data for %s on %s\n", dc.AppID, dc.Serial)

	switch dc.Platform {
	case device.PlatformAndroid:
		// pm clear force-stops and wipes all data — this is the official Android mechanism
		if _, err := dc.Manager.ADB().Shell(ctx, dc.Serial,
			"pm", "clear", dc.AppID); err != nil {
			return fmt.Errorf("clear data: %w", err)
		}
		// Auto-grant permissions before relaunch to prevent OS permission dialogs
		if dc.GrantPermissionsOnClear {
			if err := dc.GrantAllPermissions(ctx); err != nil {
				fmt.Printf("    \033[33m⚠\033[0m  auto-grant permissions: %v\n", err)
			}
		}
		time.Sleep(dc.restartDelay())
		fmt.Printf("    \033[33m↻\033[0m  Relaunching %s...\n", dc.AppID)
		if _, err := dc.Manager.ADB().Shell(ctx, dc.Serial,
			"am", "start", "-n", dc.Manager.ADB().ResolveLauncherActivity(ctx, dc.Serial, dc.AppID)); err != nil {
			return fmt.Errorf("clear data: relaunch: %w", err)
		}

	case device.PlatformIOS:
		// Physical iOS already handled above (early return).
		simctl := dc.Manager.SimCtl()
		_ = simctl.Terminate(ctx, dc.Serial, dc.AppID)

		dataPath := simctl.AppDataPath(ctx, dc.Serial, dc.AppID)
		if err := dc.validateIOSDataPath(dataPath); err != nil {
			return fmt.Errorf("clear data: %w", err)
		}

		if dataPath != "" {
			for _, subdir := range []string{"Documents", "Library", "tmp"} {
				target := dataPath + "/" + subdir
				_, _ = simctl.Spawn(ctx, dc.Serial, "rm", "-rf", target)
			}
			fmt.Printf("    \033[32m✓\033[0m  Cleared data container: %s\n", dataPath)
		}

		if err := simctl.KeychainReset(ctx, dc.Serial); err != nil {
			fmt.Printf("    \033[33m⚠\033[0m  keychain reset: %v\n", err)
		}

		if dc.GrantPermissionsOnClear {
			if err := dc.GrantAllPermissions(ctx); err != nil {
				fmt.Printf("    \033[33m⚠\033[0m  auto-grant permissions: %v\n", err)
			}
		}
		time.Sleep(dc.restartDelay())
		fmt.Printf("    \033[33m↻\033[0m  Relaunching %s...\n", dc.AppID)
		if err := simctl.Launch(ctx, dc.Serial, dc.AppID); err != nil {
			return fmt.Errorf("clear data: relaunch: %w", err)
		}
	}
	return nil
}

// validateIOSDataPath checks that a path returned by simctl get_app_container
// looks like a legitimate app container path before any deletion occurs.
func (dc *DeviceContext) validateIOSDataPath(dataPath string) error {
	if dataPath == "" {
		// App not installed or simctl failed — nothing to clear
		return nil
	}

	// Must be an absolute path
	if !strings.HasPrefix(dataPath, "/") {
		return fmt.Errorf("invalid data path (not absolute): %s", dataPath)
	}

	// Must contain CoreSimulator/Devices to be a legitimate simulator container
	if !strings.Contains(dataPath, "CoreSimulator/Devices") {
		return fmt.Errorf("data path does not look like a simulator container: %s", dataPath)
	}

	// Must contain the simulator UDID
	if !strings.Contains(dataPath, dc.Serial) {
		return fmt.Errorf("data path does not belong to device %s: %s", dc.Serial, dataPath)
	}

	// Must not be a root-level path (defense in depth)
	parts := strings.Split(strings.Trim(dataPath, "/"), "/")
	if len(parts) < 5 {
		return fmt.Errorf("data path is suspiciously short: %s", dataPath)
	}

	return nil
}

// AllowPermission grants a named permission to the app at the OS level.
func (dc *DeviceContext) AllowPermission(ctx context.Context, name string) error {
	switch dc.Platform {
	case device.PlatformAndroid:
		perms, err := device.ResolveAndroidPermissions(name)
		if err != nil {
			return err
		}
		for _, perm := range perms {
			if err := dc.Manager.ADB().GrantPermission(ctx, dc.Serial, dc.AppID, perm); err != nil {
				// Some permissions may not exist on older API levels — warn, don't fail
				fmt.Printf("    \033[33m⚠\033[0m  grant %s: %v\n", perm, err)
			}
		}
	case device.PlatformIOS:
		if dc.IsPhysical {
			fmt.Printf("    \033[33m⚠\033[0m  permission management is not supported on physical iOS devices — skipping\n")
			return nil
		}
		svc, err := device.ResolveIOSService(name)
		if err != nil {
			return err
		}
		if err := dc.Manager.SimCtl().GrantPrivacy(ctx, dc.Serial, dc.AppID, svc); err != nil {
			return fmt.Errorf("grant %s: %w", name, err)
		}
	}
	return nil
}

// DenyPermission revokes a named permission from the app at the OS level.
func (dc *DeviceContext) DenyPermission(ctx context.Context, name string) error {
	switch dc.Platform {
	case device.PlatformAndroid:
		perms, err := device.ResolveAndroidPermissions(name)
		if err != nil {
			return err
		}
		for _, perm := range perms {
			if err := dc.Manager.ADB().RevokePermission(ctx, dc.Serial, dc.AppID, perm); err != nil {
				fmt.Printf("    \033[33m⚠\033[0m  revoke %s: %v\n", perm, err)
			}
		}
	case device.PlatformIOS:
		if dc.IsPhysical {
			fmt.Printf("    \033[33m⚠\033[0m  permission management is not supported on physical iOS devices — skipping\n")
			return nil
		}
		svc, err := device.ResolveIOSService(name)
		if err != nil {
			return err
		}
		if err := dc.Manager.SimCtl().RevokePrivacy(ctx, dc.Serial, dc.AppID, svc); err != nil {
			return fmt.Errorf("revoke %s: %w", name, err)
		}
	}
	return nil
}

// GrantAllPermissions grants all known runtime permissions to the app.
func (dc *DeviceContext) GrantAllPermissions(ctx context.Context) error {
	fmt.Printf("    \033[36m🔓\033[0m  Granting all permissions for %s\n", dc.AppID)
	switch dc.Platform {
	case device.PlatformAndroid:
		for _, perms := range device.AndroidPermissions {
			for _, perm := range perms {
				_ = dc.Manager.ADB().GrantPermission(ctx, dc.Serial, dc.AppID, perm)
			}
		}
	case device.PlatformIOS:
		if dc.IsPhysical {
			fmt.Printf("    \033[33m⚠\033[0m  permission management is not supported on physical iOS devices — skipping\n")
			return nil
		}
		for _, svc := range device.IOSPrivacyServices {
			_ = dc.Manager.SimCtl().GrantPrivacy(ctx, dc.Serial, dc.AppID, svc)
		}
	}
	return nil
}

// RevokeAllPermissions revokes all runtime permissions from the app.
func (dc *DeviceContext) RevokeAllPermissions(ctx context.Context) error {
	switch dc.Platform {
	case device.PlatformAndroid:
		for _, perms := range device.AndroidPermissions {
			for _, perm := range perms {
				_ = dc.Manager.ADB().RevokePermission(ctx, dc.Serial, dc.AppID, perm)
			}
		}
	case device.PlatformIOS:
		if dc.IsPhysical {
			fmt.Printf("    \033[33m⚠\033[0m  permission management is not supported on physical iOS devices — skipping\n")
			return nil
		}
		_ = dc.Manager.SimCtl().ResetPrivacy(ctx, dc.Serial, dc.AppID)
	}
	return nil
}

// Reconnect waits for the app to boot its agent, reads the new token,
// and establishes a fresh WebSocket connection.
func (dc *DeviceContext) Reconnect(ctx context.Context) (probelink.ProbeClient, error) {
	tokenTimeout := dc.tokenTimeout()

	switch dc.Platform {
	case device.PlatformAndroid:
		// Delete stale token files so we pick up the fresh one after relaunch
		if dc.AppID != "" {
			dc.Manager.ADB().Shell(ctx, dc.Serial, "run-as", dc.AppID, "rm", "-f", "cache/probe/token")
		}
		dc.Manager.ADB().Shell(ctx, dc.Serial, "rm", "-f", "/data/local/tmp/probe/token")
		// Also clear logcat as fallback
		dc.Manager.ADB().Run(ctx, dc.Serial, "logcat", "-c") //nolint:errcheck
		// Re-establish port forward (force-stop may have dropped it)
		devPort := dc.DevicePort
		if devPort == 0 {
			devPort = dc.Port
		}
		_ = dc.Manager.ForwardPort(ctx, dc.Serial, dc.Port, devPort)
	case device.PlatformIOS:
		if dc.IsPhysical {
			// Physical iOS: no token file to delete — idevicesyslog reads live stream
			// Just wait for the app to restart
		} else {
			// Simulator: delete stale token file so ReadTokenIOS picks up the fresh one
			simctl := dc.Manager.SimCtl()
			tokenPath := dc.iosTokenPath()
			if tokenPath != "" {
				_, _ = simctl.Spawn(ctx, dc.Serial, "rm", "-f", tokenPath)
			}
		}
	}

	// Wait for the app to start and the agent to initialize
	time.Sleep(dc.reconnectDelay())

	var token string
	var err error

	switch dc.Platform {
	case device.PlatformAndroid:
		token, err = dc.Manager.ReadTokenAndroid(ctx, dc.Serial, tokenTimeout, dc.AppID, nil)
	case device.PlatformIOS:
		token, err = dc.Manager.ReadTokenIOS(ctx, dc.Serial, tokenTimeout, dc.AppID)
	}
	if err != nil {
		return nil, fmt.Errorf("reconnect: read token: %w", err)
	}

	dialOpts := probelink.DialOptions{
		Host:        dc.agentHost(),
		Port:        dc.Port,
		Token:       token,
		DialTimeout: dc.dialTimeoutVal(),
	}

	if dc.UseHTTP {
		client, err := probelink.DialHTTP(ctx, dialOpts)
		if err != nil {
			return nil, fmt.Errorf("reconnect: http dial: %w", err)
		}
		return client, nil
	}

	client, err := probelink.DialWithOptions(ctx, dialOpts)
	if err != nil {
		return nil, fmt.Errorf("reconnect: dial: %w", err)
	}

	warning, err := probelink.CheckHandshake(ctx, client, dc.CLIVersion)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("reconnect: handshake: %w", err)
	}
	if warning != "" {
		fmt.Printf("  \033[33m⚠\033[0m  %s\n", warning)
	}

	return client, nil
}

// ReconnectWithToken reconnects using a pre-shared token (set via set_next_token
// before restart). This skips token reading from device logs — critical for WiFi
// mode where idevicesyslog is unavailable.
func (dc *DeviceContext) ReconnectWithToken(ctx context.Context, token string) (probelink.ProbeClient, error) {
	// Wait for the app to restart and the agent to initialize
	time.Sleep(dc.reconnectDelay())

	dialOpts := probelink.DialOptions{
		Host:        dc.agentHost(),
		Port:        dc.Port,
		Token:       token,
		DialTimeout: dc.dialTimeoutVal(),
	}

	// For WiFi: use the original host (not 127.0.0.1)
	// The host is embedded in the HTTPClient's baseURL, but for reconnect
	// we need to try multiple times as the app is still booting
	deadline := time.Now().Add(dc.tokenTimeout())
	for time.Now().Before(deadline) {
		var client probelink.ProbeClient
		var err error
		if dc.UseHTTP {
			client, err = probelink.DialHTTP(ctx, dialOpts)
		} else {
			client, err = probelink.DialWithOptions(ctx, dialOpts)
		}
		if err == nil {
			return client, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("reconnect with pre-shared token: agent not reachable within %s", dc.tokenTimeout())
}

// iosTokenPath returns the path to the agent's token file on the simulator.
// It checks the app container first (where the agent actually writes), then
// falls back to the device-level tmp path.
func (dc *DeviceContext) iosTokenPath() string {
	if dc.Platform != device.PlatformIOS {
		return ""
	}
	// Primary: app container path (where Dart agent writes)
	if dc.AppID != "" {
		simctl := dc.Manager.SimCtl()
		if cp := simctl.AppDataPath(context.Background(), dc.Serial, dc.AppID); cp != "" {
			return cp + "/tmp/probe/token"
		}
	}
	// Fallback: device-level tmp
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home + "/Library/Developer/CoreSimulator/Devices/" + dc.Serial + "/data/tmp/probe/token"
}

// KillApp force-stops the app without relaunching it.
func (dc *DeviceContext) KillApp(ctx context.Context) error {
	fmt.Printf("    \033[31m⏹\033[0m  Killing %s on %s\n", dc.AppID, dc.Serial)
	switch dc.Platform {
	case device.PlatformAndroid:
		if _, err := dc.Manager.ADB().Shell(ctx, dc.Serial, "am", "force-stop", dc.AppID); err != nil {
			return fmt.Errorf("kill app: %w", err)
		}
	case device.PlatformIOS:
		if dc.IsPhysical {
			_ = dc.Manager.DeviceCtl().Terminate(ctx, dc.Serial, dc.AppID)
		} else {
			_ = dc.Manager.SimCtl().Terminate(ctx, dc.Serial, dc.AppID)
		}
	}
	return nil
}

// LaunchApp launches the app without force-stopping first.
func (dc *DeviceContext) LaunchApp(ctx context.Context) error {
	fmt.Printf("    \033[32m▶\033[0m  Launching %s on %s\n", dc.AppID, dc.Serial)
	switch dc.Platform {
	case device.PlatformAndroid:
		if _, err := dc.Manager.ADB().Shell(ctx, dc.Serial,
			"am", "start", "-n", dc.Manager.ADB().ResolveLauncherActivity(ctx, dc.Serial, dc.AppID)); err != nil {
			return fmt.Errorf("launch app: %w", err)
		}
	case device.PlatformIOS:
		if dc.IsPhysical {
			if err := dc.Manager.DeviceCtl().Launch(ctx, dc.Serial, dc.AppID); err != nil {
				return fmt.Errorf("launch app: %w", err)
			}
		} else {
			if err := dc.Manager.SimCtl().Launch(ctx, dc.Serial, dc.AppID); err != nil {
				return fmt.Errorf("launch app: %w", err)
			}
		}
	}
	return nil
}

// SetLocation sets the device's GPS location.
// Not supported on physical devices — skips with a warning.
func (dc *DeviceContext) SetLocation(ctx context.Context, lat, lng string) error {
	if dc.IsPhysical {
		fmt.Printf("    \033[33m⚠\033[0m  set location is not supported on physical devices — skipping\n")
		return nil
	}
	fmt.Printf("    \033[36m📍\033[0m  Setting location to %s, %s\n", lat, lng)
	switch dc.Platform {
	case device.PlatformAndroid:
		if _, err := dc.Manager.ADB().Shell(ctx, dc.Serial, "emu", "geo", "fix", lng, lat); err != nil {
			return fmt.Errorf("set location: %w", err)
		}
	case device.PlatformIOS:
		simctl := dc.Manager.SimCtl()
		if err := simctl.SetLocation(ctx, dc.Serial, lat, lng); err != nil {
			return fmt.Errorf("set location: %w", err)
		}
	}
	return nil
}

// EnrollBiometric sets the simulator/emulator's biometric enrollment state
// to "enrolled" so the app under test sees a registered Face ID / Touch ID /
// fingerprint when it requests biometric authentication.
//
//   - iOS Simulator: sends the Darwin notification
//     `com.apple.BiometricKit.enrollmentChanged` (toggles state).
//   - Android emulator: no-op — fingerprints are enrolled in Settings before
//     the test runs (typically via a CI bootstrap script).
//   - Physical devices: skipped with a warning.
func (dc *DeviceContext) EnrollBiometric(ctx context.Context) error {
	if dc.IsPhysical {
		fmt.Println("    \033[33m⚠\033[0m  enroll biometric is not supported on physical devices — skipping")
		return nil
	}
	fmt.Println("    \033[36m🔐\033[0m  Enrolling biometric")
	switch dc.Platform {
	case device.PlatformIOS:
		if _, err := dc.Manager.SimCtl().Spawn(ctx, dc.Serial,
			"notifyutil", "-s", "com.apple.BiometricKit.enrollmentChanged", "1"); err != nil {
			return fmt.Errorf("enroll biometric: set enrollment flag: %w", err)
		}
		if _, err := dc.Manager.SimCtl().Spawn(ctx, dc.Serial,
			"notifyutil", "-p", "com.apple.BiometricKit.enrollmentChanged"); err != nil {
			return fmt.Errorf("enroll biometric: post enrollment notification: %w", err)
		}
	case device.PlatformAndroid:
		// Android fingerprints are enrolled in Settings, not via adb. We
		// document the requirement in the .probe error message rather than
		// failing here — the user's CI script should pre-enroll.
		fmt.Println("       (Android: ensure fingerprint ID 1 is pre-enrolled in Settings)")
	}
	return nil
}

// BiometricMatch simulates a successful biometric capture, satisfying a
// pending Face ID / Touch ID / fingerprint prompt.
//
//   - iOS Simulator: posts `com.apple.BiometricKit_Sim.fingerTouch.match`
//     and `.faceCapture.match` so the same step works regardless of the
//     simulator's biometric kind.
//   - Android emulator: `adb -s <serial> emu finger touch 1` (matches the
//     fingerprint enrolled with ID 1).
//   - Physical devices: skipped with a warning.
func (dc *DeviceContext) BiometricMatch(ctx context.Context) error {
	return dc.biometricCapture(ctx, true)
}

// BiometricNoMatch simulates a failed biometric capture so the app's
// "authentication failed" path can be tested.
//
//   - iOS Simulator: posts `*_Sim.fingerTouch.no-match` and `.faceCapture.no-match`.
//   - Android emulator: `adb emu finger touch 9999` (an unregistered id).
//   - Physical devices: skipped with a warning.
func (dc *DeviceContext) BiometricNoMatch(ctx context.Context) error {
	return dc.biometricCapture(ctx, false)
}

func (dc *DeviceContext) biometricCapture(ctx context.Context, match bool) error {
	if dc.IsPhysical {
		fmt.Println("    \033[33m⚠\033[0m  biometric capture is not supported on physical devices — skipping")
		return nil
	}
	verb := "match"
	icon := "✓"
	if !match {
		verb = "no-match"
		icon = "✗"
	}
	fmt.Printf("    \033[36m🔐\033[0m  Biometric capture: %s %s\n", icon, verb)
	switch dc.Platform {
	case device.PlatformIOS:
		// Post both fingerprint and face notifications so the same step
		// works on Touch ID devices and Face ID devices alike — the simulator
		// ignores the one that doesn't match its hardware profile.
		// Note: on iOS 26+ simulator, no-match notifications no longer resolve
		// LAContext.evaluatePolicy. Apps using flutter_probe_agent should call
		// awaitBiometricResult() instead of local_auth.authenticate() when
		// PROBE_AGENT=true; the CLI sends probe.biometric_signal to resolve it.
		notifications := []string{
			fmt.Sprintf("com.apple.BiometricKit_Sim.fingerTouch.%s", verb),
			fmt.Sprintf("com.apple.BiometricKit_Sim.faceCapture.%s", verb),
		}
		for _, n := range notifications {
			if _, err := dc.Manager.SimCtl().Spawn(ctx, dc.Serial, "notifyutil", "-p", n); err != nil {
				return fmt.Errorf("biometric %s: post %s: %w", verb, n, err)
			}
		}
	case device.PlatformAndroid:
		// Fingerprint ID 1 is matching by convention; any unregistered ID
		// (we use 9999) returns no-match. The user's CI bootstrap script
		// enrolls fingerprint ID 1 before tests run.
		fingerID := "1"
		if !match {
			fingerID = "9999"
		}
		if _, err := dc.Manager.ADB().Run(ctx, dc.Serial, "emu", "finger", "touch", fingerID); err != nil {
			return fmt.Errorf("biometric %s: adb emu finger touch %s: %w", verb, fingerID, err)
		}
	}
	return nil
}
