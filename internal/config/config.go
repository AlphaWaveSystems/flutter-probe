package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents probe.yaml at the project root.
type Config struct {
	Project  ProjectConfig     `yaml:"project"`
	Defaults DefaultsConfig    `yaml:"defaults"`
	Devices  []DeviceEntry     `yaml:"devices"`
	Tools    ToolsConfig       `yaml:"tools"`
	Recipes  string            `yaml:"recipes_folder"`
	Reports  string            `yaml:"reports_folder"`
	Env      map[string]string `yaml:"environment"`
}

// ToolsConfig holds paths to external tools. Empty means "find in PATH".
// Useful for CI/CD pipelines or non-standard installations.
type ToolsConfig struct {
	ADB     string `yaml:"adb"`     // e.g. "/home/ci/android-sdk/platform-tools/adb"
	Flutter string `yaml:"flutter"` // e.g. "/opt/flutter/bin/flutter"
}

type ProjectConfig struct {
	Name string `yaml:"name"`
	App  string `yaml:"app"` // bundle id / package name
}

type DefaultsConfig struct {
	Platform                string        `yaml:"platform"`                  // android | ios | both
	Timeout                 time.Duration `yaml:"timeout"`
	Screenshots             string        `yaml:"screenshots"`               // always | on_failure | never
	Video                   bool          `yaml:"video"`
	RetryFailedTests        int           `yaml:"retry_failed_tests"`
	GrantPermissionsOnClear bool          `yaml:"grant_permissions_on_clear"` // auto-grant all permissions after clear app data
}

type DeviceEntry struct {
	Name   string `yaml:"name"`
	Serial string `yaml:"serial"` // emulator-5554 | auto
}

var defaultConfig = Config{
	Defaults: DefaultsConfig{
		Platform:        "android",
		Timeout:         30 * time.Second,
		Screenshots:     "on_failure",
		Video:           false,
		RetryFailedTests: 1,
	},
	Recipes: "tests/recipes",
	Reports: "reports",
}

// Load reads probe.yaml from the given directory. Falls back to defaults if missing.
func Load(dir string) (*Config, error) {
	cfg := defaultConfig

	path := dir + "/probe.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &cfg, nil
		}
		return nil, fmt.Errorf("reading probe.yaml: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing probe.yaml: %w", err)
	}

	return &cfg, nil
}

// DefaultYAML returns the content written by `probe init`.
const DefaultYAML = `project:
  name: "My Flutter App"
  app: com.example.myapp

defaults:
  platform: android        # android | ios | both
  timeout: 30s
  screenshots: on_failure  # always | on_failure | never
  video: false
  retry_failed_tests: 1

devices:
  - name: Pixel 7 Emulator
    serial: emulator-5554
  - name: iPhone 15 Sim
    serial: auto

recipes_folder: tests/recipes
reports_folder: reports

environment:
  API_BASE: "http://localhost:8080"
  TEST_USER: "admin@test.com"
`
