package cli

import (
	"fmt"
	"os"

	"github.com/flutterprobe/probe/internal/config"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "probe",
	Short: "FlutterProbe — high-performance Flutter E2E testing",
	Long: `FlutterProbe runs .probe test files against Flutter apps via direct
widget-tree access. Sub-50ms command execution. No flake.

Examples:
  probe init                       # scaffold probe.yaml in your Flutter project
  probe test                       # run all tests in ./tests/
  probe test tests/login.probe     # run a single file
  probe test --tag smoke           # run tests tagged @smoke
  probe test --watch               # re-run on file changes
  probe device list                # list connected emulators/simulators
  probe lint tests/                # validate .probe files without running
`,
	SilenceUsage: true,
}

// Execute is the entry point called by main().
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().String("config", "", "path to probe.yaml (defaults to ./probe.yaml)")
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(deviceCmd)
}

// loadConfig loads the probe config, respecting the --config flag.
// If --config is set, loads that specific file. Otherwise loads probe.yaml from cwd.
func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath != "" {
		return config.LoadFile(cfgPath)
	}
	cfgDir, _ := os.Getwd()
	return config.Load(cfgDir)
}

// exitOnErr prints an error and exits with code 1.
func exitOnErr(err error) {
	fmt.Fprintln(os.Stderr, "\033[31merror:\033[0m", err)
	os.Exit(1)
}
