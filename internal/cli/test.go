package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/cloud"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/alphawavesystems/flutter-probe/internal/visual"
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test [file|dir]...",
	Short: "Run .probe test files against the connected Flutter app",
	Long: `Run ProbeScript (.probe) test files against a running Flutter app.

Tests are executed via WebSocket connection to the on-device ProbeAgent. The CLI
handles device discovery, port forwarding, token authentication, and test
orchestration automatically.

Connection settings, timeouts, and recording options can be configured via CLI
flags (override everything), probe.yaml (project defaults), or built-in defaults.
Resolution order: CLI flag > probe.yaml > built-in default.`,
	Example: `  probe test                                # run all tests in ./tests/
  probe test tests/login.probe              # run one file
  probe test --tag smoke                    # run tests tagged @smoke
  probe test --watch                        # re-run on file changes
  probe test --format junit -o results.xml  # JUnit output
  probe test --port 9999                    # custom agent port
  probe test --token-timeout 60s            # longer token wait (slow CI)
  probe test --dial-timeout 45s             # longer connection timeout`,
	RunE: runTests,
}

func init() {
	f := testCmd.Flags()

	// Test selection & output
	f.StringP("tag", "t", "", "run only tests matching this tag (e.g. @smoke, @critical)")
	f.BoolP("watch", "w", false, "watch mode — re-run tests automatically on file changes")
	f.String("shard", "", `run a subset of test files, format: N/M (e.g. "1/3" runs shard 1 of 3)`)
	f.String("format", "terminal", "output format: terminal | junit | json")
	f.StringP("output", "o", "", "write report to file instead of stdout")
	f.Bool("dry-run", false, "parse and validate .probe files without executing against a device")

	// Device selection
	f.String("device", "", "target device serial or UDID (default: first available)")
	f.Bool("parallel", false, "run tests in parallel across all connected devices")
	f.String("devices", "", "comma-separated device serials for parallel execution")

	// Per-step timeout
	f.Duration("timeout", 0, "per-step timeout; 0 uses probe.yaml or default 30s")

	// Agent connection
	f.Int("port", 0, "ProbeAgent WebSocket port (default: 48686)")
	f.Duration("dial-timeout", 0, "max time to establish WebSocket connection (default: 30s)")
	f.Duration("token-timeout", 0, "max time to wait for agent auth token on startup (default: 30s)")
	f.Duration("reconnect-delay", 0, "delay after app restart before reconnecting WebSocket (default: 2s)")

	// Tool paths
	f.String("adb", "", "path to adb binary (overrides probe.yaml and PATH)")
	f.String("flutter", "", "path to flutter binary (overrides probe.yaml and PATH)")

	// Destructive operations
	f.BoolP("yes", "y", false, "auto-confirm destructive operations (clear app data, permissions)")

	// App installation
	f.String("app-path", "", "path to .apk or .app bundle to install before testing")

	// Video recording
	f.Bool("video", false, "enable video recording (overrides probe.yaml)")
	f.Bool("no-video", false, "disable video recording (overrides probe.yaml)")
	f.String("video-resolution", "", `Android screenrecord resolution, e.g. "720x1280" (default: "720x1280")`)
	f.Int("video-framerate", 0, "FPS for screencap-based video stitching (default: 2)")

	// Visual regression
	f.Float64("visual-threshold", 0, "max allowed pixel diff percentage, e.g. 0.5 (default: 0.5)")
	f.Int("visual-pixel-delta", 0, "per-pixel color distance threshold 0–255 (default: 8)")

	// Cloud integration
	f.Bool("cloud", false, "upload test results to FlutterProbe Cloud after run")
	f.String("cloud-token", "", "API key for FlutterProbe Cloud authentication")
	f.String("cloud-url", "", "cloud API base URL (must be set via this flag or cloud.url in probe.yaml)")

	// Cloud device farm providers
	f.String("cloud-provider", "", "cloud device farm provider (browserstack, aws, firebase, saucelabs, lambdatest)")
	f.String("cloud-app", "", "path to app binary (.apk/.ipa) to upload to cloud provider")
	f.String("cloud-device", "", "target device name on the cloud provider")
	f.String("cloud-key", "", "cloud provider API key or username")
	f.String("cloud-secret", "", "cloud provider API secret or access key")

	// x402 pay-per-use
	f.String("pay", "", `payment method for cloud upload: "x402" for pay-per-use via crypto wallet`)

	// Relay mode
	f.Bool("relay", false, "force enable relay mode for cloud device farm testing")
	f.Bool("no-relay", false, "force disable relay mode (use direct port forwarding)")
	f.String("relay-url", "", "reuse an existing relay session (WebSocket URL) — skips relay creation")
	f.String("relay-token", "", "CLI auth token for the existing relay session (used with --relay-url)")
	f.String("relay-session-id", "", "existing relay session ID for status polling and cleanup")
}

func runTests(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load config (respects --config flag for platform-specific configs)
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	format, _ := cmd.Flags().GetString("format")
	outFile, _ := cmd.Flags().GetString("output")
	tag, _ := cmd.Flags().GetString("tag")

	// Validate --format value early
	switch runner.Format(format) {
	case runner.FormatTerminal, runner.FormatJUnit, runner.FormatJSON:
		// valid
	default:
		return fmt.Errorf("unknown format %q: must be one of terminal, junit, json", format)
	}

	// When structured output (json, junit) goes to stdout (no -o file),
	// route all status messages to stderr so the output stays parseable.
	statusW := os.Stdout
	if outFile == "" && (format == "json" || format == "junit") {
		statusW = os.Stderr
	}
	timeout, _ := cmd.Flags().GetDuration("timeout")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	deviceSerial, _ := cmd.Flags().GetString("device")
	adbPath, _ := cmd.Flags().GetString("adb")
	flutterPath, _ := cmd.Flags().GetString("flutter")
	autoYes, _ := cmd.Flags().GetBool("yes")
	appPath, _ := cmd.Flags().GetString("app-path")
	videoFlag, _ := cmd.Flags().GetBool("video")
	noVideoFlag, _ := cmd.Flags().GetBool("no-video")

	// Agent connection overrides: CLI flag > probe.yaml (already loaded)
	agentPort, _ := cmd.Flags().GetInt("port")
	dialTimeout, _ := cmd.Flags().GetDuration("dial-timeout")
	tokenTimeout, _ := cmd.Flags().GetDuration("token-timeout")
	reconnectDelay, _ := cmd.Flags().GetDuration("reconnect-delay")

	// Video overrides
	videoResolution, _ := cmd.Flags().GetString("video-resolution")
	videoFramerate, _ := cmd.Flags().GetInt("video-framerate")

	// Visual regression overrides
	visualThreshold, _ := cmd.Flags().GetFloat64("visual-threshold")
	visualPixelDelta, _ := cmd.Flags().GetInt("visual-pixel-delta")

	// Apply CLI overrides to config
	if agentPort != 0 {
		cfg.Agent.Port = agentPort
	}
	if dialTimeout != 0 {
		cfg.Agent.DialTimeout = dialTimeout
	}
	if tokenTimeout != 0 {
		cfg.Agent.TokenReadTimeout = tokenTimeout
	}
	if reconnectDelay != 0 {
		cfg.Agent.ReconnectDelay = reconnectDelay
	}
	if videoResolution != "" {
		cfg.Video.Resolution = videoResolution
	}
	if videoFramerate != 0 {
		cfg.Video.Framerate = videoFramerate
	}
	if visualThreshold != 0 {
		cfg.Visual.Threshold = visualThreshold
	}
	if visualPixelDelta != 0 {
		cfg.Visual.PixelDelta = visualPixelDelta
	}

	// Per-step timeout: CLI flag > probe.yaml
	if timeout == 0 {
		timeout = cfg.Defaults.Timeout
	}

	// Collect test files
	searchPaths := args
	if len(searchPaths) == 0 {
		searchPaths = []string{"tests"}
	}
	files, err := runner.CollectFiles(searchPaths)
	if err != nil {
		return fmt.Errorf("collecting test files: %w", err)
	}
	if len(files) == 0 {
		fmt.Fprintln(statusW, msgNoProbeFiles)
		return nil
	}

	// Shard filtering: --shard "1/3" runs a deterministic subset of files
	shardStr, _ := cmd.Flags().GetString("shard")
	if shardStr != "" {
		shardIdx, shardTotal, err := runner.ParseShard(shardStr)
		if err != nil {
			return err
		}
		files = runner.ShardFiles(files, shardIdx, shardTotal)
		if len(files) == 0 {
			fmt.Fprintf(statusW, "  Shard %s: no files assigned to this shard\n", shardStr)
			return nil
		}
		fmt.Fprintf(statusW, "  Shard %s: running %d files\n", shardStr, len(files))
	}

	// Parallel mode: --parallel [--devices serial1,serial2]
	parallelMode, _ := cmd.Flags().GetBool("parallel")
	devicesStr, _ := cmd.Flags().GetString("devices")
	if devicesStr != "" {
		parallelMode = true // --devices implies --parallel
	}

	// Cloud device farm provider flags
	cloudProvider, _ := cmd.Flags().GetString("cloud-provider")
	cloudApp, _ := cmd.Flags().GetString("cloud-app")
	cloudDevice, _ := cmd.Flags().GetString("cloud-device")
	cloudKey, _ := cmd.Flags().GetString("cloud-key")
	cloudSecret, _ := cmd.Flags().GetString("cloud-secret")

	// Apply CLI overrides for cloud provider config
	if cloudProvider == "" {
		cloudProvider = cfg.Cloud.Provider
	}
	if cloudApp == "" {
		cloudApp = cfg.Cloud.App
	}

	// ── Parallel mode: distribute files across multiple devices ──
	if parallelMode && !dryRun && cloudProvider == "" {
		toolPaths := device.ToolPaths{ADB: adbPath, Flutter: flutterPath}
		if adbPath == "" {
			toolPaths.ADB = cfg.Tools.ADB
		}
		if flutterPath == "" {
			toolPaths.Flutter = cfg.Tools.Flutter
		}
		dm := device.NewManagerWithPaths(toolPaths)

		// Discover or parse devices
		var deviceRuns []runner.DeviceRun
		if devicesStr != "" {
			for _, serial := range strings.Split(devicesStr, ",") {
				serial = strings.TrimSpace(serial)
				if serial == "" {
					continue
				}
				p := device.PlatformAndroid
				// Simple heuristic: UUIDs with dashes are iOS
				if strings.Count(serial, "-") >= 4 && len(serial) > 30 {
					p = device.PlatformIOS
				}
				deviceRuns = append(deviceRuns, runner.DeviceRun{
					DeviceID: serial,
					Platform: p,
					AppID:    runner.ResolveAppID(cfg.Project.App, p),
				})
			}
		} else {
			// Auto-discover all connected devices
			devList, err := dm.List(ctx)
			if err != nil || len(devList) < 2 {
				return fmt.Errorf("parallel mode requires 2+ connected devices (found %d)", len(devList))
			}
			for _, d := range devList {
				deviceRuns = append(deviceRuns, runner.DeviceRun{
					DeviceID:   d.ID,
					DeviceName: d.Name,
					Platform:   d.Platform,
					AppID:      runner.ResolveAppID(cfg.Project.App, d.Platform),
				})
			}
		}

		// Distribute files across devices
		portBase := cfg.Agent.Port
		if portBase == 0 {
			portBase = 48686
		}
		fileBuckets := runner.DistributeFiles(files, len(deviceRuns))
		// Assign host ports: Android devices get unique ports via adb forward,
		// iOS simulators connect directly on portBase (each sim has its own loopback)
		androidIdx := 0
		for i := range deviceRuns {
			deviceRuns[i].Files = fileBuckets[i]
			if deviceRuns[i].Platform == device.PlatformAndroid {
				deviceRuns[i].Port = portBase + androidIdx + 1 // 48687, 48688, ...
				androidIdx++
			} else {
				deviceRuns[i].Port = portBase // iOS: direct localhost:48686
			}
		}

		var tags []string
		if tag != "" {
			tags = strings.Split(tag, ",")
		}

		opts := runner.RunOptions{
			Files:   files,
			Tags:    tags,
			Timeout: timeout,
			Verbose: verbose,
		}

		orch := runner.NewParallelOrchestrator(cfg, dm, deviceRuns, opts, portBase)
		results, orchErr := orch.Run(ctx)

		orch.PrintSummary()

		if orchErr != nil {
			fmt.Fprintf(statusW, "  \033[33m⚠\033[0m  %v\n", orchErr)
		}

		// Report results
		var report *runner.Reporter
		if outFile != "" {
			report, err = runner.NewFileReporter(runner.Format(format), outFile, verbose)
			if err != nil {
				return err
			}
		} else {
			report = runner.NewReporter(runner.Format(format), os.Stdout, verbose)
		}
		if err := report.Report(results); err != nil {
			return err
		}

		if !runner.AllPassed(results) {
			return fmt.Errorf("%s", runner.Summary(results))
		}
		return nil
	}

	// ── Single-device mode (default) ──
	// Connect to ProbeAgent (skip if dry-run)
	var client *probelink.Client
	var dm *device.Manager
	var platform device.Platform
	var cloudSession *cloud.Session       // non-nil when using a cloud provider
	var cloudProviderImpl cloud.CloudProvider // saved for artifact collection
	var sessionStopped bool                   // prevents double-stop in defer
	if !dryRun {

		if cloudProvider != "" {
			// ── Cloud device farm path ──────────────────────────────────────
			// Build credentials map from CLI flags and probe.yaml
			creds := make(map[string]string)
			for k, v := range cfg.Cloud.Credentials {
				creds[k] = v
			}
			// CLI flags override probe.yaml credentials
			if cloudKey != "" {
				creds["username"] = cloudKey
				creds["access_key_id"] = cloudKey // AWS uses different key name
			}
			if cloudSecret != "" {
				creds["access_key"] = cloudSecret
				creds["secret_access_key"] = cloudSecret // AWS uses different key name
			}

			provider, err := cloud.NewProvider(cloudProvider, creds)
			if err != nil {
				return fmt.Errorf("cloud provider: %w", err)
			}
			cloudProviderImpl = provider

			// Validate required cloud flags
			if cloudApp == "" {
				return fmt.Errorf("--cloud-app is required when using --cloud-provider (path to .apk/.ipa)")
			}
			if _, err := os.Stat(cloudApp); err != nil {
				return fmt.Errorf("cloud-app: %w", err)
			}

			// Determine target device
			targetDevice := cloudDevice
			if targetDevice == "" && len(cfg.Cloud.Devices) > 0 {
				targetDevice = cfg.Cloud.Devices[0]
			}
			if targetDevice == "" {
				return fmt.Errorf("--cloud-device is required when using --cloud-provider (target device name)")
			}

			// Determine relay mode: --relay/--no-relay flags > probe.yaml > auto
			relayFlag, _ := cmd.Flags().GetBool("relay")
			noRelayFlag, _ := cmd.Flags().GetBool("no-relay")
			useRelay := cfg.Cloud.Relay.RelayEnabled(true) // auto-enabled for cloud providers
			if relayFlag {
				useRelay = true
			}
			if noRelayFlag {
				useRelay = false
			}

			if useRelay {
				// ── Relay path: connect via ProbeRelay server ──────────────

				// Resolve cloud API client
				cloudToken, _ := cmd.Flags().GetString("cloud-token")
				cloudURL, _ := cmd.Flags().GetString("cloud-url")
				if cloudToken == "" {
					cloudToken = cfg.Cloud.Token
				}
				if cloudURL == "" {
					cloudURL = cfg.Cloud.URL
				}

				// Check if an existing relay session was provided (pre-created
				// in CI so the relay URL can be baked into the APK at build time)
				existingRelayURL, _ := cmd.Flags().GetString("relay-url")
				existingRelayToken, _ := cmd.Flags().GetString("relay-token")
				existingRelaySessionID, _ := cmd.Flags().GetString("relay-session-id")

				var relayURL, cliToken, relaySessionID string

				if existingRelayURL != "" && existingRelayToken != "" {
					// Reuse existing relay session (relay URL already baked into APK)
					relayURL = existingRelayURL
					cliToken = existingRelayToken
					relaySessionID = existingRelaySessionID
					statusOK(statusW, "Reusing relay session: %s", relaySessionID)
				} else {
					// Create a new relay session
					if cloudToken == "" {
						return fmt.Errorf("--cloud-token is required for relay mode (API key for relay session creation)")
					}
					if cloudURL == "" {
						return fmt.Errorf("--cloud-url or cloud.url in probe.yaml is required for relay mode")
					}
					cc := cloud.NewClient(cloudURL, cloudToken)

					fmt.Fprintln(statusW, msgCreatingRelaySession)
					relaySess, err := cc.CreateRelaySession(ctx, cloudProvider, targetDevice, cfg.Cloud.Relay.RelayTTL())
					if err != nil {
						return fmt.Errorf("relay session: %w", err)
					}
					defer func() {
						if delErr := cc.DeleteRelaySession(ctx, relaySess.SessionID); delErr != nil {
							statusWarn(statusW, "Failed to close relay session: %s", delErr)
						}
					}()
					relayURL = relaySess.RelayURL
					cliToken = relaySess.CLIToken
					relaySessionID = relaySess.SessionID
					statusOK(statusW, "Relay session: %s", relaySessionID)
				}

				// Upload app
				statusInfo(statusW, "Uploading app to %s...", provider.Name())
				appID, err := provider.UploadApp(ctx, cloudApp)
				if err != nil {
					return fmt.Errorf("cloud upload: %w", err)
				}
				statusOK(statusW, "App uploaded: %s", appID)

				// Start cloud session (app launches on device)
				statusInfo(statusW, "Starting cloud session on %s (%s)...", targetDevice, provider.Name())
				sess, err := provider.StartSession(ctx, appID, targetDevice)
				if err != nil {
					return fmt.Errorf("cloud session: %w", err)
				}
				sess.RelayURL = relayURL
				sess.CLIToken = cliToken
				cloudSession = &sess
				defer func() {
					if sessionStopped {
						return
					}
					statusInfo(statusW, "Stopping cloud session %s...", sess.ID)
					if stopErr := provider.StopSession(ctx, sess); stopErr != nil {
						statusWarn(statusW, "Failed to stop cloud session: %s", stopErr)
					} else {
						statusOK(statusW, msgCloudSessionStopped)
					}
				}()
				statusOK(statusW, "Session started: %s", sess.ID)

				// Wait for agent to connect to relay.
				// Firebase Test Lab takes longer (device allocation + app install),
				// so use a longer timeout for Firebase.
				if relaySessionID != "" && cloudToken != "" {
					cc := cloud.NewClient(cloudURL, cloudToken)
					fmt.Fprintln(statusW, msgWaitingForAgentRelay)
					connectTimeout := cfg.Cloud.Relay.RelayConnectTimeout()
					if cloudProvider == "firebase" && connectTimeout < 5*time.Minute {
						connectTimeout = 5 * time.Minute
					}
					status, err := cc.PollRelayStatus(ctx, relaySessionID, connectTimeout)
					if err != nil {
						return fmt.Errorf("relay wait: %w", err)
					}
					statusOK(statusW, "Agent connected (status: %s)", status.Status)
				} else {
					// No session ID to poll — wait a fixed duration for agent boot
					fmt.Fprintln(statusW, msgWaitingForAgentRelay)
					time.Sleep(15 * time.Second)
				}

				// CLI connects to relay
				client, err = probelink.DialRelay(ctx, relayURL, cliToken, cfg.Agent.DialTimeout)
				if err != nil {
					return fmt.Errorf("connecting via relay: %w", err)
				}
				defer client.Close()

				if err := client.Ping(ctx); err != nil {
					return fmt.Errorf("relay agent ping failed: %w", err)
				}
				statusOK(statusW, "Connected to ProbeAgent via relay on %s (%s)", targetDevice, provider.Name())
			} else {
				// ── Direct path: port forwarding (existing behavior) ──────

				// Step 1: Upload app
				statusInfo(statusW, "Uploading app to %s...", provider.Name())
				appID, err := provider.UploadApp(ctx, cloudApp)
				if err != nil {
					return fmt.Errorf("cloud upload: %w", err)
				}
				statusOK(statusW, "App uploaded: %s", appID)

				// Step 2: Start session
				statusInfo(statusW, "Starting cloud session on %s (%s)...", targetDevice, provider.Name())
				sess, err := provider.StartSession(ctx, appID, targetDevice)
				if err != nil {
					return fmt.Errorf("cloud session: %w", err)
				}
				cloudSession = &sess
				defer func() {
					if sessionStopped {
						return
					}
					statusInfo(statusW, "Stopping cloud session %s...", sess.ID)
					if stopErr := provider.StopSession(ctx, sess); stopErr != nil {
						statusWarn(statusW, "Failed to stop cloud session: %s", stopErr)
					} else {
						statusOK(statusW, msgCloudSessionStopped)
					}
				}()
				statusOK(statusW, "Session started: %s", sess.ID)

				// Step 3: Forward port
				devicePort := cfg.Agent.AgentDevicePort()
				localPort, err := provider.ForwardPort(ctx, sess, devicePort)
				if err != nil {
					return fmt.Errorf("cloud port forward: %w", err)
				}
				sess.LocalPort = localPort
				statusOK(statusW, "Port forwarded: localhost:%d -> device:%d", localPort, devicePort)

				// Step 4: Connect to ProbeAgent via the tunneled port
				dialOpts := probelink.DialOptions{
					Host:        "127.0.0.1",
					Port:        localPort,
					DialTimeout: cfg.Agent.DialTimeout,
				}
				client, err = probelink.DialWithOptions(ctx, dialOpts)
				if err != nil {
					return fmt.Errorf("connecting to cloud ProbeAgent: %w", err)
				}
				defer client.Close()

				if err := client.Ping(ctx); err != nil {
					return fmt.Errorf("cloud agent ping failed: %w", err)
				}
				statusOK(statusW, "Connected to ProbeAgent on %s (%s)", targetDevice, provider.Name())
			}

			// Use cloud device info as the "serial" for reporting
			deviceSerial = fmt.Sprintf("%s:%s", provider.Name(), targetDevice)

		} else {
			// ── Local device path (existing behavior) ───────────────────────
			// Resolve tool paths: CLI flags > probe.yaml > PATH
			toolPaths := device.ToolPaths{
				ADB:     cfg.Tools.ADB,
				Flutter: cfg.Tools.Flutter,
			}
			if adbPath != "" {
				toolPaths.ADB = adbPath
			}
			if flutterPath != "" {
				toolPaths.Flutter = flutterPath
			}
			dm = device.NewManagerWithPaths(toolPaths)
			// Validate device serial if provided
			if deviceSerial != "" {
				if err := config.ValidateDeviceSerial(deviceSerial); err != nil {
					return err
				}
			}

			// Pick device and detect platform
			platform = device.Platform(cfg.Defaults.Platform)
			if deviceSerial == "" {
				devices, err := dm.List(ctx)
				if err != nil || len(devices) == 0 {
					return fmt.Errorf("no connected devices found, run 'probe device list' to check")
				}
				deviceSerial = devices[0].ID
				platform = devices[0].Platform
			} else {
				// Detect platform from device list when serial is specified manually
				devices, _ := dm.List(ctx)
				for _, d := range devices {
					if d.ID == deviceSerial {
						platform = d.Platform
						break
					}
				}
			}

			// Install app if --app-path provided
			if appPath != "" {
				// Validate file exists
				info, err := os.Stat(appPath)
				if err != nil {
					return fmt.Errorf("app-path: %w", err)
				}
				if info.IsDir() && !strings.HasSuffix(appPath, ".app") {
					return fmt.Errorf("app-path: %s is a directory (expected .apk file or .app bundle)", appPath)
				}
				// Validate extension matches platform
				if platform == device.PlatformAndroid && !strings.HasSuffix(appPath, ".apk") {
					return fmt.Errorf("app-path: Android requires .apk file, got %s", filepath.Base(appPath))
				}
				if platform == device.PlatformIOS && !strings.HasSuffix(appPath, ".app") {
					return fmt.Errorf("app-path: iOS requires .app bundle, got %s", filepath.Base(appPath))
				}
				// Require app ID
				if cfg.Project.App == "" {
					return fmt.Errorf("app-path: project.app must be set in probe.yaml to install and launch")
				}
				// Clear logcat on Android so we catch the fresh token
				if platform == device.PlatformAndroid {
					_ = dm.ADB().ClearLogcat(ctx, deviceSerial)
				}
				if err := dm.InstallAndLaunchApp(ctx, deviceSerial, platform, appPath, cfg.Project.App, autoYes); err != nil {
					return fmt.Errorf("app install: %w", err)
				}
				statusOK(statusW, msgAppInstalledAndLaunched)
			}

			dialOpts := probelink.DialOptions{
				Host:        "127.0.0.1",
				Port:        cfg.Agent.Port,
				DialTimeout: cfg.Agent.DialTimeout,
			}

			if platform == device.PlatformIOS {
				// iOS: grant privacy permissions and relaunch the app.
				// simctl privacy grant terminates the running app, so we must
				// grant first, then relaunch. This prevents native OS dialogs
				// (camera, location, etc.) from blocking the Flutter UI.
				if autoYes && cfg.Project.App != "" {
					for _, svc := range []string{"camera", "microphone", "location", "photos", "contacts-limited", "calendar"} {
						_ = dm.SimCtl().GrantPrivacy(ctx, deviceSerial, cfg.Project.App, svc)
					}
					// Relaunch the app — simctl privacy grant terminates it
					_ = dm.SimCtl().Launch(ctx, deviceSerial, cfg.Project.App)
					time.Sleep(3 * time.Second) // give agent time to start
				}
				// iOS: simulators share host loopback — no port forwarding needed
				fmt.Fprintln(statusW, msgWaitingForTokenIOS)
				token, err := dm.ReadTokenIOS(ctx, deviceSerial, cfg.Agent.TokenReadTimeout, cfg.Project.App)
				if err != nil {
					return fmt.Errorf("agent token: %w — is the app running with probe_agent?", err)
				}
				dialOpts.Token = token
				client, err = probelink.DialWithOptions(ctx, dialOpts)
				if err != nil {
					return fmt.Errorf("connecting to ProbeAgent: %w", err)
				}
				defer client.Close()
			} else {
				// Android: grant permissions BEFORE reading token when -y is used.
				// This prevents OS permission dialogs from blocking the app on
				// first launch (especially POST_NOTIFICATIONS on Android 13+).
				if autoYes && cfg.Project.App != "" {
					for _, perms := range device.AndroidPermissions {
						for _, perm := range perms {
							_ = dm.ADB().GrantPermission(ctx, deviceSerial, cfg.Project.App, perm)
						}
					}
				}

				// Android: forward port via ADB
				if err := dm.ForwardPort(ctx, deviceSerial, cfg.Agent.Port, cfg.Agent.AgentDevicePort()); err != nil {
					return fmt.Errorf("port forward: %w", err)
				}
				defer dm.RemoveForward(ctx, deviceSerial, cfg.Agent.Port) //nolint:errcheck

				fmt.Fprintln(statusW, msgWaitingForToken)
				token, err := dm.ReadToken(ctx, deviceSerial, cfg.Agent.TokenReadTimeout)
				if err != nil {
					return fmt.Errorf("agent token: %w — is the app running with probe_agent?", err)
				}
				dialOpts.Token = token
				client, err = probelink.DialWithOptions(ctx, dialOpts)
				if err != nil {
					return fmt.Errorf("connecting to ProbeAgent: %w", err)
				}
				defer client.Close()
			}

			if err := client.Ping(ctx); err != nil {
				return fmt.Errorf("agent ping failed: %w", err)
			}
			statusOK(statusW, "Connected to ProbeAgent on %s", deviceSerial)
		}
	}
	_ = cloudSession // used by deferred StopSession above

	// Build reporter
	var report *runner.Reporter
	if outFile != "" {
		report, err = runner.NewFileReporter(runner.Format(format), outFile, verbose)
		if err != nil {
			return err
		}
	} else {
		report = runner.NewReporter(runner.Format(format), os.Stdout, verbose)
	}

	// Attach run metadata for JSON/HTML reports (also used for cloud upload)
	var runMeta runner.RunMetadata
	if !dryRun && dm != nil { //nolint:nestif
		meta := runner.RunMetadata{
			DeviceID: deviceSerial,
			Platform: string(platform),
			Provider: "local",
			AppID:    cfg.Project.App,
		}
		// Resolve config file path
		cfgPath, _ := cmd.Flags().GetString("config")
		if cfgPath != "" {
			meta.ConfigFile = cfgPath
		} else {
			meta.ConfigFile = "probe.yaml"
		}
		// Get device name and OS version from device list
		devices, _ := dm.List(ctx)
		for _, d := range devices {
			if d.ID == deviceSerial {
				meta.DeviceName = d.Name
				meta.OSVersion = d.OSVersion
				break
			}
		}
		// Android-specific: query OS version and app version via ADB
		if platform == device.PlatformAndroid {
			if meta.OSVersion == "" {
				if v, err := dm.ADB().GetProp(ctx, deviceSerial, "ro.build.version.release"); err == nil && v != "" {
					meta.OSVersion = "Android " + v
				}
			}
			if cfg.Project.App != "" {
				if v, err := dm.ADB().GetAppVersion(ctx, deviceSerial, cfg.Project.App); err == nil {
					meta.AppVersion = v
				}
			}
		}
		runMeta = meta
		report.SetMetadata(meta)
	} else if !dryRun && cloudSession != nil {
		// Cloud mode: metadata from cloud session info
		detectedPlatform := cloud.DetectPlatform(cloudSession.DeviceName)
		_, osVer := cloud.ParseDeviceString(cloudSession.DeviceName)
		meta := runner.RunMetadata{
			DeviceID:   deviceSerial,
			DeviceName: cloudSession.DeviceName,
			Platform:   detectedPlatform,
			OSVersion:  osVer,
			Provider:   cloudSession.Provider,
			AppID:      cfg.Project.App,
		}
		cfgPath, _ := cmd.Flags().GetString("config")
		if cfgPath != "" {
			meta.ConfigFile = cfgPath
		} else {
			meta.ConfigFile = "probe.yaml"
		}
		runMeta = meta
		report.SetMetadata(meta)
	}

	// Tags
	var tags []string
	if tag != "" {
		tags = []string{tag}
	}

	// Build device context for platform-level operations (restart, clear data).
	// Only available for local devices — cloud mode has no ADB/simctl access.
	var devCtx *runner.DeviceContext
	if !dryRun && client != nil && dm != nil {
		devCtx = &runner.DeviceContext{
			Manager:                 dm,
			Serial:                  deviceSerial,
			Platform:                platform,
			AppID:                   cfg.Project.App,
			Port:                    cfg.Agent.Port,
			DevicePort:              cfg.Agent.AgentDevicePort(),
			AllowClearData:          autoYes,
			Confirm:                 promptUserConfirm,
			GrantPermissionsOnClear: autoYes || cfg.Defaults.GrantPermissionsOnClear,
			ReconnectDelay:          cfg.Agent.ReconnectDelay,
			RestartDelay:            cfg.Device.RestartDelay,
			TokenReadTimeout:        cfg.Agent.TokenReadTimeout,
			DialTimeout:             cfg.Agent.DialTimeout,
		}
	}

	// Resolve video setting: CLI flags > probe.yaml
	videoEnabled := cfg.Defaults.VideoEnabled
	if videoFlag {
		videoEnabled = true
	}
	if noVideoFlag {
		videoEnabled = false
	}
	reportsBase := cfg.Reports // from probe.yaml reports_folder (default: "reports")
	videoDir := filepath.Join(reportsBase, "videos")
	if outFile != "" {
		videoDir = filepath.Join(filepath.Dir(outFile), "videos")
	}

	// Build and run
	opts := runner.RunOptions{
		Files:        files,
		Tags:         tags,
		Timeout:      timeout,
		DryRun:       dryRun,
		Verbose:      verbose,
		VideoEnabled: videoEnabled,
		VideoDir:     videoDir,
	}

	r := runner.New(cfg, client, devCtx, opts)

	// Configure visual regression if threshold is set.
	if !dryRun {
		vc := visual.NewComparatorWithConfig(".", cfg.Visual.Threshold, cfg.Visual.PixelDelta)
		r.SetVisual(vc)
	}

	statusInfo(statusW, "Running %d test file(s)...", len(files))
	results, err := r.Run(ctx)
	if err != nil {
		return err
	}

	// Pull screenshots from device to local reports/screenshots/ folder.
	// In cloud mode (devCtx is nil), screenshots are taken via the ProbeAgent
	// RPC and returned as on-device paths. Since we can't pull files from cloud
	// devices, the artifacts are already base64-encoded in the RPC response and
	// saved locally by the probelink client. We just ensure the paths are correct.
	screenshotDir := filepath.Join(reportsBase, "screenshots")
	if outFile != "" {
		screenshotDir = filepath.Join(filepath.Dir(outFile), "screenshots")
	}
	if devCtx != nil {
		runner.PullArtifacts(ctx, results, devCtx, screenshotDir)
	} else if cloudSession != nil {
		// In cloud mode, screenshots are saved locally by the probelink client
		// via the probe.screenshot RPC (base64 data in response). Ensure the
		// screenshot directory exists and relativize paths.
		runner.LocalizeArtifacts(results, screenshotDir)

		// Stop the cloud session BEFORE collecting artifacts — some providers
		// (e.g. SauceLabs) only make video available after the session ends.
		if cloudProviderImpl != nil && cloudSession != nil && !sessionStopped {
			statusInfo(statusW, "Stopping cloud session %s...", cloudSession.ID)
			if stopErr := cloudProviderImpl.StopSession(ctx, *cloudSession); stopErr != nil {
				statusWarn(statusW, "Failed to stop cloud session: %s", stopErr)
			} else {
				statusOK(statusW, msgCloudSessionStopped)
			}
			sessionStopped = true

			// Brief wait for provider to finalize artifacts (video encoding).
			time.Sleep(3 * time.Second)
		}

		// Collect artifacts from cloud provider (video, screenshots).
		if cloudProviderImpl != nil {
			if ac, ok := cloudProviderImpl.(cloud.ArtifactCollector); ok {
				fmt.Fprintln(statusW, msgCollectingArtifacts)
				arts, artErr := ac.GetSessionArtifacts(ctx, cloudSession.ID)
				if artErr != nil {
					statusWarn(statusW, "Artifact collection: %s", artErr)
				} else {
					if arts.VideoURL != "" {
						for i := range results {
							results[i].VideoURL = arts.VideoURL
						}
						statusOK(statusW, "Video: %s", arts.VideoURL)
					}
					if len(arts.ScreenshotURLs) > 0 {
						for i := range results {
							results[i].Artifacts = append(results[i].Artifacts, arts.ScreenshotURLs...)
						}
						statusOK(statusW, "Screenshots: %d collected", len(arts.ScreenshotURLs))
					}
				}
			}
		}
	}

	if err := report.Report(results); err != nil {
		return err
	}

	// Cloud upload (after report is written)
	payMethod, _ := cmd.Flags().GetString("pay")
	cloudToken, _ := cmd.Flags().GetString("cloud-token")
	cloudURL, _ := cmd.Flags().GetString("cloud-url")

	// Resolution order: CLI flag > probe.yaml > default
	if cloudToken == "" {
		cloudToken = cfg.Cloud.Token
	}
	if cloudURL == "" {
		cloudURL = cfg.Cloud.URL
	}

	// Upload when token is available or x402 payment is requested.
	if cloudToken != "" || payMethod == "x402" {
		if cloudURL == "" {
			statusFail(statusW, "Cloud upload skipped: no cloud URL configured. Set cloud.url in probe.yaml or pass --cloud-url.")
		}

		// Prepare JSON data for upload. Always use generateCloudJSON which
		// produces the format the Cloud API expects (flat fields, status strings).
		// The reporter's JSON format (--format json) is different (nested metadata,
		// boolean passed/skipped fields) and must not be sent directly.
		jsonData, err := generateCloudJSON(results, runMeta)
		if err != nil {
			statusFail(statusW, "Cloud upload: could not serialize results: %s", err)
		}

		if len(jsonData) > 0 && cloudURL != "" && payMethod == "x402" {
			// x402 pay-per-use upload — no subscription token needed.
			configDir, cfgErr := cloud.ConfigDir()
			if cfgErr != nil {
				statusFail(statusW, "x402: could not locate config dir: %s", cfgErr)
			} else {
				wallet, walletErr := cloud.LoadWallet(configDir)
				if walletErr != nil {
					statusFail(statusW, "x402: %s", walletErr)
				} else {
					statusInfo(statusW, "Uploading results via x402 (wallet %s)...", wallet.Address)
					cc := cloud.NewClient(cloudURL, "")
					runID, dashURL, uploadErr := cc.UploadResultsWithPayment(ctx, jsonData, wallet)
					if uploadErr != nil {
						statusFail(statusW, "x402 upload failed: %s", uploadErr)
					} else {
						statusOK(statusW, "Paid & uploaded (run %s)", runID)
						statusNav(statusW, "%s", dashURL)
					}
				}
			}
		} else if len(jsonData) > 0 && cloudURL != "" && cloudToken != "" {
			// Subscription-based upload.
			fmt.Fprintln(statusW, msgUploadingToCloud)
			cc := cloud.NewClient(cloudURL, cloudToken)
			runID, dashURL, uploadErr := cc.UploadResults(ctx, jsonData)
			if uploadErr != nil {
				statusFail(statusW, "Cloud upload failed: %s", uploadErr)
			} else {
				statusOK(statusW, "Uploaded (run %s)", runID)
				statusNav(statusW, "%s", dashURL)
			}
		} else if cloudToken == "" && payMethod != "x402" {
			fmt.Fprintln(statusW, msgCloudTokenMissing)
		}
	}

	if !runner.AllPassed(results) {
		os.Exit(1)
	}
	return nil
}

// generateCloudJSON serializes test results in the format expected by the
// FlutterProbe Cloud API (POST /api/v1/results).
func generateCloudJSON(results []runner.TestResult, meta runner.RunMetadata) ([]byte, error) {
	type cloudTest struct {
		Name          string  `json:"name"`
		File          string  `json:"file"`
		Status        string  `json:"status"` // "passed", "failed", "skipped"
		Duration      float64 `json:"duration"`
		Error         string  `json:"error,omitempty"`
		ScreenshotURL string  `json:"screenshot_url,omitempty"`
		VideoURL      string  `json:"video_url,omitempty"`
	}
	type cloudReport struct {
		Project    string      `json:"project"`
		Platform   string      `json:"platform,omitempty"`
		Device     string      `json:"device,omitempty"`
		OSVersion  string      `json:"os_version,omitempty"`
		Provider   string      `json:"provider,omitempty"`
		Duration   float64     `json:"duration"`
		GitSHA     string      `json:"git_sha,omitempty"`
		GitBranch  string      `json:"git_branch,omitempty"`
		TotalTests int         `json:"total_tests"`
		Passed     int         `json:"passed"`
		Failed     int         `json:"failed"`
		Skipped    int         `json:"skipped"`
		Tests      []cloudTest `json:"tests"`
	}

	rpt := cloudReport{
		Project:    meta.AppID,
		Platform:   meta.Platform,
		Device:     meta.DeviceName,
		OSVersion:  meta.OSVersion,
		Provider:   meta.Provider,
		TotalTests: len(results),
		Tests:      make([]cloudTest, 0, len(results)),
	}
	if rpt.Project == "" {
		rpt.Project = meta.DeviceID
	}

	// Resolve git info from local repo
	if sha, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		rpt.GitSHA = strings.TrimSpace(string(sha))
	}
	if branch, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		rpt.GitBranch = strings.TrimSpace(string(branch))
	}

	var totalDuration float64
	for _, res := range results {
		status := "passed"
		if res.Skipped {
			status = "skipped"
			rpt.Skipped++
		} else if !res.Passed {
			status = "failed"
			rpt.Failed++
		} else {
			rpt.Passed++
		}

		dur := res.Duration.Seconds()
		totalDuration += dur

		ct := cloudTest{
			Name:     res.TestName,
			File:     res.File,
			Status:   status,
			Duration: dur,
			VideoURL: res.VideoURL,
		}
		if res.Error != nil {
			ct.Error = res.Error.Error()
		}
		// Include all artifacts as comma-separated screenshot_url
		if len(res.Artifacts) > 0 {
			ct.ScreenshotURL = strings.Join(res.Artifacts, ",")
		}
		rpt.Tests = append(rpt.Tests, ct)
	}
	rpt.Duration = totalDuration

	return json.Marshal(rpt)
}

// promptUserConfirm asks the user for confirmation before destructive operations.
func promptUserConfirm(message string) bool {
	fmt.Printf("\n  \033[33m⚠  %s\033[0m [y/N] ", message)
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}
