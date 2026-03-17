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
	AllowClearData          bool            // if true, skip confirmation for clear app data (CI/CD mode)
	Confirm                 ConfirmFunc     // interactive confirmation callback (nil = deny destructive ops unless AllowClearData)
	GrantPermissionsOnClear bool            // if true, auto-grant all permissions after clearing data
	ReconnectDelay          time.Duration   // delay after app restart before reconnecting WebSocket (default 2s)
	RestartDelay            time.Duration   // delay after force-stop before relaunching (default 500ms)
	TokenReadTimeout        time.Duration   // max time to wait for agent token during reconnect (default 30s)
	DialTimeout             time.Duration   // max time to establish WebSocket connection (default 30s)
}

// reconnectDelay returns the configured reconnect delay or the default.
func (dc *DeviceContext) reconnectDelay() time.Duration {
	if dc.ReconnectDelay > 0 {
		return dc.ReconnectDelay
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
		// Launch via monkey (works without knowing the main activity name)
		if _, err := dc.Manager.ADB().Shell(ctx, dc.Serial,
			"monkey", "-p", dc.AppID,
			"-c", "android.intent.category.LAUNCHER", "1"); err != nil {
			return fmt.Errorf("restart: launch: %w", err)
		}

	case device.PlatformIOS:
		simctl := dc.Manager.SimCtl()
		_ = simctl.Terminate(ctx, dc.Serial, dc.AppID) // ignore if not running
		time.Sleep(dc.restartDelay())
		fmt.Printf("    \033[33m↻\033[0m  Relaunching %s...\n", dc.AppID)
		if err := simctl.Launch(ctx, dc.Serial, dc.AppID); err != nil {
			return fmt.Errorf("restart: launch: %w", err)
		}
	}
	return nil
}

// ClearAppData clears all app storage and relaunches. This is a destructive
// operation that wipes SharedPreferences, databases, and all local files.
// It requires explicit opt-in via --allow-clear-data flag or interactive confirmation.
func (dc *DeviceContext) ClearAppData(ctx context.Context) error {
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
			"monkey", "-p", dc.AppID,
			"-c", "android.intent.category.LAUNCHER", "1"); err != nil {
			return fmt.Errorf("clear data: relaunch: %w", err)
		}

	case device.PlatformIOS:
		simctl := dc.Manager.SimCtl()
		_ = simctl.Terminate(ctx, dc.Serial, dc.AppID)

		// Get the app data container path via simctl (official Apple tooling)
		dataPath := simctl.AppDataPath(ctx, dc.Serial, dc.AppID)
		if err := dc.validateIOSDataPath(dataPath); err != nil {
			return fmt.Errorf("clear data: %w", err)
		}

		// Clear data container contents (not the container dir itself)
		// This is safer than rm -rf on the container — we only remove the contents
		if dataPath != "" {
			for _, subdir := range []string{"Documents", "Library", "tmp"} {
				target := dataPath + "/" + subdir
				_, _ = simctl.Spawn(ctx, dc.Serial, "rm", "-rf", target)
			}
			fmt.Printf("    \033[32m✓\033[0m  Cleared data container: %s\n", dataPath)
		}

		// Auto-grant permissions before relaunch to prevent OS permission dialogs
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
				// Best-effort: some permissions may not apply to this API level
				_ = dc.Manager.ADB().GrantPermission(ctx, dc.Serial, dc.AppID, perm)
			}
		}
	case device.PlatformIOS:
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
		_ = dc.Manager.SimCtl().ResetPrivacy(ctx, dc.Serial, dc.AppID)
	}
	return nil
}

// Reconnect waits for the app to boot its agent, reads the new token,
// and establishes a fresh WebSocket connection.
func (dc *DeviceContext) Reconnect(ctx context.Context) (*probelink.Client, error) {
	tokenTimeout := dc.tokenTimeout()

	switch dc.Platform {
	case device.PlatformAndroid:
		// Clear logcat so we only see the new token
		dc.Manager.ADB().Run(ctx, dc.Serial, "logcat", "-c") //nolint:errcheck
		// Re-establish port forward (force-stop may have dropped it)
		devPort := dc.DevicePort
		if devPort == 0 {
			devPort = dc.Port
		}
		_ = dc.Manager.ForwardPort(ctx, dc.Serial, dc.Port, devPort)
	case device.PlatformIOS:
		// Delete stale token file so ReadTokenIOS picks up the fresh one
		simctl := dc.Manager.SimCtl()
		tokenPath := dc.iosTokenPath()
		if tokenPath != "" {
			_, _ = simctl.Spawn(ctx, dc.Serial, "rm", "-f", tokenPath)
		}
	}

	// Wait for the app to start and the agent to initialize
	time.Sleep(dc.reconnectDelay())

	var token string
	var err error

	switch dc.Platform {
	case device.PlatformAndroid:
		token, err = dc.Manager.ReadToken(ctx, dc.Serial, tokenTimeout)
	case device.PlatformIOS:
		token, err = dc.Manager.ReadTokenIOS(ctx, dc.Serial, tokenTimeout)
	}
	if err != nil {
		return nil, fmt.Errorf("reconnect: read token: %w", err)
	}

	client, err := probelink.DialWithOptions(ctx, probelink.DialOptions{
		Host:        "127.0.0.1",
		Port:        dc.Port,
		Token:       token,
		DialTimeout: dc.dialTimeoutVal(),
	})
	if err != nil {
		return nil, fmt.Errorf("reconnect: dial: %w", err)
	}

	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("reconnect: ping: %w", err)
	}

	return client, nil
}

// iosTokenPath returns the path to the agent's token file on the simulator.
func (dc *DeviceContext) iosTokenPath() string {
	if dc.Platform != device.PlatformIOS {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home + "/Library/Developer/CoreSimulator/Devices/" + dc.Serial + "/data/tmp/probe/token"
}
