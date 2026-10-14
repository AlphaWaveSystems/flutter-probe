package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check probe.yaml and the probe_version pin without connecting to a device",
	Long: `validate is the check to run in CI before probe test. It checks that probe.yaml loads, that probe_version (when
set) is a valid pin and this CLI meets it, and that tests/ has .probe files. It never connects to a device or the
network. It exits non-zero when a check fails; a missing tests/ folder is only a warning.`,
	Example: `  probe validate
  probe validate --format json`,
	Args: cobra.NoArgs,
	RunE: runValidate,
}

// errValidateFailed means a check failed. The report has already said what to fix.
var errValidateFailed = errors.New("probe validate found problems")

func init() {
	validateCmd.Flags().String("format", "text", "output format: text or json")
	rootCmd.AddCommand(validateCmd)
}

// validateCheck is one line of the report. status is "ok", "warn" or "fail"; fix says what to do about a line that is
// not ok.
type validateCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// validateReport is the JSON form of the report. schema versions the shape for tools that read it.
type validateReport struct {
	Schema       string          `json:"schema"`
	OK           bool            `json:"ok"`
	CLIVersion   string          `json:"cli_version"`
	ProbeVersion string          `json:"probe_version,omitempty"`
	Checks       []validateCheck `json:"checks"`
}

// validateInput is what the checks look at. runValidate fills it from the project in the working directory; the tests
// fill it directly, so they need no files.
type validateInput struct {
	cfgPath    string
	cfgExists  bool
	cfg        *config.Config // nil when probe.yaml does not load
	loadErr    error
	cliVersion string
	tests      []string
	testsErr   error
}

func runValidate(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("format")
	if format != "text" && format != "json" {
		return fmt.Errorf("unknown --format %q: use text or json", format)
	}
	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath == "" {
		cfgPath = "probe.yaml"
	}
	in := validateInput{cfgPath: cfgPath, cliVersion: Version}
	if _, err := os.Stat(cfgPath); err == nil {
		in.cfgExists = true
		in.cfg, in.loadErr = loadConfig(cmd)
	}
	in.tests, in.testsErr = runner.CollectFiles([]string{"tests"})

	checks := validateChecks(in)
	report := validateReport{
		Schema:     "probe.validate/1",
		OK:         validateOK(checks),
		CLIVersion: strings.TrimPrefix(Version, "v"),
		Checks:     checks,
	}
	if in.cfg != nil {
		report.ProbeVersion = in.cfg.ProbeVersion
	}

	w := cmd.OutOrStdout()
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printValidateReport(w, checks)
	}
	if !report.OK {
		// The report already says what to fix, so cobra should not repeat the failure as "Error:".
		cmd.SilenceErrors = true
		return errValidateFailed
	}
	return nil
}

// validateChecks runs the checks in order. The pin is checked only when probe.yaml loads, because there is no pin to
// read otherwise; the tests check runs either way.
func validateChecks(in validateInput) []validateCheck {
	cfgCheck := validateCheck{Name: "config"}
	switch {
	case !in.cfgExists:
		cfgCheck.Status, cfgCheck.Detail = "fail", in.cfgPath+" not found"
		cfgCheck.Fix = "run `probe init` in your Flutter project, or pass --config <file>"
	case in.loadErr != nil:
		cfgCheck.Status, cfgCheck.Detail = "fail", fmt.Sprintf("%s does not parse: %v", in.cfgPath, in.loadErr)
		cfgCheck.Fix = "fix the line the error names, or run `probe init` to start from a valid file"
	default:
		cfgCheck.Status, cfgCheck.Detail = "ok", in.cfgPath+" is valid"
	}
	checks := []validateCheck{cfgCheck}
	if in.cfg != nil {
		checks = append(checks, validatePin(in.cfg.ProbeVersion, in.cliVersion))
	}

	testsCheck := validateCheck{Name: "tests"}
	if in.testsErr != nil || len(in.tests) == 0 {
		testsCheck.Status, testsCheck.Detail = "warn", "no .probe files under tests/"
		testsCheck.Fix = "add test files under tests/, or run `probe init` for a sample project"
	} else {
		testsCheck.Status, testsCheck.Detail = "ok", fmt.Sprintf("%d test file(s) under tests/", len(in.tests))
	}
	return append(checks, testsCheck)
}

// validatePin checks probe_version against this CLI. A project without a pin passes. A value that is not a valid pin is
// refused when probe.yaml loads, so here it is reported only for a caller that skips that step.
func validatePin(value, cliVersion string) validateCheck {
	c := validateCheck{Name: "pin"}
	if value == "" {
		c.Status, c.Detail = "ok", "no probe_version in probe.yaml; any CLI version can run this project"
		return c
	}
	pin, err := config.ParseProbeVersion(value)
	if err != nil {
		c.Status, c.Detail = "fail", fmt.Sprintf("invalid probe_version: %v", err)
		c.Fix = "write an exact release such as 0.25.1, or a release line such as 0.25.x"
		return c
	}
	installed := strings.TrimPrefix(cliVersion, "v")
	if pin.Satisfied(cliVersion) {
		c.Status, c.Detail = "ok", fmt.Sprintf("CLI %s meets probe_version %s", installed, pin)
		return c
	}
	c.Status, c.Detail = "fail", fmt.Sprintf("CLI %s does not meet probe_version %s", installed, pin)
	c.Fix = probeVersionFix(pin, installed)
	return c
}

// validateOK is true when no check failed. Warnings do not fail the validator.
func validateOK(checks []validateCheck) bool {
	for _, c := range checks {
		if c.Status == "fail" {
			return false
		}
	}
	return true
}

func printValidateReport(w io.Writer, checks []validateCheck) {
	fmt.Fprintln(w, "probe validate")
	failed := 0
	for _, c := range checks {
		line := fmt.Sprintf("%-7s %s", c.Name, c.Detail)
		switch c.Status {
		case "ok":
			statusOK(w, "%s", line)
		case "warn":
			statusWarn(w, "%s", line)
		default:
			failed++
			statusFail(w, "%s", line)
		}
		if c.Status != "ok" && c.Fix != "" {
			statusInfo(w, "    fix: %s", c.Fix)
		}
	}
	if failed > 0 {
		fmt.Fprintf(w, "\n%d problem(s) to fix. Fix the lines marked ✗ and run probe validate again.\n", failed)
		return
	}
	fmt.Fprintln(w, "\nprobe.yaml is ready for probe test.")
}
