package runner

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/devsettings"
)

// TestAppearanceRequest checks what `set appearance` runs on each device: the host commands devsettings plans, a
// physical iOS device is refused before anything runs, and a mode iOS does not have is refused by the plan.
func TestAppearanceRequest(t *testing.T) {
	const udid = "1A2B3C4D-5E6F-7A8B-9C0D-1E2F3A4B5C6D"
	for _, tc := range []struct {
		name     string
		platform device.Platform
		serial   string
		physical bool
		mode     string
		want     []string // the host commands; nil when the step must fail
	}{
		{name: "android dark", platform: device.PlatformAndroid, serial: "emulator-5554", mode: "dark",
			want: []string{"adb -s emulator-5554 shell cmd uimode night yes"}},
		{name: "android light", platform: device.PlatformAndroid, serial: "emulator-5554", mode: "light",
			want: []string{"adb -s emulator-5554 shell cmd uimode night no"}},
		{name: "android auto", platform: device.PlatformAndroid, serial: "emulator-5554", mode: "auto",
			want: []string{"adb -s emulator-5554 shell cmd uimode night auto"}},
		{name: "android physical device", platform: device.PlatformAndroid, serial: "R58M123ABC", physical: true, mode: "dark",
			want: []string{"adb -s R58M123ABC shell cmd uimode night yes"}},
		{name: "ios dark", platform: device.PlatformIOS, serial: udid, mode: "dark",
			want: []string{"xcrun simctl ui " + udid + " appearance dark"}},
		{name: "ios light", platform: device.PlatformIOS, serial: udid, mode: "light",
			want: []string{"xcrun simctl ui " + udid + " appearance light"}},
		{name: "ios auto is refused", platform: device.PlatformIOS, serial: udid, mode: "auto"},
		{name: "ios physical device is refused", platform: device.PlatformIOS, serial: udid, physical: true, mode: "dark"},
		{name: "booted is not a device", platform: device.PlatformIOS, serial: "booted", mode: "dark"},
		{name: "unknown platform is refused", platform: device.Platform("windows"), serial: "x1", mode: "dark"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := appearanceRequest(tc.platform, tc.serial, tc.physical, tc.mode)
			var cmds []devsettings.Command
			if err == nil {
				cmds, err = devsettings.Plan(req)
			}
			if tc.want == nil {
				if err == nil {
					t.Fatalf("planned %v, want a refusal", cmds)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			var got []string
			for _, c := range cmds {
				got = append(got, c.String())
			}
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("commands:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
}
