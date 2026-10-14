package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// The validator's checks, by name and status, for a project and a CLI version. A missing or broken probe.yaml, a pin the
// CLI does not meet, and an empty tests folder each show up as the line that names them.
func TestValidateChecksReportWhatIsWrong(t *testing.T) {
	healthy := func(pin, cli string) validateInput {
		return validateInput{cfgPath: "probe.yaml", cfgExists: true, cfg: &config.Config{ProbeVersion: pin}, cliVersion: cli, tests: []string{"tests/login.probe"}}
	}
	cases := []struct {
		name string
		in   validateInput
		want map[string]string // check name -> status
	}{
		{"healthy project without a pin", healthy("", "0.24.0"), map[string]string{"config": "ok", "pin": "ok", "tests": "ok"}},
		{"exact pin met", healthy("0.25.1", "0.25.1"), map[string]string{"pin": "ok"}},
		{"release line met", healthy("0.25.x", "0.25.7"), map[string]string{"pin": "ok"}},
		{"exact pin not met", healthy("0.25.1", "0.24.3"), map[string]string{"pin": "fail"}},
		{"release line not met", healthy("0.25.x", "0.26.0"), map[string]string{"pin": "fail"}},
		{"dev build meets no pin", healthy("0.25.1", "dev"), map[string]string{"pin": "fail"}},
		{"no probe.yaml", validateInput{cfgPath: "probe.yaml", cliVersion: "0.25.1", tests: []string{"tests/a.probe"}}, map[string]string{"config": "fail", "tests": "ok"}},
		{"probe.yaml does not load", validateInput{cfgPath: "probe.yaml", cfgExists: true, loadErr: errors.New("line 3: bad indent"), cliVersion: "0.25.1", tests: []string{"tests/a.probe"}}, map[string]string{"config": "fail", "tests": "ok"}},
		{"no tests folder", validateInput{cfgPath: "probe.yaml", cfgExists: true, cfg: &config.Config{}, cliVersion: "0.25.1", testsErr: errors.New("no such directory")}, map[string]string{"tests": "warn"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checks := validateChecks(c.in)
			got := map[string]string{}
			for _, ch := range checks {
				got[ch.Name] = ch.Status
			}
			for name, status := range c.want {
				if got[name] != status {
					t.Errorf("%s = %q, want %q (checks: %+v)", name, got[name], status, checks)
				}
			}
		})
	}
}

// When probe.yaml does not load there is no pin to read, so the report has no pin line at all.
func TestValidateHasNoPinLineWithoutAConfig(t *testing.T) {
	in := validateInput{cfgPath: "probe.yaml", cfgExists: true, loadErr: errors.New("bad"), cliVersion: "0.25.1", tests: []string{"tests/a.probe"}}
	for _, ch := range validateChecks(in) {
		if ch.Name == "pin" {
			t.Errorf("pin check ran without a loaded probe.yaml: %+v", ch)
		}
	}
}

// A pin the CLI does not meet says how to get that release, through the tap that brew needs, or how to pin the installed
// release instead.
func TestValidatePinFixNamesTheInstallAndTheTap(t *testing.T) {
	in := validateInput{cfgPath: "probe.yaml", cfgExists: true, cfg: &config.Config{ProbeVersion: "0.25.1"}, cliVersion: "0.24.3", tests: []string{"tests/a.probe"}}
	fix := ""
	for _, ch := range validateChecks(in) {
		if ch.Name == "pin" {
			fix = ch.Fix
		}
	}
	for _, want := range []string{"releases/tag/v0.25.1", "brew tap AlphaWaveSystems/tap && brew install probe", "change probe_version in probe.yaml to 0.24.3"} {
		if !strings.Contains(fix, want) {
			t.Errorf("fix %q does not contain %q", fix, want)
		}
	}
}

// Warnings alone do not fail the validator; a failed check does.
func TestValidateOKOnlyWithoutFailures(t *testing.T) {
	if !validateOK([]validateCheck{{Name: "config", Status: "ok"}, {Name: "tests", Status: "warn"}}) {
		t.Error("warnings alone must not fail the validator")
	}
	if validateOK([]validateCheck{{Name: "pin", Status: "fail"}}) {
		t.Error("a failed check must fail the validator")
	}
}
