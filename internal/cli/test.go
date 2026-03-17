package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/cloud"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
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
	f.Int("shard", 0, "split tests across N devices in parallel (0 = no sharding)")
	f.String("format", "terminal", "output format: terminal | junit | json")
	f.StringP("output", "o", "", "write report to file instead of stdout")
	f.Bool("dry-run", false, "parse and validate .probe files without executing against a device")

	// Device selection
	f.String("device", "", "target device serial or UDID (default: first available)")

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
	f.String("cloud-url", "", `cloud API base URL (default: "https://flutterprobe-cloud.fly.dev")`)

	// Cloud device farm providers
	f.String("cloud-provider", "", "cloud device farm provider (browserstack, aws, firebase, saucelabs, lambdatest)")
	f.String("cloud-app", "", "path to app binary (.apk/.ipa) to upload to cloud provider")
	f.String("cloud-device", "", "target device name on the cloud provider")
	f.String("cloud-key", "", "cloud provider API key or username")
	f.String("cloud-secret", "", "cloud provider API secret or access key")

	// x402 pay-per-use
	f.String("pay", "", `payment method for cloud upload: "x402" for pay-per-use via crypto wallet`)
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
		fmt.Println("No .probe files found.")
		return nil
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

	// Connect to ProbeAgent (skip if dry-run)
	var client *probelink.Client
	var dm *device.Manager
	var platform device.Platform
	var cloudSession *cloud.Session // non-nil when using a cloud provider
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

			// Step 1: Upload app
			fmt.Printf("  Uploading app to %s...\n", provider.Name())
			appID, err := provider.UploadApp(ctx, cloudApp)
			if err != nil {
				return fmt.Errorf("cloud upload: %w", err)
			}
			fmt.Printf("  \033[32m✓\033[0m  App uploaded: %s\n", appID)

			// Step 2: Start session
			fmt.Printf("  Starting cloud session on %s (%s)...\n", targetDevice, provider.Name())
			sess, err := provider.StartSession(ctx, appID, targetDevice)
			if err != nil {
				return fmt.Errorf("cloud session: %w", err)
			}
			cloudSession = &sess
			defer func() {
				fmt.Printf("  Stopping cloud session %s...\n", sess.ID)
				if stopErr := provider.StopSession(ctx, sess); stopErr != nil {
					fmt.Printf("  \033[33m⚠  Failed to stop cloud session: %s\033[0m\n", stopErr)
				} else {
					fmt.Printf("  \033[32m✓\033[0m  Cloud session stopped\n")
				}
			}()
			fmt.Printf("  \033[32m✓\033[0m  Session started: %s\n", sess.ID)

			// Step 3: Forward port
			devicePort := cfg.Agent.AgentDevicePort()
			localPort, err := provider.ForwardPort(ctx, sess, devicePort)
			if err != nil {
				return fmt.Errorf("cloud port forward: %w", err)
			}
			sess.LocalPort = localPort
			fmt.Printf("  \033[32m✓\033[0m  Port forwarded: localhost:%d -> device:%d\n", localPort, devicePort)

			// Step 4: Connect to ProbeAgent via the tunneled port
			dialOpts := probelink.DialOptions{
				Host:        "127.0.0.1",
				Port:        localPort,
				DialTimeout: cfg.Agent.DialTimeout,
			}
			// Cloud sessions may not require token auth (handled by the provider)
			// but we attempt to connect with whatever the provider gives us
			client, err = probelink.DialWithOptions(ctx, dialOpts)
			if err != nil {
				return fmt.Errorf("connecting to cloud ProbeAgent: %w", err)
			}
			defer client.Close()

			if err := client.Ping(ctx); err != nil {
				return fmt.Errorf("cloud agent ping failed: %w", err)
			}
			fmt.Printf("  \033[32m✓\033[0m  Connected to ProbeAgent on %s (%s)\n\n", targetDevice, provider.Name())

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
					return fmt.Errorf("no connected devices found. Run 'probe device list' to check.")
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
				fmt.Printf("  \033[32m✓\033[0m  App installed and launched\n")
			}

			dialOpts := probelink.DialOptions{
				Host:        "127.0.0.1",
				Port:        cfg.Agent.Port,
				DialTimeout: cfg.Agent.DialTimeout,
			}

			if platform == device.PlatformIOS {
				// iOS: simulators share host loopback — no port forwarding needed
				fmt.Println("  Waiting for ProbeAgent token (iOS)...")
				token, err := dm.ReadTokenIOS(ctx, deviceSerial, cfg.Agent.TokenReadTimeout)
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
				// Android: forward port via ADB
				if err := dm.ForwardPort(ctx, deviceSerial, cfg.Agent.Port, cfg.Agent.AgentDevicePort()); err != nil {
					return fmt.Errorf("port forward: %w", err)
				}
				defer dm.RemoveForward(ctx, deviceSerial, cfg.Agent.Port) //nolint:errcheck

				fmt.Println("  Waiting for ProbeAgent token...")
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
			fmt.Printf("  \033[32m✓\033[0m  Connected to ProbeAgent on %s\n\n", deviceSerial)
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

	// Attach run metadata for JSON/HTML reports
	if !dryRun && dm != nil {
		meta := runner.RunMetadata{
			DeviceID: deviceSerial,
			Platform: string(platform),
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
		report.SetMetadata(meta)
	}

	// Tags
	var tags []string
	if tag != "" {
		tags = []string{tag}
	}

	// Build device context for platform-level operations (restart, clear data)
	var devCtx *runner.DeviceContext
	if !dryRun && client != nil {
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

	fmt.Printf("  Running %d test file(s)...\n\n", len(files))
	results, err := r.Run(ctx)
	if err != nil {
		return err
	}

	// Pull screenshots from device to local reports/screenshots/ folder
	if devCtx != nil {
		screenshotDir := filepath.Join(reportsBase, "screenshots")
		if outFile != "" {
			screenshotDir = filepath.Join(filepath.Dir(outFile), "screenshots")
		}
		runner.PullArtifacts(ctx, results, devCtx, screenshotDir)
	}

	if err := report.Report(results); err != nil {
		return err
	}

	// Cloud upload (after report is written)
	cloudEnabled, _ := cmd.Flags().GetBool("cloud")
	payMethod, _ := cmd.Flags().GetString("pay")
	if cloudEnabled || payMethod == "x402" {
		cloudToken, _ := cmd.Flags().GetString("cloud-token")
		cloudURL, _ := cmd.Flags().GetString("cloud-url")

		// Resolution order: CLI flag > probe.yaml > default
		if cloudToken == "" {
			cloudToken = cfg.Cloud.Token
		}
		if cloudURL == "" {
			cloudURL = cfg.Cloud.URL
		}

		// Prepare JSON data for upload.
		var jsonData []byte
		if format == "json" && outFile != "" {
			jsonData, err = os.ReadFile(outFile)
			if err != nil {
				fmt.Printf("\n  \033[31m✗  Cloud upload: could not read JSON results: %s\033[0m\n", err)
			}
		}
		if len(jsonData) == 0 {
			jsonData, err = generateJSONResults(results, report)
			if err != nil {
				fmt.Printf("\n  \033[31m✗  Cloud upload: could not serialize results: %s\033[0m\n", err)
			}
		}

		if len(jsonData) > 0 && payMethod == "x402" {
			// x402 pay-per-use upload — no subscription token needed.
			configDir, cfgErr := cloud.ConfigDir()
			if cfgErr != nil {
				fmt.Printf("\n  \033[31m✗  x402: could not locate config dir: %s\033[0m\n", cfgErr)
			} else {
				wallet, walletErr := cloud.LoadWallet(configDir)
				if walletErr != nil {
					fmt.Printf("\n  \033[31m✗  x402: %s\033[0m\n", walletErr)
				} else {
					fmt.Printf("\n  Uploading results via x402 (wallet %s)...\n", wallet.Address)
					cc := cloud.NewClient(cloudURL, "")
					runID, dashURL, uploadErr := cc.UploadResultsWithPayment(ctx, jsonData, wallet)
					if uploadErr != nil {
						fmt.Printf("  \033[31m✗  x402 upload failed: %s\033[0m\n", uploadErr)
					} else {
						fmt.Printf("  \033[32m✓\033[0m  Paid & uploaded (run %s)\n", runID)
						fmt.Printf("  \033[36m→  %s\033[0m\n", dashURL)
					}
				}
			}
		} else if len(jsonData) > 0 && cloudToken != "" {
			// Subscription-based upload.
			fmt.Println("\n  Uploading results to FlutterProbe Cloud...")
			cc := cloud.NewClient(cloudURL, cloudToken)
			runID, dashURL, uploadErr := cc.UploadResults(ctx, jsonData)
			if uploadErr != nil {
				fmt.Printf("  \033[31m✗  Cloud upload failed: %s\033[0m\n", uploadErr)
			} else {
				fmt.Printf("  \033[32m✓\033[0m  Uploaded (run %s)\n", runID)
				fmt.Printf("  \033[36m→  %s\033[0m\n", dashURL)
			}
		} else if cloudToken == "" && payMethod != "x402" {
			fmt.Println("\n  \033[33m⚠  --cloud-token not set and cloud.token not found in probe.yaml. Skipping cloud upload.\033[0m")
		}
	}

	if !runner.AllPassed(results) {
		os.Exit(1)
	}
	return nil
}

// generateJSONResults serializes test results to JSON for cloud upload when
// the user did not use --format json.
func generateJSONResults(results []runner.TestResult, report *runner.Reporter) ([]byte, error) {
	type jsonResult struct {
		Name     string  `json:"name"`
		File     string  `json:"file"`
		Passed   bool    `json:"passed"`
		Skipped  bool    `json:"skipped"`
		Duration float64 `json:"duration_ms"`
		Error    string  `json:"error,omitempty"`
		Row      int     `json:"row,omitempty"`
	}
	type jsonReport struct {
		TotalTests int          `json:"total_tests"`
		Passed     int          `json:"passed"`
		Failed     int          `json:"failed"`
		Skipped    int          `json:"skipped"`
		Results    []jsonResult `json:"results"`
	}

	rpt := jsonReport{TotalTests: len(results)}
	for _, res := range results {
		jr := jsonResult{
			Name:     res.TestName,
			File:     res.File,
			Passed:   res.Passed,
			Skipped:  res.Skipped,
			Duration: float64(res.Duration.Milliseconds()),
			Row:      res.Row,
		}
		if res.Error != nil {
			jr.Error = res.Error.Error()
		}
		switch {
		case res.Skipped:
			rpt.Skipped++
		case res.Passed:
			rpt.Passed++
		default:
			rpt.Failed++
		}
		rpt.Results = append(rpt.Results, jr)
	}
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
