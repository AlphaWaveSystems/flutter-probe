package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/spf13/cobra"
)

// doctorFixture describes a machine and a project for the checks. A tool that is not in tools is not on PATH.
type doctorFixture struct {
	goos      string
	tools     map[string]string
	probeYAML bool   // probe.yaml exists
	cfgErr    error  // what loading probe.yaml returns, when it exists
	platform  string // defaults.platform in probe.yaml
	testFiles []string
	devices   []device.Device
	devErr    error
}

func (f doctorFixture) env() doctorEnv {
	return doctorEnv{
		goos: f.goos,
		stat: func(string) (os.FileInfo, error) {
			if f.probeYAML {
				return nil, nil
			}
			return nil, os.ErrNotExist
		},
		load: func() (*config.Config, error) {
			if f.cfgErr != nil {
				return nil, f.cfgErr
			}
			cfg := &config.Config{}
			cfg.Defaults.Platform = f.platform
			return cfg, nil
		},
		lookPath: func(file string) (string, error) {
			if p, ok := f.tools[file]; ok {
				return p, nil
			}
			return "", fmt.Errorf("%s: not found", file)
		},
		tests: func(string) ([]string, error) { return f.testFiles, nil },
		devices: func(context.Context, *config.Config) ([]device.Device, error) {
			return f.devices, f.devErr
		},
	}
}

// healthyDoctorFixture is an Android project on a Mac with everything in place.
func healthyDoctorFixture() doctorFixture {
	return doctorFixture{
		goos:      "darwin",
		tools:     map[string]string{"flutter": "/sdk/flutter", "adb": "/sdk/adb", "xcrun": "/usr/bin/xcrun"},
		probeYAML: true,
		platform:  "android",
		testFiles: []string{"tests/login.probe"},
		devices:   []device.Device{{ID: "emulator-5554"}},
	}
}

// The status of each check, by name, for a machine and project. A broken probe.yaml, a missing tool, or no device
// each show up as the line that names it, with a fix.
func TestDoctorChecksReportWhatIsMissingAndHowToFixIt(t *testing.T) {
	with := func(change func(*doctorFixture)) doctorFixture {
		f := healthyDoctorFixture()
		f.tools = map[string]string{}
		maps.Copy(f.tools, healthyDoctorFixture().tools)
		change(&f)
		return f
	}
	cases := []struct {
		name string
		fx   doctorFixture
		want map[string]string // check name -> status
	}{
		{"everything is in place", healthyDoctorFixture(), map[string]string{"config": "ok", "tests": "ok", "flutter": "ok", "adb": "ok", "devices": "ok"}},
		{"no probe.yaml", with(func(f *doctorFixture) { f.probeYAML = false }), map[string]string{"config": "fail"}},
		{"probe.yaml does not parse", with(func(f *doctorFixture) { f.cfgErr = errors.New("line 3: bad indent") }), map[string]string{"config": "fail"}},
		{"no test files", with(func(f *doctorFixture) { f.testFiles = nil }), map[string]string{"tests": "fail"}},
		{"flutter missing", with(func(f *doctorFixture) { delete(f.tools, "flutter") }), map[string]string{"flutter": "warn"}},
		{"adb missing for android", with(func(f *doctorFixture) { delete(f.tools, "adb") }), map[string]string{"adb": "warn"}},
		{"ios on a machine without macOS", with(func(f *doctorFixture) { f.platform = "ios"; f.goos = "linux" }), map[string]string{"xcrun": "warn"}},
		{"ios on macOS without xcrun", with(func(f *doctorFixture) { f.platform = "ios"; delete(f.tools, "xcrun") }), map[string]string{"xcrun": "warn"}},
		{"ios on macOS with xcrun", with(func(f *doctorFixture) { f.platform = "ios" }), map[string]string{"xcrun": "ok"}},
		{"no device running", with(func(f *doctorFixture) { f.devices = nil }), map[string]string{"devices": "warn"}},
		{"devices cannot be listed", with(func(f *doctorFixture) { f.devErr = errors.New("adb: server not running") }), map[string]string{"devices": "warn"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checks := runDoctorChecks(context.Background(), c.fx.env(), "")
			got := map[string]string{}
			for _, ch := range checks {
				got[ch.name] = ch.status
			}
			for name, status := range c.want {
				if got[name] != status {
					t.Errorf("%s = %q, want %q (checks: %+v)", name, got[name], status, checks)
				}
			}
		})
	}
}

// Warnings alone do not fail the command, and a missing probe.yaml counts as one required failure. The later checks
// still run when probe.yaml is missing, so one run shows everything that is missing.
func TestDoctorExitsNonZeroOnlyForRequiredChecks(t *testing.T) {
	ctx := context.Background()
	warnOnly := healthyDoctorFixture()
	warnOnly.tools = map[string]string{"flutter": "/sdk/flutter"} // adb missing, no device: warnings
	warnOnly.devices = nil
	if checks := runDoctorChecks(ctx, warnOnly.env(), ""); doctorFailures(checks) != 0 {
		t.Errorf("warnings alone must not fail the command: %+v", checks)
	}

	broken := warnOnly
	broken.probeYAML = false
	checks := runDoctorChecks(ctx, broken.env(), "")
	if n := doctorFailures(checks); n != 1 {
		t.Errorf("a missing probe.yaml must count as one failure, got %d", n)
	}
	if len(checks) != 5 {
		t.Errorf("all five checks must run when probe.yaml is missing, got %d: %+v", len(checks), checks)
	}
}

// A required failure returns errDoctorFailed and silences cobra's own "Error:" line, because the report already names
// the problem. A warning alone returns nil and leaves cobra's output alone.
func TestPrintDoctorReportSilencesCobraOnlyForRequiredFailures(t *testing.T) {
	failing := &cobra.Command{Use: "doctor"}
	failing.SetOut(io.Discard)
	if err := printDoctorReport(failing, []doctorCheck{{name: "config", status: "fail", detail: "probe.yaml not found"}}); !errors.Is(err, errDoctorFailed) {
		t.Errorf("err = %v, want errDoctorFailed", err)
	}
	if !failing.SilenceErrors {
		t.Error("cobra would print a second Error line after the report")
	}

	warning := &cobra.Command{Use: "doctor"}
	warning.SetOut(io.Discard)
	if err := printDoctorReport(warning, []doctorCheck{{name: "adb", status: "warn", detail: "adb not found"}}); err != nil {
		t.Errorf("a warning alone must not fail the command: %v", err)
	}
	if warning.SilenceErrors {
		t.Error("SilenceErrors set without a required failure")
	}
}

// The message `probe test` gives when no device is usable names the next step, and keeps the underlying error when
// the listing itself failed.
// The JSON report carries the schema and the checks. ok is false only when a required check failed, and a failed required
// check gives the same exit status as the text report.
func TestPrintDoctorJSONIsOKOnlyWithoutRequiredFailures(t *testing.T) {
	failing := &cobra.Command{Use: "doctor"}
	var out bytes.Buffer
	failing.SetOut(&out)
	err := printDoctorJSON(failing, []doctorCheck{{name: "config", status: "fail", detail: "probe.yaml not found", hint: "run probe init"}})
	if !errors.Is(err, errDoctorFailed) || !failing.SilenceErrors {
		t.Errorf("err = %v, SilenceErrors = %v; want errDoctorFailed and a silenced cobra error", err, failing.SilenceErrors)
	}
	var got doctorReport
	if jerr := json.Unmarshal(out.Bytes(), &got); jerr != nil {
		t.Fatalf("report is not JSON: %v: %q", jerr, out.String())
	}
	if got.Schema != "probe.doctor/1" || got.OK || len(got.Checks) != 1 || got.Checks[0].Fix != "run probe init" {
		t.Errorf("report = %+v", got)
	}

	warning := &cobra.Command{Use: "doctor"}
	out.Reset()
	warning.SetOut(&out)
	if err := printDoctorJSON(warning, []doctorCheck{{name: "devices", status: "warn", detail: "no device"}}); err != nil {
		t.Errorf("a warning alone must not fail the command: %v", err)
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || !got.OK {
		t.Errorf("warning-only report: ok = %v, err = %v", got.OK, err)
	}
}

func TestNoDeviceErrorNamesTheNextStep(t *testing.T) {
	empty := noDeviceError(nil).Error()
	if !strings.Contains(empty, "no connected devices found") || !strings.Contains(empty, "probe doctor") {
		t.Errorf("empty listing: %q", empty)
	}
	failed := noDeviceError(errors.New("adb: server not running")).Error()
	if !strings.Contains(failed, "adb: server not running") || !strings.Contains(failed, "probe doctor") {
		t.Errorf("failed listing: %q", failed)
	}
}
