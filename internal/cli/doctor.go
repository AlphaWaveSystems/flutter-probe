package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check that this project and machine are ready for probe test",
	Long: `doctor checks what a first run needs, and says what is missing and how to fix it: probe.yaml, the tests
folder, the Flutter SDK, the platform tools (adb for Android, xcrun for iOS) and the devices they can see.
It changes nothing. It exits non-zero only when a required check fails: probe.yaml or the tests folder.`,
	Example: `  probe doctor
  probe doctor --config probe.yaml`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

// doctorCheck is one line of the report. status is "ok", "warn" or "fail"; hint says what to do about a line that
// is not ok.
type doctorCheck struct {
	name   string
	status string
	detail string
	hint   string
}

// doctorEnv is what the checks look at. runDoctor fills it from this machine; the tests fill it with fakes.
type doctorEnv struct {
	goos     string
	stat     func(path string) (os.FileInfo, error)
	load     func() (*config.Config, error)
	lookPath func(file string) (string, error)
	tests    func(dir string) ([]string, error)
	devices  func(ctx context.Context, cfg *config.Config) ([]device.Device, error)
}

// runDoctorChecks runs every check in order. The checks after probe.yaml still run when it is broken, using the
// defaults, so one run shows everything that is missing.
func runDoctorChecks(ctx context.Context, env doctorEnv, cfgPath string) []doctorCheck {
	path := cfgPath
	if path == "" {
		path = "probe.yaml"
	}
	cfg, cfgCheck := checkConfig(env, path)
	checks := []doctorCheck{cfgCheck, checkTests(env), checkFlutter(env, cfg)}
	checks = append(checks, checkPlatformTools(env, cfg)...)
	return append(checks, checkDevices(ctx, env, cfg))
}

// doctorFailures counts the checks that must be fixed before probe test can run.
func doctorFailures(checks []doctorCheck) int {
	n := 0
	for _, c := range checks {
		if c.status == "fail" {
			n++
		}
	}
	return n
}

func checkConfig(env doctorEnv, path string) (*config.Config, doctorCheck) {
	c := doctorCheck{name: "config"}
	if _, err := env.stat(path); err != nil {
		c.status, c.detail = "fail", path+" not found"
		c.hint = "run `probe init` in your Flutter project, or pass --config <file>"
		return nil, c
	}
	cfg, err := env.load()
	if err != nil {
		c.status, c.detail = "fail", fmt.Sprintf("%s does not parse: %v", path, err)
		c.hint = "fix the line the error names, or run `probe init` to start from a valid file"
		return nil, c
	}
	c.status, c.detail = "ok", fmt.Sprintf("%s is valid (platform: %s)", path, platformOf(cfg))
	return cfg, c
}

func checkTests(env doctorEnv) doctorCheck {
	c := doctorCheck{name: "tests"}
	files, err := env.tests("tests")
	if err != nil || len(files) == 0 {
		c.status, c.detail = "fail", "no .probe files under tests/"
		c.hint = "add test files under tests/, or run `probe init` for a sample project"
		return c
	}
	c.status, c.detail = "ok", fmt.Sprintf("%d test file(s) under tests/", len(files))
	return c
}

func checkFlutter(env doctorEnv, cfg *config.Config) doctorCheck {
	c := doctorCheck{name: "flutter"}
	bin := "flutter"
	if cfg != nil && cfg.Tools.Flutter != "" {
		bin = cfg.Tools.Flutter
	}
	found, err := env.lookPath(bin)
	if err != nil {
		c.status, c.detail = "warn", bin+" not found"
		c.hint = "install the Flutter SDK and put it on PATH, or set tools.flutter in probe.yaml; flutter builds the app that probe tests"
		return c
	}
	c.status, c.detail = "ok", "found at "+found
	return c
}

// checkPlatformTools checks the tools for the platforms the project tests: adb for Android, xcrun for iOS.
func checkPlatformTools(env doctorEnv, cfg *config.Config) []doctorCheck {
	platform := platformOf(cfg)
	var checks []doctorCheck
	if platform == "android" || platform == "both" {
		bin := "adb"
		if cfg != nil && cfg.Tools.ADB != "" {
			bin = cfg.Tools.ADB
		}
		c := doctorCheck{name: "adb"}
		if found, err := env.lookPath(bin); err != nil {
			c.status, c.detail = "warn", bin+" not found (needed for Android)"
			c.hint = "install Android platform-tools, or set tools.adb in probe.yaml"
		} else {
			c.status, c.detail = "ok", "found at "+found
		}
		checks = append(checks, c)
	}
	if platform == "ios" || platform == "both" {
		c := doctorCheck{name: "xcrun"}
		switch {
		case env.goos != "darwin":
			c.status, c.detail = "warn", "iOS testing needs macOS and Xcode (this machine runs "+env.goos+")"
			c.hint = "run iOS tests on a Mac, or set defaults.platform to android in probe.yaml"
		default:
			if found, err := env.lookPath("xcrun"); err != nil {
				c.status, c.detail = "warn", "xcrun not found (needed for iOS)"
				c.hint = "install Xcode and its command line tools: xcode-select --install"
			} else {
				c.status, c.detail = "ok", "found at "+found
			}
		}
		checks = append(checks, c)
	}
	return checks
}

func checkDevices(ctx context.Context, env doctorEnv, cfg *config.Config) doctorCheck {
	c := doctorCheck{name: "devices"}
	devs, err := env.devices(ctx, cfg)
	if err != nil {
		c.status, c.detail = "warn", "could not list devices: "+err.Error()
		c.hint = "fix the platform tool above, then run `probe device list`"
		return c
	}
	if len(devs) == 0 {
		c.status, c.detail = "warn", "no device or simulator is running"
		c.hint = "start an emulator (`emulator -avd <name>`) or a simulator (`xcrun simctl boot <udid>`), then run `probe device list`"
		return c
	}
	// Name the first few devices and count the rest, so a machine with many simulators still gets one short line.
	shown := make([]string, 0, 3)
	for _, d := range devs {
		if len(shown) == 3 {
			break
		}
		if d.Name != "" {
			shown = append(shown, d.Name)
		} else {
			shown = append(shown, d.ID)
		}
	}
	list := strings.Join(shown, ", ")
	if more := len(devs) - len(shown); more > 0 {
		list += fmt.Sprintf(" and %d more", more)
	}
	c.status, c.detail = "ok", fmt.Sprintf("%d visible: %s", len(devs), list)
	return c
}

// platformOf is the platform the project tests. Without a probe.yaml it is the default, android.
func platformOf(cfg *config.Config) string {
	if cfg != nil && cfg.Defaults.Platform != "" {
		return cfg.Defaults.Platform
	}
	return "android"
}

// realDoctorEnv looks at this machine and the project in the working directory.
func realDoctorEnv(cmd *cobra.Command) doctorEnv {
	return doctorEnv{
		goos:     runtime.GOOS,
		stat:     os.Stat,
		load:     func() (*config.Config, error) { return loadConfig(cmd) },
		lookPath: exec.LookPath,
		tests:    func(dir string) ([]string, error) { return runner.CollectFiles([]string{dir}) },
		devices: func(ctx context.Context, cfg *config.Config) ([]device.Device, error) {
			paths := device.ToolPaths{ADB: "adb", Flutter: "flutter"}
			if cfg != nil {
				if cfg.Tools.ADB != "" {
					paths.ADB = cfg.Tools.ADB
				}
				if cfg.Tools.Flutter != "" {
					paths.Flutter = cfg.Tools.Flutter
				}
			}
			return device.NewManagerWithPaths(paths).List(ctx)
		},
	}
}

// errDoctorFailed means a required check failed. The report has already said what to fix.
var errDoctorFailed = errors.New("required doctor check failed")

func runDoctor(cmd *cobra.Command, _ []string) error {
	cfgPath, _ := cmd.Flags().GetString("config")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return printDoctorReport(cmd, runDoctorChecks(ctx, realDoctorEnv(cmd), cfgPath))
}

// printDoctorReport prints one line per check and returns errDoctorFailed when a required check failed. cmd's
// SilenceErrors is set only for that case, so cobra does not repeat the problem as "Error:", and a bad argument
// still gets its message.
func printDoctorReport(cmd *cobra.Command, checks []doctorCheck) error {
	w := cmd.OutOrStdout()
	fmt.Fprintln(w, "probe doctor")
	warned := 0
	for _, c := range checks {
		line := fmt.Sprintf("%-10s %s", c.name, c.detail)
		switch c.status {
		case "ok":
			statusOK(w, "%s", line)
		case "warn":
			warned++
			statusWarn(w, "%s", line)
		default:
			statusFail(w, "%s", line)
		}
		if c.status != "ok" && c.hint != "" {
			statusInfo(w, "    fix: %s", c.hint)
		}
	}

	if failed := doctorFailures(checks); failed > 0 {
		fmt.Fprintf(w, "\n%d problem(s) to fix before probe test can run. Fix the lines marked ✗ and run probe doctor again.\n", failed)
		cmd.SilenceErrors = true
		return errDoctorFailed
	}
	if warned > 0 {
		fmt.Fprintf(w, "\nReady to run tests, with %d warning(s) above.\n", warned)
	} else {
		fmt.Fprintln(w, "\nReady to run tests.")
	}
	return nil
}
