// Package ios wraps Xcode's xcrun simctl for iOS simulator management.
package ios

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
func (s *SimCtl) WaitForBoot(ctx context.Context, udid string, timeout time.Duration) error {
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
		time.Sleep(2 * time.Second)
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

// ReadToken reads the ProbeAgent token from the simulator's syslog.
func (s *SimCtl) ReadToken(ctx context.Context, udid string, timeout time.Duration) (string, error) {
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(tCtx, "xcrun", "simctl", "spawn", udid, "log", "stream",
		"--predicate", `process == "Runner" && messageType == "info"`)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	defer cmd.Process.Kill() //nolint:errcheck

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "PROBE_TOKEN="); idx >= 0 {
			return strings.TrimSpace(line[idx+len("PROBE_TOKEN="):]), nil
		}
	}
	return "", fmt.Errorf("ios: probe token not found within %s", timeout)
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
