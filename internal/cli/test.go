package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flutterprobe/probe/internal/config"
	"github.com/flutterprobe/probe/internal/device"
	"github.com/flutterprobe/probe/internal/probelink"
	"github.com/flutterprobe/probe/internal/runner"
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test [file|dir]...",
	Short: "Run .probe test files against the connected Flutter app",
	Example: `  probe test                         # run all tests in ./tests/
  probe test tests/login.probe       # run one file
  probe test --tag smoke             # run tests tagged @smoke
  probe test --watch                 # re-run on file changes
  probe test --format junit -o results.xml`,
	RunE: runTests,
}

func init() {
	testCmd.Flags().StringP("tag", "t", "", "run only tests matching this tag")
	testCmd.Flags().BoolP("watch", "w", false, "watch mode — re-run on file changes")
	testCmd.Flags().Int("shard", 0, "split tests across N devices in parallel")
	testCmd.Flags().String("format", "terminal", "output format: terminal | junit | json")
	testCmd.Flags().StringP("output", "o", "", "write report to file (default: stdout)")
	testCmd.Flags().String("device", "", "target device serial (default: first available)")
	testCmd.Flags().Duration("timeout", 30*time.Second, "per-step timeout")
	testCmd.Flags().Bool("dry-run", false, "parse .probe files without executing")
	testCmd.Flags().String("adb", "", "path to adb binary (overrides probe.yaml and PATH)")
	testCmd.Flags().String("flutter", "", "path to flutter binary (overrides probe.yaml and PATH)")
	testCmd.Flags().BoolP("yes", "y", false, "auto-confirm destructive operations (clear app data, etc.)")
	testCmd.Flags().String("app-path", "", "path to APK (.apk) or iOS app bundle (.app) to install before testing")
	testCmd.Flags().Bool("video", false, "enable video recording (overrides probe.yaml)")
	testCmd.Flags().Bool("no-video", false, "disable video recording (overrides probe.yaml)")
}

func runTests(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load config
	cfgDir, _ := os.Getwd()
	cfg, err := config.Load(cfgDir)
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

	// Connect to ProbeAgent (skip if dry-run)
	var client *probelink.Client
	var dm *device.Manager
	var platform device.Platform
	if !dryRun {
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
		// Pick device and detect platform
		platform = device.Platform(cfg.Defaults.Platform)
		if deviceSerial == "" {
			devices, err := dm.List(ctx)
			if err != nil || len(devices) == 0 {
				return fmt.Errorf("no connected devices found. Run 'probe device list' to check.")
			}
			deviceSerial = devices[0].ID
			platform = devices[0].Platform
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

		if platform == device.PlatformIOS {
			// iOS: simulators share host loopback — no port forwarding needed
			fmt.Println("  Waiting for ProbeAgent token (iOS)...")
			token, err := dm.ReadTokenIOS(ctx, deviceSerial, 30*time.Second)
			if err != nil {
				return fmt.Errorf("agent token: %w — is the app running with probe_agent?", err)
			}
			client, err = probelink.Dial(ctx, "127.0.0.1", 8686, token)
			if err != nil {
				return fmt.Errorf("connecting to ProbeAgent: %w", err)
			}
			defer client.Close()
		} else {
			// Android: forward port via ADB
			if err := dm.ForwardPort(ctx, deviceSerial, 8686, 8686); err != nil {
				return fmt.Errorf("port forward: %w", err)
			}
			defer dm.RemoveForward(ctx, deviceSerial, 8686) //nolint:errcheck

			fmt.Println("  Waiting for ProbeAgent token...")
			token, err := dm.ReadToken(ctx, deviceSerial, 30*time.Second)
			if err != nil {
				return fmt.Errorf("agent token: %w — is the app running with probe_agent?", err)
			}
			client, err = probelink.Dial(ctx, "127.0.0.1", 8686, token)
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
			Port:                    8686,
			AllowClearData:          autoYes,
			Confirm:                 promptUserConfirm,
			GrantPermissionsOnClear: autoYes || cfg.Defaults.GrantPermissionsOnClear,
		}
	}

	// Resolve video setting: CLI flags > probe.yaml
	videoEnabled := cfg.Defaults.Video
	if videoFlag {
		videoEnabled = true
	}
	if noVideoFlag {
		videoEnabled = false
	}
	videoDir := "reports/videos"
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
		screenshotDir := "reports/screenshots"
		if outFile != "" {
			screenshotDir = filepath.Join(filepath.Dir(outFile), "screenshots")
		}
		runner.PullArtifacts(ctx, results, devCtx, screenshotDir)
	}

	if err := report.Report(results); err != nil {
		return err
	}

	if !runner.AllPassed(results) {
		os.Exit(1)
	}
	return nil
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
