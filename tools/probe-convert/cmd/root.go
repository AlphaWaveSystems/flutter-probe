package cmd

import (
	"fmt"
	"os"

	"github.com/alphawavesystems/probe-convert/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "probe-convert [flags] <file|dir>...",
	Short: "Convert test files from other frameworks to ProbeScript",
	Long: `probe-convert transforms test files from popular mobile testing frameworks
into FlutterProbe's ProbeScript (.probe) format.

Supported formats:
  maestro    Maestro YAML flows (.yaml, .yml)
  gherkin    Cucumber/Gherkin feature files (.feature)
  robot      Robot Framework test suites (.robot)
  detox      Detox JavaScript/TypeScript tests (.js, .ts)
  appium     Appium tests in Python, Java, or JS (.py, .java, .kt, .js)

Format auto-detection:
  .feature → Gherkin       .robot → Robot Framework
  .yaml/.yml → Maestro     .js/.ts → Detox (if element(by.) found) or Appium
  .py → Appium Python      .java/.kt → Appium Java

Verification:
  --lint       After conversion, run 'probe lint' to validate ProbeScript syntax
  --verify     After conversion, run 'probe test --dry-run' for full validation
               (parse + recipe resolution + test enumeration, no device needed)

Examples:
  probe-convert tests/maestro/                    # convert all Maestro YAMLs
  probe-convert login.feature -o probe_tests/     # convert Gherkin to output dir
  probe-convert --from appium test_login.py       # force Appium format
  probe-convert --dry-run suite.robot             # preview without writing
  probe-convert -r tests/ -o probe_tests/         # recursive batch convert
  probe-convert --lint login.feature              # convert + lint
  probe-convert --verify tests/ -o out/           # convert + full dry-run verify

Use "probe-convert formats <format>" for format-specific docs and examples.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.ArbitraryArgs,
	RunE:          runConvert,
}

func init() {
	rootCmd.Flags().StringP("from", "f", "", "force format: maestro|gherkin|robot|detox|appium")
	rootCmd.Flags().StringP("output", "o", "", "output directory or file")
	rootCmd.Flags().Bool("dry-run", false, "preview to stdout, don't write files")
	rootCmd.Flags().BoolP("recursive", "r", false, "recurse into subdirectories")
	rootCmd.Flags().BoolP("verbose", "v", false, "per-step conversion details")
	rootCmd.Flags().Bool("no-color", false, "disable colored output")
	rootCmd.Flags().Bool("lint", false, "validate generated .probe files with 'probe lint' after conversion")
	rootCmd.Flags().Bool("verify", false, "run 'probe test --dry-run' on generated files (full parse + recipe resolution, no device)")
	rootCmd.Flags().String("probe-path", "", "path to probe binary (auto-detected if not set)")
}

// Execute is the CLI entry point.
func Execute() {
	if os.Getenv("NO_COLOR") != "" {
		ui.SetNoColor(true)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "\033[31merror:\033[0m %s\n", err)
		os.Exit(1)
	}
}
