// Package ios wraps Xcode's xcrun simctl for iOS simulator management.
package ios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Simulator represents an iOS simulator.
type Simulator struct {
	UDID          string
	Name          string
	State         string // Booted | Shutdown
	Runtime       string // com.apple.CoreSimulator.SimRuntime.iOS-17-5
	DeviceTypeID  string
}

// HumanRuntime converts the raw runtime string (e.g.
// "com.apple.CoreSimulator.SimRuntime.iOS-18-6") to a human-readable form
// like "iOS 18.6".
func (s Simulator) HumanRuntime() string {
	// e.g. "com.apple.CoreSimulator.SimRuntime.iOS-18-6"
	r := s.Runtime
	if idx := strings.LastIndex(r, "."); idx >= 0 {
		r = r[idx+1:] // "iOS-18-6"
	}
	// Split on first dash to separate platform from version numbers
	parts := strings.SplitN(r, "-", 2) // ["iOS", "18-6"]
	if len(parts) == 2 {
		ver := strings.ReplaceAll(parts[1], "-", ".")
		return parts[0] + " " + ver // "iOS 18.6"
	}
	return r
}

// SimCtl wraps xcrun simctl.
type SimCtl struct{}

func New() *SimCtl { return &SimCtl{} }

// List returns all available iOS simulators.
func (s *SimCtl) List(ctx context.Context) ([]Simulator, error) {
	out, err := s.run(ctx, "list", "devices", "--json")
	if err != nil {
		return nil, fmt.Errorf("simctl list: %w", err)
	}

	var result struct {
		Devices map[string][]struct {
			UDID         string `json:"udid"`
			Name         string `json:"name"`
			State        string `json:"state"`
			DeviceTypeID string `json:"deviceTypeIdentifier"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("simctl parse: %w", err)
	}

	var sims []Simulator
	for runtime, devices := range result.Devices {
		for _, d := range devices {
			sims = append(sims, Simulator{
				UDID:         d.UDID,
				Name:         d.Name,
				State:        d.State,
				Runtime:      runtime,
				DeviceTypeID: d.DeviceTypeID,
			})
		}
	}
	return sims, nil
}

// Boot boots a simulator by UDID.
func (s *SimCtl) Boot(ctx context.Context, udid string) error {
	_, err := s.run(ctx, "boot", udid)
	if err != nil && !strings.Contains(err.Error(), "already booted") {
		return fmt.Errorf("simctl boot: %w", err)
	}
	return nil
}

// Shutdown shuts down a simulator.
func (s *SimCtl) Shutdown(ctx context.Context, udid string) error {
	_, err := s.run(ctx, "shutdown", udid)
	return err
}

// WaitForBoot polls until the simulator is booted or timeout elapses.
// pollInterval controls how often to check (0 = default 2s).
func (s *SimCtl) WaitForBoot(ctx context.Context, udid string, timeout, pollInterval time.Duration) error {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sims, err := s.List(ctx)
		if err == nil {
			for _, sim := range sims {
				if sim.UDID == udid && sim.State == "Booted" {
					return nil
				}
			}
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("simctl: simulator %s did not boot within %s", udid, timeout)
}

// Install installs an .app bundle on the simulator.
func (s *SimCtl) Install(ctx context.Context, udid, appPath string) error {
	_, err := s.run(ctx, "install", udid, appPath)
	return err
}

// Launch launches an installed app by bundle ID.
func (s *SimCtl) Launch(ctx context.Context, udid, bundleID string) error {
	_, err := s.run(ctx, "launch", udid, bundleID)
	return err
}

// Terminate kills a running app.
func (s *SimCtl) Terminate(ctx context.Context, udid, bundleID string) error {
	_, err := s.run(ctx, "terminate", udid, bundleID)
	return err
}

// IO exposes screenshot and video capture.
func (s *SimCtl) Screenshot(ctx context.Context, udid, outPath string) error {
	_, err := s.run(ctx, "io", udid, "screenshot", outPath)
	return err
}

// Spawn runs a binary inside the simulator.
func (s *SimCtl) Spawn(ctx context.Context, udid string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"spawn", udid}, args...)
	return s.run(ctx, cmdArgs...)
}

// ReadToken reads the ProbeAgent token. It first tries the token file written
// by the agent (checking both the app container path and the legacy device-level
// path), then falls back to polling the simulator's system log via `log show`.
// bundleID is optional — if provided, the app container path is checked first
// for faster token pickup.
func (s *SimCtl) ReadToken(ctx context.Context, udid string, timeout time.Duration, bundleID ...string) (string, error) {
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build list of token file paths to check
	var tokenPaths []string

	// Primary: app container path (where the Dart agent actually writes)
	if len(bundleID) > 0 && bundleID[0] != "" {
		if containerPath := s.appContainerDataPath(tCtx, udid, bundleID[0]); containerPath != "" {
			tokenPaths = append(tokenPaths, containerPath+"/tmp/probe/token")
		}
	}

	// Fallback: device-level tmp (legacy path)
	tokenPaths = append(tokenPaths, s.simDataPath(udid)+"/tmp/probe/token")

	// Poll token file until timeout. Re-resolve the app container path
	// periodically in case the container changed (e.g. after clear app data).
	fileAttempt := 0
	for {
		// Re-resolve container path every 5 attempts (container ID may change)
		if fileAttempt > 0 && fileAttempt%5 == 0 && len(bundleID) > 0 && bundleID[0] != "" {
			if cp := s.appContainerDataPath(tCtx, udid, bundleID[0]); cp != "" {
				newPath := cp + "/tmp/probe/token"
				if len(tokenPaths) == 0 || tokenPaths[0] != newPath {
					tokenPaths = append([]string{newPath}, tokenPaths...)
				}
			}
		}

		for _, tp := range tokenPaths {
			if data, err := os.ReadFile(tp); err == nil {
				token := strings.TrimSpace(string(data))
				if len(token) >= 16 {
					return token, nil
				}
			}
		}

		fileAttempt++
		select {
		case <-tCtx.Done():
			return "", fmt.Errorf("ios: probe token not found within %s", timeout)
		case <-time.After(1 * time.Second):
		}
	}
}

// appContainerDataPath returns the data container path for an app on the simulator.
func (s *SimCtl) appContainerDataPath(ctx context.Context, udid, bundleID string) string {
	out, err := s.run(ctx, "get_app_container", udid, bundleID, "data")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GrantPrivacy grants a privacy service permission to an app on the simulator.
func (s *SimCtl) GrantPrivacy(ctx context.Context, udid, bundleID, service string) error {
	_, err := s.run(ctx, "privacy", udid, "grant", service, bundleID)
	return err
}

// RevokePrivacy revokes a privacy service permission from an app on the simulator.
func (s *SimCtl) RevokePrivacy(ctx context.Context, udid, bundleID, service string) error {
	_, err := s.run(ctx, "privacy", udid, "revoke", service, bundleID)
	return err
}

// ResetPrivacy resets all privacy permissions for an app on the simulator.
func (s *SimCtl) ResetPrivacy(ctx context.Context, udid, bundleID string) error {
	_, err := s.run(ctx, "privacy", udid, "reset", "all", bundleID)
	return err
}

// KeychainReset resets the simulator's keychain, clearing all stored passwords,
// tokens, and certificates. This is necessary because the Keychain persists
// outside the app's data container and survives app data clearing.
func (s *SimCtl) KeychainReset(ctx context.Context, udid string) error {
	_, err := s.run(ctx, "keychain", udid, "reset")
	return err
}

// Uninstall removes an app from the simulator.
func (s *SimCtl) Uninstall(ctx context.Context, udid, bundleID string) error {
	_, err := s.run(ctx, "uninstall", udid, bundleID)
	return err
}

// AppDataPath returns the data container path for an app on the simulator.
func (s *SimCtl) AppDataPath(ctx context.Context, udid, bundleID string) string {
	out, err := s.run(ctx, "get_app_container", udid, bundleID, "data")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// simDataPath returns the data directory for a simulator UDID.
func (s *SimCtl) simDataPath(udid string) string {
	home, _ := os.UserHomeDir()
	return home + "/Library/Developer/CoreSimulator/Devices/" + udid + "/data"
}

// ForwardPort uses idb or socat to forward a port from simulator to host.
// Falls back to direct connection since simulator shares the host network.
func (s *SimCtl) ForwardPort(_ context.Context, _, _ int) error {
	// iOS simulators share the host's loopback — no forwarding needed.
	return nil
}

// run executes xcrun simctl with the given arguments.
func (s *SimCtl) run(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{"simctl"}, args...)
	cmd := exec.CommandContext(ctx, "xcrun", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("xcrun simctl %s: %s", strings.Join(args, " "), stderr.String())
	}
	return out, nil
}

// AutoSelect picks the best available booted simulator, or the first available.
func (s *SimCtl) AutoSelect(ctx context.Context) (*Simulator, error) {
	sims, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	// Prefer booted
	for i := range sims {
		if sims[i].State == "Booted" {
			return &sims[i], nil
		}
	}
	if len(sims) > 0 {
		return &sims[0], nil
	}
	return nil, fmt.Errorf("no iOS simulators available — create one with Xcode")
}
