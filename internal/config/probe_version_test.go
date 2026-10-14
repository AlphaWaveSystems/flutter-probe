package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProbeVersion(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string // the pin as String(); "" means ParseProbeVersion must fail
	}{
		{"exact release", "0.25.1", "0.25.1"},
		{"release line", "0.25.x", "0.25.x"},
		{"leading v is accepted", "v0.25.1", "0.25.1"},
		{"surrounding space is ignored", "  0.25.x \n", "0.25.x"},
		{"two parts", "0.25", ""},
		{"pre-release suffix", "0.25.1-rc.1", ""},
		{"wildcard in the minor", "0.x.1", ""},
		{"a word", "latest", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pin, err := ParseProbeVersion(tc.value)
			if tc.want == "" {
				if err == nil {
					t.Errorf("ParseProbeVersion(%q) = %v, want an error", tc.value, pin)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseProbeVersion(%q): %v", tc.value, err)
			}
			if got := pin.String(); got != tc.want {
				t.Errorf("ParseProbeVersion(%q) = %s, want %s", tc.value, got, tc.want)
			}
		})
	}
}

func TestProbeVersionSatisfied(t *testing.T) {
	cases := []struct {
		name string
		pin  string
		cli  string
		want bool
	}{
		{"exact release satisfied", "0.25.1", "0.25.1", true},
		{"exact pin, other patch", "0.25.1", "0.25.2", false},
		{"exact pin, older minor", "0.25.1", "0.24.9", false},
		{"release line takes any patch", "0.25.x", "0.25.7", true},
		{"release line, other minor", "0.25.x", "0.26.0", false},
		{"release line, other major", "0.25.x", "1.25.0", false},
		{"build of the pinned release", "0.25.1", "0.25.1-fp3", true},
		{"v-prefixed CLI version", "0.25.1", "v0.25.1", true},
		{"dev build meets no pin", "0.25.x", "dev", false},
		{"empty CLI version", "0.25.1", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pin, err := ParseProbeVersion(tc.pin)
			if err != nil {
				t.Fatalf("ParseProbeVersion(%q): %v", tc.pin, err)
			}
			if got := pin.Satisfied(tc.cli); got != tc.want {
				t.Errorf("%s.Satisfied(%q) = %v, want %v", tc.pin, tc.cli, got, tc.want)
			}
		})
	}
}

func TestLoadFileProbeVersion(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		want    string // ProbeVersion after loading; "" with no error means no pin
		wantErr bool
	}{
		{"field absent leaves no pin", "defaults:\n  platform: android\n", "", false},
		{"exact release", "probe_version: 0.25.1\n", "0.25.1", false},
		{"release line", "probe_version: 0.25.x\n", "0.25.x", false},
		{"malformed value fails the load", "probe_version: 0.25\n", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "probe.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadFile(path)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "probe_version") {
					t.Fatalf("LoadFile error = %v, want one that names probe_version", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}
			if cfg.ProbeVersion != tc.want {
				t.Errorf("ProbeVersion = %q, want %q", cfg.ProbeVersion, tc.want)
			}
		})
	}
}
