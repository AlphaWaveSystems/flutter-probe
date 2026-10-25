package cli

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// A single-device run uses the application ID the device has installed. The fake device reports which IDs are
// installed and records which IDs it was asked about.
func TestResolveDeviceAppID(t *testing.T) {
	const bundle = "com.alphawavesystems.probeTestApp"
	const pkg = "com.alphawavesystems.probe_test_app"
	tests := []struct {
		name      string
		app       string
		platform  device.Platform
		installed []string
		want      string
		asked     []string
	}{
		{"android: the converted ID is the installed one, so it is used", bundle, device.PlatformAndroid, []string{pkg}, pkg, []string{bundle, pkg}},
		{"android: the ID as written is installed, so it is kept", bundle, device.PlatformAndroid, []string{bundle, pkg}, bundle, []string{bundle}},
		{"android: neither form is installed, so the ID as written is kept", bundle, device.PlatformAndroid, nil, bundle, []string{bundle, pkg}},
		{"android: a snake_case package needs no check", pkg, device.PlatformAndroid, nil, pkg, nil},
		{"ios: the bundle ID is used as written, with no check", bundle, device.PlatformIOS, nil, bundle, nil},
		{"an empty app stays empty", "", device.PlatformAndroid, nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var asked []string
			installed := func(id string) bool {
				asked = append(asked, id)
				for _, p := range tt.installed {
					if p == id {
						return true
					}
				}
				return false
			}
			if got := resolveDeviceAppID(tt.app, tt.platform, installed); got != tt.want {
				t.Fatalf("app ID = %q, want %q", got, tt.want)
			}
			if !sameIDs(asked, tt.asked) {
				t.Fatalf("device asked about %v, want %v", asked, tt.asked)
			}
		})
	}
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
