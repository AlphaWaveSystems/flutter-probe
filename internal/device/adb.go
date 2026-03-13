package device

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ADB wraps the Android Debug Bridge binary.
type ADB struct {
	bin string // path to adb binary
}

// NewADB creates an ADB wrapper using "adb" from PATH.
func NewADB() *ADB {
	return &ADB{bin: "adb"}
}

// Devices returns all currently connected Android emulators/devices.
func (a *ADB) Devices(ctx context.Context) ([]Device, error) {
	out, err := a.run(ctx, "devices", "-l")
	if err != nil {
		return nil, fmt.Errorf("adb devices: %w", err)
	}

	var devices []Device
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "List of") || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		serial := parts[0]
		state := parts[1]
		name := serial
		// Extract "model:" from the long listing
		for _, f := range parts[2:] {
			if strings.HasPrefix(f, "model:") {
				name = strings.ReplaceAll(strings.TrimPrefix(f, "model:"), "_", " ")
				break
			}
		}
		devices = append(devices, Device{
			ID:       serial,
			Name:     name,
			Platform: PlatformAndroid,
			State:    state,
		})
	}
	return devices, nil
}

// StartEmulator boots an AVD and returns a Device once it appears.
func (a *ADB) StartEmulator(ctx context.Context, avdName string) (*Device, error) {
	// Start the emulator in background
	cmd := exec.CommandContext(ctx, "emulator", "-avd", avdName, "-no-audio", "-no-boot-anim")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("adb: emulator start: %w", err)
	}

	// Poll for the device to appear
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		devices, err := a.Devices(ctx)
		if err == nil {
			for _, d := range devices {
				if strings.Contains(d.Name, avdName) || strings.Contains(d.ID, "emulator") {
					return &d, nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("adb: emulator %q did not appear within 120s", avdName)
}

// Forward creates an adb forward rule: tcp:hostPort -> tcp:devicePort.
func (a *ADB) Forward(ctx context.Context, serial string, hostPort, devicePort int) error {
	rule := fmt.Sprintf("tcp:%d", hostPort)
	target := fmt.Sprintf("tcp:%d", devicePort)
	_, err := a.run(ctx, "-s", serial, "forward", rule, target)
	if err != nil {
		return fmt.Errorf("adb forward: %w", err)
	}
	return nil
}

// RemoveForward removes an adb forward rule for the given host port.
func (a *ADB) RemoveForward(ctx context.Context, serial string, hostPort int) error {
	rule := fmt.Sprintf("tcp:%d", hostPort)
	_, err := a.run(ctx, "-s", serial, "forward", "--remove", rule)
	return err
}

// Shell runs an adb shell command on the given device.
func (a *ADB) Shell(ctx context.Context, serial string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"-s", serial, "shell"}, args...)
	return a.run(ctx, cmdArgs...)
}

// Pull copies a file from the device to the host.
func (a *ADB) Pull(ctx context.Context, serial, remotePath, localPath string) error {
	_, err := a.run(ctx, "-s", serial, "pull", remotePath, localPath)
	return err
}

// run executes an adb command and returns combined stdout.
func (a *ADB) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, a.bin, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s: %s", strings.Join(args, " "), ee.Stderr)
		}
		return nil, err
	}
	return out, nil
}

// ListAVDs returns the list of available AVD names.
func (a *ADB) ListAVDs(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "emulator", "-list-avds").Output()
	if err != nil {
		return nil, err
	}
	var avds []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			avds = append(avds, name)
		}
	}
	return avds, nil
}
