package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// For an iOS device, --device follows the rule of device.ios_device_id: a simulator UDID or a device UDID. Android keeps the serial rule.
func TestIOSDeviceFlagMustBeAUDID(t *testing.T) {
	for _, ok := range []string{"6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B", "00008110-001A2C3E0E12801E", "0123456789abcdef0123456789abcdef01234567"} {
		if err := validIOSDeviceFlag(device.PlatformIOS, ok); err != nil {
			t.Errorf("%q must be accepted: %v", ok, err)
		}
	}
	for _, bad := range []string{"../../x", "/etc", "iPhone.16", "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B/../x", "abc"} {
		err := validIOSDeviceFlag(device.PlatformIOS, bad)
		if err == nil || !strings.Contains(err.Error(), "probe device list") || !strings.Contains(err.Error(), "--device") {
			t.Errorf("%q must be refused with a pointer to probe device list, got %v", bad, err)
		}
	}
	for _, serial := range []string{"emulator-5554", "192.168.1.5:5555", "R5CT1234ABC"} {
		if err := validIOSDeviceFlag(device.PlatformAndroid, serial); err != nil {
			t.Errorf("an Android serial %q must keep its rule: %v", serial, err)
		}
	}
}

// probe test checks --device after the platform is known.
func TestProbeTestChecksTheIOSDeviceFlag(t *testing.T) {
	b, err := os.ReadFile("test.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "if err := validIOSDeviceFlag(platform, deviceSerial); err != nil {") {
		t.Error("probe test must check --device for an iOS device")
	}
}
