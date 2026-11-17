package device

import "testing"

// Two application ids that differ only in case are the same app on iOS (a bundle id) and another app on Android (a package name is case-sensitive).
func TestPlatformAppIDIsCaseInsensitive(t *testing.T) {
	for _, tc := range []struct {
		p    Platform
		want bool
	}{
		{PlatformIOS, true},
		{PlatformAndroid, false},
		{Platform(""), false},
		{Platform("IOS"), false}, // only the exact value the CLI uses counts
		{Platform("windows"), false},
	} {
		if got := tc.p.AppIDIsCaseInsensitive(); got != tc.want {
			t.Errorf("Platform(%q).AppIDIsCaseInsensitive() = %v, want %v", tc.p, got, tc.want)
		}
	}
}
