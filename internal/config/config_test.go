package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

func TestLoadFile_Defaults(t *testing.T) {
	// Loading a non-existent file should return defaults without error
	cfg, err := config.LoadFile("/nonexistent/path/probe.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Defaults.Platform != "android" {
		t.Errorf("platform: got %q, want %q", cfg.Defaults.Platform, "android")
	}
	if cfg.Defaults.Timeout != 30*time.Second {
		t.Errorf("timeout: got %v, want %v", cfg.Defaults.Timeout, 30*time.Second)
	}
	if cfg.Defaults.Screenshots != "on_failure" {
		t.Errorf("screenshots: got %q, want %q", cfg.Defaults.Screenshots, "on_failure")
	}
	if cfg.Agent.Port != 48686 {
		t.Errorf("agent port: got %d, want %d", cfg.Agent.Port, 48686)
	}
	if cfg.Recipes != "tests/recipes" {
		t.Errorf("recipes: got %q, want %q", cfg.Recipes, "tests/recipes")
	}
	if cfg.Reports != "reports" {
		t.Errorf("reports: got %q, want %q", cfg.Reports, "reports")
	}
}

func TestLoadFile_PartialYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.yaml")
	yaml := `project:
  name: "Test App"
  app: com.test.app
defaults:
  platform: ios
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Project.Name != "Test App" {
		t.Errorf("project name: got %q, want %q", cfg.Project.Name, "Test App")
	}
	if cfg.Defaults.Platform != "ios" {
		t.Errorf("platform: got %q, want %q", cfg.Defaults.Platform, "ios")
	}
	// Defaults should fill in zero values
	if cfg.Defaults.Timeout != 30*time.Second {
		t.Errorf("timeout should default: got %v", cfg.Defaults.Timeout)
	}
	if cfg.Agent.Port != 48686 {
		t.Errorf("agent port should default: got %d", cfg.Agent.Port)
	}
}

func TestLoadFile_InvalidAppID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.yaml")
	yaml := `project:
  app: "rm -rf /; evil"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadFile(path)
	if err == nil {
		t.Fatal("expected error for invalid app ID")
	}
}

func TestLoadFile_ValidAppID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.yaml")
	yaml := `project:
  app: com.example.myapp
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Project.App != "com.example.myapp" {
		t.Errorf("app: got %q", cfg.Project.App)
	}
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.yaml")
	if err := os.WriteFile(path, []byte("{{invalid yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_FromDir(t *testing.T) {
	dir := t.TempDir()
	yaml := `project:
  name: "Dir Test"
`
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Project.Name != "Dir Test" {
		t.Errorf("name: got %q", cfg.Project.Name)
	}
}

func TestAgentDevicePort_Default(t *testing.T) {
	ac := config.AgentConfig{Port: 48686}
	if got := ac.AgentDevicePort(); got != 48686 {
		t.Errorf("got %d, want 48686", got)
	}
}

func TestAgentDevicePort_Override(t *testing.T) {
	ac := config.AgentConfig{Port: 48686, DevicePort: 9999}
	if got := ac.AgentDevicePort(); got != 9999 {
		t.Errorf("got %d, want 9999", got)
	}
}

func TestValidateDeviceSerial(t *testing.T) {
	tests := []struct {
		serial  string
		wantErr bool
	}{
		{"emulator-5554", false},
		{"192.168.1.100:5555", false},
		{"ABCD1234-5678-90EF", false},
		{"", true},
		{"rm -rf /", true},
		{"$(evil)", true},
		{"serial;whoami", true},
	}

	for _, tt := range tests {
		err := config.ValidateDeviceSerial(tt.serial)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateDeviceSerial(%q): err=%v, wantErr=%v", tt.serial, err, tt.wantErr)
		}
	}
}

func TestRelayConfig_TTL(t *testing.T) {
	r := config.RelayConfig{}
	if got := r.RelayTTL(); got != 600 {
		t.Errorf("default TTL: got %d, want 600", got)
	}

	r.TTL = 300
	if got := r.RelayTTL(); got != 300 {
		t.Errorf("custom TTL: got %d, want 300", got)
	}
}

func TestRelayConfig_ConnectTimeout(t *testing.T) {
	r := config.RelayConfig{}
	if got := r.RelayConnectTimeout(); got != 60*time.Second {
		t.Errorf("default: got %v, want 60s", got)
	}

	r.ConnectTimeout = 30 * time.Second
	if got := r.RelayConnectTimeout(); got != 30*time.Second {
		t.Errorf("custom: got %v, want 30s", got)
	}
}

func TestRelayConfig_Enabled(t *testing.T) {
	r := config.RelayConfig{}

	// nil Enabled + no provider = disabled
	if r.RelayEnabled(false) {
		t.Error("expected disabled when no provider and nil Enabled")
	}

	// nil Enabled + provider = enabled (auto)
	if !r.RelayEnabled(true) {
		t.Error("expected enabled when provider configured and nil Enabled")
	}

	// Explicit override
	b := true
	r.Enabled = &b
	if !r.RelayEnabled(false) {
		t.Error("expected enabled when explicitly set to true")
	}

	b = false
	r.Enabled = &b
	if r.RelayEnabled(true) {
		t.Error("expected disabled when explicitly set to false")
	}
}

func TestLoadFile_FullConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.yaml")
	yaml := `project:
  name: "Full Test"
  app: com.full.test
defaults:
  platform: both
  timeout: 10s
  screenshots: always
  video: true
  retry_failed_tests: 3
agent:
  port: 9000
  device_port: 9001
  dial_timeout: 5s
  ping_interval: 2s
  token_read_timeout: 15s
  reconnect_delay: 1s
device:
  emulator_boot_timeout: 60s
  simulator_boot_timeout: 30s
  boot_poll_interval: 1s
  token_file_retries: 3
  restart_delay: 200ms
video:
  resolution: "1080x1920"
  framerate: 4
  screenrecord_cycle: 160s
visual:
  threshold: 1.0
  pixel_delta: 16
recipes_folder: my_recipes
reports_folder: my_reports
environment:
  KEY: "value"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Defaults.Platform != "both" {
		t.Errorf("platform: %q", cfg.Defaults.Platform)
	}
	if cfg.Defaults.Timeout != 10*time.Second {
		t.Errorf("timeout: %v", cfg.Defaults.Timeout)
	}
	if cfg.Defaults.Screenshots != "always" {
		t.Errorf("screenshots: %q", cfg.Defaults.Screenshots)
	}
	if !cfg.Defaults.VideoEnabled {
		t.Error("video should be enabled")
	}
	if cfg.Defaults.RetryFailedTests != 3 {
		t.Errorf("retries: %d", cfg.Defaults.RetryFailedTests)
	}
	if cfg.Agent.Port != 9000 {
		t.Errorf("agent port: %d", cfg.Agent.Port)
	}
	if cfg.Agent.DevicePort != 9001 {
		t.Errorf("device port: %d", cfg.Agent.DevicePort)
	}
	if cfg.Video.Resolution != "1080x1920" {
		t.Errorf("resolution: %q", cfg.Video.Resolution)
	}
	if cfg.Visual.Threshold != 1.0 {
		t.Errorf("threshold: %f", cfg.Visual.Threshold)
	}
	if cfg.Visual.PixelDelta != 16 {
		t.Errorf("pixel_delta: %d", cfg.Visual.PixelDelta)
	}
	if cfg.Recipes != "my_recipes" {
		t.Errorf("recipes: %q", cfg.Recipes)
	}
	if cfg.Reports != "my_reports" {
		t.Errorf("reports: %q", cfg.Reports)
	}
	if cfg.Env["KEY"] != "value" {
		t.Errorf("env KEY: %q", cfg.Env["KEY"])
	}
}
