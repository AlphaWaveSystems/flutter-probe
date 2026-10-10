package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ios"
)

// Simulator is the named iOS simulator the suite runs the fixture app on.
// Every device the suite opens has a name (never an anonymous "booted").
type Simulator struct {
	Name string
	UDID string
	sim  *ios.SimCtl
}

// EnsureSimulator finds or creates a simulator with the given name and
// boots it. deviceType/runtime are simctl identifiers, e.g.
// "com.apple.CoreSimulator.SimDeviceType.iPhone-17" and
// "com.apple.CoreSimulator.SimRuntime.iOS-26-5"; empty runtime picks the
// newest installed iOS runtime.
func EnsureSimulator(ctx context.Context, name, deviceType, runtime string) (*Simulator, error) {
	sc := ios.New()
	sims, err := sc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("simctl list: %w", err)
	}
	s := &Simulator{Name: name, sim: sc}
	for _, d := range sims {
		if d.Name == name {
			s.UDID = d.UDID
			break
		}
	}
	if s.UDID == "" {
		if runtime == "" {
			runtime, err = newestIOSRuntime(ctx)
			if err != nil {
				return nil, err
			}
		}
		udid, err := sc.Create(ctx, name, deviceType, runtime)
		if err != nil {
			return nil, fmt.Errorf("simctl create %s: %w", name, err)
		}
		s.UDID = udid
	}
	if err := sc.Boot(ctx, s.UDID); err != nil && !strings.Contains(err.Error(), "Booted") {
		return nil, fmt.Errorf("simctl boot %s: %w", name, err)
	}
	// bootstatus -b blocks until SpringBoard is up; without it the first
	// install/launch races the boot and fails with FBSOpenApplication errors.
	cmd := exec.CommandContext(ctx, "xcrun", "simctl", "bootstatus", s.UDID, "-b")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("bootstatus: %v: %s", err, out)
	}
	return s, nil
}

// InstallAndLaunch installs the .app bundle and (re)launches bundleID.
func (s *Simulator) InstallAndLaunch(ctx context.Context, appPath, bundleID string) error {
	if err := s.sim.Install(ctx, s.UDID, appPath); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	_ = s.sim.Terminate(ctx, s.UDID, bundleID)
	if err := s.sim.Launch(ctx, s.UDID, bundleID); err != nil {
		return fmt.Errorf("launch: %w", err)
	}
	// Wait until the app's agent is accepting connections (it also writes
	// its token on startup); a fixed sleep raced slow launches.
	return waitForAgent(ctx, agentPort(), 20*time.Second)
}

// agentPort is the port the fixture app's agent listens on
// (STUDIO_E2E_AGENT_PORT, default 48686 — the agent's default when the
// workspace has no probe.yaml).
func agentPort() int {
	if v := os.Getenv("STUDIO_E2E_AGENT_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 48686
}

func waitForAgent(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for {
		c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			c.Close()
			// Let the agent finish its own startup after the socket opens.
			time.Sleep(1 * time.Second)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("agent not listening on %s after %s", addr, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
}

// Terminate stops the app so the next test starts from a known state.
func (s *Simulator) Terminate(ctx context.Context, bundleID string) {
	_ = s.sim.Terminate(ctx, s.UDID, bundleID)
}

// Tap clicks inside this simulator's Simulator.app window at a fraction of
// its width/height (0..1) — a real user gesture on the device, which the
// agent's recorder sees. The window is found by the simulator's name.
// MoveToTargetDisplay puts this simulator's own window (matched by its name,
// never another session's simulator) on the suite's display.
func (s *Simulator) MoveToTargetDisplay() error {
	return moveWindowToDisplay("Simulator", s.Name)
}

func (s *Simulator) Tap(fx, fy float64) error {
	script := fmt.Sprintf(`tell application "System Events" to tell process "Simulator"
  set frontmost to true
  repeat with w in windows
    if name of w starts with %q then
      set p to position of w
      set sz to size of w
      return (item 1 of p as text) & "," & (item 2 of p as text) & "," & (item 1 of sz as text) & "," & (item 2 of sz as text)
    end if
  end repeat
  error "no Simulator window for " & %q
end tell`, s.Name, s.Name)
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("simulator window: %v: %s", err, strings.TrimSpace(string(out)))
	}
	var x, y, w, h int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d,%d,%d,%d", &x, &y, &w, &h); err != nil {
		return fmt.Errorf("simulator window rect %q: %w", out, err)
	}
	cx, cy := x+int(float64(w)*fx), y+int(float64(h)*fy)
	if path, err := exec.LookPath("cliclick"); err == nil {
		if out, err := exec.Command(path, clickArg(cx, cy)).CombinedOutput(); err != nil {
			return fmt.Errorf("cliclick: %v: %s", err, out)
		}
		return nil
	}
	return exec.Command("osascript", "-e", fmt.Sprintf(`tell application "System Events" to click at {%d, %d}`, cx, cy)).Run()
}

func newestIOSRuntime(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "xcrun", "simctl", "list", "runtimes").Output()
	if err != nil {
		return "", err
	}
	var last string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "iOS ") {
			if i := strings.LastIndex(line, " - "); i >= 0 {
				last = strings.TrimSpace(line[i+3:])
			}
		}
	}
	if last == "" {
		return "", fmt.Errorf("no iOS runtime installed")
	}
	return last, nil
}
