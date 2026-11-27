package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the probe version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("probe version %s\n", Version)
	},
}

var rootCmd = &cobra.Command{
	Use:   "probe",
	Short: "FlutterProbe — high-performance Flutter E2E testing",
	Long: `FlutterProbe runs .probe test files against Flutter apps via direct
widget-tree access.

Examples:
  probe init                       # scaffold probe.yaml in your Flutter project
  probe test                       # run all tests in ./tests/
  probe test tests/login.probe     # run a single file
  probe test --tag smoke           # run tests tagged @smoke
  probe device list                # list connected emulators/simulators
  probe lint tests/                # validate .probe files without running
`,
	SilenceUsage: true,
}

// UsageError is a command line the command refuses before it does anything (a bad --property, say). The process exits 2 for
// it, not 1, so a script can tell "not run" from "ran and failed".
type UsageError struct{ Err error }

func (e UsageError) Error() string { return e.Err.Error() }
func (e UsageError) Unwrap() error { return e.Err }

// ExitCode is the process exit code for an error returned by Execute: 2 for a UsageError, 1 for everything else.
func ExitCode(err error) int {
	var u UsageError
	if errors.As(err, &u) {
		return 2
	}
	return 1
}

// Execute is the entry point called by main(). The update check starts before the command runs and its notice is printed after it,
// on stderr only; a failed check prints nothing (see updatecheck).
func Execute() error {
	check := startUpdateCheck(os.Args[1:])
	err := rootCmd.Execute()
	printUpdateNotice(os.Stderr, check)
	return err
}

func init() {
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().String("config", "", "path to probe.yaml (defaults to ./probe.yaml)")
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(deviceCmd)
	rootCmd.AddCommand(prefsCmd)
	rootCmd.AddCommand(versionCmd)
}

// loadConfig loads the probe config, respecting the --config flag.
// If --config is set, loads that specific file. Otherwise loads probe.yaml from cwd.
func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	cfgPath, _ := cmd.Flags().GetString("config")
	var cfg *config.Config
	var err error
	if cfgPath != "" {
		cfg, err = config.LoadFile(cfgPath)
	} else {
		cfgDir, _ := os.Getwd()
		cfg, err = config.Load(cfgDir)
	}
	if err != nil {
		return nil, err
	}
	cfg.CLIVersion = Version
	return cfg, nil
}
