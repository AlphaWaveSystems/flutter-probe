package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// `probe test` warns through printAppIDWarning. An id that differs only in case is another app on Android (a package name is case-sensitive)
// and the same app on iOS (a bundle id is not), so the platform decides.
func TestPrintAppIDWarningByPlatform(t *testing.T) {
	cases := []struct {
		name       string
		platform   device.Platform
		reported   string
		project    string
		wantWarned bool
	}{
		{"android: the same id", device.PlatformAndroid, "com.example.app", "com.example.app", false},
		{"android: a difference of case alone is another app", device.PlatformAndroid, "com.Example.app", "com.example.app", true},
		{"android: another app", device.PlatformAndroid, "com.other.app", "com.example.app", true},
		{"ios: the same id", device.PlatformIOS, "com.example.app", "com.example.app", false},
		{"ios: a difference of case alone is the same app", device.PlatformIOS, "com.Example.app", "com.example.app", false},
		{"ios: another app", device.PlatformIOS, "com.other.app", "com.example.app", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			printAppIDWarning(&out, probelink.HandshakeResult{AppID: c.reported}, c.project, c.platform)
			got := out.String()
			if (got != "") != c.wantWarned {
				t.Fatalf("warned = %v, want %v; output %q", got != "", c.wantWarned, got)
			}
			if c.wantWarned && (!strings.Contains(got, c.reported) || !strings.Contains(got, c.project)) {
				t.Errorf("the warning should name both ids %q and %q: %q", c.reported, c.project, got)
			}
		})
	}
}
