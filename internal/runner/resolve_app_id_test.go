package runner

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// Flutter writes camelCase bundle IDs for iOS and snake_case package names for Android; only the last segment changes.
func TestResolveAppID(t *testing.T) {
	tests := []struct {
		name     string
		app      string
		platform device.Platform
		want     string
	}{
		{"android converts a camelCase last segment", "com.example.myApp", device.PlatformAndroid, "com.example.my_app"},
		{"android leaves the other segments as written", "com.Example.my_app", device.PlatformAndroid, "com.Example.my_app"},
		{"android keeps a lowercase ID", "com.example.app", device.PlatformAndroid, "com.example.app"},
		{"ios keeps a camelCase bundle ID", "com.example.myApp", device.PlatformIOS, "com.example.myApp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveAppID(tt.app, tt.platform); got != tt.want {
				t.Fatalf("ResolveAppID(%q, %q) = %q, want %q", tt.app, tt.platform, got, tt.want)
			}
		})
	}
}
