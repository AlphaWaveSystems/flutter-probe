package main

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

func TestPhysicalHintSaysWhatEachDeviceNeeds(t *testing.T) {
	cases := []struct {
		name string
		d    device.Device
		kind string
		want []string // all must appear; nil = no hint
	}{
		{"simulator has no hint", device.Device{Platform: device.PlatformIOS, State: "booted"}, "simulator", nil},
		{"emulator has no hint", device.Device{Platform: device.PlatformAndroid, State: "device"}, "emulator", nil},
		{"iphone by usb", device.Device{Platform: device.PlatformIOS, State: "online", Transport: "usb"}, "physical",
			[]string{"USB", "iproxy", "PROBE_AGENT=true", "profile build", "PROBE_WIFI=true", "foreground"}},
		{"iphone on the network only", device.Device{Platform: device.PlatformIOS, State: "paired", Transport: "network"}, "physical",
			[]string{"not attached by USB", "WiFi button", "Connect manually", "PROBE_AGENT=true", "PROBE_WIFI=true", "foreground"}},
		{"android over adb", device.Device{Platform: device.PlatformAndroid, State: "device"}, "physical",
			[]string{"adb", "PROBE_AGENT=true", "foreground"}},
		{"android unauthorized", device.Device{Platform: device.PlatformAndroid, State: "unauthorized"}, "physical",
			[]string{"unauthorized", "USB debugging"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := physicalHint(c.d, c.kind)
			if c.want == nil {
				if got != "" {
					t.Fatalf("hint = %q", got)
				}
				return
			}
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("hint lacks %q: %s", w, got)
				}
			}
		})
	}
}

func TestNetworkOnlyIPhoneIsNotReadyButAUSBOneIs(t *testing.T) {
	if isDeviceReady(device.Device{Platform: device.PlatformIOS, State: "paired", Transport: "network"}) {
		t.Error("a phone seen only over the network cannot be reached through iproxy")
	}
	if !isDeviceReady(device.Device{Platform: device.PlatformIOS, State: "online", Transport: "usb"}) {
		t.Error("a phone attached by USB is ready")
	}
}
