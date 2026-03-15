package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// validAppID matches valid bundle identifiers / Android package names.
var validAppID = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.]*$`)

// validDeviceSerial matches valid ADB serials (emulator-5554, IP:port) and iOS UDIDs.
var validDeviceSerial = regexp.MustCompile(`^[a-zA-Z0-9._:/-]+$`)

// Config represents probe.yaml at the project root.
type Config struct {
	Project  ProjectConfig     `yaml:"project"`
	Defaults DefaultsConfig    `yaml:"defaults"`
	Agent    AgentConfig       `yaml:"agent"`
	Device   DeviceConfig      `yaml:"device"`
	Video    VideoConfig       `yaml:"video"`
	Visual   VisualConfig      `yaml:"visual"`
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
	Timeout                 time.Duration `yaml:"timeout"`                   // per-step timeout
	Screenshots             string        `yaml:"screenshots"`               // always | on_failure | never
	VideoEnabled            bool          `yaml:"video"`                     // enable video recording
	RetryFailedTests        int           `yaml:"retry_failed_tests"`        // number of retries for failed tests
	GrantPermissionsOnClear bool          `yaml:"grant_permissions_on_clear"` // auto-grant all permissions after clear app data
}

// AgentConfig controls the ProbeAgent WebSocket connection.
type AgentConfig struct {
	Port             int           `yaml:"port"`               // WebSocket port the agent listens on (default: 48686)
	DialTimeout      time.Duration `yaml:"dial_timeout"`       // max time to establish a WebSocket connection (default: 30s)
	PingInterval     time.Duration `yaml:"ping_interval"`      // interval between WebSocket keepalive pings (default: 5s)
	TokenReadTimeout time.Duration `yaml:"token_read_timeout"` // max time to wait for the agent to emit its auth token (default: 30s)
	ReconnectDelay   time.Duration `yaml:"reconnect_delay"`    // delay after app restart before attempting WebSocket reconnect (default: 2s)
}

// DeviceConfig controls emulator/simulator startup and polling.
type DeviceConfig struct {
	EmulatorBootTimeout  time.Duration `yaml:"emulator_boot_timeout"`  // max time to wait for an Android emulator to boot (default: 120s)
	SimulatorBootTimeout time.Duration `yaml:"simulator_boot_timeout"` // max time to wait for an iOS simulator to boot (default: 60s)
	BootPollInterval     time.Duration `yaml:"boot_poll_interval"`     // how often to check if the device is ready (default: 2s)
	TokenFileRetries     int           `yaml:"token_file_retries"`     // number of attempts to read the token from file before falling back to log stream (default: 5)
	RestartDelay         time.Duration `yaml:"restart_delay"`          // delay after force-stopping an app before relaunching (default: 500ms)
}

// VideoConfig controls device screen recording.
type VideoConfig struct {
	Resolution         string        `yaml:"resolution"`          // Android screenrecord resolution, e.g. "720x1280" (default: "720x1280")
	Framerate          int           `yaml:"framerate"`           // frames per second for screencap-based video stitching (default: 2)
	ScreenrecordCycle  time.Duration `yaml:"screenrecord_cycle"`  // interval to restart Android screenrecord to avoid the 180s limit (default: 170s)
}

// VisualConfig controls visual regression testing.
type VisualConfig struct {
	Threshold  float64 `yaml:"threshold"`   // max allowed percentage of differing pixels, e.g. 0.5 means 0.5% (default: 0.5)
	PixelDelta int     `yaml:"pixel_delta"` // per-pixel color distance threshold on a 0–255 scale; differences below this are ignored (default: 8)
}

type DeviceEntry struct {
	Name   string `yaml:"name"`
	Serial string `yaml:"serial"` // emulator-5554 | auto
}

var defaultConfig = Config{
	Defaults: DefaultsConfig{
		Platform:         "android",
		Timeout:          30 * time.Second,
		Screenshots:      "on_failure",
		VideoEnabled:     false,
		RetryFailedTests: 1,
	},
	Agent: AgentConfig{
		Port:             48686,
		DialTimeout:      30 * time.Second,
		PingInterval:     5 * time.Second,
		TokenReadTimeout: 30 * time.Second,
		ReconnectDelay:   2 * time.Second,
	},
	Device: DeviceConfig{
		EmulatorBootTimeout:  120 * time.Second,
		SimulatorBootTimeout: 60 * time.Second,
		BootPollInterval:     2 * time.Second,
		TokenFileRetries:     5,
		RestartDelay:         500 * time.Millisecond,
	},
	Video: VideoConfig{
		Resolution:        "720x1280",
		Framerate:         2,
		ScreenrecordCycle: 170 * time.Second,
	},
	Visual: VisualConfig{
		Threshold:  0.5,
		PixelDelta: 8,
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

	// Apply defaults for zero-valued fields that were not set in YAML
	applyDefaults(&cfg)

	// Validate security-sensitive fields
	if cfg.Project.App != "" && !validAppID.MatchString(cfg.Project.App) {
		return nil, fmt.Errorf("invalid project.app %q: must be alphanumeric with dots/underscores (e.g. com.example.myapp)", cfg.Project.App)
	}

	return &cfg, nil
}

// ValidateDeviceSerial checks that a device serial/UDID is safe to pass to shell commands.
func ValidateDeviceSerial(serial string) error {
	if serial == "" {
		return fmt.Errorf("device serial is empty")
	}
	if !validDeviceSerial.MatchString(serial) {
		return fmt.Errorf("invalid device serial %q: contains unsafe characters", serial)
	}
	return nil
}

// applyDefaults fills in zero-valued fields with sensible defaults so that
// a partial probe.yaml doesn't result in broken zero values.
func applyDefaults(cfg *Config) {
	d := defaultConfig

	if cfg.Defaults.Timeout == 0 {
		cfg.Defaults.Timeout = d.Defaults.Timeout
	}
	if cfg.Defaults.Screenshots == "" {
		cfg.Defaults.Screenshots = d.Defaults.Screenshots
	}
	if cfg.Defaults.Platform == "" {
		cfg.Defaults.Platform = d.Defaults.Platform
	}

	if cfg.Agent.Port == 0 {
		cfg.Agent.Port = d.Agent.Port
	}
	if cfg.Agent.DialTimeout == 0 {
		cfg.Agent.DialTimeout = d.Agent.DialTimeout
	}
	if cfg.Agent.PingInterval == 0 {
		cfg.Agent.PingInterval = d.Agent.PingInterval
	}
	if cfg.Agent.TokenReadTimeout == 0 {
		cfg.Agent.TokenReadTimeout = d.Agent.TokenReadTimeout
	}
	if cfg.Agent.ReconnectDelay == 0 {
		cfg.Agent.ReconnectDelay = d.Agent.ReconnectDelay
	}

	if cfg.Device.EmulatorBootTimeout == 0 {
		cfg.Device.EmulatorBootTimeout = d.Device.EmulatorBootTimeout
	}
	if cfg.Device.SimulatorBootTimeout == 0 {
		cfg.Device.SimulatorBootTimeout = d.Device.SimulatorBootTimeout
	}
	if cfg.Device.BootPollInterval == 0 {
		cfg.Device.BootPollInterval = d.Device.BootPollInterval
	}
	if cfg.Device.TokenFileRetries == 0 {
		cfg.Device.TokenFileRetries = d.Device.TokenFileRetries
	}
	if cfg.Device.RestartDelay == 0 {
		cfg.Device.RestartDelay = d.Device.RestartDelay
	}

	if cfg.Video.Resolution == "" {
		cfg.Video.Resolution = d.Video.Resolution
	}
	if cfg.Video.Framerate == 0 {
		cfg.Video.Framerate = d.Video.Framerate
	}
	if cfg.Video.ScreenrecordCycle == 0 {
		cfg.Video.ScreenrecordCycle = d.Video.ScreenrecordCycle
	}

	if cfg.Visual.Threshold == 0 {
		cfg.Visual.Threshold = d.Visual.Threshold
	}
	if cfg.Visual.PixelDelta == 0 {
		cfg.Visual.PixelDelta = d.Visual.PixelDelta
	}

	if cfg.Recipes == "" {
		cfg.Recipes = d.Recipes
	}
	if cfg.Reports == "" {
		cfg.Reports = d.Reports
	}
}

// DefaultYAML returns the content written by `probe init`.
const DefaultYAML = `project:
  name: "My Flutter App"
  app: com.example.myapp

defaults:
  platform: android        # android | ios | both
  timeout: 30s             # per-step timeout
  screenshots: on_failure  # always | on_failure | never
  video: false             # record device screen during test runs
  retry_failed_tests: 1    # number of retries for failed tests

# ProbeAgent WebSocket connection settings
agent:
  port: 48686              # port the on-device agent listens on
  dial_timeout: 30s        # max time to establish a WebSocket connection
  ping_interval: 5s        # WebSocket keepalive ping interval
  token_read_timeout: 30s  # max time to wait for the agent auth token
  reconnect_delay: 2s      # delay after app restart before reconnecting

# Emulator / simulator startup settings
device:
  emulator_boot_timeout: 120s   # max time for an Android emulator to boot
  simulator_boot_timeout: 60s   # max time for an iOS simulator to boot
  boot_poll_interval: 2s        # polling interval while waiting for boot
  token_file_retries: 5         # file-read attempts before falling back to log stream
  restart_delay: 500ms          # delay after force-stop before relaunch

# Video recording settings
video:
  resolution: "720x1280"        # Android screenrecord resolution (WxH)
  framerate: 2                  # FPS for screencap-based video stitching
  screenrecord_cycle: 170s      # restart interval to avoid Android's 180s limit

# Visual regression testing
visual:
  threshold: 0.5                # max allowed pixel diff percentage (0.5 = 0.5%)
  pixel_delta: 8                # per-pixel color distance threshold (0–255)

devices:
  - name: Pixel 7 Emulator
    serial: emulator-5554
  - name: iPhone 15 Sim
    serial: auto

tools:
  adb: ""                  # path to adb binary (empty = find in PATH)
  flutter: ""              # path to flutter binary (empty = find in PATH)

recipes_folder: tests/recipes
reports_folder: reports

environment:
  API_BASE: "http://localhost:8080"
  TEST_USER: "admin@test.com"
`
