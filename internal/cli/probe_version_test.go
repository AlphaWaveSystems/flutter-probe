package cli

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

func TestCheckProbeVersion(t *testing.T) {
	cases := []struct {
		name    string
		pin     string
		cli     string
		wantErr string // a substring of the error; "" means the run may start
	}{
		{"no probe_version changes nothing", "", "0.24.0", ""},
		{"exact pin satisfied", "0.25.1", "0.25.1", ""},
		{"release line satisfied", "0.25.x", "0.25.7", ""},
		{"exact pin not satisfied", "0.25.1", "0.24.3", "probe_version 0.25.1"},
		{"release line not satisfied", "0.25.x", "0.26.0", "probe_version 0.25.x"},
		{"dev build cannot be checked", "0.25.1", "dev", "the installed CLI is dev"},
		{"malformed value", "0.25", "0.25.1", "invalid probe_version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkProbeVersion(&config.Config{ProbeVersion: tc.pin}, tc.cli)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("checkProbeVersion(%q, %q) = %v, want nil", tc.pin, tc.cli, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("checkProbeVersion(%q, %q) = %v, want an error containing %q", tc.pin, tc.cli, err, tc.wantErr)
			}
		})
	}
}

// The refusal names the installed CLI, the required version and how to install it.
func TestProbeVersionRefusalNamesInstalledRequiredAndInstall(t *testing.T) {
	err := checkProbeVersion(&config.Config{ProbeVersion: "0.25.1"}, "0.24.3")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	for _, want := range []string{"installed CLI is 0.24.3", "requires probe_version 0.25.1", "brew install probe", "releases/tag/v0.25.1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not contain %q", err, want)
		}
	}
}

func TestProbeLine(t *testing.T) {
	cases := []struct {
		name  string
		cli   string
		agent string
		want  string
	}{
		{"release versions", "0.25.1", "0.25.1", "probe CLI 0.25.1 / agent 0.25.1"},
		{"v prefixes are dropped", "v0.25.1", "v0.25.1", "probe CLI 0.25.1 / agent 0.25.1"},
		{"agent without a version field", "0.25.1", "", "probe CLI 0.25.1 / agent unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := probeLine(tc.cli, tc.agent); got != tc.want {
				t.Errorf("probeLine(%q, %q) = %q, want %q", tc.cli, tc.agent, got, tc.want)
			}
		})
	}
}
