package device

import (
	"errors"
	"testing"
)

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

// A device list that cannot be read is no device: not disposable (m13).
func TestDisposableInFailsClosed(t *testing.T) {
	devs := []Device{{Platform: PlatformAndroid, ID: "emulator-5554"}, {Platform: PlatformIOS, ID: "SIM-1"}}
	if !disposableIn(devs, nil, PlatformAndroid, "emulator-5554") || !disposableIn(devs, nil, PlatformIOS, "SIM-1") {
		t.Error("a listed emulator and simulator are disposable")
	}
	if disposableIn(devs, errors.New("adb: no daemon"), PlatformAndroid, "emulator-5554") {
		t.Error("a list that could not be read made a device disposable")
	}
	if disposableIn(devs, nil, PlatformAndroid, "emulator-9999") || disposableIn(devs, nil, PlatformIOS, "emulator-5554") || disposableIn(nil, nil, PlatformAndroid, "emulator-5554") {
		t.Error("a device that is not in the list, or is listed for another platform, is disposable")
	}
}

// An Android device is an emulator only when it says so: either property is exactly 1; an error, empty or other answer is no.
func TestIsQEMUFailsClosed(t *testing.T) {
	answer := func(m map[string]string, fail ...string) func(string) (string, error) {
		return func(p string) (string, error) {
			for _, f := range fail {
				if f == p {
					return "", errors.New("adb: device offline")
				}
			}
			return m[p], nil
		}
	}
	for name, tc := range map[string]struct {
		get  func(string) (string, error)
		want bool
	}{
		"kernel.qemu is 1":                  {answer(map[string]string{"ro.kernel.qemu": "1"}), true},
		"boot.qemu is 1":                    {answer(map[string]string{"ro.boot.qemu": "1"}), true},
		"both 1":                            {answer(map[string]string{"ro.kernel.qemu": "1", "ro.boot.qemu": "1"}), true},
		"the first errors, the second is 1": {answer(map[string]string{"ro.boot.qemu": "1"}, "ro.kernel.qemu"), true},
		"a phone: both empty":               {answer(map[string]string{}), false},
		"a phone: both 0":                   {answer(map[string]string{"ro.kernel.qemu": "0", "ro.boot.qemu": "0"}), false},
		"something else":                    {answer(map[string]string{"ro.kernel.qemu": "yes"}), false},
		"both error":                        {answer(map[string]string{"ro.kernel.qemu": "1", "ro.boot.qemu": "1"}, "ro.kernel.qemu", "ro.boot.qemu"), false},
		"a one with spaces":                 {answer(map[string]string{"ro.kernel.qemu": " 1"}), false},
	} {
		if got := isQEMU(tc.get); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

// The verified check: the list must say emulator or simulator, and an Android device must also say so itself.
func TestVerifiedDisposable(t *testing.T) {
	devs := []Device{{Platform: PlatformAndroid, ID: "emulator-5554"}, {Platform: PlatformIOS, ID: "SIM-1"}, {Platform: PlatformIOS, ID: "PHONE-1", Transport: "usb"}}
	qemu := func(string) (string, error) { return "1", nil }
	none := func(string) (string, error) { return "", nil }
	broken := func(string) (string, error) { return "", errors.New("offline") }
	calls := 0
	count := func(string) (string, error) { calls++; return "1", nil }
	for name, tc := range map[string]struct {
		platform Platform
		serial   string
		get      func(string) (string, error)
		want     bool
	}{
		"an emulator that says so":               {PlatformAndroid, "emulator-5554", qemu, true},
		"an emulator-named device that does not": {PlatformAndroid, "emulator-5554", none, false},
		"an emulator that cannot be asked":       {PlatformAndroid, "emulator-5554", broken, false},
		"an unlisted android serial":             {PlatformAndroid, "emulator-1", qemu, false},
		"a simulator needs no property":          {PlatformIOS, "SIM-1", broken, true},
		"an iphone":                              {PlatformIOS, "PHONE-1", qemu, false},
	} {
		if got := verifiedDisposable(devs, nil, tc.platform, tc.serial, tc.get); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
	if verifiedDisposable(devs, errors.New("list failed"), PlatformAndroid, "emulator-5554", qemu) {
		t.Error("a list error made a device verified")
	}
	verifiedDisposable(devs, nil, PlatformIOS, "SIM-1", count)
	verifiedDisposable(devs, nil, PlatformAndroid, "emulator-9", count)
	if calls != 0 {
		t.Errorf("the device was asked for properties %d times when the list had already said no or it is iOS", calls)
	}
}
