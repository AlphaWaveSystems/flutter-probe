package cli

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// A single-device run uses the application ID the device knows, as --devices and parallel runs do.
func TestResolveDeviceAppID(t *testing.T) {
	tests := []struct {
		name     string
		app      string
		platform device.Platform
		want     string
	}{
		{"android converts a camelCase bundle ID", "com.alphawavesystems.probeTestApp", device.PlatformAndroid, "com.alphawavesystems.probe_test_app"},
		{"ios keeps the bundle ID", "com.alphawavesystems.probeTestApp", device.PlatformIOS, "com.alphawavesystems.probeTestApp"},
		{"android keeps a snake_case package", "com.example.my_app", device.PlatformAndroid, "com.example.my_app"},
		{"an empty app stays empty", "", device.PlatformAndroid, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Project.App = tt.app
			resolveDeviceAppID(cfg, tt.platform)
			if cfg.Project.App != tt.want {
				t.Fatalf("project.app = %q, want %q", cfg.Project.App, tt.want)
			}
		})
	}
}
