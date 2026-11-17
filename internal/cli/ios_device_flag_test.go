package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

const (
	testSimUDID = "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B"
	testDevUDID = "00008110-001A2C3E0E12801E"
)

func listOf(ds ...device.Device) func() ([]device.Device, error) {
	return func() ([]device.Device, error) { return ds, nil }
}

// The device of a local run: the platform comes from the device list, and the iOS rule is checked once it is known.
func TestPickLocalDevice(t *testing.T) {
	sim := device.Device{ID: testSimUDID, Platform: device.PlatformIOS}
	emu := device.Device{ID: "emulator-5554", Platform: device.PlatformAndroid}
	cases := []struct {
		name, serial     string
		def              device.Platform
		iosPref, andPref string
		list             func() ([]device.Device, error)
		wantSerial       string
		wantPlatform     device.Platform
		wantErr          string
	}{
		{"a listed simulator by UDID", testSimUDID, "", "", "", listOf(sim, emu), testSimUDID, device.PlatformIOS, ""},
		{"a listed emulator, with the iOS default: the list decides", "emulator-5554", device.PlatformIOS, "", "", listOf(sim, emu), "emulator-5554", device.PlatformAndroid, ""},
		{"an unlisted device UDID on iOS", testDevUDID, device.PlatformIOS, "", "", listOf(emu), testDevUDID, device.PlatformIOS, ""},
		{"an unlisted Android serial on the Android default", "R5CT1234", device.PlatformAndroid, "", "", listOf(), "R5CT1234", device.PlatformAndroid, ""},
		{"an unlisted Android serial with the iOS default", "R5CT1234", device.PlatformIOS, "", "", listOf(sim), "", device.PlatformIOS, "is not in the device list"},
		{"a listed iOS id that is not a UDID", "../x", "", "", "", listOf(device.Device{ID: "../x", Platform: device.PlatformIOS}), "", device.PlatformIOS, "invalid --device"},
		{"no serial: the iOS preference", "", device.PlatformIOS, testSimUDID, "emulator-1", listOf(emu), testSimUDID, device.PlatformIOS, ""},
		{"no serial: the Android preference", "", device.PlatformAndroid, testSimUDID, "emulator-5554", listOf(emu), "emulator-5554", device.PlatformAndroid, ""},
		{"no serial, no preference: the first device", "", "", "", "", listOf(emu, sim), "emulator-5554", device.PlatformAndroid, ""},
		{"no device at all", "", "", "", "", func() ([]device.Device, error) { return nil, errors.New("adb missing") }, "", "", "device"},
	}
	for _, c := range cases {
		serial, platform, err := pickLocalDevice(c.serial, c.def, c.iosPref, c.andPref, c.list)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%s: err = %v, want %q", c.name, err, c.wantErr)
			}
			if c.wantErr != "device" && !strings.Contains(err.Error(), "probe device list") {
				t.Errorf("%s: the message must point at probe device list: %v", c.name, err)
			}
			continue
		}
		if err != nil || serial != c.wantSerial || platform != c.wantPlatform {
			t.Errorf("%s: got %q %q %v, want %q %q", c.name, serial, platform, err, c.wantSerial, c.wantPlatform)
		}
	}
}
