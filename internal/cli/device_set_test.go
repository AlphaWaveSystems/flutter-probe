package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

func TestPickDevice(t *testing.T) {
	devs := []device.Device{
		{ID: "emulator-5554", Name: "Pixel 8", Platform: device.PlatformAndroid, State: "online"},
		{ID: "emulator-5556", Name: "Pixel 8", Platform: device.PlatformAndroid, State: "online"},
		{ID: "UDID-1", Name: "StudioSim", Platform: device.PlatformIOS, State: "online"},
		{ID: "emulator-5580", Name: "AdbState", Platform: device.PlatformAndroid, State: "device"},
		{ID: "UDID-2", Name: "Off", Platform: device.PlatformIOS, State: "offline"},
		{ID: "PHONE-1", Name: "Real Phone", Platform: device.PlatformIOS, State: "online", Transport: "usb"},
	}
	for _, tc := range []struct {
		id      string
		want    string
		wantErr string
	}{
		{"emulator-5554", "emulator-5554", ""},
		{"StudioSim", "UDID-1", ""},
		{"Pixel 8", "", "2 devices are named"},
		{"nope", "", "no device"},
		{"UDID-2", "", "is offline"},
		{"PHONE-1", "", "physical iOS device"},
	} {
		got, err := pickDevice(devs, tc.id)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%q: want error %q, got %v", tc.id, tc.wantErr, err)
			}
			continue
		}
		if err != nil || got.ID != tc.want {
			t.Errorf("%q: got %v, %v; want %s", tc.id, got.ID, err, tc.want)
		}
	}
}

func TestResolveNamedDeviceRefusesEmptyAndBooted(t *testing.T) {
	for _, id := range []string{"", "booted", "Booted"} {
		if _, err := resolveNamedDevice(context.Background(), id); err == nil || !strings.Contains(err.Error(), "--device is required") {
			t.Errorf("%q: got %v", id, err)
		}
	}
}
