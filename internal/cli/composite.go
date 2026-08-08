package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// parseCompositeDeviceSpecs merges CLI flags with probe.yaml composite.devices.
// CLI flags take priority. Returns a map of alias → connection spec.
//
// Connection spec formats:
//   - "host:port/token"   — WiFi/HTTP direct connect
//   - "<ios-udid>"        — iOS simulator (auto token read, WebSocket)
//   - "<android-serial>"  — Android device (ADB forward, token read, WebSocket)
func parseCompositeDeviceSpecs(flags []string, yamlDevices map[string]string) map[string]string {
	specs := make(map[string]string)
	// probe.yaml values as baseline
	for k, v := range yamlDevices {
		specs[k] = v
	}
	// CLI flags override
	for _, f := range flags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) == 2 {
			specs[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return specs
}

// setupCompositeDevices establishes connections for all composite device aliases.
// Returns the connected devices and a cleanup function (closing all clients).
//
// Supported spec formats:
//   - "host:port/token" — WiFi or physical iOS HTTP mode
//   - "<udid>"          — iOS simulator (36-char UUID with dashes)
//   - "<serial>"        — Android device (ADB serial)
func setupCompositeDevices(
	ctx context.Context,
	cfg *config.Config,
	dm *device.Manager,
	specs map[string]string,
	timeout time.Duration,
	verbose bool,
) ([]runner.CompositeDevice, func(), error) {
	if len(specs) == 0 {
		return nil, func() {}, nil
	}

	var devices []runner.CompositeDevice
	var closers []io.Closer

	cleanup := func() {
		for _, c := range closers {
			c.Close()
		}
	}

	for alias, spec := range specs {
		dev, client, err := connectCompositeDevice(ctx, cfg, dm, alias, spec, verbose)
		if err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("composite device %s=%q: %w", alias, spec, err)
		}
		closers = append(closers, client)
		devices = append(devices, dev)
	}

	return devices, cleanup, nil
}

// connectCompositeDevice connects to a single composite device by spec and
// returns a CompositeDevice along with the underlying client (for cleanup).
func connectCompositeDevice(
	ctx context.Context,
	cfg *config.Config,
	dm *device.Manager,
	alias, spec string,
	verbose bool,
) (runner.CompositeDevice, probelink.ProbeClient, error) {
	// WiFi/HTTP format: "host:port/token"
	if idx := strings.Index(spec, "/"); idx > 0 {
		hostPort := spec[:idx]
		token := spec[idx+1:]

		// Parse host and port from "host:port"
		lastColon := strings.LastIndex(hostPort, ":")
		if lastColon < 0 {
			return runner.CompositeDevice{}, nil, fmt.Errorf("invalid WiFi spec %q: expected host:port/token", spec)
		}
		host := hostPort[:lastColon]
		portStr := hostPort[lastColon+1:]
		port := 0
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port == 0 {
			return runner.CompositeDevice{}, nil, fmt.Errorf("invalid port in %q", spec)
		}

		dialOpts := probelink.DialOptions{
			Host:        host,
			Port:        port,
			Token:       token,
			DialTimeout: cfg.Agent.DialTimeout,
		}
		client, err := probelink.DialHTTP(ctx, dialOpts)
		if err != nil {
			return runner.CompositeDevice{}, nil, fmt.Errorf("dial: %w", err)
		}
		warning, err := probelink.CheckHandshake(ctx, client, cfg.CLIVersion)
		if err != nil {
			client.Close()
			return runner.CompositeDevice{}, nil, fmt.Errorf("handshake: %w", err)
		}
		if warning != "" {
			fmt.Printf("  \033[33m⚠\033[0m  [%s] %s\n", alias, warning)
		}

		dev := runner.CompositeDevice{
			Alias:      alias,
			Client:     client,
			DeviceID:   spec,
			DeviceName: fmt.Sprintf("%s [WiFi]", alias),
		}
		fmt.Printf("  \033[32m✓\033[0m  [%s] Connected via WiFi to %s\n", alias, hostPort)
		return dev, client, nil
	}

	// Local device format (UDID or ADB serial) — requires device manager.
	if dm == nil {
		return runner.CompositeDevice{}, nil, fmt.Errorf("device manager not available; use host:port/token WiFi format for composite devices")
	}

	// Detect platform from connected device list.
	platform := device.Platform(cfg.Defaults.Platform)
	deviceName := spec
	devList, _ := dm.List(ctx)
	for _, d := range devList {
		if d.ID == spec {
			platform = d.Platform
			deviceName = d.Name
			break
		}
	}

	var (
		client    probelink.ProbeClient
		devCtx    *runner.DeviceContext
		err       error
	)

	dialOpts := probelink.DialOptions{
		Host:        "127.0.0.1",
		Port:        cfg.Agent.Port,
		DialTimeout: cfg.Agent.DialTimeout,
	}

	switch platform {
	case device.PlatformIOS:
		isPhysical := dm.IsPhysicalIOS(ctx, spec)
		if isPhysical {
			// Physical iOS: set up iproxy
			cleanupIProxy, iproxyErr := dm.EnsureIProxy(ctx, spec, cfg.Agent.Port, cfg.Agent.AgentDevicePort())
			if iproxyErr != nil {
				return runner.CompositeDevice{}, nil, fmt.Errorf("iproxy: %w", iproxyErr)
			}
			_ = cleanupIProxy // intentional: iproxy outlives this function; caller closes client
		}
		token, tokenErr := dm.ReadTokenIOS(ctx, spec, cfg.Agent.TokenReadTimeout, cfg.Project.App)
		if tokenErr != nil {
			return runner.CompositeDevice{}, nil, fmt.Errorf("token: %w", tokenErr)
		}
		dialOpts.Token = token
		if isPhysical {
			client, err = probelink.DialHTTP(ctx, dialOpts)
		} else {
			client, err = probelink.DialWithOptions(ctx, dialOpts)
		}
		if err != nil {
			return runner.CompositeDevice{}, nil, fmt.Errorf("dial: %w", err)
		}
		devCtx = &runner.DeviceContext{
			Manager:          dm,
			Serial:           spec,
			Platform:         platform,
			AppID:            cfg.Project.App,
			Port:             cfg.Agent.Port,
			DevicePort:       cfg.Agent.AgentDevicePort(),
			IsPhysical:       isPhysical,
			UseHTTP:          isPhysical,
			ReconnectDelay:   cfg.Agent.ReconnectDelay,
			RestartDelay:     cfg.Device.RestartDelay,
			TokenReadTimeout: cfg.Agent.TokenReadTimeout,
			DialTimeout:      cfg.Agent.DialTimeout,
			CLIVersion:       cfg.CLIVersion,
		}

	default: // Android
		if err := dm.EnsureADB(ctx, spec, cfg.Agent.Port); err != nil {
			return runner.CompositeDevice{}, nil, fmt.Errorf("adb: %w", err)
		}
		if err := dm.ForwardPort(ctx, spec, cfg.Agent.Port, cfg.Agent.AgentDevicePort()); err != nil {
			return runner.CompositeDevice{}, nil, fmt.Errorf("port forward: %w", err)
		}
		token, tokenErr := dm.ReadToken(ctx, spec, cfg.Agent.TokenReadTimeout)
		if tokenErr != nil {
			return runner.CompositeDevice{}, nil, fmt.Errorf("token: %w", tokenErr)
		}
		dialOpts.Token = token
		client, err = probelink.DialWithOptions(ctx, dialOpts)
		if err != nil {
			return runner.CompositeDevice{}, nil, fmt.Errorf("dial: %w", err)
		}
		devCtx = &runner.DeviceContext{
			Manager:          dm,
			Serial:           spec,
			Platform:         platform,
			AppID:            cfg.Project.App,
			Port:             cfg.Agent.Port,
			DevicePort:       cfg.Agent.AgentDevicePort(),
			ReconnectDelay:   cfg.Agent.ReconnectDelay,
			RestartDelay:     cfg.Device.RestartDelay,
			TokenReadTimeout: cfg.Agent.TokenReadTimeout,
			DialTimeout:      cfg.Agent.DialTimeout,
			CLIVersion:       cfg.CLIVersion,
		}
	}

	warning, err := probelink.CheckHandshake(ctx, client, cfg.CLIVersion)
	if err != nil {
		client.Close()
		return runner.CompositeDevice{}, nil, fmt.Errorf("handshake: %w", err)
	}
	if warning != "" {
		fmt.Printf("  \033[33m⚠\033[0m  [%s] %s\n", alias, warning)
	}

	dev := runner.CompositeDevice{
		Alias:      alias,
		Client:     client,
		DeviceCtx:  devCtx,
		DeviceID:   spec,
		DeviceName: deviceName,
	}
	fmt.Printf("  \033[32m✓\033[0m  [%s] Connected to %s (%s)\n", alias, deviceName, spec)
	return dev, client, nil
}

// buildCompositeRunner creates a CompositeRunner from the resolved device specs.
// Returns nil (not an error) if no composite devices are configured.
func buildCompositeRunner(
	ctx context.Context,
	cfg *config.Config,
	dm *device.Manager,
	flags []string,
	timeout time.Duration,
	verbose bool,
) (*runner.CompositeRunner, func(), error) {
	specs := parseCompositeDeviceSpecs(flags, cfg.Composite.Devices)
	if len(specs) == 0 {
		return nil, func() {}, nil
	}

	fmt.Printf("  \033[36mℹ\033[0m  Setting up %d composite device(s)...\n", len(specs))
	devices, cleanup, err := setupCompositeDevices(ctx, cfg, dm, specs, timeout, verbose)
	if err != nil {
		return nil, func() {}, err
	}

	opts := runner.RunOptions{Timeout: timeout, Verbose: verbose}
	cr := runner.NewCompositeRunner(cfg, devices, opts)
	return cr, cleanup, nil
}

// compositeDeviceUsage returns the usage hint for --composite-device.
func compositeDeviceUsage() string {
	return `composite test device alias mapping. Repeat for each device:
  WiFi mode:      --composite-device "A=192.168.1.10:48686/my-token"
  iOS simulator:  --composite-device "B=<simulator-udid>"
  Android device: --composite-device "C=emulator-5554"
Can also be set in probe.yaml under composite.devices.`
}

// durationFromTimeout returns a sensible default if timeout is zero.
func durationFromTimeout(d time.Duration) time.Duration {
	if d == 0 {
		return 30 * time.Second
	}
	return d
}
