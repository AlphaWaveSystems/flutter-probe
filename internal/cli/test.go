package cli

import (
	"context"
	"fmt"
	"os"
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
	if !dryRun {
		dm := device.NewManager()
		// Pick device and detect platform
		platform := device.Platform(cfg.Defaults.Platform)
		if deviceSerial == "" {
			devices, err := dm.List(ctx)
			if err != nil || len(devices) == 0 {
				return fmt.Errorf("no connected devices found. Run 'probe device list' to check.")
			}
			deviceSerial = devices[0].ID
			platform = devices[0].Platform
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

	// Build and run
	opts := runner.RunOptions{
		Files:   files,
		Tags:    tags,
		Timeout: timeout,
		DryRun:  dryRun,
		Verbose: verbose,
	}

	r := runner.New(cfg, client, opts)

	fmt.Printf("  Running %d test file(s)...\n\n", len(files))
	results, err := r.Run(ctx)
	if err != nil {
		return err
	}

	if err := report.Report(results); err != nil {
		return err
	}

	if !runner.AllPassed(results) {
		os.Exit(1)
	}
	return nil
}
