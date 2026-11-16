package device

import "testing"

// Only a device that is positively a simulator or an emulator is disposable; everything else, and anything unknown, asks.
func TestDisposable(t *testing.T) {
	for name, tc := range map[string]struct {
		d    Device
		want bool
	}{
		"an android emulator":              {Device{Platform: PlatformAndroid, ID: "emulator-5554"}, true},
		"an android phone":                 {Device{Platform: PlatformAndroid, ID: "R58M123ABC"}, false},
		"a phone over wifi":                {Device{Platform: PlatformAndroid, ID: "192.168.1.20:5555"}, false},
		"a serial that merely contains it": {Device{Platform: PlatformAndroid, ID: "my-emulator-5554"}, false},
		"an ios simulator":                 {Device{Platform: PlatformIOS, ID: "9E0BD0DC-CBDA-4EF6-B3A4-C8F30231E179"}, true},
		"an iphone over usb":               {Device{Platform: PlatformIOS, ID: "00008110-000", Transport: "usb"}, false},
		"an iphone over the network":       {Device{Platform: PlatformIOS, ID: "00008110-001", Transport: "network"}, false},
		"an unknown platform":              {Device{Platform: "", ID: "emulator-1"}, false},
		"nothing":                          {Device{}, false},
	} {
		if got := tc.d.Disposable(); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}
