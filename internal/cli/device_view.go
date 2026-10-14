package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/deviceview"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/spf13/cobra"
)

var deviceViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Serve a live, interactive view of a device on a loopback WebSocket",
	Long: `Connects to the ProbeAgent on a device and serves its screen on a
loopback-only WebSocket (ws://127.0.0.1:<port>/device/v1) for Studio and the
VS Code extension. Pointer and keyboard input is relayed to the app.

On start one JSON line is written to stdout:

  {"url":"ws://127.0.0.1:PORT/device/v1","token":"...","device":"<id>"}

Connect with ws://127.0.0.1:PORT/device/v1?token=<token>. The token is random
per run and is printed only here. The command writes no files and runs until
interrupted. See docs/studio-device-protocol.md.`,
	Example: `  probe device view --device emulator-5554
  probe device view --device <udid> --port 48800`,
	Args: cobra.NoArgs,
	RunE: runDeviceView,
}

func init() {
	deviceViewCmd.Flags().String("device", "", "device id (default: the only connected device)")
	deviceViewCmd.Flags().Int("port", 0, "loopback port to listen on (default: a free port)")
	deviceViewCmd.Flags().Bool("json", true, "print the startup line as JSON on stdout")
	deviceCmd.AddCommand(deviceViewCmd)
}

func runDeviceView(cmd *cobra.Command, _ []string) error {
	devID, _ := cmd.Flags().GetString("device")
	port, _ := cmd.Flags().GetInt("port")
	asJSON, _ := cmd.Flags().GetBool("json")
	if port < 0 || port > 65535 {
		return fmt.Errorf("invalid --port %d", port)
	}
	cfg, err := loadConfig(cmd)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stderr := cmd.ErrOrStderr()

	dm := device.NewManager()
	dev, err := pickViewDevice(ctx, dm, devID)
	if err != nil {
		return err
	}
	conn := &viewConnector{dm: dm, cfg: cfg, dev: dev, warn: stderr}
	defer conn.cleanup()

	fmt.Fprintf(stderr, "  Connecting to the agent on %s (%s)...\n", dev.Name, dev.ID)
	agent, err := conn.connect(ctx)
	if err != nil {
		return err
	}

	return serveDeviceView(ctx, deviceview.Options{
		Agent:  agent,
		Dial:   conn.connect,
		Device: deviceview.DeviceInfo{ID: dev.ID, Platform: string(dev.Platform), Name: dev.Name},
		Port:   port,
	}, asJSON, cmd.OutOrStdout(), stderr)
}

// serveDeviceView binds the hub, prints the startup lines and serves until ctx
// ends. The token appears only in the startup output: on stdout as JSON, or on
// stderr when --json=false. It is never logged.
func serveDeviceView(ctx context.Context, opts deviceview.Options, asJSON bool, stdout, stderr io.Writer) error {
	hub, err := deviceview.New(opts)
	if err != nil {
		if opts.Agent != nil {
			_ = opts.Agent.Close()
		}
		return err
	}
	if asJSON {
		line, _ := json.Marshal(struct {
			URL    string `json:"url"`
			Token  string `json:"token"`
			Device string `json:"device"`
		}{hub.URL(), hub.Token(), opts.Device.ID})
		fmt.Fprintf(stdout, "%s\n", line)
		fmt.Fprintf(stderr, "  Device view for %s is listening on %s (token printed on stdout). Press Ctrl+C to stop.\n",
			opts.Device.Name, hub.URL())
	} else {
		fmt.Fprintf(stderr, "  Device view for %s is listening on %s?token=%s . Press Ctrl+C to stop.\n",
			opts.Device.Name, hub.URL(), hub.Token())
	}
	return hub.Run(ctx)
}

// pickViewDevice resolves --device, or the single ready device.
func pickViewDevice(ctx context.Context, dm *device.Manager, id string) (device.Device, error) {
	devs, err := dm.List(ctx)
	if err != nil {
		return device.Device{}, fmt.Errorf("listing devices: %w", err)
	}
	if id != "" {
		for _, d := range devs {
			if d.ID == id {
				return d, nil
			}
		}
		return device.Device{}, fmt.Errorf("device %q not found (see 'probe device list')", id)
	}
	var ready []device.Device
	for _, d := range devs {
		switch strings.ToLower(d.State) {
		case "booted", "online", "device":
			ready = append(ready, d)
		}
	}
	switch len(ready) {
	case 0:
		return device.Device{}, errors.New("no ready device found; start one with 'probe device start' or pass --device")
	case 1:
		return ready[0], nil
	}
	ids := make([]string, len(ready))
	for i, d := range ready {
		ids[i] = d.ID
	}
	return device.Device{}, fmt.Errorf("several devices are ready (%s); choose one with --device", strings.Join(ids, ", "))
}

// viewConnector opens (and re-opens) the agent connection the same way
// 'probe test' does: port forward or iproxy, token read from the device with a
// re-read on rejection, dial, version handshake.
type viewConnector struct {
	dm   *device.Manager
	cfg  *config.Config
	dev  device.Device
	warn io.Writer

	prepared bool
	cleanups []func()
}

func (v *viewConnector) cleanup() {
	for i := len(v.cleanups) - 1; i >= 0; i-- {
		v.cleanups[i]()
	}
	v.cleanups = nil
}

func (v *viewConnector) connect(ctx context.Context) (deviceview.Agent, error) {
	cfg, dm, serial := v.cfg, v.dm, v.dev.ID
	opts := probelink.DialOptions{Host: "127.0.0.1", Port: cfg.Agent.Port, DialTimeout: cfg.Agent.DialTimeout}

	var token string
	physicalIOS := false
	switch v.dev.Platform {
	case device.PlatformAndroid:
		if !v.prepared {
			if err := dm.EnsureADB(ctx, serial, cfg.Agent.Port); err != nil {
				return nil, fmt.Errorf("android setup: %w", err)
			}
			v.cleanups = append(v.cleanups, func() {
				_ = dm.RemoveForward(context.Background(), serial, cfg.Agent.Port)
			})
		}
		// Re-create the forward on every attempt: it disappears when adb restarts.
		forward := func(c context.Context) error {
			return dm.ForwardPort(c, serial, cfg.Agent.Port, cfg.Agent.AgentDevicePort())
		}
		if err := forward(ctx); err != nil {
			return nil, fmt.Errorf("port forward: %w", err)
		}
		opts.OnConnectRefused = func(c context.Context) { _ = forward(c) }
		opts.OwnAdbForward = true
		t, err := dm.ReadTokenAndroid(ctx, serial, cfg.Agent.TokenReadTimeout, cfg.Project.App, nil)
		if err != nil {
			return nil, fmt.Errorf("agent token: %w (is the app running with probe_agent?)", err)
		}
		token = t
		last := t
		opts.RefreshToken = func(c context.Context) (string, error) {
			t, err := dm.ReadTokenAndroid(c, serial, 5*time.Second, cfg.Project.App, nil)
			if err == nil && t == last {
				if lt := dm.ReadTokenAndroidLogcat(c, serial); lt != "" && lt != t {
					t = lt
				}
			}
			if err == nil {
				last = t
			}
			return t, err
		}
	case device.PlatformIOS:
		physicalIOS = dm.IsPhysicalIOS(ctx, serial)
		if physicalIOS && !v.prepared {
			stop, err := dm.EnsureIProxy(ctx, serial, cfg.Agent.Port, cfg.Agent.AgentDevicePort())
			if err != nil {
				return nil, fmt.Errorf("iproxy setup: %w", err)
			}
			v.cleanups = append(v.cleanups, stop)
		}
		t, err := dm.ReadTokenIOS(ctx, serial, cfg.Agent.TokenReadTimeout, cfg.Project.App)
		if err != nil {
			return nil, fmt.Errorf("agent token: %w (is the app running with probe_agent?)", err)
		}
		token = t
		opts.RefreshToken = func(c context.Context) (string, error) {
			return dm.ReadTokenIOS(c, serial, 5*time.Second, cfg.Project.App)
		}
	default:
		return nil, fmt.Errorf("unsupported platform %q", v.dev.Platform)
	}
	v.prepared = true
	opts.Token = token

	var client probelink.ProbeClient
	var err error
	if physicalIOS {
		client, err = probelink.DialHTTP(ctx, opts) // same fallback as 'probe test'
	} else {
		client, err = probelink.DialWithOptions(ctx, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to ProbeAgent: %s", redactAgentToken(err.Error(), token))
	}
	warning, err := probelink.CheckHandshake(ctx, client, cfg.CLIVersion)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("agent handshake failed: %s", redactAgentToken(err.Error(), token))
	}
	if warning != "" {
		statusWarn(v.warn, "%s", warning)
	}
	return deviceview.NewProbeAgent(client), nil
}

func redactAgentToken(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "[token]")
}
