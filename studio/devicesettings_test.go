package main

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

type recordRunner struct{ calls []string }

func (r *recordRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	return nil, nil
}

func TestSettingsTarget(t *testing.T) {
	devs := []device.Device{
		{ID: "emulator-5554", Name: "Pixel", Platform: device.PlatformAndroid, State: "device"},
		{ID: "SIM-1", Name: "StudioSim", Platform: device.PlatformIOS, State: "booted"},
		{ID: "PHONE-1", Name: "Real", Platform: device.PlatformIOS, State: "online", Transport: "usb"},
	}
	for _, tc := range []struct {
		id        string
		platform  device.Platform
		available bool
		reason    string
		orient    bool
	}{
		{"emulator-5554", device.PlatformAndroid, true, "", true},
		{"SIM-1", device.PlatformIOS, true, "", false},
		{"PHONE-1", device.PlatformIOS, false, "Physical iPhones", false},
		{"192.168.1.5", device.PlatformAndroid, false, "not an emulator or simulator", false},
	} {
		info := settingsTarget(tc.id, "n", tc.platform, "com.example.app", devs)
		if info.Available != tc.available || !strings.Contains(info.Reason, tc.reason) {
			t.Errorf("%s: %+v", tc.id, info)
		}
		hasOrient := false
		for _, c := range info.Capabilities {
			if c.Setting == "orientation" {
				hasOrient = true
			}
		}
		if hasOrient != tc.orient {
			t.Errorf("%s: orientation offered = %v, want %v", tc.id, hasOrient, tc.orient)
		}
		if !tc.available && len(info.Capabilities) != 0 {
			t.Errorf("%s: capabilities offered for an unavailable device", tc.id)
		}
	}
}

func TestApplyDeviceSettingNeedsAConnection(t *testing.T) {
	a := NewApp()
	rr := &recordRunner{}
	old := settingsRunner
	settingsRunner = rr
	defer func() { settingsRunner = old }()
	if err := a.ApplyDeviceSetting("appearance", "dark", ""); err == nil {
		t.Fatal("a change without a connection must fail")
	}
	if len(rr.calls) != 0 {
		t.Errorf("nothing may run without a connection: %v", rr.calls)
	}
}
