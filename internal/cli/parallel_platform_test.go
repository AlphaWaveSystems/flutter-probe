package cli

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// A parallel run refuses devices of two platforms at start: its lanes cannot connect both. The refusal names the --devices list when that is
// how the devices were given.
func TestOnePlatformErrorRefusesAMixedList(t *testing.T) {
	mixed := []runner.DeviceRun{{DeviceID: "emulator-5554", Platform: device.PlatformAndroid}, {DeviceID: "0B1C2D3E-4F5A-6B7C-8D9E-0F1A2B3C4D5E", Platform: device.PlatformIOS}}
	err := onePlatformError(mixed, true)
	if err == nil {
		t.Fatal("a --devices list of Android and iOS devices must be refused")
	}
	if want := "--devices must list devices of one platform; run one probe test per platform"; err.Error() != want {
		t.Errorf("the message must say what to do, got %q, want %q", err.Error(), want)
	}
}

// Auto-discovered devices of two platforms are refused too, with a message that does not name --devices.
func TestOnePlatformErrorRefusesAMixedDiscovery(t *testing.T) {
	mixed := []runner.DeviceRun{{DeviceID: "emulator-5554", Platform: device.PlatformAndroid}, {DeviceID: "0B1C2D3E-4F5A-6B7C-8D9E-0F1A2B3C4D5E", Platform: device.PlatformIOS}}
	err := onePlatformError(mixed, false)
	if err == nil || !strings.Contains(err.Error(), "more than one platform") || strings.Contains(err.Error(), "--devices") {
		t.Errorf("a discovered mix must be refused with the discovery message, got %v", err)
	}
}

// One platform, or no devices, is not refused.
func TestOnePlatformErrorAllowsOnePlatform(t *testing.T) {
	android := []runner.DeviceRun{{DeviceID: "emulator-5554", Platform: device.PlatformAndroid}, {DeviceID: "emulator-5556", Platform: device.PlatformAndroid}}
	ios := []runner.DeviceRun{{DeviceID: "0B1C2D3E-4F5A-6B7C-8D9E-0F1A2B3C4D5E", Platform: device.PlatformIOS}, {DeviceID: "1C2D3E4F-5A6B-7C8D-9E0F-1A2B3C4D5E6F", Platform: device.PlatformIOS}}
	for name, runs := range map[string][]runner.DeviceRun{"android": android, "ios": ios, "none": nil} {
		for _, fromFlag := range []bool{true, false} {
			if err := onePlatformError(runs, fromFlag); err != nil {
				t.Errorf("%s (flag %v) must not be refused, got %v", name, fromFlag, err)
			}
		}
	}
}
